package tui

import (
	"DockSTARTer2/internal/strutil"
	"regexp"
	"strings"
	"unicode/utf8"

	"sort"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// Position constants for overlay placement
type OverlayPosition int

const (
	OverlayCenter OverlayPosition = iota
	OverlayTop
	OverlayBottom
	OverlayLeft
	OverlayRight
)

// ansiRegex matches all ANSI escape sequences
var ansiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*((?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?\u0007|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))")

// zoneMarkerRegex matches bubblezone markers: \x1b_bubblezone:...: and \x1b\\
// These are OSC sequences that lipgloss.Width() doesn't strip correctly
var zoneMarkerRegex = regexp.MustCompile("\x1b_[^\x1b]*|\x1b\\\\")

// WidthWithoutZones returns the visual width of a string, stripping zone markers first
// Use this when measuring content that may contain bubblezone markers
func WidthWithoutZones(s string) int {
	stripped := zoneMarkerRegex.ReplaceAllString(s, "")
	return lipgloss.Width(stripped)
}

// Overlay composites a foreground string over a background string at the specified position.
// This version is "safe" for ANSI markers like bubblezone because it uses manual string slicing
// and concatenation instead of the lipgloss v2 compositor which may strip them.
func Overlay(foreground, background string, hPos, vPos OverlayPosition, xOffset, yOffset int) string {
	if foreground == "" {
		return background
	}
	if background == "" {
		return foreground
	}

	// Get dimensions
	bgWidth := lipgloss.Width(background)
	bgHeight := lipgloss.Height(background)
	fgWidth := lipgloss.Width(foreground)
	fgHeight := lipgloss.Height(foreground)

	// Calculate position based on alignment
	var x, y int
	switch hPos {
	case OverlayLeft:
		x = 0
	case OverlayRight:
		x = bgWidth - fgWidth
	default: // Center
		x = (bgWidth - fgWidth) / 2
	}
	switch vPos {
	case OverlayTop:
		y = 0
	case OverlayBottom:
		y = bgHeight - fgHeight
	default: // Center
		y = (bgHeight - fgHeight) / 2
	}

	// Apply offsets
	x += xOffset
	y += yOffset

	// Ensure non-negative
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Manual overlay using line-by-line concatenation to preserve ANSI markers
	bgLines := strings.Split(background, "\n")
	fgLines := strings.Split(foreground, "\n")

	for i, fgLine := range fgLines {
		bgRow := y + i
		if bgRow >= len(bgLines) {
			break
		}

		bgLine := bgLines[bgRow]

		// 1. Left portion of background
		// We use manual TruncateRight to ensure 100% marker safety
		left := TruncateRight(bgLine, x)

		// 2. Padding if background is shorter than target X
		lWidth := lipgloss.Width(left)
		if lWidth < x {
			left += strutil.Repeat(" ", x-lWidth)
		}

		// 3. Middle portion (foreground)
		// We use fgLine, but we MUST ensure it doesn't exceed the background width
		// to prevent terminal wrapping and layout corruption.
		availableSpace := bgWidth - x
		middle := fgLine
		if lipgloss.Width(middle) > availableSpace {
			middle = TruncateRight(middle, availableSpace)
		}

		// 4. Right portion of background
		// We skip the width that the foreground occupies
		right := TruncateLeft(bgLine, x+lipgloss.Width(middle))

		bgLines[bgRow] = left + middle + right
	}

	return strings.Join(bgLines, "\n")
}

// truncateString is the shared implementation for TruncateLeft and TruncateRight.
// keepLeft=true: keep first 'width' cells (TruncateRight)
// keepLeft=false: keep everything after first 'width' cells (TruncateLeft)
func truncateString(s string, width int, keepLeft bool) string {
	if width <= 0 {
		if keepLeft {
			return ""
		}
		return s
	}

	var b strings.Builder
	cells := 0

	// Find all ANSI sequences and zone markers (both are invisible)
	ansiMatches := ansiRegex.FindAllStringIndex(s, -1)
	zoneMatches := zoneMarkerRegex.FindAllStringIndex(s, -1)

	// Merge and sort all invisible sequence positions
	allMatches := make([][]int, 0, len(ansiMatches)+len(zoneMatches))
	allMatches = append(allMatches, ansiMatches...)
	allMatches = append(allMatches, zoneMatches...)
	sort.Slice(allMatches, func(i, j int) bool {
		return allMatches[i][0] < allMatches[j][0]
	})

	lastIdx := 0
	for lastIdx < len(s) {
		// Is there an invisible sequence at the current position?
		foundSequence := false
		for _, m := range allMatches {
			if m[0] == lastIdx {
				// Keep ALL invisible sequences to maintain state (colors, markers)
				b.WriteString(s[m[0]:m[1]])
				lastIdx = m[1]
				foundSequence = true
				break
			}
		}

		if foundSequence {
			continue
		}

		if cells >= width {
			if keepLeft {
				// TruncateRight: we've reached desired width, stop
				break
			}
			// TruncateLeft: we've reached cut point, append the rest
			b.WriteString(s[lastIdx:])
			break
		}

		// Not an invisible sequence. Count this character.
		r, size := utf8.DecodeRuneInString(s[lastIdx:])
		if keepLeft {
			// TruncateRight: write characters before cut point
			b.WriteString(s[lastIdx : lastIdx+size])
		}
		// TruncateLeft: skip characters before cut point (already handled above)
		lastIdx += size
		cells += runewidth.RuneWidth(r)
	}

	return b.String()
}

// TruncateLeft returns the substring of s starting after 'width' terminal cells.
// Preserves ANSI sequences and bubblezone markers.
func TruncateLeft(s string, width int) string {
	return truncateString(s, width, false)
}

// TruncateRight returns the first 'width' terminal cells of s.
// Preserves ANSI sequences and bubblezone markers.
func TruncateRight(s string, width int) string {
	return truncateString(s, width, true)
}

// LayerSpec defines a layer for MultiOverlay
type LayerSpec struct {
	Content string
	X, Y, Z int
}

// MultiOverlay composites multiple layers.
// For safety with ANSI markers, it applies each layer sequentially using Overlay.
// Layers are sorted by Z-index (lowest first) before compositing.
func MultiOverlay(layers ...LayerSpec) string {
	if len(layers) == 0 {
		return ""
	}
	if len(layers) == 1 {
		return layers[0].Content
	}

	// Create a copy to avoid mutating the original slice
	sorted := make([]LayerSpec, len(layers))
	copy(sorted, layers)

	// Sort by Z-index (stable sort preserve original order for same Z)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Z < sorted[j].Z
	})

	// Start with the base layer (lowest Z)
	output := sorted[0].Content

	// Overlay subsequent layers
	for i := 1; i < len(sorted); i++ {
		l := sorted[i]
		output = Overlay(l.Content, output, OverlayLeft, OverlayTop, l.X, l.Y)
	}

	return output
}

