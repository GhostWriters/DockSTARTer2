package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// scrollbarGutterWidth is the number of columns reserved for the right scrollbar/padding column.
// This slot is always reserved (space when scrollbar is off, track/thumb when on).
const scrollbarGutterWidth = 1

// applyScrollbarColumn appends one scrollbar/gutter character to the right of every line
// in content (a newline-joined string). It is the single point of scrollbar application
// for both the standard list path and the variable-height list path in ViewString.
//
// When scrollbar is disabled the gutter is filled with neutral spaces so layout
// stays identical whether scrollbar is on or off.
func applyScrollbarColumn(content string, total, visible, offset int, enabled bool, lineChars bool, ctx StyleContext) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	var col []string
	if enabled {
		col = buildScrollbarColumn(total, visible, offset, len(lines), lineChars, ctx)
	} else {
		blank := lipgloss.NewStyle().Background(ctx.Dialog.GetBackground()).Render(" ")
		col = make([]string, len(lines))
		for i := range col {
			col[i] = blank
		}
	}
	for i, line := range lines {
		if i < len(col) {
			lines[i] = line + col[i]
		}
	}
	return strings.Join(lines, "\n")
}

// buildScrollbarColumn returns a slice of height styled single-character strings
// representing a vertical scrollbar column.
//
// When total <= visible (no scrolling needed) the column is filled with styled
// blank characters so the gutter stays the same width.
func buildScrollbarColumn(total, visible, offset, height int, lineChars bool, ctx StyleContext) []string {
	col := make([]string, height)

	bg := ctx.Dialog.GetBackground()
	trackStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.Border2Color)

	thumbStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.BorderColor)

	// No scrollbar needed — fill with spaces to hold the gutter width.
	if total <= visible || height < 1 {
		blank := trackStyle.Render(" ")
		for i := range col {
			col[i] = blank
		}
		return col
	}

	// Choose characters based on line-art mode.
	var trackChar, thumbChar string
	if lineChars {
		trackChar = "░"
		thumbChar = "█"
	} else {
		trackChar = ";"
		thumbChar = "#"
	}

	// Compute thumb size and start position.
	thumbH := max(1, height*visible/total)
	thumbStart := 0
	if total > visible {
		thumbStart = height * offset / total
	}
	if thumbStart+thumbH > height {
		thumbStart = height - thumbH
	}
	if thumbStart < 0 {
		thumbStart = 0
	}

	for i := range col {
		if i >= thumbStart && i < thumbStart+thumbH {
			col[i] = thumbStyle.Render(thumbChar)
		} else {
			col[i] = trackStyle.Render(trackChar)
		}
	}
	return col
}
