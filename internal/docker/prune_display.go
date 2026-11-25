package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/GhostWriters/semstyle"
	"github.com/distribution/reference"
	"github.com/docker/go-units"
	"github.com/moby/moby/api/types/image"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
)

// PruneReport holds structured prune results for display.
type PruneReport struct {
	ImagesDeleted     []image.DeleteResponse
	NetworksDeleted   []string
	VolumesDeleted    []string
	ContainersDeleted []string
	SpaceReclaimed    uint64
	AsciiMode         bool
	ImagesError       error
	NetworksError     error
	VolumesError      error
	ContainersError   error

	// ContainerInfo maps a container ID (full and short forms) to its compose service and
	// display name, captured before pruning. Deleted containers that match a service whose
	// image was also pruned are shown nested under that service; the rest are listed under
	// their own heading (by container name when known, else the truncated raw ID).
	ContainerInfo map[string]containerMeta

	// ImageLayers maps each image's ID to its layers (RootFS diff IDs), inspected before
	// pruning, so a pruned image shows its real layers and leaves out those an image that
	// stays still has. nil when the images couldn't be listed.
	ImageLayers map[string][]string
}

// containerMeta is the pre-flight info for one container.
type containerMeta struct {
	service string // compose service name
	name    string // display container name
}

func (r *PruneReport) hasErrors() bool {
	return r.ImagesError != nil || r.NetworksError != nil ||
		r.VolumesError != nil || r.ContainersError != nil
}

// imageGroup is the display unit: one pruned image, shown by ref, with its layers.
type imageGroup struct {
	ref       string   // the name shown: a tag, else a digest ref, else what prune reported
	refs      []string // every name prune untagged for the image
	layers    []layerEntry
	refStatus entryStatus
}

type layerEntry struct {
	id     string
	status entryStatus
	kept   bool // an image that stays has it too, so it wasn't freed
}

// showKeptLayers lists a pruned image's layers that an image which stays still has, as
// Kept rows outside the counts; off, they're left out.
var showKeptLayers = false

type entryStatus int

const (
	statusRemoved entryStatus = iota // ✓ Removed
	statusFailed                     // ⚠ Failed — expected but not in deleted list
)

// shortVolumeName returns an anonymous volume's name, a 64-character hex ID, cut to
// its first 12 characters, as Docker shows IDs; a named volume's name as it is.
func shortVolumeName(name string) string {
	if len(name) != 64 {
		return name
	}
	for _, c := range name {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return name
		}
	}
	return name[:12]
}

// pruneTermWidth returns the terminal's width, for wrapping folded lines, or 80 when it
// can't be read.
func pruneTermWidth() int {
	if w, _, err := console.GetTerminalSize(); err == nil && w > 0 {
		return w
	}
	return 80
}

// LogPruneReport formats and outputs a structured prune report. Matches
// compose's display_summary.go behavior: layer rows show at -vv
// (dockerlayout.VerboseLayers) and every row unfolded at -v in the console/TUI print,
// but the log file always gets them all regardless of -v, so log lines are built
// separately rather than reusing the (possibly folded, layer-less) display lines.
func LogPruneReport(ctx context.Context, r PruneReport, imageServices map[string][]string) {
	// buildPruneLines is the only part that needs a session's render context
	// (see console.RenderWithSessionContext) -- it's pure string-building,
	// no I/O. The write loop below must run outside it: when out is a
	// ProgramBox's TUI writer (an io.Pipe), each Fprintln blocks until
	// bubbletea's main loop reads it via Update(), which is itself wrapped
	// in the same render context on every tick -- holding that context
	// across the write would deadlock this goroutine against the main loop.
	var dispLines, errs []string
	console.RenderWithSessionContext(ctx, func() {
		dispLines, errs = buildPruneLines(r, imageServices, console.VerboseLevel() >= dockerlayout.VerboseLayers,
			console.VerboseLevel() >= dockerlayout.VerboseDetails, pruneTermWidth())
	})
	if len(dispLines) == 0 && len(errs) == 0 {
		return
	}

	var out io.Writer = os.Stdout
	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		out = w
	}
	console.PauseSpinner()
	for _, line := range dispLines {
		fmt.Fprintln(out, line)
	}
	console.ResumeSpinner()

	logCtx := logger.WithSuppressWriter(ctx, logger.ConsoleWriter())
	if tuiW := console.GetTUIWriter(ctx); tuiW != nil {
		logCtx = logger.WithSuppressWriter(logCtx, tuiW)
	}
	var logLines []string
	console.RenderWithSessionContext(ctx, func() {
		logLines, _ = buildPruneLines(r, imageServices, true, true, pruneTermWidth())
	})
	const pfx = "\t{{|RunningCommand|}}docker:{{[-]}} "
	for _, line := range logLines {
		logger.Notice(logCtx, pfx+"%s", semstyle.ToPlain(line))
	}
	for _, e := range errs {
		logger.Error(logCtx, "%s", e)
	}
}

