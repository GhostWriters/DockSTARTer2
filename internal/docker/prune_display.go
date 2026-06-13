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
)

var (
	pruneGlobalIndent       = strings.Repeat(" ", pruneGlobalIndentW)
	pruneSectionChildIndent = strings.Repeat(" ", pruneSectionChildIndentW)
)

// PruneReport holds structured prune results for display.
type PruneReport struct {
	Candidates        []ImageCandidate
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
	ref        string
	layers     []layerEntry
	refStatus  entryStatus // Untagged or Failed
	anyFailed  bool
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
	lines := buildPruneLines(r, imageServices)
	if len(lines) == 0 {
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
	const pfx = "{{|RunningCommand|}}docker:{{[-]}} "
	for _, line := range lines {
		logger.Notice(logCtx, pfx+"%s", console.Strip(line))
	}
}

func buildPruneLines(r PruneReport, imageServices map[string][]string) []string {
	doneIcon := "{{|DockerSuccess|}}✓{{[-]}}"
	warnIcon := "{{|DockerWarn|}}⚠{{[-]}}"
	if r.AsciiMode {
		doneIcon = "{{|DockerSuccess|}}+{{[-]}}"
		warnIcon = "{{|DockerWarn|}}!{{[-]}}"
	}
	doneIconANSI := console.ToConsoleANSI(doneIcon)
	warnIconANSI := console.ToConsoleANSI(warnIcon)

	untaggedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Untagged{{[-]}}")
	deletedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Deleted{{[-]}}")
	removedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Removed{{[-]}}")
	failedStatus := console.ToConsoleANSI("{{|DockerWarn|}}Failed{{[-]}}")
	untaggedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Untagged"))
	deletedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Deleted"))
	removedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Removed"))
	failedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Failed"))
	incompletePad := strutil.Repeat(" ", pruneSectionStatusW-len("Incomplete"))
	incompleteStatus := console.ToConsoleANSI("{{|DockerWarn|}}Incomplete{{[-]}}")

	iconStatus := func(s entryStatus) (icon, status, pad string) {
		if s == statusFailed {
			return warnIconANSI, failedStatus, failedPad
		}
		return doneIconANSI, removedStatus, removedPad
	}

	var lines []string
	add := func(line string) { lines = append(lines, line) }

	sectionHeader := func(label string, err error) string {
		if err != nil {
			return pruneGlobalIndent + warnIconANSI + " " + incompleteStatus + console.CodeReset + incompletePad +
				console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}} {{|DockerWarn|}}"+err.Error()+"{{[-]}}")
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
	// Index what was actually deleted.
	untaggedSet := make(map[string]bool)
	deletedSet := make(map[string]bool)
	for _, item := range r.ImagesDeleted {
		if item.Untagged != "" {
			untaggedSet[item.Untagged] = true
		}
		if item.Deleted != "" {
			deletedSet[item.Deleted] = true
		}
	}

	var groups []imageGroup

	if len(r.Candidates) > 0 {
		// Build a map from ref → deleted layers (from actual prune output).
		// Layers in ImagesDeleted are grouped: Untagged entries mark image boundaries,
		// Deleted entries following belong to that image.
		refLayers := make(map[string][]string)
		var curRef string
		for _, item := range r.ImagesDeleted {
			if item.Untagged != "" {
				curRef = item.Untagged
			}
			if item.Deleted != "" && curRef != "" {
				refLayers[curRef] = append(refLayers[curRef], item.Deleted)
			}
		}

		// Use pre-flight candidate refs; layers come from actual deleted output.
		// Candidate layers (from ImageHistory) are used only for failure detection.
		candidateLayerSet := make(map[string]map[string]bool)
		for _, c := range r.Candidates {
			if len(c.Layers) > 0 {
				s := make(map[string]bool, len(c.Layers))
				for _, l := range c.Layers {
					s[l] = true
				}
				candidateLayerSet[c.Ref] = s
			}
		}

		for _, c := range r.Candidates {
			ref := c.Ref
			refStatus := statusRemoved
			if !untaggedSet[ref] {
				refStatus = statusFailed
			}

			// Layers: use what was actually deleted for this ref.
			deleted := refLayers[ref]
			// Detect failed layers: candidate history layers not in deleted set.
			anyFailed := refStatus == statusFailed
			var layerEntries []layerEntry
			for _, lid := range deleted {
				short := strings.TrimPrefix(lid, "sha256:")
				if len(short) > 12 {
					short = short[:12]
				}
				layerEntries = append(layerEntries, layerEntry{id: short, status: statusRemoved})
			}
			// Check candidate layers against deleted to find failures.
			if cls := candidateLayerSet[ref]; len(cls) > 0 {
				for lid := range cls {
					if !deletedSet[lid] {
						anyFailed = true
						break
					}
				}
			}

			groups = append(groups, imageGroup{
				ref:       ref,
				layers:    layerEntries,
				refStatus: refStatus,
				anyFailed: anyFailed,
			})
		}
		// Also include anything deleted that wasn't in candidates (dangling etc).
		inCandidates := make(map[string]bool, len(r.Candidates))
		for _, c := range r.Candidates {
			inCandidates[c.Ref] = true
		}
		var danglingCurrent *imageGroup
		for _, item := range r.ImagesDeleted {
			if item.Untagged != "" && !inCandidates[item.Untagged] {
				groups = append(groups, imageGroup{ref: item.Untagged, refStatus: statusRemoved})
				danglingCurrent = &groups[len(groups)-1]
			}
			if item.Deleted != "" && danglingCurrent != nil {
				short := strings.TrimPrefix(item.Deleted, "sha256:")
				if len(short) > 12 {
					short = short[:12]
				}
				danglingCurrent.layers = append(danglingCurrent.layers, layerEntry{id: short, status: statusRemoved})
			}
		}
	} else {
		// No candidates — fall back to deleted-only view (original behaviour).
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
				short := strings.TrimPrefix(item.Deleted, "sha256:")
				if len(short) > 12 {
					short = short[:12]
				}
				current.layers = append(current.layers, layerEntry{id: short, status: statusRemoved})
			}
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

		imageLabel := strings.Repeat(" ", 2*pruneSectionChildIndentW) +
			console.ToConsoleANSI("{{|DockerSuccess|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")
		imageLabelPlainW := 2*pruneSectionChildIndentW + len("image: ")
		layerIndent := strings.Repeat(" ", imageLabelPlainW+pruneSectionChildIndentW)

		renderImageGroup := func(g imageGroup) {
			ref := g.ref
			if ref == "" {
				ref = "<dangling>"
			}
			refANSI := styleImageRef(ref)
			refPad := strutil.Repeat(" ", maxRefW-utf8.RuneCountInString(console.Strip(refANSI)))
			layerCount := console.ToConsoleANSI(fmt.Sprintf(" {{[::D]}}(%d %s){{[-]}}", len(g.layers), prunePlural(len(g.layers), "layer", "layers")))

			// Image ref row — Untagged or Failed.
			var imgIcon, imgStatus, imgPad string
			if g.refStatus == statusFailed {
				imgIcon, imgStatus, imgPad = warnIconANSI, failedStatus, failedPad
			} else {
				imgIcon, imgStatus, imgPad = doneIconANSI, untaggedStatus, untaggedPad
			}
			add(pruneGlobalIndent + imgIcon + " " + imgStatus + console.CodeReset + imgPad +
				imageLabel + refANSI + refPad + layerCount)

			// Layer rows — Deleted or Failed.
			for _, l := range g.layers {
				var lIcon, lStatus, lPad string
				if l.status == statusFailed {
					lIcon, lStatus, lPad = warnIconANSI, failedStatus, failedPad
				} else {
					lIcon, lStatus, lPad = doneIconANSI, deletedStatus, deletedPad
				}
				add(pruneGlobalIndent + lIcon + " " + lStatus + console.CodeReset + lPad +
					layerIndent + console.ToConsoleANSI("{{[::D]}}"+l.id+"{{[-]}}"))
			}
		}

		// Determine services: section error — warn if any group failed.
		var svcsErr error
		if r.ImagesError != nil {
			svcsErr = r.ImagesError
		}
		add(sectionHeader("services", svcsErr))
		for _, g := range groups {
			svcs := imageServices[g.ref]
			svcStatus := statusRemoved
			if g.anyFailed {
				svcStatus = statusFailed
			}
			if len(svcs) > 0 {
				for _, svc := range svcs {
					add(childRow(svc+":", "{{|App|}}", svcStatus))
				}
			} else {
				add(childRow("<Unknown>:", "{{[::D]}}", svcStatus))
			}
			renderImageGroup(g)
		}
	} else if r.ImagesError != nil {
		add(sectionHeader("services", r.ImagesError))
	}

	// ── networks ─────────────────────────────────────────────────────────────
	if len(r.NetworksDeleted) > 0 || r.NetworksError != nil {
		add(sectionHeader("networks", r.NetworksError))
		for _, net := range r.NetworksDeleted {
			add(childRow(net, "{{|App|}}", statusRemoved))
		}
	}

	// ── volumes ──────────────────────────────────────────────────────────────
	if len(r.VolumesDeleted) > 0 || r.VolumesError != nil {
		add(sectionHeader("volumes", r.VolumesError))
		for _, vol := range r.VolumesDeleted {
			add(childRow(vol, "{{|App|}}", statusRemoved))
		}
	}

	// ── containers ───────────────────────────────────────────────────────────
	if len(r.ContainersDeleted) > 0 || r.ContainersError != nil {
		add(sectionHeader("containers", r.ContainersError))
		for _, ctr := range r.ContainersDeleted {
			add(childRow(ctr, "{{|App|}}", statusRemoved))
		}
	}

	// ── summary ───────────────────────────────────────────────────────────────
	if r.SpaceReclaimed > 0 {
		add(console.ToConsoleANSI("{{[white::B]}}Total reclaimed space:{{[-]}} {{|DockerSuccess|}}" +
			units.HumanSize(float64(r.SpaceReclaimed)) + "{{[-]}}"))
	}

	return lines
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
