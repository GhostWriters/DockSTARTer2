package classic

import (
	"fmt"

	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// aeLabelsWidth is the width of both column labels and the dash between them.
const aeLabelsWidth = (len(ColAddLabel) + 2) + 1 + (len(ColEnableLabel) + 2)

// aeColumnLabel renders a column header label, with focus indicators when its
// column has focus.
func aeColumnLabel(label string, targetCol, activeCol CheckboxColumn, focused bool, ctx StyleContext, borderStyle lipgloss.Style) string {
	var content string
	if focused && activeCol == targetCol {
		l, r := ">", "<"
		if ctx.LineCharacters {
			l, r = "▸", "◂"
		}
		content = "{{|TitleFocusIndicator|}}" + l + "{{[-]}}{{|TitleCheckboxFocused|}}" + label + "{{[-]}}{{|TitleFocusIndicator|}}" + r + "{{[-]}}"
	} else {
		content = " {{|TitleCheckbox|}}" + label + "{{[-]}} "
	}
	return RenderThemeText(content, borderStyle)
}

// BuildAETopBorder constructs the top border for the app-selection inner list box.
// Each label is centered over its checkbox. prefixDashes is the number of
// dashes between the left corner and the first label.
//
// Row layout:    │ g0 g1 [cb_add 3ch] [aeColumnGap] [cb_enabled 3ch] [aeEnableTrail] [expand 3ch] [1sp] AppName
// Border layout (Add focused): ╭─▸Add◂─ Enable ────────────────────────╮
func BuildAETopBorder(totalWidth int, prefixDashes int, focused bool, activeCol CheckboxColumn, ctx StyleContext) string {
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
	borderStyle := ctx.BorderFlags.Apply(lipgloss.NewStyle()).Foreground(ctx.BorderColor).Background(ctx.Dialog.GetBackground())

	aLabel := aeColumnLabel(ColAddLabel, ColAdd, activeCol, focused, ctx, borderStyle)
	eLabel := aeColumnLabel(ColEnableLabel, ColEnable, activeCol, focused, ctx, borderStyle)

	// Total fixed = corner(1) + dashes + labels + corner(1)
	innerWidth := totalWidth - (2 + prefixDashes + aeLabelsWidth)
	if innerWidth < 0 {
		innerWidth = 0
	}
	// We use light color for everything except the top-right corner to maintain 3D look.
	return lipgloss.JoinHorizontal(lipgloss.Top,
		borderStyle.Render(border.TopLeft+strutil.Repeat(border.Top, prefixDashes)),
		aLabel, borderStyle.Render(border.Top), eLabel,
		borderStyle.Render(strutil.Repeat(border.Top, innerWidth)+border.TopRight))
}

// BuildAEBottomBorder constructs the AE-compatible bottom border.
// When scrollPct >= 0, a scroll-percent label is rendered on the right side.
// Pass scrollPct < 0 (e.g. -1) to omit the right label.
func BuildAEBottomBorder(totalWidth int, prefixDashes int, focused bool, activeCol CheckboxColumn, scrollPct float64, ctx StyleContext) string {
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
	// Bottom border consistently uses the "dark" color for 3D depth effect.
	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).Foreground(ctx.Border2Color).Background(ctx.Dialog.GetBackground())

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
			leftT = "|"
			rightT = "|"
		}
	}

	aLabel := aeColumnLabel(ColAddLabel, ColAdd, activeCol, focused, ctx, borderStyle)
	eLabel := aeColumnLabel(ColEnableLabel, ColEnable, activeCol, focused, ctx, borderStyle)

	// prefixDashes is the number of dashes between the left corner and the first label.
	prefix := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, prefixDashes))
	middle := borderStyle.Render(border.Bottom)

	// Total fixed (no right label) = corner(1) + dashes + labels + corner(1)
	if scrollPct < 0 {
		tailWidth := totalWidth - (2 + prefixDashes + aeLabelsWidth)
		if tailWidth < 0 {
			tailWidth = 0
		}
		tail := borderStyle.Render(strutil.Repeat(border.Bottom, tailWidth) + border.BottomRight)
		return lipgloss.JoinHorizontal(lipgloss.Bottom, prefix, aLabel, middle, eLabel, tail)
	}

	// With scroll percent: right side gets leftT + pct + rightT + 1×dash + BottomRight
	pctLabel := ctx.PositionIndicator.Render(fmt.Sprintf("%3d%%", int(scrollPct*100)))
	pctW := lipgloss.Width(pctLabel)
	// right segment width: connector(1) + pct + connector(1) + dash(1) + corner(1)
	rightSegW := 1 + pctW + 1 + 1 + 1
	// left fixed: corner(1) + dashes + labels
	leftFixedW := 1 + prefixDashes + aeLabelsWidth
	tailW := totalWidth - leftFixedW - rightSegW
	if tailW < 0 {
		tailW = 0
	}
	tail := borderStyle.Render(strutil.Repeat(border.Bottom, tailW))
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, 1) + border.BottomRight)
	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		prefix, aLabel, middle, eLabel,
		tail,
		borderStyle.Render(leftT), pctLabel, borderStyle.Render(rightT),
		rightPart,
	)
}
