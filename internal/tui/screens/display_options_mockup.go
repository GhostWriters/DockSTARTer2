package screens

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/tui"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

func (s *DisplayOptionsScreen) renderMockup(targetHeight int) string {
	width := 44 // Reduced width to fit the screen better

	bgStyle := tui.SemanticRawStyle("Preview_Theme_Screen")
	dContent := tui.SemanticRawStyle("Preview_Theme_Dialog")
	dBorder1 := tui.SemanticRawStyle("Preview_Theme_Border")
	dBorder2 := tui.SemanticRawStyle("Preview_Theme_Border2")

	// Adjust border colors based on setting
	switch s.config.UI.BorderColor {
	case 1:
		dBorder2 = dBorder1
	case 2:
		dBorder1 = dBorder2
	}

	var b lipgloss.Border
	if !s.config.UI.Borders {
		b = lipgloss.HiddenBorder()
	} else if s.config.UI.LineCharacters {
		b = lipgloss.RoundedBorder()
	} else {
		b = tui.RoundedAsciiBorder
	}

	// Build StyleContext for the preview
	previewCtx := tui.StyleContext{
		LineCharacters:      s.config.UI.LineCharacters,
		DrawBorders:         s.config.UI.Borders,
		ButtonBorders:       s.config.UI.ButtonBorders,
		Screen:              bgStyle,
		Dialog:              dContent,
		DialogTitle:         tui.SemanticRawStyle("Preview_Theme_Title"),
		DialogTitleHelp:     tui.SemanticRawStyle("Preview_Theme_TitleHelp"),
		SubmenuTitle:        tui.SemanticRawStyle("Preview_Theme_TitleSubMenu"),
		SubmenuTitleFocused: tui.SemanticRawStyle("Preview_Theme_TitleSubMenuFocused"),
		Border:              b,
		BorderColor:         dBorder1.GetForeground(),
		Border2Color:        dBorder2.GetForeground(),
		ButtonActive:        tui.SemanticRawStyle("Preview_Theme_ButtonActive"),
		ButtonInactive:      tui.SemanticRawStyle("Preview_Theme_ButtonInactive"),
		ItemNormal:          tui.SemanticRawStyle("Preview_Theme_Item"),
		ItemSelected:        tui.SemanticRawStyle("Preview_Theme_ItemSelected"),
		TagNormal:           tui.SemanticRawStyle("Preview_Theme_Tag"),
		TagSelected:         tui.SemanticRawStyle("Preview_Theme_TagSelected"),
		TagKey:              tui.SemanticRawStyle("Preview_Theme_TagKey"),
		TagKeySelected:      tui.SemanticRawStyle("Preview_Theme_TagKeySelected"),
		Shadow:              tui.SemanticRawStyle("Preview_Theme_Shadow"),
		ShadowColor:         getPreviewShadowColor(),
		ShadowLevel:         s.config.UI.ShadowLevel,
		HelpLine:            tui.SemanticRawStyle("Preview_Theme_Helpline"),
		StatusSuccess:       tui.SemanticRawStyle("Preview_Theme_TitleNotice"),
		StatusWarn:          tui.SemanticRawStyle("Preview_Theme_TitleWarn"),
		StatusError:         tui.SemanticRawStyle("Preview_Theme_TitleError"),
		DialogTitleAlign:    s.config.UI.DialogTitleAlign,
		SubmenuTitleAlign:   s.config.UI.SubmenuTitleAlign,
		LogTitleAlign:       s.config.UI.LogTitleAlign,
		Prefix:              "Preview_",
		DrawShadow:          s.config.UI.Shadow,
	}

	paddedLine := func(text string, style lipgloss.Style, fallback string, ctx ...tui.StyleContext) string {
		activeCtx := previewCtx
		if len(ctx) > 0 {
			activeCtx = ctx[0]
		}
		rendered := tui.RenderThemeTextCtx(text, activeCtx)
		plain := tui.GetPlainText(rendered)
		wt := lipgloss.Width(plain)
		if wt < width {
			leftPad := (width - wt) / 2
			rightPad := width - wt - leftPad
			return style.Render(strutil.Repeat(fallback, leftPad) + rendered + strutil.Repeat(fallback, rightPad))
		}
		return style.Render(plain[:width])
	}

	// --- 1. Header (Status Bar) ---
	hStyle := tui.SemanticRawStyle("Preview_Theme_StatusBar")
	headerCtx := previewCtx
	headerCtx.Dialog = hStyle // Ensure snapping back to StatusBar background

	themeName := s.previewTheme

	// Border style for the status bar frame (falls back to StatusBar if undefined)
	bStyle := tui.SemanticRawStyle("Preview_Theme_StatusBarBorder")
	if bStyle.GetForeground() == nil && bStyle.GetBackground() == nil {
		bStyle = hStyle
	}

	// Border characters (unfocused, since the preview is static)
	var leftChar, rightChar, bottomChar, bottomLeftChar, bottomRightChar string
	if s.config.UI.LineCharacters {
		bottomLeftChar = "╰"
		bottomRightChar = "╯"
		leftChar = "│"
		rightChar = "│"
		bottomChar = "─"
	} else {
		bottomLeftChar = "'"
		bottomRightChar = "'"
		leftChar = "|"
		rightChar = "|"
		bottomChar = "-"
	}

	// Header content is width-2 (border chars occupy 1 char each side)
	innerWidth := width - 2

	// Left: Host
	leftText := " {{|Theme_Hostname|}}HOST{{[-]}}"
	leftRendered := tui.RenderThemeTextCtx(leftText, headerCtx)
	leftW := lipgloss.Width(tui.GetPlainText(leftRendered))

	// Center: App Name
	centerText := "{{|Theme_ApplicationName|}}" + tui.GetPlainText(themeName) + "{{[-]}}"
	centerRendered := tui.RenderThemeTextCtx(centerText, headerCtx)
	centerW := lipgloss.Width(tui.GetPlainText(centerRendered))

	// Right: Version
	rightText := "{{|Theme_ApplicationVersion|}}A:[{{[-]}}{{|Theme_ApplicationVersion|}}2.1{{[-]}}{{|Theme_ApplicationVersion|}}]{{[-]}} "
	rightRendered := tui.RenderThemeTextCtx(rightText, headerCtx)
	rightW := lipgloss.Width(tui.GetPlainText(rightRendered))

	centerX := (innerWidth - centerW) / 2
	if centerX < 0 {
		centerX = 0
	}

	fitsLine1 := true
	if leftW+1 > centerX || centerX+centerW+1+rightW > innerWidth {
		fitsLine1 = false
	}

	var headerContent string
	if fitsLine1 {
		space1 := centerX - leftW
		space2 := innerWidth - (centerX + centerW) - rightW
		fullLine := leftRendered + strutil.Repeat(" ", space1) + centerRendered + strutil.Repeat(" ", space2) + rightRendered
		headerContent = hStyle.Render(fullLine)
	} else if leftW+1 <= centerX {
		line1 := leftRendered + strutil.Repeat(" ", centerX-leftW) + centerRendered
		line1 += strutil.Repeat(" ", innerWidth-lipgloss.Width(tui.GetPlainText(line1)))
		line2 := strutil.Repeat(" ", innerWidth-rightW) + rightRendered
		headerContent = hStyle.Render(line1) + "\n" + hStyle.Render(line2)
	} else {
		line1 := leftRendered + strutil.Repeat(" ", innerWidth-leftW)
		line2 := centerRendered + strutil.Repeat(" ", innerWidth-centerW)
		line3 := strutil.Repeat(" ", innerWidth-rightW) + rightRendered
		headerContent = hStyle.Render(line1) + "\n" + hStyle.Render(line2) + "\n" + hStyle.Render(line3)
	}
	headerRow := bStyle.Render(leftChar) + headerContent + bStyle.Render(rightChar)

	// Bottom border between header and content
	bottomBorderRow := bStyle.Render(bottomLeftChar + strutil.Repeat(bottomChar, width-2) + bottomRightChar)

	// --- 2. Backdrop Content (Dialog Simulation) ---
	contentLines := []string{
		" {{|Theme_Subtitle|}}A Subtitle Line{{[-]}}",
		"   {{|Theme_CommandLine|}}ds2 --theme{{[-]}}",
		"",
		" Heading: {{|Theme_HeadingValue|}}Value{{[-]}} {{|Theme_HeadingTag|}}[*Tag*]{{[-]}}",
		"",
		"    Caps: {{|Theme_KeyCap|}}[up]{{[-]}} {{|Theme_KeyCap|}}[down]{{[-]}} {{|Theme_KeyCap|}}[left]{{[-]}} {{|Theme_KeyCap|}}[right]",
		"",
		" Normal text",
		" {{|Theme_Highlight|}}Highlighted text{{[-]}}",
		"",
		// Menu Items Simulation
		" {{|Theme_Item|}}Item 1      Item Description{{[-]}}",
		" {{|Theme_Item|}}Item 2      {{|Theme_ListAppUserDefined|}}User Description{{[-]}}",
		"",
		" {{|Theme_LineHeading|}}*** .env ***{{[-]}}",
		" {{|Theme_LineComment|}}### Sample comment{{[-]}}",
		" {{|Theme_LineVar|}}Var='Default'{{[-]}}",
		" {{|Theme_LineModifiedVar|}}Var='Modified'{{[-]}}",
	}

	for i, l := range contentLines {
		contentLines[i] = tui.RenderThemeTextCtx(l, previewCtx)
	}
	contentStr := strings.Join(contentLines, "\n")

	titleParts := []string{
		"{{|Theme_Title|}}Title{{[-]}}",
		"{{|Theme_TitleSuccess|}}S{{[-]}}",
		"{{|Theme_TitleWarning|}}W{{[-]}}",
		"{{|Theme_TitleError|}}E{{[-]}}",
		"{{|Theme_TitleQuestion|}}Q{{[-]}}",
	}
	dTitle := tui.RenderThemeTextCtx(strings.Join(titleParts, " "), previewCtx)

	layout := tui.GetLayout()
	fixedLines := layout.BorderHeight() + 4
	backdropHeight := targetHeight - fixedLines
	if backdropHeight < 10 {
		backdropHeight = 10
	}
	backdropLines := make([]string, backdropHeight)
	filler := bgStyle.Render(strutil.Repeat(" ", width))
	for i := range backdropLines {
		backdropLines[i] = filler
	}
	backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)

	dialogBox := tui.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, true, false, previewCtx.DialogTitleAlign, "Theme_Title", previewCtx)
	dialogBox = tui.AddShadowCtx(dialogBox, previewCtx)
	backdropBlock = tui.Overlay(dialogBox, backdropBlock, tui.OverlayCenter, tui.OverlayCenter, 0, 0)

	// --- 3. Help Line ---
	helpStyle := tui.SemanticRawStyle("Preview_Theme_Helpline")
	helpCtx := previewCtx
	helpCtx.Dialog = helpStyle // Ensure snapping back to Helpline background

	// Help line is full width (no side borders)
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ", helpCtx)

	// --- 4. Log Toggle Strip ---
	// Actual UI uses HelpLine background for the strip, but LogBox/LogPanel for the label
	logBoxStyle := tui.SemanticRawStyle("Preview_Theme_LogBox")
	logPanelStyle := tui.SemanticRawStyle("Preview_Theme_LogPanel")

	logStripCtx := previewCtx
	logStripCtx.Dialog = logBoxStyle // Label should have LogBox (Black) background

	marker := "^"
	label := tui.RenderThemeTextCtx(" "+marker+" Log "+marker+" ", logStripCtx)

	// The strip itself uses HelpLine (Cyan) background
	stripStyle := lipgloss.NewStyle().
		Foreground(logPanelStyle.GetForeground()).
		Background(helpStyle.GetBackground())

	var leftT, rightT, borderTop, topLeftC, topRightC string
	if s.config.UI.LineCharacters {
		leftT = "┤"
		rightT = "├"
		borderTop = "─"
		topLeftC = "┌"
		topRightC = "┐"
	} else {
		leftT = "+"
		rightT = "+"
		borderTop = "-"
		topLeftC = "+"
		topRightC = "+"
	}

	labelW := lipgloss.Width(tui.GetPlainText(label))
	titleSectionLen := 1 + 1 + labelW + 1 + 1

	// Strip has side borders/corners in the mockup
	innerWidthStrip := width - 2
	var leftPad int
	if s.config.UI.LogTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (innerWidthStrip - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	leftDashes := strutil.Repeat(borderTop, leftPad)
	remaining := innerWidthStrip - leftPad - titleSectionLen
	rightDashes := strutil.Repeat(borderTop, remaining)

	// Render in pieces to prevent inner resets from Clearing the background
	logStripRow := stripStyle.Render(topLeftC+leftDashes+leftT+" ") +
		label +
		stripStyle.Render(" "+rightT+rightDashes+topRightC)

	mockupParts := []string{
		headerRow,
		bottomBorderRow,
		backdropBlock,
		helpRow,
		logStripRow,
	}
	for i, p := range mockupParts {
		mockupParts[i] = strings.TrimRight(p, "\n")
	}
	mockup := lipgloss.JoinVertical(lipgloss.Left, mockupParts...)

	// Wrap in a standard dialog using the current (active) theme
	mockupWidth := lipgloss.Width(mockup)
	// Synchronize preview height with settings dialog
	return tui.RenderBorderedBoxCtx("Preview", mockup, mockupWidth, targetHeight, false, true, false, tui.GetActiveContext().DialogTitleAlign, "Theme_Title", tui.GetActiveContext())
}

// getPreviewShadowColor extracts the shadow color from the preview theme
// Prefers foreground (for shade chars), falls back to background
func getPreviewShadowColor() color.Color {
	shadowStyle := tui.SemanticRawStyle("Preview_Theme_Shadow")
	if fg := shadowStyle.GetForeground(); fg != nil {
		return fg
	}
	return shadowStyle.GetBackground()
}
