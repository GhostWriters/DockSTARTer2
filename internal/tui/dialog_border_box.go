package tui

import (
	"strings"

	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

// GetBlockBorders returns a BorderPair with solid block borders for both states
func GetBlockBorders(lineCharacters bool) BorderPair {
	var block lipgloss.Border
	if lineCharacters {
		block = lipgloss.BlockBorder()
	} else {
		block = lipgloss.Border{
			Top:         "█",
			Bottom:      "█",
			Left:        "█",
			Right:       "█",
			TopLeft:     "█",
			TopRight:    "█",
			BottomLeft:  "█",
			BottomRight: "█",
		}
	}
	return BorderPair{Focused: block, Unfocused: block}
}

// RenderTopBorderBoxCtx renders only the top border line with a title (suitable for log panel)
func RenderTopBorderBoxCtx(title, rightTitle, content string, contentWidth int, focused bool, titleStyle, borderStyle lipgloss.Style, ctx StyleContext) string {
	borderStyle = ctx.BorderFlags.Apply(borderStyle)
	var border lipgloss.Border
	if !ctx.DrawBorders {
		border = lipgloss.HiddenBorder()
	} else if ctx.LineCharacters {
		if focused {
			border = lipgloss.ThickBorder()
		} else {
			border = lipgloss.NormalBorder()
		}
	} else {
		if focused {
			border = lipgloss.Border{
				Top:         "=",
				Bottom:      "=",
				Left:        "|",
				Right:       "|",
				TopLeft:     "+",
				TopRight:    "+",
				BottomLeft:  "+",
				BottomRight: "+",
			}
		} else {
			border = lipgloss.Border{
				Top:         "-",
				Bottom:      "-",
				Left:        "|",
				Right:       "|",
				TopLeft:     "+",
				TopRight:    "+",
				BottomLeft:  "+",
				BottomRight: "+",
			}
		}
	}

	// Render titles
	renderedTitle := RenderThemeText(title, titleStyle)
	renderedRightTitle := ""
	if rightTitle != "" {
		renderedRightTitle = RenderThemeText(rightTitle, titleStyle)
	}

	// actualWidth is the space between corners. Total width is actualWidth + 2.
	actualWidth := contentWidth - 2

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

	titleSectionLen := 1 + 1 + WidthWithoutZones(renderedTitle) + 1 + 1
	var leftPad int
	if ctx.LogTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	// Calculate right padding, accounting for rightTitle
	remainingWidth := actualWidth - titleSectionLen - leftPad
	var rightPadMid, rightPadEnd int
	if rightTitle != "" {
		rightTitleWidth := WidthWithoutZones(renderedRightTitle)
		rightPadEnd = 1 // One dash minimum after right title
		rightPadMid = remainingWidth - rightTitleWidth - rightPadEnd
		if rightPadMid < 0 {
			rightPadMid = 0
		}
	} else {
		rightPadMid = remainingWidth
		rightPadEnd = 0
	}

	var result strings.Builder
	result.WriteString(borderStyle.Render(border.TopLeft))
	result.WriteString(borderStyle.Render(strutil.Repeat(border.Top, leftPad)))
	result.WriteString(borderStyle.Render(leftT))
	if focused {
		if ctx.LineCharacters {
			result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}▸")))
		} else {
			result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}>")))
		}
	} else {
		result.WriteString(borderStyle.Render(" "))
	}
	result.WriteString(renderedTitle)
	if focused {
		if ctx.LineCharacters {
			result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}◂")))
		} else {
			result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}<")))
		}
	} else {
		result.WriteString(borderStyle.Render(" "))
	}
	result.WriteString(borderStyle.Render(rightT))
	result.WriteString(borderStyle.Render(strutil.Repeat(border.Top, rightPadMid)))
	if rightTitle != "" {
		result.WriteString(renderedRightTitle)
		result.WriteString(borderStyle.Render(strutil.Repeat(border.Top, rightPadEnd)))
	}
	result.WriteString(borderStyle.Render(border.TopRight))
	result.WriteString("\n")

	// Append original content without side borders
	result.WriteString(content)

	return result.String()
}

// renderDialogWithBorderCtx handles internal shared rendering logic using a specific context
func renderDialogWithBorderCtx(title, content string, border lipgloss.Border, focused bool, targetHeight int, threeD bool, useConnectors bool, titleStyle lipgloss.Style, ctx StyleContext) string {
	if title != "" && !strings.HasSuffix(title, "{{[-]}}") {
		title += "{{[-]}}"
	}

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(borderBG)
	borderStyleDark := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(borderBG)

	if !threeD {
		borderStyleLight = borderStyleDark
	}

	if !hasExplicitBackground(titleStyle) {
		titleStyle = titleStyle.Background(borderBG)
	}

	title = RenderThemeText(title, titleStyle)
	content = RenderThemeText(content, ctx.ContentBackground)

	lines := strings.Split(content, "\n")
	actualWidth := 0
	for _, line := range lines {
		// Use WidthWithoutZones to avoid zone markers inflating width
		w := WidthWithoutZones(line)
		if w > actualWidth {
			actualWidth = w
		}
	}

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
	if title == "" {
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
	} else {
		var leftT, rightT string
		if useConnectors {
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
		} else {
			leftT = border.Top
			rightT = border.Top
		}

		titleSectionLen := 1 + 1 + lipgloss.Width(title) + 1 + 1
		if titleSectionLen > actualWidth {
			actualWidth = titleSectionLen
		}

		var leftPad int
		if ctx.DialogTitleAlign == "left" {
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
		if focused {
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}▸")))
			} else {
				result.WriteString(borderStyleLight.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}>")))
			}
		} else {
			result.WriteString(borderStyleLight.Render(" "))
		}
		result.WriteString(title)
		if focused {
			if ctx.LineCharacters {
				result.WriteString(borderStyleLight.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}◂")))
			} else {
				result.WriteString(borderStyleLight.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}<")))
			}
		} else {
			result.WriteString(borderStyleLight.Render(" "))
		}
		result.WriteString(borderStyleLight.Render(rightT))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPad)))
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	maxLines := len(lines)
	if targetHeight > 2 {
		maxLines = targetHeight - 2
	}

	for i, line := range lines {
		if i >= maxLines {
			break
		}
		result.WriteString(borderStyleLight.Render(border.Left))
		// Use WidthWithoutZones to get accurate visual width (zone markers are invisible)
		textWidth := WidthWithoutZones(line)
		padding := ""
		contentBG := ctx.ContentBackground.GetBackground()
		if textWidth < actualWidth {
			padding = lipgloss.NewStyle().Background(contentBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
		}
		fullLine := MaintainBackground(line+padding, ctx.ContentBackground)
		result.WriteString(fullLine)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strutil.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}