// buildPruneLines returns the prune report's lines and its error notices. Everything a
// prune does is a removal, so it all goes under changed:, as in the compose display:
// one heading per kind with its count, its rows nested under it, or for a kind whose
// rows all say Removed, one folded line standing in for the heading. showLayers adds
// each image's layers; below open (-v), rows that show the same thing fold into one
// line, wrapped to termW.
func buildPruneLines(r PruneReport, imageServices map[string][]string, showLayers, open bool, termW int) ([]string, []string) {
	doneIcon := "{{|DockerMarkerDone|}}✓{{[-]}}"
	errorIcon := "{{|DockerMarkerError|}}×{{[-]}}"
	if r.AsciiMode {
		doneIcon = "{{|DockerMarkerDone|}}+{{[-]}}"
		errorIcon = "{{|DockerMarkerError|}}x{{[-]}}"
	}
	doneIconANSI := semstyle.ToANSI(doneIcon)
	errorIconANSI := semstyle.ToANSI(errorIcon)

	// Final statuses take their row's style, as in the compose display: headings
	// DockerSection (changed: DockerGroupStatus); services, networks, volumes and containers
	// App; images, and their heading, DockerImage; the container_name line App (its label a
	// detail label); layers dim.
	imageRemovedStatus := semstyle.ToANSI("{{|DockerImage|}}Removed{{[-]}}")
	removedStatus := semstyle.ToANSI("{{|App|}}Removed{{[-]}}")
	errorStatus := semstyle.ToANSI("{{|DockerStatusFail|}}Error{{[-]}}")
	removedPad := strutil.Repeat(" ", SectionStatusW-len("Removed"))
	errorPad := strutil.Repeat(" ", SectionStatusW-len("Error"))
	indent := func(depth int) string { return strutil.Repeat(" ", depth*SectionChildIndentW) }

	var lines []string
	add := func(line string) { lines = append(lines, line) }

	// heading returns a heading at depth (changed: at 0, a kind's count at 1): the word
	// all its rows show, or Complete when they differ, or, when its part of the prune
	// failed, Error. changed:'s word is in the group's hue (DockerGroupStatus).
	heading := func(depth int, label, style string, hasErr bool, word string) string {
		icon, status := doneIconANSI, ""
		if word == "" {
			word = "Complete"
		}
		tag := "DockerSection"
		switch {
		case depth == 0:
			tag = "DockerGroupStatus"
		case style == dockerlayout.TitleStyle("images"):
			tag = "DockerImage" // an images heading's word is an image row's
		}
		status = semstyle.ToANSI("{{|" + tag + "|}}" + word + "{{[-]}}")
		if hasErr {
			icon, status, word = errorIconANSI, errorStatus, "Error"
		}
		pad := strutil.Repeat(" ", SectionStatusW-len(word))
		return GlobalIndent + icon + " " + status + semstyle.CodeReset + pad + indent(depth) +
			semstyle.ToANSI("{{|"+style+"|}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
	}
	// removedRow returns a removed row under a kind's heading.
	removedRow := func(nameANSI string) string {
		return GlobalIndent + doneIconANSI + " " + removedStatus + semstyle.CodeReset + removedPad +
			indent(2) + nameANSI + semstyle.ToANSI("{{|DockerColon|}}:{{[-]}}")
	}
	// foldedRow returns removed rows that show the same thing as one line at depth: how
	// many, underlined, then their names in style, wrapped to termW under the first. At
	// depth 1 it stands in for the kind's heading, in the heading's status style
	// (statusTag), not a row's.
	foldedRow := func(names []string, style, singular, plural string, depth int, statusTag string) []string {
		label := fmt.Sprintf("%d %s", len(names), Plural(len(names), singular, plural))
		status := semstyle.ToANSI("{{|" + statusTag + "|}}Removed{{[-]}}")
		prefix := GlobalIndent + doneIconANSI + " " + status + semstyle.CodeReset + removedPad +
			indent(depth) + semstyle.ToANSI("{{|"+dockerlayout.TitleStyle(plural)+"|}}{{[::U]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}") + " "
		prefixW := GlobalIndentW + IconW + SpaceW + IconW + SpaceW + SectionChildIndentW + SectionStatusW + depth*SectionChildIndentW + len(label) + 2
		room := max(termW-prefixW, 20)
		var out []string
		line, lineW := "", 0
		for i, name := range names {
			styled := semstyle.ToANSI("{{|" + style + "|}}" + name + "{{[-]}}")
			if singular == "service" { // a service's name is a link or a user app's, as everywhere
				styled = semstyle.ToANSI(dockerlayout.StyleServiceName(name))
			}
			text := name
			if i < len(names)-1 {
				text += ","
				styled += semstyle.ToANSI("{{|DockerColon|}},{{[-]}}")
			}
			w := len([]rune(text))
			if lineW > 0 && lineW+1+w > room {
				out = append(out, line)
				line, lineW = "", 0
			}
			if lineW > 0 {
				line += " "
				lineW++
			}
			line += styled
			lineW += w
		}
		out = append(out, line)
		cont := strutil.Repeat(" ", prefixW-(IconW+SpaceW+SectionChildIndentW))
		for i := range out {
			if i == 0 {
				out[i] = prefix + out[i]
			} else {
				out[i] = cont + out[i]
			}
		}
		return out
	}
	// containerNameRow renders a "container_name: <name>" sub-line under a service,
	// parallel to the image: line, with Removed status.
	containerNameRow := func(name string) string {
		return GlobalIndent + doneIconANSI + " " + removedStatus + semstyle.CodeReset + removedPad + indent(3) +
			semstyle.ToANSI("{{|DockerDetailLabel|}}container_name{{[-]}}{{|DockerColon|}}:{{[-]}} {{|App|}}"+name+"{{[-]}}")
	}

	// ── build image groups ───────────────────────────────────────────────────
	groups := pruneImageGroups(r)
	var allRefs []string
	for _, g := range groups {
		allRefs = append(allRefs, g.refs...)
	}
	refServices := servicesOfPrunedImages(allRefs, imageServices)
	imageServices = make(map[string][]string, len(groups))
	for _, g := range groups {
		var svcs []string
		for _, ref := range g.refs {
			svcs = append(svcs, refServices[ref]...)
		}
		if len(svcs) > 0 {
			slices.Sort(svcs)
			imageServices[g.ref] = slices.Compact(svcs)
		}
	}

	// Sort: compose-known first by service name, then unknown by ref.
	sort.SliceStable(groups, func(i, j int) bool {
		si := imageServices[groups[i].ref]
		sj := imageServices[groups[j].ref]
		switch {
		case len(si) > 0 && len(sj) == 0:
			return true
		case len(si) == 0 && len(sj) > 0:
			return false
		case len(si) > 0 && len(sj) > 0:
			return si[0] < sj[0]
		default:
			return groups[i].ref < groups[j].ref
		}
	})

	// ── header ───────────────────────────────────────────────────────────────
	var totalImages, totalLayers int
	seenSvcsForHeader := make(map[string]bool)
	seenLayers := make(map[string]bool)
	for _, g := range groups {
		totalImages++
		for _, l := range g.layers {
			if !l.kept && !seenLayers[l.id] {
				seenLayers[l.id] = true
				totalLayers++
			}
		}
		for _, svc := range imageServices[g.ref] {
			seenSvcsForHeader[svc] = true
		}
	}
	totalServices := len(seenSvcsForHeader)

	var headerParts []string
	if totalServices > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", totalServices, Plural(totalServices, "service", "services")))
	}
	if totalImages > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|DockerImage|}}%d %s{{[-]}}", totalImages, Plural(totalImages, "image", "images")))
	}
	if totalLayers > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{[::D]}}%d %s{{[-]}}", totalLayers, Plural(totalLayers, "layer", "layers")))
	}
	if len(r.NetworksDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.NetworksDeleted), Plural(len(r.NetworksDeleted), "network", "networks")))
	}
	if len(r.VolumesDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.VolumesDeleted), Plural(len(r.VolumesDeleted), "volume", "volumes")))
	}
	if len(r.ContainersDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.ContainersDeleted), Plural(len(r.ContainersDeleted), "container", "containers")))
	}
	if len(headerParts) > 0 {
		// Overall marker: error if any category failed, else success. Prune is rendered
		// after completion, so there's no spinner — just the final marker (icon + space),
		// matching the compose summary header. The docker command is bold yellow; colon
		// DockerColon.
		marker := doneIconANSI
		if r.hasErrors() {
			marker = errorIconANSI
		}
		header := semstyle.ToANSI(fmt.Sprintf("{{[yellow::B]}}docker system prune{{[-]}}{{|DockerColon|}}:{{[-]}} %s", strings.Join(headerParts, ", ")))
		add(GlobalIndent + marker + " " + header)
	}
	// All lines after the header nest under it (indent by icon + space), matching compose.
	headerEnd := len(lines)

	// Resolve deleted containers to their compose service/name. A container nests under
	// its service (when that service's image was also pruned); the rest are listed under
	// their own heading. svcContainerName maps service -> container name for nesting;
	// leftover IDs (no service, or service not in an image group) are listed apart.
	svcContainerName := make(map[string]string)
	servicesShown := map[string]bool{}
	for _, g := range groups {
		for _, svc := range imageServices[g.ref] {
			servicesShown[svc] = true
		}
	}
	var leftoverContainers []string
	for _, id := range r.ContainersDeleted {
		meta, ok := r.ContainerInfo[id]
		if ok && servicesShown[meta.service] {
			svcContainerName[meta.service] = meta.name
			continue
		}
		// Not nestable: show the container name if known, else the truncated raw ID.
		if ok && meta.name != "" {
			leftoverContainers = append(leftoverContainers, meta.name)
		} else {
			tid := id
			if len(tid) > 12 {
				tid = tid[:12]
			}
			leftoverContainers = append(leftoverContainers, tid)
		}
	}

	var serviceGroups, imageOnly []imageGroup
	for _, g := range groups {
		if len(imageServices[g.ref]) > 0 {
			serviceGroups = append(serviceGroups, g)
		} else {
			imageOnly = append(imageOnly, g)
		}
	}

	// What the rows under each heading say, when they all say it: its word passes up to
	// the heading, and a kind's folded line stands in for its heading. All rows say
	// Removed, as compose says for each kind, so images pass it up unless one failed or,
	// with layers shown, a layer failed or is Kept.
	imagesWordOf := func(gs []imageGroup) string {
		if len(gs) == 0 || r.ImagesError != nil {
			return ""
		}
		for _, g := range gs {
			if g.refStatus == statusFailed {
				return ""
			}
			for _, l := range g.layers {
				if showLayers && (l.kept || l.status == statusFailed) {
					return ""
				}
			}
		}
		return "Removed"
	}
	imagesWord := imagesWordOf(imageOnly)
	servicesWord := imagesWordOf(serviceGroups)
	removedWord := func(n int, err error) string {
		if n > 0 && err == nil {
			return "Removed"
		}
		return ""
	}
	var changedWords []string
	if len(serviceGroups) > 0 {
		changedWords = append(changedWords, servicesWord)
	}
	if len(imageOnly) > 0 {
		changedWords = append(changedWords, imagesWord)
	}
	for _, k := range []struct {
		n   int
		err error
	}{{len(r.NetworksDeleted), r.NetworksError}, {len(r.VolumesDeleted), r.VolumesError}, {len(leftoverContainers), r.ContainersError}} {
		if k.n > 0 || k.err != nil {
			changedWords = append(changedWords, removedWord(k.n, k.err))
		}
	}
	changedWord := ""
	if len(changedWords) > 0 && !r.hasErrors() {
		changedWord = changedWords[0]
		for _, w := range changedWords[1:] {
			if w != changedWord {
				changedWord = ""
			}
		}
	}
	if len(groups) > 0 || len(r.NetworksDeleted) > 0 || len(r.VolumesDeleted) > 0 || len(leftoverContainers) > 0 || r.hasErrors() {
		add(heading(0, "changed", "DockerGroupHeading", r.hasErrors(), changedWord))
	}

	// ── services and images ─────────────────────────────────────────────────
	layerRemovedANSI := semstyle.ToANSI("{{[::D]}}Removed{{[-]}}") + removedPad
	layerFailedANSI := semstyle.ToANSI("{{|DockerStatusFail|}}Failed{{[-]}}") + strutil.Repeat(" ", SectionStatusW-len("Failed"))
	layerKeptANSI := semstyle.ToANSI("{{[::D]}}Kept{{[-]}}") + strutil.Repeat(" ", SectionStatusW-len("Kept"))

	// renderImage adds an image's line with its label at depth, then with showLayers
	// its layers, one step in from that label.
	renderImage := func(g imageGroup, depth int) {
		ref := g.ref
		if ref == "" {
			ref = "<dangling>"
		}
		// Match compose: append the layer count directly to the image URL ("ref [N]")
		// rather than in a separate padded column. "[N]" uses DockerTag brackets, dim interior.
		freed := 0
		for _, l := range g.layers {
			if !l.kept {
				freed++
			}
		}
		layerCount := fmt.Sprintf(" {{|DockerTag|}}[{{[-]}}{{[::D]}}%d{{[-]}}{{|DockerTag|}}]{{[-]}}", freed)
		imgIcon, imgStatus, imgPad := doneIconANSI, imageRemovedStatus, removedPad
		if g.refStatus == statusFailed {
			imgIcon, imgStatus, imgPad = errorIconANSI, errorStatus, errorPad
		}
		add(GlobalIndent + imgIcon + " " + imgStatus + semstyle.CodeReset + imgPad + indent(depth) +
			semstyle.ToANSI(dockerlayout.Title("image", "image", false)+"{{|DockerColon|}}:{{[-]}} "+StyleImageRef(shortRefDigest(ref))+layerCount))
		if !showLayers {
			return
		}
		for _, l := range g.layers {
			lIcon, lStatus := doneIconANSI, layerRemovedANSI
			switch {
			case l.status == statusFailed:
				lIcon, lStatus = errorIconANSI, layerFailedANSI
			case l.kept:
				lStatus = layerKeptANSI
			}
			lid := strings.TrimPrefix(l.id, "sha256:")
			if len(lid) > 12 {
				lid = lid[:12]
			}
			add(GlobalIndent + lIcon + " " + lStatus + indent(depth+1) + semstyle.ToANSI("{{[::D]}}"+lid+"{{[-]}}"))
		}
	}

	if len(serviceGroups) > 0 {
		add(heading(1, fmt.Sprintf("%d %s", totalServices, Plural(totalServices, "service", "services")), dockerlayout.TitleStyle("services"), r.ImagesError != nil, servicesWord))
		for _, g := range serviceGroups {
			svcs := imageServices[g.ref]
			// Services of one image that show the same thing (none with a container line
			// of its own) fold into one line, as in the compose display.
			fold := !open && len(svcs) > 1
			for _, svc := range svcs {
				if _, ok := svcContainerName[svc]; ok {
					fold = false
				}
			}
			if fold {
				for _, line := range foldedRow(svcs, "App", "service", "services", 2, "App") {
					add(line)
				}
			} else {
				for _, svc := range svcs {
					add(removedRow(semstyle.ToANSI(dockerlayout.StyleServiceName(svc))))
					if name, ok := svcContainerName[svc]; ok {
						add(containerNameRow(name))
					}
				}
			}
			renderImage(g, 3)
		}
	}
	// Images no compose service names: their own heading beside the services, each
	// image its own row.
	if len(imageOnly) > 0 || r.ImagesError != nil && len(groups) == 0 {
		label := "images"
		if len(imageOnly) > 0 {
			label = "<no service>"
		}
		add(heading(1, label, dockerlayout.TitleStyle("images"), r.ImagesError != nil, imagesWord))
		for _, g := range imageOnly {
			renderImage(g, 2)
		}
	}

	// ── networks, volumes, containers ───────────────────────────────────────
	// Removed rows with nothing under them, which fold into one line below -v.
	kind := func(names []string, hasErr bool, style, singular, plural string) {
		if len(names) == 0 && !hasErr {
			return
		}
		// A kind whose rows all say Removed folds onto its heading: one line, or below -v
		// the same, a lone row included.
		if !hasErr && (len(names) == 1 || !open) {
			for _, line := range foldedRow(names, style, singular, plural, 1, "DockerSection") {
				add(line)
			}
			return
		}
		label := plural
		if len(names) > 0 {
			label = fmt.Sprintf("%d %s", len(names), Plural(len(names), singular, plural))
		}
		add(heading(1, label, dockerlayout.TitleStyle(plural), hasErr, removedWord(len(names), nil)))
		if !open && len(names) > 1 {
			for _, line := range foldedRow(names, style, singular, plural, 2, "App") {
				add(line)
			}
			return
		}
		for _, name := range names {
			add(removedRow(semstyle.ToANSI("{{|" + style + "|}}" + name + "{{[-]}}")))
		}
	}
	kind(r.NetworksDeleted, r.NetworksError != nil, "IPAddress", "network", "networks")
	volumes := make([]string, len(r.VolumesDeleted))
	for i, v := range r.VolumesDeleted {
		volumes[i] = shortVolumeName(v)
	}
	kind(volumes, r.VolumesError != nil, "Folder", "volume", "volumes")
	kind(leftoverContainers, r.ContainersError != nil, "App", "container", "containers")

	// ── error notices ─────────────────────────────────────────────────────────
	var errs []string
	if r.ImagesError != nil {
		errs = append(errs, "images: "+r.ImagesError.Error())
	}
	if r.NetworksError != nil {
		errs = append(errs, "networks: "+r.NetworksError.Error())
	}
	if r.VolumesError != nil {
		errs = append(errs, "volumes: "+r.VolumesError.Error())
	}
	if r.ContainersError != nil {
		errs = append(errs, "containers: "+r.ContainersError.Error())
	}

	// Indent all lines after the header so the block nests under it (matching compose).
	// Lines carry their own global indent, so add icon + space + one child-indent step to
	// place content markers under the 3rd char of the header text.
	headerIndent := strutil.Repeat(" ", IconW+SpaceW+SectionChildIndentW)
	for i := headerEnd; i < len(lines); i++ {
		lines[i] = headerIndent + lines[i]
	}

	// ── summary ───────────────────────────────────────────────────────────────
	// Added after the indent loop so it aligns with "prune:" in the header, not the children.
	if r.SpaceReclaimed > 0 {
		lines = append(lines, GlobalIndent+strutil.Repeat(" ", IconW+SpaceW)+semstyle.ToANSI("{{[white::B]}}Total reclaimed space:{{[-]}} {{|DockerMarkerDone|}}"+
			units.HumanSize(float64(r.SpaceReclaimed))+"{{[-]}}"))
	}

	return lines, errs
}

