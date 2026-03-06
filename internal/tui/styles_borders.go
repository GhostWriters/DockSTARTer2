package tui

import "charm.land/lipgloss/v2"

// Apply3DBorder applies 3D border effect to a style
// Light color on top/left, dark color on bottom/right
func Apply3DBorder(style lipgloss.Style) lipgloss.Style {
	return Apply3DBorderCtx(style, GetActiveContext())
}

// Apply3DBorderCtx applies 3D border effect using a specific context
func Apply3DBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	borderStyle := lipgloss.NewStyle().
		Background(borderBG).
		Border(ctx.Border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG).
		Padding(0, 1)

	return style.Inherit(borderStyle)
}

// ApplyStraightBorder applies a 3D border with straight edges
// Uses asciiBorder or NormalBorder based on ctx.LineCharacters setting
func ApplyStraightBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyStraightBorderCtx(style, ctx)
}

// ApplyStraightBorderCtx applies a straight border using a specific context
func ApplyStraightBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.NormalBorder()
	} else {
		border = AsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyThickBorder applies a 3D border with thick edges
func ApplyThickBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyThickBorderCtx(style, ctx)
}

// ApplyThickBorderCtx applies a thick border using a specific context
func ApplyThickBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.ThickBorder()
	} else {
		border = thickAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyRoundedBorder applies a 3D border with rounded corners
func ApplyRoundedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyRoundedBorderCtx(style, ctx)
}

// ApplyRoundedBorderCtx applies a rounded border using a specific context
func ApplyRoundedBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.RoundedBorder()
	} else {
		border = RoundedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyInnerBorder applies a rounded border that becomes thick when focused
func ApplyInnerBorder(style lipgloss.Style, focused bool, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyInnerBorderCtx(style, focused, ctx)
}

// ApplyInnerBorderCtx applies a rounded border that becomes thick when focused
func ApplyInnerBorderCtx(style lipgloss.Style, focused bool, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.Type == DialogTypeConfirm {
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
	} else if ctx.LineCharacters {
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

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplySlantedBorder applies a 3D border with slanted/beveled corners
func ApplySlantedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplySlantedBorderCtx(style, ctx)
}

// ApplySlantedBorderCtx applies a slanted border using a specific context
func ApplySlantedBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = SlantedBorder
	} else {
		border = SlantedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// Render3DBorder manually renders content with a 3D border effect
func Render3DBorder(content string, padding int) string {
	return Render3DBorderCtx(content, padding, GetActiveContext())
}
