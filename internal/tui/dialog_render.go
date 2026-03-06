package tui

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// RenderDialog renders a dialog with optional title embedded in the top border.
func RenderDialog(title, content string, focused bool, targetHeight int, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, DialogTypeInfo, GetActiveContext(), borders...)
}

// RenderDialogCtx renders a dialog using a specific context
func RenderDialogCtx(title, content string, focused bool, targetHeight int, ctx StyleContext, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, DialogTypeInfo, ctx, borders...)
}

// RenderDialogWithType renders a dialog with a specific type for title styling.
func RenderDialogWithType(title, content string, focused bool, targetHeight int, dialogType DialogType, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, dialogType, GetActiveContext(), borders...)
}

// RenderDialogWithTypeCtx renders a dialog with a specific type using a specific context
func RenderDialogWithTypeCtx(title, content string, focused bool, targetHeight int, dialogType DialogType, ctx StyleContext, borders ...BorderPair) string {
	var border lipgloss.Border
	if len(borders) > 0 {
		if focused {
			border = borders[0].Focused
		} else {
			border = borders[0].Unfocused
		}
	} else if dialogType == DialogTypeConfirm {
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
	} else {
		if ctx.LineCharacters {
			if focused {
				border = lipgloss.ThickBorder()
			} else {
				border = lipgloss.NormalBorder()
			}
		} else {
			if focused {
				border = thickAsciiBorder
			} else {
				border = AsciiBorder
			}
		}
	}

	titleStyle := ctx.DialogTitle
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(ctx.StatusSuccess.GetForeground())
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(ctx.StatusWarn.GetForeground())
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(ctx.StatusError.GetForeground())
	case DialogTypeConfirm:
		titleStyle = SemanticRawStyle("Theme_TitleQuestion") // Semantic
	}

	return renderDialogWithBorderCtx(title, content, border, focused, targetHeight, true, true, titleStyle, ctx)
}

// RenderUniformBlockDialog renders a dialog with block borders and uniform dark colors
func RenderUniformBlockDialog(title, content string) string {
	return RenderUniformBlockDialogCtx(title, content, GetActiveContext())
}

// RenderUniformBlockDialogCtx renders a uniform block dialog using specific context
func RenderUniformBlockDialogCtx(title, content string, ctx StyleContext) string {
	borders := GetBlockBorders(ctx.LineCharacters)
	return renderDialogWithBorderCtx(title, content, borders.Focused, true, 0, false, false, ctx.DialogTitleHelp, ctx)
}

// RenderBorderedBoxCtx renders a dialog with title and borders using a specific context.
// Unlike renderDialogWithBorderCtx, this accepts a known contentWidth instead of measuring content.
func RenderBorderedBoxCtx(rawTitle, content string, contentWidth int, targetHeight int, focused bool, rounded bool, titleAlign string, titleTag string, ctx StyleContext) string {
	var border lipgloss.Border
	if !ctx.DrawBorders {
		border = lipgloss.HiddenBorder()
	} else if ctx.Type == DialogTypeConfirm {
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
		if rounded {
			if focused {
				border = ThickRoundedBorder
			} else {
				border = lipgloss.RoundedBorder()
			}
		} else {
			if focused {
				border = lipgloss.ThickBorder()
			} else {
				border = lipgloss.NormalBorder()
			}
		}
	} else {
		if rounded {
			if focused {
				border = RoundedThickAsciiBorder
			} else {
				border = RoundedAsciiBorder
			}
		} else {
			if focused {
				border = thickAsciiBorder
			} else {
				border = AsciiBorder
			}
		}
	}

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(ctx.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(borderBG)

	if titleTag != "" {
		rawTitle = console.WrapSemantic(titleTag) + rawTitle
	}

	// Render title with Dialog background as the base for inheritance
	renderedTitle := RenderThemeTextCtx(rawTitle, ctx)

	lines := strings.Split(content, "\n")
	// Trust the passed contentWidth - don't expand based on line widths
	// Zone markers and other invisible ANSI sequences can inflate lipgloss.Width()
	// causing incorrect width calculations. The caller knows the correct width.
	actualWidth := contentWidth

	if targetHeight > 2 {
		contentHeight := len(lines)
		neededPadding := (targetHeight - 2) - contentHeight
		if neededPadding > 0 {
			for i := 0; i < neededPadding; i++ {
				lines = append(lines, "")
			}
		}
	}

	var result strings.Builder
	result.WriteString(borderStyleLight.Render(border.TopLeft))

	// Only render title section if there is actual text to show
	if WidthWithoutZones(renderedTitle) == 0 {
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
	} else {
		var leftT, rightT string
		if !ctx.DrawBorders {
			leftT = " "
			rightT = " "
		} else if ctx.LineCharacters {
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

		titleSectionLen := 1 + 1 + lipgloss.Width(renderedTitle) + 1 + 1
		if titleSectionLen > actualWidth {
			actualWidth = titleSectionLen
		}

		var leftPad int
		if titleAlign == "left" {
			leftPad = 0
		} else {
			leftPad = (actualWidth - titleSectionLen) / 2
		}
		rightPad := actualWidth - titleSectionLen - leftPad
		if rightPad < 0 {
			rightPad = 0
		}

		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, leftPad)))
		result.WriteString(borderStyleLight.Render(leftT))

		focusTag := "Theme_TitleFocusIndicator"

		if focused {
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+focusTag+"|}}▸", ctx.Prefix)))
			} else {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+focusTag+"|}}>", ctx.Prefix)))
			}
		} else {
			unfocusTag := "Theme_TitleUnfocusedIndicator"
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+unfocusTag+"|}} ", ctx.Prefix)))
			} else {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+unfocusTag+"|}} ", ctx.Prefix)))
			}
		}
		result.WriteString(renderedTitle)
		if focused {
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+focusTag+"|}}◂", ctx.Prefix)))
			} else {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+focusTag+"|}}<", ctx.Prefix)))
			}
		} else {
			unfocusTag := "Theme_TitleUnfocusedIndicator"
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+unfocusTag+"|}} ", ctx.Prefix)))
			} else {
				result.WriteString(borderStyleLight.Render(console.ToANSIWithPrefix("{{|"+unfocusTag+"|}} ", ctx.Prefix)))
			}
		}
		result.WriteString(borderStyleLight.Render(rightT))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPad)))
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		// Use WidthWithoutZones to get accurate visual width (zone markers are invisible)
		textWidth := WidthWithoutZones(line)

		var fullLine string
		if textWidth > actualWidth {
			// Truncate lines that are too wide to prevent bleeding
			truncated := TruncateRight(line, actualWidth)
			fullLine = MaintainBackground(truncated, ctx.Dialog)
		} else if textWidth < actualWidth {
			// Pad lines that are too narrow
			padding := lipgloss.NewStyle().Background(borderBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
			fullLine = MaintainBackground(line+padding, ctx.Dialog)
		} else {
			fullLine = MaintainBackground(line, ctx.Dialog)
		}

		result.WriteString(fullLine)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strutil.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}
