package tui

import (
	"fmt"

	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// BuildAETopBorder constructs the top border for the app-selection inner list box.
// Labels " A " and " E " use MarkerAdded style (green) and are left-positioned to align
// above the Add and Enabled checkboxes in the row.
//
// Row layout:    │ g0 g1 [cb_add 3ch] [1sp] [cb_enabled 3ch] [1sp] AppName
// Border layout: ╭ ─── ├ A ┤├ E ┤ ─────────────────────────────────── ╮
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

	// Build 3-character labeled blocks (e.g. "▸A◂" or " A ")
	buildLabel := func(char string, targetCol CheckboxColumn) string {
		var content string
		if focused && activeCol == targetCol {
			if ctx.LineCharacters {
				content = "{{|TitleFocusIndicator|}}▸{{[-]}}{{|TitleCheckboxFocused|}}" + char + "{{[-]}}{{|TitleFocusIndicator|}}◂{{[-]}}"
			} else {
				content = "{{|TitleFocusIndicator|}}>{{[-]}}{{|TitleCheckboxFocused|}}" + char + "{{[-]}}{{|TitleFocusIndicator|}}<{{[-]}}"
			}
		} else {
			// Marker style (e.g. green) but without focus arrows/background
			content = " {{|TitleCheckbox|}}" + char + "{{[-]}} "
		}
		return RenderThemeText(content, borderStyle)
	}

	aLabel := buildLabel("A", ColAdd)
	eLabel := buildLabel("E", ColEnable)

	// Total fixed = corner(1) + dashes + labelA(3) + dash(1) + labelE(3) + corner(1) = 9 + dashes
	innerWidth := totalWidth - (9 + prefixDashes)
	if innerWidth < 0 {
		innerWidth = 0
	}
	// Layout follows '── A ─ E ──' with ONE dash divider.
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
	labelStyle := ctx.TagKey.Bold(true)

	// Build 3-character labeled blocks (e.g. "▸A◂" or " A ") — same logic as top border
	buildLabel := func(char string, targetCol CheckboxColumn) string {
		var content string
		if focused && activeCol == targetCol {
			if ctx.LineCharacters {
				content = "{{|TitleFocusIndicator|}}▸{{[-]}}{{|TitleCheckboxFocused|}}" + char + "{{[-]}}{{|TitleFocusIndicator|}}◂{{[-]}}"
			} else {
				content = "{{|TitleFocusIndicator|}}>{{[-]}}{{|TitleCheckboxFocused|}}" + char + "{{[-]}}{{|TitleFocusIndicator|}}<{{[-]}}"
			}
		} else {
			content = " {{|TitleCheckbox|}}" + char + "{{[-]}} "
		}
		return RenderThemeText(content, borderStyle)
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

	aLabel := buildLabel("A", ColAdd)
	eLabel := buildLabel("E", ColEnable)

	// prefixDashes is the number of dashes between the left corner and the first label.
	prefix := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, prefixDashes))
	middle := borderStyle.Render(border.Bottom)

	// Total fixed (no right label) = corner(1) + dashes + labelA(3) + dash(1) + labelE(3) + corner(1) = 9 + dashes
	if scrollPct < 0 {
		tailWidth := totalWidth - (9 + prefixDashes)
		if tailWidth < 0 {
			tailWidth = 0
		}
		tail := borderStyle.Render(strutil.Repeat(border.Bottom, tailWidth) + border.BottomRight)
		return lipgloss.JoinHorizontal(lipgloss.Bottom, prefix, aLabel, middle, eLabel, tail)
	}

	// With scroll percent: right side gets leftT + pct + rightT + 1×dash + BottomRight
	pctLabel := labelStyle.Render(fmt.Sprintf("%d%%", int(scrollPct*100)))
	pctW := lipgloss.Width(pctLabel)
	// right segment width: connector(1) + pct + connector(1) + dash(1) + corner(1)
	rightSegW := 1 + pctW + 1 + 1 + 1
	// left fixed: corner(1) + dashes + labelA(3) + dash(1) + labelE(3) = 8 + dashes
	leftFixedW := 8 + prefixDashes
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
