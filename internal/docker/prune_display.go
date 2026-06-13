package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-units"
)

// pruneLayout mirrors the column widths used by display_console.go so the
// prune output looks consistent with compose output.
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
	ImagesDeleted     []image.DeleteResponse
	NetworksDeleted   []string
	VolumesDeleted    []string
	ContainersDeleted []string
	SpaceReclaimed    uint64
	AsciiMode         bool
}

// LogPruneReport formats and outputs a structured prune report using the same
// visual style as the compose live display. imageServices maps image ref →
// []service names from the compose project (best-effort; non-compose images
// are shown without a service label).
//
// Mirrors logSummary in display_console.go:
//   - Lines written directly to stdout (or TUI writer) for immediate display
//   - Then logged with "docker: " prefix, suppressing the console writer to
//     avoid double-printing.
func LogPruneReport(ctx context.Context, r PruneReport, imageServices map[string][]string) {
	lines := buildPruneLines(r, imageServices)
	if len(lines) == 0 {
		return
	}

	// Write directly to terminal (or TUI writer when inside a program box).
	var out io.Writer = os.Stdout
	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		out = w
	}
	// Pause the CLI spinner, write the report, then resume.
	console.PauseSpinner()
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	console.ResumeSpinner()

	// Log with suppressed console writer — already shown directly above.
	logCtx := logger.WithSuppressWriter(ctx, logger.ConsoleWriter())
	const pfx = "{{|RunningCommand|}}docker:{{[-]}} "
	for _, line := range lines {
		logger.Notice(logCtx, pfx+"%s", console.Strip(line))
	}
}

