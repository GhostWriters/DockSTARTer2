package tui

import (
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
// Uses lipgloss.Place for centering with screen background for whitespace.
func Overlay(foreground, background string, hPos, vPos OverlayPosition, xOffset, yOffset int) string {
	if foreground == "" {
		return background
	}
	if background == "" {
		return foreground
	}

	// Get background dimensions
	bgWidth := lipgloss.Width(background)
	bgHeight := lipgloss.Height(background)

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
