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

// TitleBarState carries the state needed to render title bar widgets ([?]/[×]).
// The zero value means "no widgets shown".
type TitleBarState struct {
	Show             bool             // Whether to render the title bar widgets
	Focused          bool             // Whether the title bar has keyboard focus
	ActiveWidget     TitleBarWidget   // Which widget has focus
	PressedWidget    TitleBarWidget   // Which widget is currently pressed (click flash)
	Widgets          []TitleBarWidget // Ordered widget set; nil means defaultWidgets
	SpinnerIndicator      string // When non-empty, replaces left focus indicator with this spinner frame
	SpinnerIndicatorRight string // When non-empty, replaces right focus indicator (defaults to SpinnerIndicator)
}

func (s TitleBarState) rightSpinner() string {
	if s.SpinnerIndicatorRight != "" {
		return s.SpinnerIndicatorRight
	}
	return s.SpinnerIndicator
}

func (s TitleBarState) activeWidgets() []TitleBarWidget {
	if len(s.Widgets) > 0 {
		return s.Widgets
	}
	return defaultWidgets
}

// titleTagFromAreaName derives the small-titlebar titleTag from a LargeTitleArea style name.
// e.g. "LargeTitleAreaQuestion" → "TitleQuestion", "LargeTitleArea" → "Title"
func titleTagFromAreaName(areaStyleName string) string {
	suffix := strings.TrimPrefix(areaStyleName, "LargeTitleArea")
	if suffix == "" {
		return "Title"
	}
	return "Title" + suffix
}

// semanticRawStyleCtx looks up a theme style, using ctx.Prefix when set so that contexts
// like the appearance-settings preview (Prefix="Preview_") resolve styles from the correct
// theme namespace rather than the currently active theme.
func semanticRawStyleCtx(name string, ctx StyleContext) lipgloss.Style {
	if ctx.Prefix != "" {
		if s := SemanticRawStyle(ctx.Prefix + name); hasExplicitBackground(s) {
			return s
		}
	}
	return SemanticRawStyle(name)
}

// largeTitleSepConnectors returns the left/right T-junction characters for the separator line
// that sits between the large titlebar row and the dialog content.
func largeTitleSepConnectors(border lipgloss.Border, lineChars bool) (left, right string) {
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
func renderLargeTitleRow(rawTitle string, actualWidth int, focused bool, showIndicators bool, titleTag string, titleAlign string, tbs TitleBarState, borderStyleLight, borderStyleDark lipgloss.Style, border lipgloss.Border, ctx StyleContext) string {
	areaStyleName := "LargeTitleArea" + strings.TrimPrefix(titleTag, "Title")
	areaStyle := semanticRawStyleCtx(areaStyleName, ctx)
	if !hasExplicitBackground(areaStyle) {
		areaStyle = semanticRawStyleCtx("LargeTitleArea", ctx)
		if !hasExplicitBackground(areaStyle) {
			areaStyle = areaStyle.Background(ctx.Dialog.GetBackground())
		}
	}
	// Build title segment with LargeTitleArea as the reset base
	titleCtx := ctx
	titleCtx.Dialog = areaStyle
	largeTitleTag := "Large" + titleTag
	renderedTitle := RenderThemeTextCtx("{{|"+largeTitleTag+"|}}" + rawTitle + "{{[-]}}", titleCtx)
	titleWidth := lipgloss.Width(renderedTitle)

	// Focus indicators — plain spaces when not shown; spinner visible when running regardless of focus
	indL, indR := " ", " "
	if showIndicators {
		if tbs.SpinnerIndicator != "" {
			tag := "LargeTitleFocusIndicator"
			if !focused {
				tag = "LargeTitleUnfocusedIndicator"
			}
			indL = RenderThemeTextCtx("{{|"+tag+"|}}" + tbs.SpinnerIndicator + "{{[-]}}", titleCtx)
			indR = RenderThemeTextCtx("{{|"+tag+"|}}" + tbs.rightSpinner() + "{{[-]}}", titleCtx)
		} else if focused {
			if ctx.LineCharacters {
				indL = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}▸{{[-]}}", titleCtx)
				indR = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}◂{{[-]}}", titleCtx)
			} else {
				indL = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}>{{[-]}}", titleCtx)
				indR = RenderThemeTextCtx("{{|LargeTitleFocusIndicator|}}<{{[-]}}", titleCtx)
			}
		}
	}

	// Total title section width (indicator + title + indicator)
	indWidth := 0
	if showIndicators {
		indWidth = 1
	}
	titleSectionWidth := indWidth + titleWidth + indWidth

	// Right widget — built from TitleBarState so large styles are always used in this path.
	renderedWidget := ""
	rightWidgetWidth := 0
	if tbs.Show {
		rawWidget := buildLargeTitleBarWidgets(tbs.Focused, tbs.ActiveWidget, tbs.PressedWidget, tbs.activeWidgets(), ctx)
		renderedWidget = RenderThemeTextCtx(rawWidget, titleCtx)
		rightWidgetWidth = lipgloss.Width(renderedWidget)
	}

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
	if tbs.Show {
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
	if tbs.Show {
		inner.WriteString(renderedWidget)
		inner.WriteString(pad(rightPadEnd))
	}

	var row strings.Builder
	row.WriteString(borderStyleLight.Render(border.Left))
	row.WriteString(MaintainBackground(RenderThemeTextCtx(inner.String(), titleCtx), areaStyle))
	row.WriteString(borderStyleDark.Render(border.Right))
	row.WriteString("\n")

	// Separator: left connector light (left side), dashes + right connector dark (bottom-right side).
	sepL, sepR := largeTitleSepConnectors(border, ctx.LineCharacters)
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
	return renderDialogWithTypeAndWidgets(title, content, focused, targetHeight, dialogType, ctx, TitleBarState{Show: true}, borders...)
}

