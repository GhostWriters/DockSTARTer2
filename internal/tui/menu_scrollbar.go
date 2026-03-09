package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// scrollbarGutterWidth is the number of columns reserved for the right scrollbar/padding column.
// This slot is always reserved (space when scrollbar is off, track/thumb when on).
const scrollbarGutterWidth = 1

// ScrollbarInfo describes the geometry of a rendered scrollbar column.
// It is returned by applyScrollbarColumnTracked so callers can compute hit regions.
type ScrollbarInfo struct {
	Needed     bool // true when total > visible and height >= 3
	Height     int  // total column height (== number of lines in content)
	ThumbStart int  // row index of thumb top (>= 1 because row 0 is the up arrow)
	ThumbEnd   int  // exclusive row index of thumb bottom (<= Height-1 because last row is down arrow)
}

// computeScrollbarInfo computes scrollbar geometry without rendering anything.
func computeScrollbarInfo(total, visible, offset, height int) ScrollbarInfo {
	if total <= visible || height < 3 {
		return ScrollbarInfo{Height: height}
	}
	trackH := height - 2 // rows 1..height-2 are the track; row 0 and height-1 are arrows
	thumbH := max(1, trackH*visible/total)
	// thumb start in track-relative coords, then shift by 1 for the top arrow
	thumbTrackStart := 0
	if total > visible {
		thumbTrackStart = trackH * offset / total
	}
	thumbStart := 1 + thumbTrackStart
	if thumbStart+thumbH > height-1 {
		thumbStart = height - 1 - thumbH
	}
	if thumbStart < 1 {
		thumbStart = 1
	}
	return ScrollbarInfo{
		Needed:     true,
		Height:     height,
		ThumbStart: thumbStart,
		ThumbEnd:   thumbStart + thumbH,
	}
}

// applyScrollbarColumnTracked appends one scrollbar/gutter character to the right of every line
// in content and returns the rendered string together with the scrollbar geometry.
func applyScrollbarColumnTracked(content string, total, visible, offset int, enabled bool, lineChars bool, ctx StyleContext) (string, ScrollbarInfo) {
	if content == "" {
		return content, ScrollbarInfo{}
	}
	lines := strings.Split(content, "\n")
	height := len(lines)

	var col []string
	var info ScrollbarInfo

	if enabled {
		info = computeScrollbarInfo(total, visible, offset, height)
		col = buildScrollbarColumn(info, lineChars, ctx)
	} else {
		blank := lipgloss.NewStyle().Background(ctx.Dialog.GetBackground()).Render(" ")
		col = make([]string, height)
		for i := range col {
			col[i] = blank
		}
	}

	for i, line := range lines {
		if i < len(col) {
			lines[i] = line + col[i]
		}
	}
	return strings.Join(lines, "\n"), info
}

// applyScrollbarColumn is the non-tracking variant kept for callers that don't need geometry.
func applyScrollbarColumn(content string, total, visible, offset int, enabled bool, lineChars bool, ctx StyleContext) string {
	result, _ := applyScrollbarColumnTracked(content, total, visible, offset, enabled, lineChars, ctx)
	return result
}

// buildScrollbarColumn returns a slice of height styled single-character strings
// representing a vertical scrollbar column, given pre-computed geometry.
//
// When info.Needed is false the column is filled with blank styled spaces.
func buildScrollbarColumn(info ScrollbarInfo, lineChars bool, ctx StyleContext) []string {
	height := info.Height
	col := make([]string, height)

	bg := ctx.Dialog.GetBackground()
	trackStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.Border2Color)

	thumbStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.BorderColor)

	// No scrollbar needed — fill with spaces to hold the gutter width.
	if !info.Needed || height < 1 {
		blank := trackStyle.Render(" ")
		for i := range col {
			col[i] = blank
		}
		return col
	}

	// Choose characters based on line-art mode.
	var trackChar, thumbChar string
	var upArrow, downArrow string
	if lineChars {
		trackChar = "░"
		thumbChar = "█"
		upArrow = "▴"
		downArrow = "▾"
	} else {
		trackChar = ";"
		thumbChar = "#"
		upArrow = "^"
		downArrow = "v"
	}

	for i := range col {
		switch {
		case i == 0:
			col[i] = thumbStyle.Render(upArrow)
		case i == height-1:
			col[i] = thumbStyle.Render(downArrow)
		case i >= info.ThumbStart && i < info.ThumbEnd:
			col[i] = thumbStyle.Render(thumbChar)
		default:
			col[i] = trackStyle.Render(trackChar)
		}
	}
	return col
}