// MultiOverlayWithBounds composites layers with explicit bounds checking.
// Layers that would extend beyond screenW/screenH are clipped.
// This prevents content bleeding outside the visible area.
func MultiOverlayWithBounds(screenW, screenH int, layers ...LayerSpec) string {
	if len(layers) == 0 {
		return ""
	}

	// Create a copy to avoid mutating the original slice
	sorted := make([]LayerSpec, len(layers))
	copy(sorted, layers)

	// Sort by Z-index (stable sort preserves original order for same Z)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Z < sorted[j].Z
	})

	// Start with the base layer (lowest Z)
	output := sorted[0].Content

	// Ensure base layer matches screen dimensions
	baseW := lipgloss.Width(output)
	baseH := lipgloss.Height(output)
	if baseW < screenW || baseH < screenH {
		// Pad the base layer to fill screen if needed
		output = padToSize(output, screenW, screenH)
	}

	// Overlay subsequent layers with bounds checking
	for i := 1; i < len(sorted); i++ {
		l := sorted[i]

		// Calculate available space for this layer
		availW := screenW - l.X
		availH := screenH - l.Y

		if availW <= 0 || availH <= 0 {
			// Layer is entirely off-screen, skip it
			continue
		}

		// Constrain layer content to available space
		constrained := constrainToSize(l.Content, availW, availH)

		output = Overlay(constrained, output, OverlayLeft, OverlayTop, l.X, l.Y)
	}

	return output
}

// padToSize pads content to fill the specified dimensions
func padToSize(content string, width, height int) string {
	lines := strings.Split(content, "\n")

	// Pad each line to width
	for i, line := range lines {
		lineW := lipgloss.Width(line)
		if lineW < width {
			lines[i] = line + strings.Repeat(" ", width-lineW)
		}
	}

	// Add empty lines to reach height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// constrainToSize clips content to fit within width/height
func constrainToSize(content string, maxW, maxH int) string {
	lines := strings.Split(content, "\n")

	// Limit number of lines
	if len(lines) > maxH {
		lines = lines[:maxH]
	}

	// Truncate each line to width
	for i, line := range lines {
		if lipgloss.Width(line) > maxW {
			lines[i] = TruncateRight(line, maxW)
		}
	}

	return strings.Join(lines, "\n")
}