// RenderDialogWithTypeAndWidgets renders a dialog with explicit title bar widget state.
// Use this when the dialog embeds TitleBarFocus and manages its own widget state.
func RenderDialogWithTypeAndWidgets(title, content string, focused bool, targetHeight int, dialogType DialogType, tbs TitleBarState, borders ...BorderPair) string {
	return renderDialogWithTypeAndWidgets(title, content, focused, targetHeight, dialogType, GetActiveContext(), tbs, borders...)
}

// renderDialogWithTypeAndWidgets is the internal implementation used by both
// RenderDialogWithTypeCtx (inactive widgets) and dialogs that manage title bar focus
// (active/inactive widgets based on state).
func renderDialogWithTypeAndWidgets(title, content string, focused bool, targetHeight int, dialogType DialogType, ctx StyleContext, tbs TitleBarState, borders ...BorderPair) string {
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
	areaStyleName := "LargeTitleArea"
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(ctx.StatusSuccess.GetForeground())
		areaStyleName = "LargeTitleAreaSuccess"
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(ctx.StatusWarn.GetForeground())
		areaStyleName = "LargeTitleAreaWarn"
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(SemanticRawStyle("TitleError").GetForeground())
		areaStyleName = "LargeTitleAreaError"
	case DialogTypeConfirm:
		titleStyle = SemanticRawStyle("TitleQuestion") // Semantic
		areaStyleName = "LargeTitleAreaQuestion"
	}

	return renderDialogWithBorderCtx(title, content, border, focused, targetHeight, true, true, titleStyle, ctx, tbs, areaStyleName)
}

// RenderUniformBlockDialog renders a dialog with block borders and uniform dark colors
func RenderUniformBlockDialog(title, content string) string {
	return RenderUniformBlockDialogCtx(title, content, GetActiveContext())
}

// RenderUniformBlockDialogCtx renders a uniform block dialog using specific context
func RenderUniformBlockDialogCtx(title, content string, ctx StyleContext) string {
	borders := GetBlockBorders(ctx.LineCharacters)
	return renderDialogWithBorderCtx(title, content, borders.Focused, true, 0, false, false, ctx.DialogTitleHelp, ctx, TitleBarState{}, "LargeTitleArea")
}

