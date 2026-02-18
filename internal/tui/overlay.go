package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

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
			left += strings.Repeat(" ", x-lWidth)
		}

		// 3. Middle portion (foreground)
		// We use fgLine as-is to preserve all markers
		middle := fgLine

		// 4. Right portion of background
		// We skip the width that the foreground occupies
		right := TruncateLeft(bgLine, x+fgWidth)

		bgLines[bgRow] = left + middle + right
	}

	return strings.Join(bgLines, "\n")
}

// TruncateLeft returns the substring of s starting after 'width' terminal cells.
// Optimized to preserve ANSI sequences.
func TruncateLeft(s string, width int) string {
	if width <= 0 {
		return s
	}

	var b strings.Builder
	cells := 0

	// Find all ANSI sequences
	matches := ansiRegex.FindAllStringIndex(s, -1)

	lastIdx := 0
	for lastIdx < len(s) {
		// Is there an ANSI sequence at the current position?
		foundANSI := false
		for _, m := range matches {
			if m[0] == lastIdx {
				// We keep ALL ANSI sequences even if they are before the cut point,
				// because we want to maintain the state (colors, markers).
				b.WriteString(s[m[0]:m[1]])
				lastIdx = m[1]
				foundANSI = true
				break
			}
		}

		if foundANSI {
			continue
		}

		if cells >= width {
			// We've reached the cut point. Append the rest of the string.
			b.WriteString(s[lastIdx:])
			break
		}

		// Not an ANSI sequence. Count this character.
		r, size := utf8.DecodeRuneInString(s[lastIdx:])
		lastIdx += size
		cells += runewidth.RuneWidth(r)
	}

	return b.String()
}

// TruncateRight returns the first 'width' terminal cells of s.
// Optimized to preserve ANSI sequences.
func TruncateRight(s string, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	cells := 0

	// Find all ANSI sequences
	matches := ansiRegex.FindAllStringIndex(s, -1)

	lastIdx := 0
	for lastIdx < len(s) {
		// Is there an ANSI sequence at the current position?
		foundANSI := false
		for _, m := range matches {
			if m[0] == lastIdx {
				// We keep ALL ANSI sequences found within or before the range.
				b.WriteString(s[m[0]:m[1]])
				lastIdx = m[1]
				foundANSI = true
				break
			}
		}

		if foundANSI {
			continue
		}

		if cells >= width {
			// We've reached the desired width.
			break
		}

		// Not an ANSI sequence. Count this character.
		r, size := utf8.DecodeRuneInString(s[lastIdx:])
		b.WriteString(s[lastIdx : lastIdx+size])
		lastIdx += size
		cells += runewidth.RuneWidth(r)
	}

	return b.String()
}

// LayerSpec defines a layer for MultiOverlay
type LayerSpec struct {
	Content string
	X, Y, Z int
}

// MultiOverlay composites multiple layers.
// For safety with ANSI markers, it applies each layer sequentially using Overlay.
func MultiOverlay(layers ...LayerSpec) string {
	if len(layers) == 0 {
		return ""
	}
	if len(layers) == 1 {
		return layers[0].Content
	}

	// Start with the base layer (usually Z=0)
	output := layers[0].Content

	// Overlay subsequent layers
	for i := 1; i < len(layers); i++ {
		l := layers[i]
		// Map LayerSpec to Overlay parameters
		// Since results of Overlay are always relative to background size:
		output = Overlay(l.Content, output, OverlayLeft, OverlayTop, l.X, l.Y)
	}

	return output
}
