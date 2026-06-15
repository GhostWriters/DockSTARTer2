package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-units"
)

const (
	// pruneImageColW: column where the 2*childIndent prefix before "image:" starts
	pruneImageColW = GlobalIndentW + IconW + SpaceW + SectionStatusW
	// pruneLayerPrefixW: indent for layer icon — sits under the "a" of "image:"
	pruneLayerPrefixW = pruneImageColW + 2*SectionChildIndentW + 2
	// pruneLayerStatusW: matches compose layerStatusW so columns align when -p and -c are combined
	pruneLayerStatusW = LayerStatusW
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
}

func (r *PruneReport) hasErrors() bool {
	return r.ImagesError != nil || r.NetworksError != nil ||
		r.VolumesError != nil || r.ContainersError != nil
}

// imageGroup is the display unit — one ref with its layers and per-layer status.
type imageGroup struct {
	ref       string
	layers    []layerEntry
	refStatus entryStatus
}

type layerEntry struct {
	id     string
	status entryStatus
}

type entryStatus int

const (
	statusRemoved   entryStatus = iota // ✓ Removed / Untagged / Deleted
	statusFailed                       // ⚠ Failed — expected but not in deleted list
)

// LogPruneReport formats and outputs a structured prune report.
func LogPruneReport(ctx context.Context, r PruneReport, imageServices map[string][]string) {
	lines, errs := buildPruneLines(r, imageServices)
	if len(lines) == 0 && len(errs) == 0 {
		return
	}

	var out io.Writer = console.ViewportWriter()
	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		out = w
	}
	console.PauseSpinner()
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	console.ResumeSpinner()

	logCtx := logger.WithSuppressWriter(ctx, logger.ConsoleWriter())
	if tuiW := console.GetTUIWriter(ctx); tuiW != nil {
		logCtx = logger.WithSuppressWriter(logCtx, tuiW)
	}
	const pfx = "{{|RunningCommand|}}docker:{{[-]}} "
	for _, line := range lines {
		logger.Notice(logCtx, pfx+"%s", console.Strip(line))
	}
	for _, e := range errs {
		logger.Error(logCtx, "%s", e)
	}
}

