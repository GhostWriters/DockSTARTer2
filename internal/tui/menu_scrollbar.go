package tui

import "charm.land/lipgloss/v2"

// scrollbarGutterWidth is the number of columns reserved for the right scrollbar gutter.
// Space is always reserved when scrollbars are enabled, so the layout never jumps.
const scrollbarGutterWidth = 1

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
		Foreground(ctx.TagKey.GetForeground())

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
