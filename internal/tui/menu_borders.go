package tui

import (
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
	borderStyle := lipgloss.NewStyle().Foreground(ctx.BorderColor).Background(ctx.Dialog.GetBackground())

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

// BuildAEBottomBorder constructs the AE-compatible bottom border
func BuildAEBottomBorder(totalWidth int, prefixDashes int, focused bool, activeCol CheckboxColumn, ctx StyleContext) string {
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
	borderStyle := lipgloss.NewStyle().Foreground(ctx.Border2Color).Background(ctx.Dialog.GetBackground())

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

	aLabel := buildLabel("A", ColAdd)
	eLabel := buildLabel("E", ColEnable)

	// prefixDashes is the number of dashes between the left corner and the first label.
	// Prefix = BottomLeft + prefixDashes
	prefix := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, prefixDashes))
	// middle is the spacing between labels. Top border uses 1 dash between [A] and [E].
	middle := borderStyle.Render(border.Bottom)

	// Tail calculation
	// Total fixed = corner(1) + dashes + labelA(3) + dash(1) + labelE(3) + corner(1) = 9 + dashes
	tailWidth := totalWidth - (9 + prefixDashes)
	if tailWidth < 0 {
		tailWidth = 0
	}
	tail := borderStyle.Render(strutil.Repeat(border.Bottom, tailWidth) + border.BottomRight)

	return lipgloss.JoinHorizontal(lipgloss.Bottom, prefix, aLabel, middle, eLabel, tail)
}
