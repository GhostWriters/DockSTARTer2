package tui

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

// LargeTitleBarOverhead is the extra lines a large titlebar adds over a small one (title row + separator).
const LargeTitleBarOverhead = 2

// largeTitleSepConnectors returns the left/right T-junction characters for the separator line
// that sits between the large titlebar row and the dialog content.
func largeTitleSepConnectors(border lipgloss.Border, focused bool, lineChars bool) (left, right string) {
	if !lineChars {
		return "+", "+"
	}
	// Normal borders use ├/┤; thick borders use ┠/┨
	if border.TopLeft == "┏" { // thick
		return "┠", "┨"
	}
	return "├", "┤"
}

// renderLargeTitleRow renders the title row and separator line for the large titlebar style.
// Returns two lines (no trailing newline on the second): the title row and the separator.
func renderLargeTitleRow(rawTitle string, actualWidth int, focused bool, showIndicators bool, titleTag string, titleAlign string, rightWidget string, borderStyleLight, borderStyleDark lipgloss.Style, border lipgloss.Border, ctx StyleContext) string {
	areaStyle := ctx.LargeTitleArea
	if !hasExplicitBackground(areaStyle) {
		areaStyle = areaStyle.Background(ctx.Dialog.GetBackground())
	}
	areaBG := areaStyle.GetBackground()

	// Build title segment with LargeTitleArea as the reset base
	titleCtx := ctx
	titleCtx.Dialog = areaStyle
	largeTitleTag := "Large" + titleTag
	renderedTitle := RenderThemeTextCtx("{{|"+largeTitleTag+"|}}" + rawTitle + "{{[-]}}", titleCtx)
	titleWidth := lipgloss.Width(renderedTitle)

	// Focus indicators — plain spaces when not shown/not focused; MaintainBackground handles coloring
	indL, indR := " ", " "
	if showIndicators && focused {
		if ctx.LineCharacters {
			indL = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}▸{{[-]}}", titleCtx)
			indR = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}◂{{[-]}}", titleCtx)
		} else {
			indL = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}>{{[-]}}", titleCtx)
			indR = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}<{{[-]}}", titleCtx)
		}
	}

	// Total title section width (indicator + title + indicator)
	indWidth := 0
	if showIndicators {
		indWidth = 1
	}
	titleSectionWidth := indWidth + titleWidth + indWidth

	// Right widget
	rightWidgetWidth := WidthWithoutZones(rightWidget)

	// Center the title in the full inner width; widget floats to the right.
	innerWidth := actualWidth
	var leftPad, rightPadMid, rightPadEnd int
	if titleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (innerWidth - titleSectionWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
	}
	if rightWidget != "" {
		rightPadEnd = 1
		rightPadMid = innerWidth - leftPad - titleSectionWidth - rightWidgetWidth - rightPadEnd
		if rightPadMid < 0 {
			// Title is too wide to center; push it left to make room for widget.
			leftPad = innerWidth - titleSectionWidth - rightWidgetWidth - rightPadEnd
			if leftPad < 0 {
				leftPad = 0
			}
			rightPadMid = innerWidth - leftPad - titleSectionWidth - rightWidgetWidth - rightPadEnd
			if rightPadMid < 0 {
				rightPadMid = 0
			}
		}
	} else {
		rightPadMid = innerWidth - leftPad - titleSectionWidth
		if rightPadMid < 0 {
			rightPadMid = 0
		}
	}

	pad := func(n int) string {
		return strutil.Repeat(" ", n)
	}

	// Build the inner title row content (between border chars), then apply MaintainBackground
	// with LargeTitleArea as the base — same pattern as content lines use ContentBackground.
	var inner strings.Builder
	inner.WriteString(pad(leftPad))
	inner.WriteString(indL)
	inner.WriteString(renderedTitle)
	inner.WriteString(indR)
	inner.WriteString(pad(rightPadMid))
	if rightWidget != "" {
		inner.WriteString(rightWidget)
		inner.WriteString(pad(rightPadEnd))
	}

	// Title-row border chars use LargeTitleArea background so themes with a distinct
	// title area colour don't show a seam between the │ and the padding cells.
	titleBorderLight := borderStyleLight.Background(areaBG)
	titleBorderDark := borderStyleDark.Background(areaBG)

	var row strings.Builder
	row.WriteString(titleBorderLight.Render(border.Left))
	row.WriteString(MaintainBackground(inner.String(), areaStyle))
	row.WriteString(titleBorderDark.Render(border.Right))
	row.WriteString("\n")

	// Separator: left connector light (left side), dashes + right connector dark (bottom-right side).
	sepL, sepR := largeTitleSepConnectors(border, focused, ctx.LineCharacters)
	row.WriteString(borderStyleLight.Render(sepL))
	row.WriteString(borderStyleDark.Render(strutil.Repeat(border.Top, actualWidth)))
	row.WriteString(borderStyleDark.Render(sepR))

	return row.String()
}

