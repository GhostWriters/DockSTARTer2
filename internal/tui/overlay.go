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
// Uses lipgloss v2 Compositor with z-index for proper layering that preserves the background.
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

	// Create layers: background at z=0, foreground at z=1
	bgLayer := lipgloss.NewLayer(background).X(0).Y(0).Z(0)
	fgLayer := lipgloss.NewLayer(foreground).X(x).Y(y).Z(1)

	// Create compositor and render
	compositor := lipgloss.NewCompositor(bgLayer, fgLayer)
	return compositor.Render()
}