func buildPruneLines(r PruneReport, imageServices map[string][]string) ([]string, []string) {
	doneIcon := "{{|DockerMarkerDone|}}✓{{[-]}}"
	errorIcon := "{{|DockerMarkerError|}}×{{[-]}}"
	if r.AsciiMode {
		doneIcon = "{{|DockerMarkerDone|}}+{{[-]}}"
		errorIcon = "{{|DockerMarkerError|}}x{{[-]}}"
	}
	doneIconANSI := console.ToConsoleANSI(doneIcon)
	errorIconANSI := console.ToConsoleANSI(errorIcon)

	untaggedStatus := console.ToConsoleANSI("{{|DockerStatusFinal|}}Untagged{{[-]}}")
	removedStatus := console.ToConsoleANSI("{{|DockerStatusFinal|}}Removed{{[-]}}")
	errorStatus := console.ToConsoleANSI("{{|DockerStatusFail|}}Error{{[-]}}")
	untaggedPad := strutil.Repeat(" ", SectionStatusW-len("Untagged"))
	removedPad := strutil.Repeat(" ", SectionStatusW-len("Removed"))
	errorPad := strutil.Repeat(" ", SectionStatusW-len("Error"))

	iconStatus := func(s entryStatus) (icon, status, pad string) {
		if s == statusFailed {
			return errorIconANSI, errorStatus, errorPad
		}
		return doneIconANSI, removedStatus, removedPad
	}

	var lines []string
	add := func(line string) { lines = append(lines, line) }

	sectionHeader := func(label string, hasErr bool) string {
		if hasErr {
			return GlobalIndent + errorIconANSI + " " + errorStatus + console.CodeReset + errorPad +
				console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
		}
		return GlobalIndent + doneIconANSI + " " + removedStatus + console.CodeReset + removedPad +
			console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
	}
	childRow := func(name, colorTag string, s entryStatus) string {
		icon, status, pad := iconStatus(s)
		return GlobalIndent + icon + " " + status + console.CodeReset + pad +
			SectionChildIndent + console.ToConsoleANSI(colorTag+name+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
	}

	// ── build image groups ───────────────────────────────────────────────────
	var groups []imageGroup
	var current *imageGroup
	for _, item := range r.ImagesDeleted {
		if item.Untagged != "" {
			groups = append(groups, imageGroup{ref: item.Untagged, refStatus: statusRemoved})
			current = &groups[len(groups)-1]
		}
		if item.Deleted != "" {
			if current == nil {
				groups = append(groups, imageGroup{ref: ""})
				current = &groups[len(groups)-1]
			}
			current.layers = append(current.layers, layerEntry{id: item.Deleted, status: statusRemoved})
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
	for _, g := range groups {
		totalImages++
		totalLayers += len(g.layers)
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
		// matching the compose summary header. Command word is bold yellow; colon DockerColon.
		anyErr := r.ImagesError != nil || r.NetworksError != nil || r.VolumesError != nil || r.ContainersError != nil
		marker := doneIconANSI
		if anyErr {
			marker = errorIconANSI
		}
		header := console.ToConsoleANSI(fmt.Sprintf("{{[yellow::B]}}prune{{[-]}}{{|DockerColon|}}:{{[-]}} %s", strings.Join(headerParts, ", ")))
		add(GlobalIndent + marker + " " + header)
	}
	// All lines after the header nest under it (indent by icon + space), matching compose.
	headerEnd := len(lines)

	// ── images / services section ─────────────────────────────────────────────
	if len(groups) > 0 {
		imageLabel := strutil.Repeat(" ", 2*SectionChildIndentW) +
			console.ToConsoleANSI("{{|DockerMarkerDone|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")

		layerIconIndent := strutil.Repeat(" ", pruneLayerPrefixW)

		layerDeletedPad := strutil.Repeat(" ", pruneLayerStatusW-len("Deleted"))
		layerFailedPad  := strutil.Repeat(" ", pruneLayerStatusW-len("Failed"))
		layerDeletedANSI := console.ToConsoleANSI("{{|DockerStatusFinal|}}Deleted{{[-]}}") + layerDeletedPad
		layerFailedANSI  := console.ToConsoleANSI("{{|DockerStatusFail|}}Failed{{[-]}}") + layerFailedPad

		renderImageGroup := func(g imageGroup) {
			ref := g.ref
			if ref == "" {
				ref = "<dangling>"
			}
			refANSI := StyleImageRef(ref)
			// Match compose: append the layer count directly to the image URL ("ref [N]")
			// rather than in a separate padded column. "[N]" uses DockerTag brackets, dim interior.
			layerCount := console.ToConsoleANSI(fmt.Sprintf(" {{|DockerTag|}}[{{[-]}}{{[::D]}}%d{{[-]}}{{|DockerTag|}}]{{[-]}}", len(g.layers)))

			// Image ref row — Untagged or Error.
			var imgIcon, imgStatus, imgPad string
			if g.refStatus == statusFailed {
				imgIcon, imgStatus, imgPad = errorIconANSI, errorStatus, errorPad
			} else {
				imgIcon, imgStatus, imgPad = doneIconANSI, untaggedStatus, untaggedPad
			}
			imgLine := GlobalIndent + imgIcon + " " + imgStatus + console.CodeReset + imgPad +
				imageLabel + refANSI + layerCount
			add(imgLine)

			// Layer rows — only shown in verbose mode, compact dim style like compose.
			if console.GlobalVerbose {
				for _, l := range g.layers {
					var lIcon, lStatus string
					if l.status == statusFailed {
						lIcon, lStatus = errorIconANSI, layerFailedANSI
					} else {
						lIcon, lStatus = doneIconANSI, layerDeletedANSI
					}
					lid := strings.TrimPrefix(l.id, "sha256:")
					if len(lid) > 12 {
						lid = lid[:12]
					}
				add(layerIconIndent + console.CodeDim + lIcon + " " + lStatus + " " +
						console.ToConsoleANSI("{{[::D]}}"+lid+"{{[-]}}") + console.CodeDimOff)
				}
			}
		}

		add(sectionHeader("services", r.ImagesError != nil))
		unknownCount := 0
		for _, g := range groups {
			svcs := imageServices[g.ref]
			if len(svcs) > 0 {
				for _, svc := range svcs {
					add(childRow(svc, "{{|App|}}", statusRemoved))
				}
			} else {
				unknownCount++
				add(childRow(fmt.Sprintf("<Unknown%d>", unknownCount), "{{|App|}}", statusRemoved))
			}
			renderImageGroup(g)
		}
	} else if r.ImagesError != nil {
		add(sectionHeader("services", true))
	}

	// ── networks ─────────────────────────────────────────────────────────────
	if len(r.NetworksDeleted) > 0 || r.NetworksError != nil {
		add(sectionHeader("networks", r.NetworksError != nil))
		for _, net := range r.NetworksDeleted {
			add(childRow(net, "{{|App|}}", statusRemoved))
		}
	}

	// ── volumes ──────────────────────────────────────────────────────────────
	if len(r.VolumesDeleted) > 0 || r.VolumesError != nil {
		add(sectionHeader("volumes", r.VolumesError != nil))
		for _, vol := range r.VolumesDeleted {
			add(childRow(vol, "{{|App|}}", statusRemoved))
		}
	}

	// ── containers ───────────────────────────────────────────────────────────
	if len(r.ContainersDeleted) > 0 || r.ContainersError != nil {
		add(sectionHeader("containers", r.ContainersError != nil))
		for _, ctr := range r.ContainersDeleted {
			add(childRow(ctr, "{{|App|}}", statusRemoved))
		}
	}

	// ── summary ───────────────────────────────────────────────────────────────
	if r.SpaceReclaimed > 0 {
		add(console.ToConsoleANSI("{{[white::B]}}Total reclaimed space:{{[-]}} {{|DockerMarkerDone|}}" +
			units.HumanSize(float64(r.SpaceReclaimed)) + "{{[-]}}"))
	}

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
	return lines, errs
}

