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

	paddedLine := func(text string, style lipgloss.Style, fallback string) string {
		rendered := tui.RenderThemeText(text, style)
		plain := tui.GetPlainText(rendered)
		wt := lipgloss.Width(plain)
		if wt < width {
			leftPad := (width - wt) / 2
			rightPad := width - wt - leftPad
			return style.Render(strutil.Repeat(fallback, leftPad) + rendered + strutil.Repeat(fallback, rightPad))
		}
		return style.Render(plain[:width])
	}

	hStyle := tui.SemanticRawStyle("Preview_Theme_StatusBar")

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
	leftWidth := innerWidth / 3
	centerWidth := innerWidth / 3
	rightWidth := innerWidth - leftWidth - centerWidth

	// Left: Host
	leftText := " {{|Preview_Theme_Hostname|}}HOST{{[-]}}"
	leftSec := hStyle.Width(leftWidth).Align(lipgloss.Left).Render(tui.RenderThemeText(leftText, hStyle))

	// Center: App Name (centered within its third)
	centerText := "{{|Preview_Theme_ApplicationName|}}" + tui.GetPlainText(themeName) + "{{[-]}}"
	centerSec := hStyle.Width(centerWidth).Align(lipgloss.Center).Render(tui.RenderThemeText(centerText, hStyle))

	// Right: Version
	rightText := "{{|Preview_Theme_ApplicationVersion|}}A:[{{[-]}}{{|Preview_Theme_ApplicationVersion|}}2.1{{[-]}}{{|Preview_Theme_ApplicationVersion|}}]{{[-]}} "
	rightSec := hStyle.Width(rightWidth).Align(lipgloss.Right).Render(tui.RenderThemeText(rightText, hStyle))

	headerContent := lipgloss.JoinHorizontal(lipgloss.Top, leftSec, centerSec, rightSec)
	headerRow := bStyle.Render(leftChar) + headerContent + bStyle.Render(rightChar)

	// Bottom border replaces the old separator line
	bottomBorderRow := bStyle.Render(bottomLeftChar + strutil.Repeat(bottomChar, width-2) + bottomRightChar)

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
		b = tui.RoundedAsciiBorder // Use exported variant from tui package
	}

	// Build StyleContext for the preview
	previewCtx := tui.StyleContext{
		LineCharacters:      s.config.UI.LineCharacters,
		DrawBorders:         s.config.UI.Borders,
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
	}

	// Backdrop Content (Dialog Simulation)
	// Shortened strings to prevent overflow in the 44-cell width
	contentLines := []string{
		" {{|Preview_Theme_Subtitle|}}A Subtitle Line{{[-]}}",
		"   {{|Preview_Theme_CommandLine|}}ds2 --theme{{[-]}}",
		"",
		" Heading: {{|Preview_Theme_HeadingValue|}}Value{{[-]}} {{|Preview_Theme_HeadingTag|}}[*Tag*]{{[-]}}",
		"",
		"    Caps: {{|Preview_Theme_KeyCap|}}[up]{{[-]}} {{|Preview_Theme_KeyCap|}}[down]{{[-]}} {{|Preview_Theme_KeyCap|}}[left]{{[-]}} {{|Preview_Theme_KeyCap|}}[right]",
		"",
		" Normal text",
		" {{|Preview_Theme_Highlight|}}Highlighted text{{[-]}}",
		"",
		// Menu Items Simulation
		" {{|Preview_Theme_Item|}}Item 1      Item Description{{[-]}}",
		" {{|Preview_Theme_Item|}}Item 2      {{|Preview_Theme_ListAppUserDefined|}}User Description{{[-]}}",
		"",
		" {{|Preview_Theme_LineHeading|}}*** .env ***{{[-]}}",
		" {{|Preview_Theme_LineComment|}}### Sample comment{{[-]}}",
		" {{|Preview_Theme_LineVar|}}Var='Default'{{[-]}}",
		" {{|Preview_Theme_LineModifiedVar|}}Var='Modified'{{[-]}}",
	}

	for i, l := range contentLines {
		contentLines[i] = tui.RenderThemeText(l, dContent)
	}
	contentStr := strings.Join(contentLines, "\n")

	// Multi-segment title on the border
	titleParts := []string{
		"{{|Preview_Theme_Title|}}Title{{[-]}}",
		"{{|Preview_Theme_TitleSuccess|}}S{{[-]}}",
		"{{|Preview_Theme_TitleWarning|}}W{{[-]}}",
		"{{|Preview_Theme_TitleError|}}E{{[-]}}",
		"{{|Preview_Theme_TitleQuestion|}}Q{{[-]}}",
	}
	dTitle := strings.Join(titleParts, " ")

	layout := tui.GetLayout()
	// Backdrop - calculate height to fill available space
	// Structure: headerRow(1) + bottomBorderRow(1) + backdrop + helpRow(1) + logStripRow(1) + outer borders(2)
	fixedLines := layout.BorderHeight() + 4
	backdropHeight := targetHeight - fixedLines
	if backdropHeight < 10 {
		backdropHeight = 10 // minimum height for content visibility
	}
	backdropLines := make([]string, backdropHeight)
	filler := bgStyle.Render(strutil.Repeat(" ", width))
	for i := range backdropLines {
		backdropLines[i] = filler
	}
	backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)

	// Inner dialog mockup (centralized shadowing handles the effect)
	dialogBox := tui.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, false, previewCtx.DialogTitleAlign, "Preview_Theme_Title", previewCtx)
	// Backdrop Block with inner dialog overlay
	backdropBlock = tui.Overlay(dialogBox, backdropBlock, tui.OverlayCenter, tui.OverlayCenter, 0, 0)

	helpStyle := tui.SemanticRawStyle("Preview_Theme_Helpline")
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ")

	// Log Toggle Strip (Border)
	stripSepChar := "-"
	if s.config.UI.LineCharacters {
		stripSepChar = "─"
	} else {
		stripSepChar = "-"
	}
	marker := "^"
	label := " " + marker + " Log " + marker + " "
	stripStyle := lipgloss.NewStyle().
		Foreground(tui.SemanticRawStyle("Preview_Theme_LogPanel").GetForeground()).
		Background(helpStyle.GetBackground())

	labelW := lipgloss.Width(label)
	var dashW int
	if s.config.UI.LogTitleAlign != "left" {
		dashW = (width - labelW) / 2
	}
	leftDashes := strutil.Repeat(stripSepChar, dashW)
	rightTotal := width - dashW - labelW
	rightDashes := strutil.Repeat(stripSepChar, rightTotal)

	logStripRow := stripStyle.Render(leftDashes + label + rightDashes)

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
	preview := tui.RenderBorderedBoxCtx("Preview", mockup, mockupWidth, targetHeight, false, false, tui.GetActiveContext().DialogTitleAlign, "Theme_Title", tui.GetActiveContext())

	return preview
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