// buildPruneLines constructs the ANSI display lines for a prune report.
//
// Layout per group:
//
//	 services:
//	   autobrr:
//	   deluge:
//	 images:
//	✓ Untagged     image: ghcr.io/autobrr/autobrr:latest   (2 layers)
//	✓ Deleted            944b1c438302
//	✓ Deleted            c85d153d0a29
func buildPruneLines(r PruneReport, imageServices map[string][]string) []string {
	doneIcon := "{{|DockerSuccess|}}✓{{[-]}}"
	if r.AsciiMode {
		doneIcon = "{{|DockerSuccess|}}+{{[-]}}"
	}
	doneIconANSI := console.ToConsoleANSI(doneIcon)

	untaggedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Untagged{{[-]}}")
	deletedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Deleted{{[-]}}")
	untaggedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Untagged"))
	deletedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Deleted"))

	var lines []string
	add := func(line string) { lines = append(lines, line) }

	// ── header ───────────────────────────────────────────────────────────────
	// Count totals up-front so the header can be the first line.
	var totalImages, totalLayers int
	seenSvcsForHeader := make(map[string]bool)
	for _, item := range r.ImagesDeleted {
		if item.Untagged != "" {
			totalImages++
			for _, svc := range imageServices[item.Untagged] {
				seenSvcsForHeader[svc] = true
			}
		}
		if item.Deleted != "" {
			totalLayers++
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

	removedStatus := console.ToConsoleANSI("{{|DockerFinal|}}Removed{{[-]}}")
	removedPad := strutil.Repeat(" ", pruneSectionStatusW-len("Removed"))

	// sectionHeader renders a section label with icon+status prefix, matching
	// the compose "✓ Complete  services:" style.
	sectionHeader := func(label string) string {
		return pruneGlobalIndent + doneIconANSI + " " + removedStatus + console.CodeReset + removedPad +
			console.ToConsoleANSI("{{[white::B]}}"+label+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
	}
	// childRow renders an indented name under a section header.
	childRow := func(name, colorTag string) string {
		return pruneGlobalIndent + doneIconANSI + " " + removedStatus + console.CodeReset + removedPad +
			pruneSectionChildIndent + console.ToConsoleANSI(colorTag+name+"{{[-]}}")
	}

	// ── images: section ──────────────────────────────────────────────────────
	// Group deleted items by Untagged ref — each untagged line starts a new group.
	type imageGroup struct {
		ref    string
		layers []string
	}
	var groups []imageGroup
	var current *imageGroup
	for _, item := range r.ImagesDeleted {
		if item.Untagged != "" {
			groups = append(groups, imageGroup{ref: item.Untagged})
			current = &groups[len(groups)-1]
		}
		if item.Deleted != "" {
			if current == nil {
				// Dangling layer with no preceding Untagged.
				groups = append(groups, imageGroup{ref: ""})
				current = &groups[len(groups)-1]
			}
			current.layers = append(current.layers, item.Deleted)
		}
	}

	if len(groups) > 0 {
		// Sort: compose-known images first (by service name), then unknown by ref.
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

		// Find widest image ref for column alignment.
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

		// services: section — all unique services in image-sort order, before images:.
		seenSvcs := make(map[string]bool)
		var orderedSvcs []string
		for _, g := range groups {
			for _, svc := range imageServices[g.ref] {
				if !seenSvcs[svc] {
					seenSvcs[svc] = true
					orderedSvcs = append(orderedSvcs, svc)
				}
			}
		}
		if len(orderedSvcs) > 0 {
			add(sectionHeader("services"))
			for _, svc := range orderedSvcs {
				add(childRow(svc+":", "{{|App|}}"))
			}
		}

		imageLabel := strings.Repeat(" ", 2*pruneSectionChildIndentW) +
			console.ToConsoleANSI("{{|DockerSuccess|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")
		// Layer indent: same prefix as the image ref column, plus 2 extra chars (matching compose).
		// imageLabel plain width = 2*childIndent + len("image: ") = 4 + 7 = 11
		imageLabelPlainW := 2*pruneSectionChildIndentW + len("image: ")
		layerIndent := strings.Repeat(" ", imageLabelPlainW+pruneSectionChildIndentW)

		add(sectionHeader("images"))

		for _, g := range groups {
			ref := g.ref
			if ref == "" {
				ref = "<dangling>"
			}
			refANSI := styleImageRef(ref)
			refPad := strutil.Repeat(" ", maxRefW-utf8.RuneCountInString(console.Strip(refANSI)))
			layerCount := console.ToConsoleANSI(fmt.Sprintf(" {{[::D]}}(%d %s){{[-]}}", len(g.layers), prunePlural(len(g.layers), "layer", "layers")))

			// Untagged row — image ref.
			add(pruneGlobalIndent + doneIconANSI + " " + untaggedStatus + console.CodeReset + untaggedPad +
				imageLabel + refANSI + refPad + layerCount)

			// Deleted rows — one per layer, abbreviated to first 12 hex chars.
			for _, layer := range g.layers {
				short := strings.TrimPrefix(layer, "sha256:")
				if len(short) > 12 {
					short = short[:12]
				}
				add(pruneGlobalIndent + doneIconANSI + " " + deletedStatus + console.CodeReset + deletedPad +
					layerIndent + console.ToConsoleANSI("{{[::D]}}"+short+"{{[-]}}"))
			}
		}
	}

	// ── networks: section ────────────────────────────────────────────────────
	if len(r.NetworksDeleted) > 0 {
		add(sectionHeader("networks"))
		for _, net := range r.NetworksDeleted {
			add(childRow(net, "{{|App|}}"))
		}
	}

	// ── volumes: section ─────────────────────────────────────────────────────
	if len(r.VolumesDeleted) > 0 {
		add(sectionHeader("volumes"))
		for _, vol := range r.VolumesDeleted {
			add(childRow(vol, "{{|App|}}"))
		}
	}

	// ── containers: section ──────────────────────────────────────────────────
	if len(r.ContainersDeleted) > 0 {
		add(sectionHeader("containers"))
		for _, ctr := range r.ContainersDeleted {
			add(childRow(ctr, "{{|App|}}"))
		}
	}

	// ── summary ──────────────────────────────────────────────────────────────
	if r.SpaceReclaimed > 0 {
		add(console.ToConsoleANSI("{{[white::B]}}Total reclaimed space:{{[-]}} {{|DockerSuccess|}}" +
			units.HumanSize(float64(r.SpaceReclaimed)) + "{{[-]}}"))
	}

	return lines
}

// styleImageRef styles an image reference consistently with display_console.go.
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