// renderLargeTitleRowFromRendered is like renderLargeTitleRow but takes a pre-rendered
// title string (already styled) instead of a raw title + tag. Used by renderDialogWithBorderCtx
// where the title has already been passed through RenderThemeText.
func renderLargeTitleRowFromRendered(renderedTitle string, actualWidth int, focused bool, titleAlign string, rightWidget string, borderStyleLight, borderStyleDark lipgloss.Style, border lipgloss.Border, ctx StyleContext) string {
	areaStyle := ctx.LargeTitleArea
	if !hasExplicitBackground(areaStyle) {
		areaStyle = areaStyle.Background(ctx.Dialog.GetBackground())
	}
	areaBG := areaStyle.GetBackground()

	titleWidth := lipgloss.Width(renderedTitle)

	// Focus indicators — plain spaces when not focused; MaintainBackground handles coloring
	titleCtx := ctx
	titleCtx.Dialog = areaStyle
	indL, indR := " ", " "
	if focused {
		ind := "▸"
		indEnd := "◂"
		if !ctx.LineCharacters {
			ind = ">"
			indEnd = "<"
		}
		indL = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}"+ind+"{{[-]}}", titleCtx)
		indR = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}"+indEnd+"{{[-]}}", titleCtx)
	}

	titleSectionWidth := 1 + titleWidth + 1
	rightWidgetWidth := WidthWithoutZones(rightWidget)

	// Center the title in the full inner width; widget floats to the right.
	var leftPad, rightPadMid, rightPadEnd int
	if titleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
	}
	if rightWidget != "" {
		rightPadEnd = 1
		rightPadMid = actualWidth - leftPad - titleSectionWidth - rightWidgetWidth - rightPadEnd
		if rightPadMid < 0 {
			leftPad = actualWidth - titleSectionWidth - rightWidgetWidth - rightPadEnd
			if leftPad < 0 {
				leftPad = 0
			}
			rightPadMid = actualWidth - leftPad - titleSectionWidth - rightWidgetWidth - rightPadEnd
			if rightPadMid < 0 {
				rightPadMid = 0
			}
		}
	} else {
		rightPadMid = actualWidth - leftPad - titleSectionWidth
		if rightPadMid < 0 {
			rightPadMid = 0
		}
	}

	pad := func(n int) string {
		return strutil.Repeat(" ", n)
	}

	// Build the inner title row content (between border chars), then apply MaintainBackground
	// with LargeTitleArea as the base — same pattern as content lines use ContentBackground.
	var inner strings.Builder
	inner.WriteString(pad(leftPad))
	inner.WriteString(indL)
	inner.WriteString(renderedTitle)
	inner.WriteString(indR)
	inner.WriteString(pad(rightPadMid))
	if rightWidget != "" {
		inner.WriteString(rightWidget)
		inner.WriteString(pad(rightPadEnd))
	}

	// Title-row border chars use LargeTitleArea background so themes with a distinct
	// title area colour don't show a seam between the │ and the padding cells.
	titleBorderLight := borderStyleLight.Background(areaBG)
	titleBorderDark := borderStyleDark.Background(areaBG)

	var row strings.Builder
	row.WriteString(titleBorderLight.Render(border.Left))
	row.WriteString(MaintainBackground(inner.String(), areaStyle))
	row.WriteString(titleBorderDark.Render(border.Right))
	row.WriteString("\n")

	// Separator: left connector light (left side), dashes + right connector dark (bottom-right side).
	sepL, sepR := largeTitleSepConnectors(border, focused, ctx.LineCharacters)
	row.WriteString(borderStyleLight.Render(sepL))
	row.WriteString(borderStyleDark.Render(strutil.Repeat(border.Top, actualWidth)))
	row.WriteString(borderStyleDark.Render(sepR))

	return row.String()
}

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
	widgets := BuildInactiveTitleWidgets(ctx)
	return renderDialogWithTypeAndWidgets(title, content, focused, targetHeight, dialogType, ctx, widgets, borders...)
}

