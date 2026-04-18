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

	bgStyle := tui.SemanticRawStyle("Preview_Screen")
	dContent := tui.SemanticRawStyle("Preview_Dialog")
	dBorder1 := tui.SemanticRawStyle("Preview_Border")
	dBorder2 := tui.SemanticRawStyle("Preview_Border2")

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
		ContentBackground:   dContent,
		DialogTitle:         tui.SemanticRawStyle("Preview_Title"),
		DialogTitleHelp:     tui.SemanticRawStyle("Preview_TitleHelp"),
		SubmenuTitle:        tui.SemanticRawStyle("Preview_TitleSubMenu"),
		SubmenuTitleFocused: tui.SemanticRawStyle("Preview_TitleSubMenuFocused"),
		Border:              b,
		BorderColor:         dBorder1.GetForeground(),
		Border2Color:        dBorder2.GetForeground(),
		ButtonActive:        tui.SemanticRawStyle("Preview_ButtonActive"),
		ButtonInactive:      tui.SemanticRawStyle("Preview_ButtonInactive"),
		ItemNormal:          tui.SemanticRawStyle("Preview_Item"),
		ItemSelected:        tui.SemanticRawStyle("Preview_ItemSelected"),
		TagNormal:           tui.SemanticRawStyle("Preview_Tag"),
		TagSelected:         tui.SemanticRawStyle("Preview_TagSelected"),
		TagKey:              tui.SemanticRawStyle("Preview_TagKey"),
		TagKeySelected:      tui.SemanticRawStyle("Preview_TagKeySelected"),
		Shadow:              tui.SemanticRawStyle("Preview_Shadow"),
		ShadowColor:         getPreviewShadowColor(),
		ShadowLevel:         s.config.UI.ShadowLevel,
		HelpLine:            tui.SemanticRawStyle("Preview_Helpline"),
		StatusSuccess:       tui.SemanticRawStyle("Preview_TitleNotice"),
		StatusWarn:          tui.SemanticRawStyle("Preview_TitleWarn"),
		StatusError:         tui.SemanticRawStyle("Preview_TitleError"),
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
	hStyle := tui.SemanticRawStyle("Preview_StatusBar")
	headerCtx := previewCtx
	headerCtx.Dialog = hStyle // Ensure snapping back to StatusBar background

	themeName := s.previewTheme

	// Border style for the status bar frame (falls back to StatusBar if undefined).
	// lipgloss v2 returns NoColor{} (never nil) for unset colors, so use type assertion
	// to detect truly unset properties and fall back to hStyle colors.
	// Background is ALWAYS forced to hStyle so the │ chars blend with the status bar
	// (StatusBarBorder in the theme uses Screen/white bg for the real header's bottom
	// corners, which would create a visible seam here).
	bStyle := tui.SemanticRawStyle("Preview_StatusBarBorder")
	if _, noFG := bStyle.GetForeground().(lipgloss.NoColor); noFG {
		bStyle = bStyle.Foreground(hStyle.GetForeground())
	}
	bStyle = bStyle.Background(hStyle.GetBackground())

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
	leftText := " {{|Hostname|}}HOST{{[-]}}"
	leftRendered := tui.RenderThemeTextCtx(leftText, headerCtx)
	leftW := lipgloss.Width(tui.GetPlainText(leftRendered))

	// Center: App Name
	centerText := "{{|ApplicationName|}}" + tui.GetPlainText(themeName) + "{{[-]}}"
	centerRendered := tui.RenderThemeTextCtx(centerText, headerCtx)
	centerW := lipgloss.Width(tui.GetPlainText(centerRendered))

	// Right: Version
	rightText := "{{|ApplicationVersion|}}A:[{{[-]}}{{|ApplicationVersion|}}2.1{{[-]}}{{|ApplicationVersion|}}]{{[-]}} "
	rightRendered := tui.RenderThemeTextCtx(rightText, headerCtx)
	rightW := lipgloss.Width(tui.GetPlainText(rightRendered))

	centerX := (innerWidth - centerW) / 2
	if centerX < 0 {
		centerX = 0
	}

	fitsLine1 := leftW+1 <= centerX && centerX+centerW+1+rightW <= innerWidth

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
		" {{|Subtitle|}}A Subtitle Line{{[-]}}",
		"   {{|CommandLine|}}ds2 --theme{{[-]}}",
		"",
		" Heading: {{|HeadingValue|}}Value{{[-]}} {{|HeadingTag|}}[*Tag*]{{[-]}}",
		"",
		"    Caps: {{|KeyCap|}}[up]{{[-]}} {{|KeyCap|}}[down]{{[-]}} {{|KeyCap|}}[left]{{[-]}} {{|KeyCap|}}[right]",
		"",
		" Normal text",
		" {{|Highlight|}}Highlighted text{{[-]}}",
		"",
		// Menu Items Simulation
		" {{|Item|}}Item 1      Item Description{{[-]}}",
		" {{|Item|}}Item 2      {{|ListAppUserDefined|}}User Description{{[-]}}",
		"",
		" {{|LineHeading|}}*** .env ***{{[-]}}",
		" {{|LineComment|}}### Sample comment{{[-]}}",
		" {{|LineVar|}}Var='Default'{{[-]}}",
		" {{|ModifiedText|}}Var='Modified'{{[-]}}",
	}

	for i, l := range contentLines {
		contentLines[i] = tui.RenderThemeTextCtx(l, previewCtx)
	}
	contentStr := strings.Join(contentLines, "\n")

	titleParts := []string{
		"{{|Title|}}Title{{[-]}}",
		"{{|TitleSuccess|}}S{{[-]}}",
		"{{|TitleWarning|}}W{{[-]}}",
		"{{|TitleError|}}E{{[-]}}",
		"{{|TitleQuestion|}}Q{{[-]}}",
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

	dialogBox := tui.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, true, false, previewCtx.DialogTitleAlign, "Title", previewCtx)
	dialogBox = tui.AddShadowCtx(dialogBox, previewCtx)

	// Pad dialogBox to full backdrop width with explicit Screen bg chars so no
	// exposed backdrop positions remain after overlay (avoids ANSI-state artifacts).
	dialogW := lipgloss.Width(dialogBox)
	if dialogW < width {
		padLeft := (width - dialogW) / 2
		padRight := width - dialogW - padLeft
		leftPad := bgStyle.Render(strutil.Repeat(" ", padLeft))
		rightPad := bgStyle.Render(strutil.Repeat(" ", padRight))
		dLines := strings.Split(dialogBox, "\n")
		for i, dl := range dLines {
			dLines[i] = leftPad + dl + rightPad
		}
		dialogBox = strings.Join(dLines, "\n")
	}
	backdropBlock = tui.Overlay(dialogBox, backdropBlock, tui.OverlayCenter, tui.OverlayCenter, 0, 0)

	// --- 3. Help Line ---
	helpStyle := tui.SemanticRawStyle("Preview_Helpline")
	helpCtx := previewCtx
	helpCtx.Dialog = helpStyle // Ensure snapping back to Helpline background

	// Help line is full width (no side borders)
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ", helpCtx)

	// --- 4. Console Toggle Strip ---
	// Both strip and label: ConsoleTitle fg on ConsoleBox bg.
	consoleTitleStyle := tui.SemanticRawStyle("Preview_ConsoleTitle")
	consoleBorderStyle := tui.SemanticRawStyle("Preview_ConsoleBorder")

	marker := "^"
	label := consoleTitleStyle.Render(" " + marker + " Console " + marker + " ")

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
	logStripRow := consoleBorderStyle.Render(topLeftC+leftDashes+leftT+" ") +
		label +
		consoleBorderStyle.Render(" "+rightT+rightDashes+topRightC)

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
	return tui.RenderBorderedBoxCtx("Preview", mockup, mockupWidth, targetHeight, false, true, false, tui.GetActiveContext().DialogTitleAlign, "Title", tui.GetActiveContext())
}

// getPreviewShadowColor extracts the shadow color from the preview theme
// Prefers foreground (for shade chars), falls back to background
func getPreviewShadowColor() color.Color {
	shadowStyle := tui.SemanticRawStyle("Preview_Shadow")
	if fg := shadowStyle.GetForeground(); fg != nil {
		return fg
	}
	return shadowStyle.GetBackground()
}