// pruneImageGroups splits prune's deleted list into images: each image's untagged names,
// then its deletions. An image found in r.ImageLayers, by its ID among those deletions,
// shows the layers inspected before pruning, since the rest of the deletions aren't
// always layers (on the containerd store they're the image's manifests); a layer an
// image that stays still has is marked kept. Any other image shows its deletions.
func pruneImageGroups(r PruneReport) []imageGroup {
	type run struct{ refs, deleted []string }
	var runs []run
	for _, item := range r.ImagesDeleted {
		if item.Untagged != "" && (len(runs) == 0 || len(runs[len(runs)-1].deleted) > 0) {
			runs = append(runs, run{})
		}
		if len(runs) == 0 {
			runs = append(runs, run{})
		}
		cur := &runs[len(runs)-1]
		if item.Untagged != "" {
			cur.refs = append(cur.refs, item.Untagged)
		}
		if item.Deleted != "" {
			cur.deleted = append(cur.deleted, item.Deleted)
		}
	}

	ids := make([]string, len(runs))
	removed := make(map[string]bool)
	for i, rn := range runs {
		for _, d := range rn.deleted {
			if _, ok := r.ImageLayers[d]; ok {
				ids[i] = d
				removed[d] = true
				break
			}
		}
	}
	keptLayers := make(map[string]bool)
	for id, layers := range r.ImageLayers {
		if !removed[id] {
			for _, l := range layers {
				keptLayers[l] = true
			}
		}
	}

	groups := make([]imageGroup, 0, len(runs))
	for i, rn := range runs {
		g := imageGroup{ref: displayRef(rn.refs), refs: rn.refs, refStatus: statusRemoved}
		if ids[i] == "" {
			for _, d := range rn.deleted {
				g.layers = append(g.layers, layerEntry{id: d, status: statusRemoved})
			}
		} else {
			for _, l := range r.ImageLayers[ids[i]] {
				kept := keptLayers[l]
				if kept && !showKeptLayers {
					continue
				}
				g.layers = append(g.layers, layerEntry{id: l, status: statusRemoved, kept: kept})
			}
		}
		groups = append(groups, g)
	}
	return groups
}

