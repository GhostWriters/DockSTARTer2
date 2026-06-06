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

// RenderTopBorderBoxCtx renders only the top border line with a title (suitable for log panel).
// rightTitle is wrapped in T-bar connectors (like the left title). rightSuffix is appended after
// the T-bar section without additional styling (use it for pre-rendered icon strings).
// indicators[0]: spinner frame character — replaces ▸/◂ focus indicators when non-empty.
// indicators[1]: "1" when the spinner indicator is a changed indicator (uses ConsoleTitleChangedIndicator style).
func RenderTopBorderBoxCtx(title, rightTitle, rightSuffix, content string, contentWidth int, focused bool, titleStyle, borderStyle lipgloss.Style, ctx StyleContext, indicators ...string) string {
	spinInd := ""
	isChanged := false
	if len(indicators) > 0 {
		spinInd = indicators[0]
	}
	if len(indicators) > 1 && indicators[1] == "1" {
		isChanged = true
	}
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
			leftT = "|"
			rightT = "|"
		}
	}

	titleSectionLen := 1 + 1 + WidthWithoutZones(renderedTitle) + 1 + 1
	var leftPad int
	if ctx.PanelTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	// Calculate right padding, accounting for rightTitle (T-bar wrapped) and rightSuffix
	remainingWidth := actualWidth - titleSectionLen - leftPad
	var rightPadMid, rightPadEnd int
	rightSuffixWidth := WidthWithoutZones(rightSuffix)
	if rightTitle != "" {
		rightTitleWidth := WidthWithoutZones(renderedRightTitle)
		rightPadEnd = 1 // One dash minimum after suffix
		// +2 for the T-bar connectors on each side of rightTitle
		rightPadMid = remainingWidth - 2 - rightTitleWidth - rightSuffixWidth - rightPadEnd
		if rightPadMid < 0 {
			rightPadMid = 0
		}
	} else if rightSuffix != "" {
		rightPadEnd = 1
		rightPadMid = remainingWidth - rightSuffixWidth - rightPadEnd
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
		var indL, indR string
		if spinInd != "" {
			indL = spinInd
			indR = spinInd
		} else if ctx.LineCharacters {
			indL = "▸"
			indR = "◂"
		} else {
			indL = ">"
			indR = "<"
		}
		result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}" + indL)))
		result.WriteString(renderedTitle)
		result.WriteString(borderStyle.Render(theme.ToThemeANSI("{{|TitleFocusIndicator|}}" + indR)))
	} else if spinInd != "" {
		indStyle := "{{|TitleUnfocusedIndicator|}}"
		if isChanged {
			indStyle = "{{|ConsoleTitleChangedIndicator|}}"
		}
		result.WriteString(borderStyle.Render(theme.ToThemeANSI(indStyle + spinInd)))
		result.WriteString(renderedTitle)
		result.WriteString(borderStyle.Render(theme.ToThemeANSI(indStyle + spinInd)))
	} else {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(renderedTitle)
		result.WriteString(borderStyle.Render(" "))
	}
	result.WriteString(borderStyle.Render(rightT))
	result.WriteString(borderStyle.Render(strutil.Repeat(border.Top, rightPadMid)))
	if rightTitle != "" {
		result.WriteString(borderStyle.Render(leftT))
		result.WriteString(renderedRightTitle)
		result.WriteString(borderStyle.Render(rightT))
	}
	if rightSuffix != "" || rightTitle != "" {
		result.WriteString(rightSuffix)
		result.WriteString(borderStyle.Render(strutil.Repeat(border.Top, rightPadEnd)))
	}
	result.WriteString(borderStyle.Render(border.TopRight))
	result.WriteString("\n")

	// Append original content without side borders
	result.WriteString(content)

	return result.String()
}

