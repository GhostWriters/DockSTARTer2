package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
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

// Overlay composites a foreground string over a background string at the specified position.
// This replaces the external bubbletea-overlay library with a lipgloss v2 compatible implementation.
func Overlay(foreground, background string, hPos, vPos OverlayPosition, xOffset, yOffset int) string {
	if foreground == "" {
		return background
	}
	if background == "" {
		return foreground
	}

	// Get dimensions
	bgLines := strings.Split(background, "\n")
	fgLines := strings.Split(foreground, "\n")

	bgHeight := len(bgLines)
	fgHeight := len(fgLines)

	bgWidth := 0
	for _, line := range bgLines {
		if w := lipgloss.Width(line); w > bgWidth {
			bgWidth = w
		}
	}

	fgWidth := 0
	for _, line := range fgLines {
		if w := lipgloss.Width(line); w > fgWidth {
			fgWidth = w
		}
	}

	// Calculate starting position
	var startX, startY int

	switch hPos {
	case OverlayCenter:
		startX = (bgWidth - fgWidth) / 2
	case OverlayLeft:
		startX = 0
	case OverlayRight:
		startX = bgWidth - fgWidth
	default:
		startX = (bgWidth - fgWidth) / 2
	}

	switch vPos {
	case OverlayCenter:
		startY = (bgHeight - fgHeight) / 2
	case OverlayTop:
		startY = 0
	case OverlayBottom:
		startY = bgHeight - fgHeight
	default:
		startY = (bgHeight - fgHeight) / 2
	}

	// Apply offsets
	startX += xOffset
	startY += yOffset

	// Ensure non-negative
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	// Build the result by overlaying foreground onto background
	result := make([]string, bgHeight)
	for i := 0; i < bgHeight; i++ {
		if i < len(bgLines) {
			result[i] = bgLines[i]
		} else {
			result[i] = ""
		}
	}

	// Overlay foreground lines
	for i, fgLine := range fgLines {
		bgLineIdx := startY + i
		if bgLineIdx < 0 || bgLineIdx >= bgHeight {
			continue
		}

		bgLine := result[bgLineIdx]
		bgLineWidth := lipgloss.Width(bgLine)

		// Ensure background line is wide enough
		if bgLineWidth < startX {
			bgLine += strings.Repeat(" ", startX-bgLineWidth)
			bgLineWidth = startX
		}

		// Extract runes for proper handling of wide characters
		bgRunes := []rune(bgLine)
		fgLineWidth := lipgloss.Width(fgLine)

		// Build the new line: prefix + foreground + suffix
		var newLine strings.Builder

		// Calculate prefix (characters before the foreground)
		prefixWidth := 0
		prefixEnd := 0
		for prefixEnd < len(bgRunes) && prefixWidth < startX {
			r := bgRunes[prefixEnd]
			prefixWidth++
			prefixEnd++
		}

		// Write prefix
		newLine.WriteString(string(bgRunes[:prefixEnd]))

		// Add padding if needed
		if prefixWidth < startX {
			newLine.WriteString(strings.Repeat(" ", startX-prefixWidth))
		}

		// Write foreground
		newLine.WriteString(fgLine)

		// Calculate where the suffix starts (after foreground)
		suffixStartX := startX + fgLineWidth
		if suffixStartX < bgLineWidth {
			// Find the position in bgRunes that corresponds to suffixStartX
			currentWidth := 0
			suffixStart := 0
			for suffixStart < len(bgRunes) && currentWidth < suffixStartX {
				currentWidth++
				suffixStart++
			}
			if suffixStart < len(bgRunes) {
				newLine.WriteString(string(bgRunes[suffixStart:]))
			}
		}

		result[bgLineIdx] = newLine.String()
	}

	return strings.Join(result, "\n")
}
