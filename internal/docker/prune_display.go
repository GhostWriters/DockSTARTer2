package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-units"
)

const (
	pruneGlobalIndentW       = 1
	pruneIconW               = 1
	pruneSpaceW              = 1
	pruneSectionStatusW      = 13
	pruneSectionChildIndentW = 2
	pruneImageLabelTextW     = len("image: ")

	// pruneImageColW: column where the 2*childIndent prefix before "image:" starts
	pruneImageColW = pruneGlobalIndentW + pruneIconW + pruneSpaceW + pruneSectionStatusW
	// pruneLayerPrefixW: indent for layer icon — sits under the "a" of "image:"
	pruneLayerPrefixW = pruneImageColW + 2*pruneSectionChildIndentW + 2
)

var (
	pruneGlobalIndent       = strutil.Repeat(" ", pruneGlobalIndentW)
	pruneSectionChildIndent = strutil.Repeat(" ", pruneSectionChildIndentW)
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
	untaggedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Untagged"))
	removedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Removed"))
	errorPad := strutil.Repeat(" ", pruneSectionStatusW-len("Error"))

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
			return pruneGlobalIndent + errorIconANSI + " " + errorStatus + console.CodeReset + errorPad +
				console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
		}
		return pruneGlobalIndent + doneIconANSI + " " + removedStatus + console.CodeReset + removedPad +
			console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
	}
	childRow := func(name, colorTag string, s entryStatus) string {
		icon, status, pad := iconStatus(s)
		return pruneGlobalIndent + icon + " " + status + console.CodeReset + pad +
			pruneSectionChildIndent + console.ToConsoleANSI(colorTag+name+"{{[-]}}")
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
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", totalServices, prunePlural(totalServices, "service", "services")))
	}
	if totalImages > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|DockerImage|}}%d %s{{[-]}}", totalImages, prunePlural(totalImages, "image", "images")))
	}
	if totalLayers > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{[::D]}}%d %s{{[-]}}", totalLayers, prunePlural(totalLayers, "layer", "layers")))
	}
	if len(r.NetworksDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.NetworksDeleted), prunePlural(len(r.NetworksDeleted), "network", "networks")))
	}
	if len(r.VolumesDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.VolumesDeleted), prunePlural(len(r.VolumesDeleted), "volume", "volumes")))
	}
	if len(r.ContainersDeleted) > 0 {
		headerParts = append(headerParts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", len(r.ContainersDeleted), prunePlural(len(r.ContainersDeleted), "container", "containers")))
	}
	if len(headerParts) > 0 {
		add(console.ToConsoleANSI(fmt.Sprintf("{{|RunningCommand|}}prune:{{[-]}} %s", strings.Join(headerParts, ", "))))
	}

	// ── images / services section ─────────────────────────────────────────────
	if len(groups) > 0 {
		maxRefW := 0
		for _, g := range groups {
			ref := g.ref
			if ref == "" {
				ref = "<dangling>"
			}
			if w := utf8.RuneCountInString(console.Strip(styleImageRef(ref))); w > maxRefW {
				maxRefW = w
			}
		}

		imageLabel := strutil.Repeat(" ", 2*pruneSectionChildIndentW) +
			console.ToConsoleANSI("{{|DockerMarkerDone|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")

		layerIconIndent := strutil.Repeat(" ", pruneLayerPrefixW)

		// Compact layer status labels (no full status-column padding — matches compose layer style).
		layerDeletedANSI := console.ToConsoleANSI("{{|DockerStatusFinal|}}Deleted   {{[-]}}")
		layerFailedANSI  := console.ToConsoleANSI("{{|DockerStatusFail|}}Failed    {{[-]}}")

		renderImageGroup := func(g imageGroup) {
			ref := g.ref
			if ref == "" {
				ref = "<dangling>"
			}
			refANSI := styleImageRef(ref)
			refPad := strutil.Repeat(" ", maxRefW-utf8.RuneCountInString(console.Strip(refANSI)))
			layerCount := console.ToConsoleANSI(fmt.Sprintf(" {{[::D]}}(%d %s){{[-]}}", len(g.layers), prunePlural(len(g.layers), "layer", "layers")))

			// Image ref row — Untagged or Error.
			var imgIcon, imgStatus, imgPad string
			if g.refStatus == statusFailed {
				imgIcon, imgStatus, imgPad = errorIconANSI, errorStatus, errorPad
			} else {
				imgIcon, imgStatus, imgPad = doneIconANSI, untaggedStatus, untaggedPad
			}
			imgLine := pruneGlobalIndent + imgIcon + " " + imgStatus + console.CodeReset + imgPad +
				imageLabel + refANSI + refPad + layerCount
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
				add(layerIconIndent + console.CodeDim + lIcon + " " + lStatus +
						console.ToConsoleANSI("{{[::D]}}"+lid+"{{[-]}}") + console.CodeDimOff)
				}
			}
		}

		add(sectionHeader("services", r.ImagesError != nil))
		for _, g := range groups {
			svcs := imageServices[g.ref]
			if len(svcs) > 0 {
				for _, svc := range svcs {
					add(childRow(svc+":", "{{|App|}}", statusRemoved))
				}
			} else {
				add(childRow("<Unknown>:", "{{[::D]}}", statusRemoved))
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
	return lines, errs
}

func styleImageRef(ref string) string {
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		return console.ToConsoleANSI("{{|DockerImage|}}" + ref[:idx] + "{{[-]}}{{|DockerTag|}}:" + ref[idx+1:] + "{{[-]}}")
	}
	return console.ToConsoleANSI("{{|DockerImage|}}" + ref + "{{[-]}}")
}

func prunePlural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