// displayRef returns the name an image is shown by: its first tag, else its first
// digest ref, else its first name; "" for none.
func displayRef(refs []string) string {
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "sha256:") && !strings.Contains(ref, "@") {
			return ref
		}
	}
	for _, ref := range refs {
		if strings.Contains(ref, "@") {
			return ref
		}
	}
	if len(refs) > 0 {
		return refs[0]
	}
	return ""
}

// servicesOfPrunedImages maps each pruned image ref to the compose services configured
// with that exact image: the same name and tag (a missing tag meaning latest) or the
// same digest. An image of a service's repository under another tag or digest isn't
// known to be that service's, so it matches nothing; nor do image IDs.
func servicesOfPrunedImages(refs []string, imageServices map[string][]string) map[string][]string {
	byKey := map[string][]string{}
	for img, svcs := range imageServices {
		if key := imageKey(img); key != "" {
			byKey[key] = append(byKey[key], svcs...)
		}
	}
	out := make(map[string][]string, len(refs))
	for _, ref := range refs {
		if svcs, ok := imageServices[ref]; ok {
			out[ref] = svcs
			continue
		}
		if ref == "" || strings.HasPrefix(ref, "sha256:") {
			continue
		}
		if svcs := byKey[imageKey(ref)]; len(svcs) > 0 {
			svcs = slices.Clone(svcs)
			slices.Sort(svcs)
			out[ref] = slices.Compact(svcs)
		}
	}
	return out
}

// imageKey returns ref in its full form, with latest for a missing tag
// (busybox → docker.io/library/busybox:latest); "" when it doesn't parse.
func imageKey(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ""
	}
	return reference.TagNameOnly(named).String()
}

// shortRefDigest returns ref with a digest after its repository cut to its first 12
// characters, as Docker shows IDs; other refs as they are.
func shortRefDigest(ref string) string {
	repo, digest, ok := strings.Cut(ref, "@sha256:")
	if !ok || len(digest) <= 12 {
		return ref
	}
	return repo + "@sha256:" + digest[:12]
}