// RenderTitleSegmentCtx renders a single title segment with connectors and optional indicators.
// This is the "title routine" that can be called multiple times for side-by-side titles.
// spinnerIndicator, when non-empty, replaces the focus indicators with the given frame character.
// Pass a second value to use a different frame for the right indicator (counter-clockwise effect).
func RenderTitleSegmentCtx(rawTitle string, borderFocused bool, contentFocused bool, showIndicators bool, titleTag string, ctx StyleContext, spinnerIndicator ...string) string {
	spinInd := ""
	spinIndR := ""
	if len(spinnerIndicator) > 0 {
		spinInd = spinnerIndicator[0]
		spinIndR = spinInd
	}
	if len(spinnerIndicator) > 1 {
		spinIndR = spinnerIndicator[1]
	}
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

	if showIndicators {
		if contentFocused {
			var ind string
			if spinInd != "" {
				ind = spinInd
			} else if ctx.LineCharacters {
				ind = "▸"
			} else {
				ind = ">"
			}
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleFocusIndicator|}}"+ind+"{{[-]}}", ctx.Prefix)))
		} else if spinInd != "" {
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}}"+spinInd+"{{[-]}}", ctx.Prefix)))
		} else {
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}} {{[-]}}", ctx.Prefix)))
		}
	}

	result.WriteString(renderedTitle)

	if showIndicators {
		if contentFocused {
			var ind string
			if spinIndR != "" {
				ind = spinIndR
			} else if ctx.LineCharacters {
				ind = "◂"
			} else {
				ind = "<"
			}
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleFocusIndicator|}}"+ind+"{{[-]}}", ctx.Prefix)))
		} else if spinIndR != "" {
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}}"+spinIndR+"{{[-]}}", ctx.Prefix)))
		} else {
			result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}} {{[-]}}", ctx.Prefix)))
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
func RenderBorderedBoxCtx(rawTitle, content string, contentWidth int, targetHeight int, focused bool, showIndicators bool, rounded bool, titleAlign string, titleTag string, ctx StyleContext, tbs ...TitleBarState) string {
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
	// Extract the single optional TitleBarState (zero value = no widgets).
	var tbsState TitleBarState
	if len(tbs) > 0 {
		tbsState = tbs[0]
	}

	isSubmenu := titleTag == "TitleSubMenu" || titleTag == "TitleSubMenuFocused"
	useLarge := ctx.LargeTitleBars && GetPlainText(rawTitle) != "" && titleTag != "RAW" && !isSubmenu
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
		// renderLargeTitleRow builds large widgets internally from tbsState.
		result.WriteString(borderStyleLight.Render(border.TopLeft))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
		result.WriteString(borderStyleLight.Render(border.TopRight))
		result.WriteString("\n")
		result.WriteString(renderLargeTitleRow(rawTitle, actualWidth, focused, showIndicators, titleTag, titleAlign, tbsState, borderStyleLight, borderStyleDark, border, ctx))
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
				renderedSegment = RenderTitleSegmentCtx(rawTitle, focused, focused, showIndicators, titleTag, ctx, tbsState.SpinnerIndicator)
				titleSectionLen = WidthOfTitleSegment(rawTitle, showIndicators, ctx)
			}
			if titleSectionLen > actualWidth {
				actualWidth = titleSectionLen
			}
			// Small titlebar: build small-style widgets from tbsState.
			rightWidget := ""
			if tbsState.Show {
				rightWidget = buildDialogTitleWidgets(tbsState.Focused, tbsState.ActiveWidget, tbsState.PressedWidget, tbsState.activeWidgets(), ctx)
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
func renderDialogWithBorderCtx(title, content string, border lipgloss.Border, focused bool, targetHeight int, threeD bool, useConnectors bool, _ lipgloss.Style, ctx StyleContext, tbs TitleBarState, areaStyleName string) string {
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

	// Render content first to measure actual width (used by both large and small paths).
	content = RenderThemeText(content, ctx.ContentBackground)
	lines := strings.Split(content, "\n")
	actualWidth := 0
	for _, line := range lines {
		w := WidthWithoutZones(line)
		if w > actualWidth {
			actualWidth = w
		}
	}

	// Determine large title bar mode.
	useLargeInner := ctx.LargeTitleBars && title != ""
	if useLargeInner && targetHeight > 0 {
		if len(lines)+LargeTitleBarOverhead+2 > targetHeight {
			useLargeInner = false
		}
	}

	// Save raw title for the large path (renderLargeTitleRow handles its own styling).
	// For the small path, render using the semantic title tag derived from areaStyleName
	// (e.g. "LargeTitleAreaQuestion" → "TitleQuestion") so colors match the large path.
	rawTitle := strings.TrimSuffix(title, "{{[-]}}")
	if !useLargeInner {
		titleTag := titleTagFromAreaName(areaStyleName)
		titleCtx := ctx
		titleCtx.Dialog = titleCtx.Dialog.Background(borderBG)
		title = RenderThemeTextCtx("{{|"+titleTag+"|}}" + rawTitle + "{{[-]}}", titleCtx)
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
		// Large titlebar: use renderLargeTitleRow with raw title and LargeTitle* styles,
		// matching the menu/RenderBorderedBoxCtx path exactly.
		result.WriteString(borderStyleLight.Render(border.TopLeft))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
		result.WriteString(borderStyleLight.Render(border.TopRight))
		result.WriteString("\n")
		result.WriteString(renderLargeTitleRow(rawTitle, actualWidth, focused, true, titleTagFromAreaName(areaStyleName), ctx.DialogTitleAlign, tbs, borderStyleLight, borderStyleDark, border, ctx))
		result.WriteString("\n")
	} else {
		// Small titlebar: build small-style widgets from tbs.
		rightWidget := ""
		if tbs.Show {
			rightWidget = buildDialogTitleWidgets(tbs.Focused, tbs.ActiveWidget, tbs.PressedWidget, tbs.activeWidgets(), ctx)
		}
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
			spinInd := tbs.SpinnerIndicator
			spinIndR := tbs.rightSpinner()
			if focused {
				var ind string
				if spinInd != "" {
					ind = spinInd
				} else if ctx.LineCharacters {
					ind = "▸"
				} else {
					ind = ">"
				}
				result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleFocusIndicator|}}" + ind)))
			} else if spinInd != "" {
				result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}}" + spinInd)))
			} else {
				result.WriteString(borderStyleLight.Render(" "))
			}
			result.WriteString(title)
			if focused {
				var ind string
				if spinIndR != "" {
					ind = spinIndR
				} else if ctx.LineCharacters {
					ind = "◂"
				} else {
					ind = "<"
				}
				result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleFocusIndicator|}}" + ind)))
			} else if spinIndR != "" {
				result.WriteString(borderStyleLight.Render(theme.ToANSI("{{|TitleUnfocusedIndicator|}}" + spinIndR)))
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
	}

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