// renderDialogWithTypeAndWidgets is the internal implementation used by both
// RenderDialogWithTypeCtx (inactive widgets) and dialogs that manage title bar focus
// (active/inactive widgets based on state).
func renderDialogWithTypeAndWidgets(title, content string, focused bool, targetHeight int, dialogType DialogType, ctx StyleContext, widgets string, borders ...BorderPair) string {
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
		titleStyle = titleStyle.Foreground(SemanticRawStyle("TitleError").GetForeground())
	case DialogTypeConfirm:
		titleStyle = SemanticRawStyle("TitleQuestion") // Semantic
	}

	return renderDialogWithBorderCtx(title, content, border, focused, targetHeight, true, true, titleStyle, ctx, widgets)
}

// RenderUniformBlockDialog renders a dialog with block borders and uniform dark colors
func RenderUniformBlockDialog(title, content string) string {
	return RenderUniformBlockDialogCtx(title, content, GetActiveContext())
}

// RenderUniformBlockDialogCtx renders a uniform block dialog using specific context
func RenderUniformBlockDialogCtx(title, content string, ctx StyleContext) string {
	borders := GetBlockBorders(ctx.LineCharacters)
	return renderDialogWithBorderCtx(title, content, borders.Focused, true, 0, false, false, ctx.DialogTitleHelp, ctx, "")
}

// RenderTitleSegmentCtx renders a single title segment with connectors and optional indicators.
// This is the "title routine" that can be called multiple times for side-by-side titles.
func RenderTitleSegmentCtx(rawTitle string, borderFocused bool, contentFocused bool, showIndicators bool, titleTag string, ctx StyleContext) string {
	if titleTag != "" {
		rawTitle = console.WrapSemantic(titleTag) + rawTitle
	}
	renderedTitle := RenderThemeTextCtx(rawTitle, ctx)

	var leftT, rightT string
	if !ctx.DrawBorders {
		leftT = " "
		rightT = " "
	} else if ctx.LineCharacters {
		if borderFocused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if borderFocused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "|"
			rightT = "|"
		}
	}

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(borderBG)

	var result strings.Builder
	result.WriteString(borderStyleLight.Render(leftT))

	focusTag := "TitleFocusIndicator"
	if showIndicators {
		if contentFocused {
			ind := "▸"
			if !ctx.LineCharacters {
				ind = ">"
			}
			result.WriteString(borderStyleLight.Render(theme.ToThemeANSIWithPrefix("{{|"+focusTag+"|}}"+ind+"{{[-]}}", ctx.Prefix)))
		} else {
			unfocusTag := "TitleUnfocusedIndicator"
			result.WriteString(borderStyleLight.Render(theme.ToThemeANSIWithPrefix("{{|"+unfocusTag+"|}} {{[-]}}", ctx.Prefix)))
		}
	}

	result.WriteString(renderedTitle)

	if showIndicators {
		if contentFocused {
			ind := "◂"
			if !ctx.LineCharacters {
				ind = "<"
			}
			result.WriteString(borderStyleLight.Render(theme.ToThemeANSIWithPrefix("{{|"+focusTag+"|}}"+ind+"{{[-]}}", ctx.Prefix)))
		} else {
			unfocusTag := "TitleUnfocusedIndicator"
			result.WriteString(borderStyleLight.Render(theme.ToThemeANSIWithPrefix("{{|"+unfocusTag+"|}} {{[-]}}", ctx.Prefix)))
		}
	}

	result.WriteString(borderStyleLight.Render(rightT))
	return result.String()
}

// WidthOfTitleSegment returns the visual width of a title segment including connectors and indicators.
func WidthOfTitleSegment(rawTitle string, showIndicators bool, ctx StyleContext) int {
	indicatorLen := 0
	if showIndicators {
		indicatorLen = 1
	}
	return 1 + indicatorLen + WidthWithoutZones(RenderThemeTextCtx(rawTitle, ctx)) + indicatorLen + 1
}

