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
// Uses lipgloss.Place with the screen background color for proper ANSI handling.
func Overlay(foreground, background string, hPos, vPos OverlayPosition, xOffset, yOffset int) string {
	if foreground == "" {
		return background
	}
	if background == "" {
		return foreground
	}

	// Get background dimensions
	bgLines := strings.Split(background, "\n")
	bgHeight := len(bgLines)
	bgWidth := 0
	for _, line := range bgLines {
		if w := lipgloss.Width(line); w > bgWidth {
			bgWidth = w
		}
	}

	// Convert position to lipgloss alignment
	var hAlign, vAlign lipgloss.Position
	switch hPos {
	case OverlayLeft:
		hAlign = lipgloss.Left
	case OverlayRight:
		hAlign = lipgloss.Right
	default:
		hAlign = lipgloss.Center
	}
	switch vPos {
	case OverlayTop:
		vAlign = lipgloss.Top
	case OverlayBottom:
		vAlign = lipgloss.Bottom
	default:
		vAlign = lipgloss.Center
	}

	// Use lipgloss.Place with the screen background for whitespace
	// This properly handles ANSI codes and fills empty space with the screen bg
	styles := GetStyles()
	return lipgloss.Place(
		bgWidth,
		bgHeight,
		hAlign,
		vAlign,
		foreground,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(styles.Screen.GetBackground())),
	)
}
