package tui

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// ScrollbarGutterWidth is the number of columns reserved for the right scrollbar/padding column.
// This slot is always reserved (space when scrollbar is off, track/thumb when on).
const ScrollbarGutterWidth = 1

// IsScrollbarEnabled reports whether the scrollbar is enabled in the current config.
func IsScrollbarEnabled() bool { return currentConfig.UI.Scrollbar }

// ScrollbarInfo describes the geometry of a rendered scrollbar column.
// It is returned by applyScrollbarColumnTracked so callers can compute hit regions.
type ScrollbarInfo struct {
	Needed     bool // true when total > visible and height >= 3
	Height     int  // total column height (== number of lines in content)
	ThumbStart int  // row index of thumb top (>= 1 because row 0 is the up arrow)
	ThumbEnd   int  // exclusive row index of thumb bottom (<= Height-1 because last row is down arrow)
}

// ComputeScrollbarInfo computes scrollbar geometry without rendering anything.
func ComputeScrollbarInfo(total, visible, offset, height int) ScrollbarInfo {
	if total <= visible || height < 3 {
		return ScrollbarInfo{Height: height}
	}
	trackH := height - 2 // rows 1..height-2 are the track; row 0 and height-1 are arrows
	thumbH := max(1, trackH*visible/total)
	// Map offset linearly over [0, total-visible] → thumbTrackStart over [0, trackH-thumbH]
	// so the thumb reaches the very bottom when scrolled to the end.
	thumbTrackStart := 0
	maxOff := total - visible
	if maxOff > 0 {
		thumbTrackStart = (trackH - thumbH) * offset / maxOff
		if thumbTrackStart > trackH-thumbH {
			thumbTrackStart = trackH - thumbH
		}
	}
	thumbStart := 1 + thumbTrackStart
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

// ApplyScrollbarColumnTracked appends one scrollbar/gutter character to the right of every line
// in content and returns the rendered string together with the scrollbar geometry.
func ApplyScrollbarColumnTracked(content string, total, visible, offset int, enabled bool, lineChars bool, ctx StyleContext) (string, ScrollbarInfo) {
	if content == "" {
		return content, ScrollbarInfo{}
	}
	lines := strings.Split(content, "\n")
	height := len(lines)

	var col []string
	var info ScrollbarInfo

	if enabled {
		info = ComputeScrollbarInfo(total, visible, offset, height)
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

// ApplyScrollbarColumn is the non-tracking variant kept for callers that don't need geometry.
func ApplyScrollbarColumn(content string, total, visible, offset int, enabled bool, lineChars bool, ctx StyleContext) string {
	result, _ := ApplyScrollbarColumnTracked(content, total, visible, offset, enabled, lineChars, ctx)
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

// BuildPlainBottomBorder constructs a plain bottom border line (no label) matching the inner box style.
func BuildPlainBottomBorder(totalWidth int, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}
	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	inner := strutil.Repeat(border.Bottom, max(0, totalWidth-2))
	return borderStyle.Render(border.BottomLeft + inner + border.BottomRight)
}

// BuildLabeledBottomBorderCtx constructs a bottom border line with a short label
// on the LEFT side (e.g. "INS" or "OVR"), styled to match the box border.
// totalWidth is the full visual width of the bordered box including side border chars.
// The function selects border characters based on ctx.Type and ctx.LineCharacters.
func BuildLabeledBottomBorderCtx(totalWidth int, label string, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	var leftT, rightT string

	if ctx.Type == DialogTypeConfirm {
		// Slanted border variant used by the prompt dialog input box
		if ctx.LineCharacters {
			if focused {
				border = SlantedThickBorder
			} else {
				border = SlantedBorder
			}
		} else {
			if focused {
				border = SlantedThickAsciiBorder
			} else {
				border = SlantedAsciiBorder
			}
		}
		if ctx.LineCharacters {
			leftT = "┤"
			rightT = "├"
		} else {
			leftT = "+"
			rightT = "+"
		}
	} else {
		// Rounded border variant used by set-value / add-var input sections
		if ctx.LineCharacters {
			if focused {
				border = ThickRoundedBorder
			} else {
				border = lipgloss.RoundedBorder()
			}
		} else {
			if focused {
				border = RoundedThickAsciiBorder
			} else {
				border = RoundedAsciiBorder
			}
		}
		if ctx.LineCharacters {
			if focused {
				leftT = "┫"
				rightT = "┣"
			} else {
				leftT = "┤"
				rightT = "├"
			}
		} else {
			if focused {
				leftT = "H"
				rightT = "H"
			} else {
				leftT = "+"
				rightT = "+"
			}
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	labelStyle := ctx.TagKey.Bold(true)

	labelWidth := lipgloss.Width(label)
	leftPadCnt := 1 // one border char before label connector
	totalLabelWidth := 1 + labelWidth + 1
	rightPadCnt := totalWidth - 2 - totalLabelWidth - leftPadCnt
	if rightPadCnt < 0 {
		rightPadCnt = 0
	}

	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, max(0, leftPadCnt)))
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, max(0, rightPadCnt)) + border.BottomRight)
	return lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, labelStyle.Render(label), rightConnector, rightPart)
}