// RenderBorderedBoxCtx renders a dialog with title and borders using a specific context.
// Unlike renderDialogWithBorderCtx, this accepts a known contentWidth instead of measuring content.
func RenderBorderedBoxCtx(rawTitle, content string, contentWidth int, targetHeight int, focused bool, showIndicators bool, rounded bool, titleAlign string, titleTag string, ctx StyleContext, rightWidgets ...string) string {
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
	} else if rounded {
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

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(borderBG)
	borderStyleDark := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(borderBG)

	// Trim trailing newline so we don't treat a terminal newline as an extra blank line.
	// Standard bubbletea components (like list) usually include a trailing newline.
	content = strings.TrimSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	actualWidth := contentWidth

	// Decide useLarge BEFORE padding so the line-count check uses the original content height.
	// Large requires a title, is not used for submenus (caller sets LargeTitleBars=false),
	// and falls back to small when height is constrained.
	useLarge := ctx.LargeTitleBars && GetPlainText(rawTitle) != "" && titleTag != "RAW"
	if useLarge && targetHeight > 0 {
		// Fall back if there isn't room for the extra 2 lines.
		if len(lines)+LargeTitleBarOverhead+2 > targetHeight {
			useLarge = false
		}
	}

	// Pad content to fill the height budget using the correct overhead.
	overhead := 2 // top border + bottom border
	if useLarge {
		overhead += LargeTitleBarOverhead
	}
	if targetHeight > overhead {
		contentHeight := len(lines)
		neededPadding := (targetHeight - overhead) - contentHeight
		if neededPadding > 0 {
			for i := 0; i < neededPadding; i++ {
				lines = append(lines, "")
			}
		}
	}

	var result strings.Builder

	if useLarge {
		// Large titlebar: plain top border, then title row + separator.
		result.WriteString(borderStyleLight.Render(border.TopLeft))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
		result.WriteString(borderStyleLight.Render(border.TopRight))
		result.WriteString("\n")
		rightWidget := ""
		if len(rightWidgets) > 0 {
			rightWidget = rightWidgets[0]
		}
		result.WriteString(renderLargeTitleRow(rawTitle, actualWidth, focused, showIndicators, titleTag, titleAlign, rightWidget, borderStyleLight, borderStyleDark, border, ctx))
		result.WriteString("\n")
	} else {
		result.WriteString(borderStyleLight.Render(border.TopLeft))
		if GetPlainText(rawTitle) == "" {
			result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
		} else {
			var renderedSegment string
			var titleSectionLen int
			if titleTag == "RAW" {
				renderedSegment = rawTitle
				titleSectionLen = WidthWithoutZones(rawTitle)
			} else {
				renderedSegment = RenderTitleSegmentCtx(rawTitle, focused, focused, showIndicators, titleTag, ctx)
				titleSectionLen = WidthOfTitleSegment(rawTitle, showIndicators, ctx)
			}
			if titleSectionLen > actualWidth {
				actualWidth = titleSectionLen
			}
			rightWidget := ""
			if len(rightWidgets) > 0 {
				rightWidget = rightWidgets[0]
			}
			rightWidgetWidth := WidthWithoutZones(rightWidget)
			if rightWidget != "" {
				needed := rightWidgetWidth + 1
				for {
					lp := 0
					if titleAlign != "left" {
						lp = (actualWidth - titleSectionLen) / 2
					}
					if actualWidth-titleSectionLen-lp >= needed {
						break
					}
					actualWidth++
				}
			}
			var leftPad int
			if titleAlign == "left" {
				leftPad = 0
			} else {
				leftPad = (actualWidth - titleSectionLen) / 2
			}
			remaining := actualWidth - titleSectionLen - leftPad
			var rightPadMid, rightPadEnd int
			if rightWidget != "" {
				rightPadEnd = 1
				rightPadMid = remaining - rightWidgetWidth - rightPadEnd
				if rightPadMid < 0 {
					rightPadMid = 0
				}
			} else {
				rightPadMid = remaining
				if rightPadMid < 0 {
					rightPadMid = 0
				}
			}
			result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, leftPad)))
			result.WriteString(renderedSegment)
			result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPadMid)))
			if rightWidget != "" {
				result.WriteString(rightWidget)
				result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPadEnd)))
			}
		}
		result.WriteString(borderStyleLight.Render(border.TopRight))
		result.WriteString("\n")
	}

	maxLines := len(lines)
	if targetHeight > overhead {
		maxLines = targetHeight - overhead
	}

	for i, line := range lines {
		if i >= maxLines {
			break
		}
		result.WriteString(borderStyleLight.Render(border.Left))
		// Use WidthWithoutZones to get accurate visual width (zone markers are invisible)
		textWidth := WidthWithoutZones(line)

		contentBG := ctx.ContentBackground.GetBackground()
		var fullLine string
		if textWidth > actualWidth {
			// Truncate lines that are too wide to prevent bleeding
			truncated := TruncateRight(line, actualWidth)
			fullLine = MaintainBackground(truncated, ctx.ContentBackground)
		} else if textWidth < actualWidth {
			// Pad lines that are too narrow
			padding := lipgloss.NewStyle().Background(contentBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
			fullLine = MaintainBackground(line+padding, ctx.ContentBackground)
		} else {
			fullLine = MaintainBackground(line, ctx.ContentBackground)
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

// renderDialogWithBorderCtx handles internal shared rendering logic using a specific context.
func renderDialogWithBorderCtx(title, content string, border lipgloss.Border, focused bool, targetHeight int, threeD bool, useConnectors bool, titleStyle lipgloss.Style, ctx StyleContext, rightWidget string) string {
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

	useLargeInner := ctx.LargeTitleBars && title != ""
	if useLargeInner && targetHeight > 0 {
		if len(lines)+LargeTitleBarOverhead+2 > targetHeight {
			useLargeInner = false
		}
	}
	innerOverhead := 2
	if useLargeInner {
		innerOverhead += LargeTitleBarOverhead
	}

	if targetHeight > innerOverhead {
		contentHeight := len(lines)
		neededPadding := (targetHeight - innerOverhead) - contentHeight
		if neededPadding > 0 {
			for i := 0; i < neededPadding; i++ {
				lines = append(lines, "")
			}
		}
	}

	var result strings.Builder

	if useLargeInner {
		result.WriteString(borderStyleLight.Render(border.TopLeft))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
		result.WriteString(borderStyleLight.Render(border.TopRight))
		result.WriteString("\n")
		largeWidget := ""
		if rightWidget != "" {
			largeWidget = buildLargeTitleBarWidgets(false, 0, ctx)
		}
		result.WriteString(renderLargeTitleRowFromRendered(title, actualWidth, focused, ctx.DialogTitleAlign, largeWidget, borderStyleLight, borderStyleDark, border, ctx))
		result.WriteString("\n")
	} else {
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
		rightWidgetWidth := WidthWithoutZones(rightWidget)
		if rightWidget != "" {
			// Grow actualWidth until the right side (after centering) fits the widget + 1 end dash.
			needed := rightWidgetWidth + 1
			for {
				lp := 0
				if ctx.DialogTitleAlign != "left" {
					lp = (actualWidth - titleSectionLen) / 2
				}
				if actualWidth-titleSectionLen-lp >= needed {
					break
				}
				actualWidth++
			}
		}

		var leftPad int
		if ctx.DialogTitleAlign == "left" {
			leftPad = 0
		} else {
			leftPad = (actualWidth - titleSectionLen) / 2
		}
		rightPad := actualWidth - titleSectionLen - leftPad
		var rightPadMid, rightPadEnd int
		if rightWidget != "" {
			rightPadEnd = 1
			rightPadMid = rightPad - rightWidgetWidth - rightPadEnd
			if rightPadMid < 0 {
				rightPadMid = 0
			}
		} else {
			rightPadMid = rightPad
			if rightPadMid < 0 {
				rightPadMid = 0
			}
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
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPadMid)))
		if rightWidget != "" {
			result.WriteString(rightWidget)
			result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPadEnd)))
		}
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")
	} // end else (small titlebar)

	maxLines := len(lines)
	if targetHeight > innerOverhead {
		maxLines = targetHeight - innerOverhead
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