// BuildDualLabelBottomBorderCtx constructs a bottom border line with a label on the LEFT
// (e.g. "INS"/"OVR") and an optional label on the RIGHT (e.g. "42%").
// Pass an empty string for rightLabel when no right label is needed.
// Uses rounded borders (for editor inner boxes, not confirm-style slanted borders).
func BuildDualLabelBottomBorderCtx(totalWidth int, leftLabel, rightLabel string, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}

	var leftT, rightT string
	if ctx.LineCharacters {
		if focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	labelStyle := ctx.TagKey.Bold(true)

	// Left segment: BottomLeft + 1×bottom + leftT + leftLabel + rightT
	leftLabelW := lipgloss.Width(leftLabel)
	leftSegW := 1 + 1 + 1 + leftLabelW + 1 // BottomLeft(1) + bottom(1) + leftT(1) + label + rightT(1)

	// Right segment (optional): leftT + rightLabel + rightT + 1×bottom + BottomRight
	rightLabelW := 0
	rightSegW := 0
	if rightLabel != "" {
		rightLabelW = lipgloss.Width(rightLabel)
		rightSegW = 1 + rightLabelW + 1 + 1 + 1 // leftT(1) + label + rightT(1) + bottom(1) + BottomRight(1)
	} else {
		rightSegW = 1 // just BottomRight
	}

	// Middle dashes fill the remaining width
	middleW := totalWidth - leftSegW - rightSegW
	if middleW < 0 {
		middleW = 0
	}

	parts := []string{
		borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, 1)),
		borderStyle.Render(leftT),
		labelStyle.Render(leftLabel),
		borderStyle.Render(rightT),
		borderStyle.Render(strutil.Repeat(border.Bottom, max(0, middleW))),
	}
	if rightLabel != "" {
		parts = append(parts,
			borderStyle.Render(leftT),
			labelStyle.Render(rightLabel),
			borderStyle.Render(rightT),
			borderStyle.Render(strutil.Repeat(border.Bottom, 1)+border.BottomRight),
		)
	} else {
		parts = append(parts, borderStyle.Render(border.BottomRight))
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

// BuildScrollPercentBottomBorder constructs a bottom border line for an inner box
// with a scroll-percent label on the right, styled identically to the programbox indicator.
// totalWidth is the full visual width of the bordered box including side border chars.
// Only call this when a scrollbar is needed (sbInfo.Needed == true).
func BuildScrollPercentBottomBorder(totalWidth int, scrollPct float64, focused bool, ctx StyleContext) string {
	scrollIndicator := ctx.TagKey.Bold(true).Render(fmt.Sprintf("%d%%", int(scrollPct*100)))

	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}

	var leftT, rightT string
	if ctx.LineCharacters {
		if focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())

	labelWidth := lipgloss.Width(scrollIndicator)
	rightPadCnt := 2
	totalLabelWidth := 1 + labelWidth + 1 // connector + label + connector
	if totalWidth < totalLabelWidth+rightPadCnt+2 {
		rightPadCnt = (totalWidth - totalLabelWidth) / 2
	}
	leftPadCnt := totalWidth - labelWidth - 4 - rightPadCnt
	if leftPadCnt < 0 {
		leftPadCnt = 0
		rightPadCnt = totalWidth - labelWidth - 4
		if rightPadCnt < 0 {
			rightPadCnt = 0
		}
	}

	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, max(0, leftPadCnt)))
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, max(0, rightPadCnt)) + border.BottomRight)
	return lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, scrollIndicator, rightConnector, rightPart)
}
