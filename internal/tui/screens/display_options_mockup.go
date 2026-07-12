package screens

import (
	"image/color"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

func (s *DisplayOptionsScreen) renderMockup(targetHeight int) string {
	width := 44 // Reduced width to fit the screen better

	// Resolve the Preview_Border/Preview_Border2 tags based on the staged
	// Border Color setting, so the mockup reflects the same merge every
	// other border consumer gets for free -- without touching the shared
	// theme registry.
	borderOverrides := displayengine.ResolveThemeOverrides(s.config.UI.BorderColor, "Preview_")

	bgStyle := displayengine.SemanticRawStyle("Preview_Screen")
	dContent := displayengine.SemanticRawStyle("Preview_Dialog")
	dBorder1 := borderOverrides["Border"].Style
	dBorder2 := borderOverrides["Border2"].Style

	pMode := displayengine.EffectivePanelMode(s.config, s.connType)
	showStrip := pMode != "none"

	var b lipgloss.Border
	if !s.config.UI.Borders {
		b = lipgloss.HiddenBorder()
	} else if s.config.UI.LineCharacters {
		b = lipgloss.RoundedBorder()
	} else {
		b = displayengine.RoundedAsciiBorder
	}

	// Build StyleContext for the preview
	previewCtx := displayengine.StyleContext{
		LineCharacters:      s.config.UI.LineCharacters,
		DrawBorders:         s.config.UI.Borders,
		LargeButtons:        s.config.UI.LargeButtons,
		LargeTitleBars:      s.config.UI.LargeTitleBars,
		LargeTitleArea:      displayengine.SemanticRawStyle("Preview_LargeTitleArea"),
		Screen:              bgStyle,
		Dialog:              dContent,
		ContentBackground:   dContent,
		DialogTitle:         displayengine.SemanticRawStyle("Preview_Title"),
		DialogTitleHelp:     displayengine.SemanticRawStyle("Preview_TitleHelp"),
		SubmenuTitle:        displayengine.SemanticRawStyle("Preview_TitleSubMenu"),
		SubmenuTitleFocused: displayengine.SemanticRawStyle("Preview_TitleSubMenuFocused"),
		Border:              b,
		BorderColor:         dBorder1.GetForeground(),
		Border2Color:        dBorder2.GetForeground(),
		ButtonActive:        displayengine.SemanticRawStyle("Preview_ButtonActive"),
		ButtonInactive:      displayengine.SemanticRawStyle("Preview_ButtonInactive"),
		ButtonKeyActive:     displayengine.SemanticRawStyle("Preview_ButtonKeyActive"),
		ButtonKeyInactive:   displayengine.SemanticRawStyle("Preview_ButtonKeyInactive"),
		IconFocused:         displayengine.SemanticRawStyle("Preview_IconFocused"),
		IconPressed:         displayengine.SemanticRawStyle("Preview_IconPressed"),
		IconInactive:        displayengine.SemanticRawStyle("Preview_IconInactive"),
		HelpIconInactive:    displayengine.SemanticRawStyle("Preview_HelpIconInactive"),
		RefreshIconInactive: displayengine.SemanticRawStyle("Preview_RefreshIconInactive"),
		ExitIconInactive:    displayengine.SemanticRawStyle("Preview_ExitIconInactive"),
		ItemNormal:          displayengine.SemanticRawStyle("Preview_Item"),
		ItemFocused:         displayengine.SemanticRawStyle("Preview_ItemFocused"),
		TagNormal:           displayengine.SemanticRawStyle("Preview_Tag"),
		TagFocused:          displayengine.SemanticRawStyle("Preview_TagFocused"),
		TagKey:              displayengine.SemanticRawStyle("Preview_TagKey"),
		TagKeyFocused:       displayengine.SemanticRawStyle("Preview_TagKeyFocused"),
		Shadow:              displayengine.SemanticRawStyle("Preview_Shadow"),
		ShadowColor:         getPreviewShadowColor(),
		ShadowLevel:         s.config.UI.ShadowLevel,
		HelpLine:            displayengine.SemanticRawStyle("Preview_Helpline"),
		StatusSuccess:       displayengine.SemanticRawStyle("Preview_TitleNotice"),
		StatusWarn:          displayengine.SemanticRawStyle("Preview_TitleWarn"),
		DialogTitleAlign:    s.config.UI.DialogTitleAlign,
		SubmenuTitleAlign:   s.config.UI.SubmenuTitleAlign,
		PanelTitleAlign:     s.config.UI.PanelTitleAlign,
		Prefix:              "Preview_",
		DrawShadow:          s.config.UI.Shadow,
	}

	paddedLine := func(text string, style lipgloss.Style, fallback string, ctx ...displayengine.StyleContext) string {
		activeCtx := previewCtx
		if len(ctx) > 0 {
			activeCtx = ctx[0]
		}
		rendered := displayengine.RenderThemeTextCtx(text, activeCtx)
		plain := displayengine.GetPlainText(rendered)
		wt := lipgloss.Width(plain)
		if wt < width {
			leftPad := (width - wt) / 2
			rightPad := width - wt - leftPad
			return style.Render(strutil.Repeat(fallback, leftPad) + rendered + strutil.Repeat(fallback, rightPad))
		}
		return style.Render(plain[:width])
	}

	// --- 1. Header (Status Bar) ---
	hStyle := displayengine.SemanticRawStyle("Preview_StatusBar")
	headerCtx := previewCtx
	headerCtx.Dialog = hStyle // Ensure snapping back to StatusBar background

	themeName := s.previewTheme

	// Border style for the status bar frame (falls back to StatusBar if undefined).
	// lipgloss v2 returns NoColor{} (never nil) for unset colors, so use type assertion
	// to detect truly unset properties and fall back to hStyle colors.
	// Background is ALWAYS forced to hStyle so the │ chars blend with the status bar
	// (StatusBarBorder in the theme uses Screen/white bg for the real header's bottom
	// corners, which would create a visible seam here).
	bStyle := displayengine.SemanticRawStyle("Preview_StatusBarBorder")
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
	leftText := " {{|StatusHostname|}}HOST{{[-]}}"
	leftRendered := displayengine.RenderThemeTextCtx(leftText, headerCtx)
	leftW := lipgloss.Width(displayengine.GetPlainText(leftRendered))

	// Center: App Name
	centerText := "{{|StatusName|}}" + displayengine.GetPlainText(themeName) + "{{[-]}}"
	centerRendered := displayengine.RenderThemeTextCtx(centerText, headerCtx)
	centerW := lipgloss.Width(displayengine.GetPlainText(centerRendered))

	// Right: Version
	rightText := "{{|StatusVersion|}}A:[{{[-]}}{{|StatusVersion|}}2.1{{[-]}}{{|StatusVersion|}}]{{[-]}} "
	rightRendered := displayengine.RenderThemeTextCtx(rightText, headerCtx)
	rightW := lipgloss.Width(displayengine.GetPlainText(rightRendered))

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
		line1 += strutil.Repeat(" ", innerWidth-lipgloss.Width(displayengine.GetPlainText(line1)))
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
		" {{|Item|}}Item 2      {{|ListItemUserDefined|}}User Description{{[-]}}",
		"",
		" {{|LineComment|}}### Sample comment{{[-]}}",
		" Var='Default'",
		" {{|ModifiedText|}}Var='Modified'{{[-]}}",
		" {{|EnvReadOnly|}}Var='ReadOnly'{{[-]}}",
	}

	for i, l := range contentLines {
		contentLines[i] = displayengine.RenderThemeTextCtx(l, previewCtx)
	}
	contentStr := strings.Join(contentLines, "\n")

	// Add a button row so large vs flat buttons are visible in the preview.
	// Show a spinner on OK when spinners are enabled so the style is visible.
	okSpec := displayengine.ButtonSpec{Text: "OK", Active: true}
	if console.SpinnerEnabled {
		okSpec.Spinning = true
		okSpec.SpinnerFrame = 0
	}
	buttonRow := displayengine.RenderCenteredButtonsCtx(38, previewCtx,
		okSpec,
		displayengine.ButtonSpec{Text: "Cancel"},
	)
	buttonRow = strings.TrimSuffix(buttonRow, "\n")
	contentStr = strings.TrimSuffix(contentStr, "\n") + "\n" + buttonRow

	titleParts := []string{
		"{{|Title|}}Title{{[-]}}",
		"{{|TitleSuccess|}}S{{[-]}}",
		"{{|TitleWarning|}}W{{[-]}}",
		"{{|TitleError|}}E{{[-]}}",
		"{{|TitleQuestion|}}Q{{[-]}}",
	}
	dTitle := displayengine.RenderThemeTextCtx(strings.Join(titleParts, " "), previewCtx)

	layout := displayengine.GetLayout()
	fixedLines := layout.BorderHeight() + 3 // headerRow + bottomBorderRow + helpRow
	if showStrip {
		fixedLines++ // logStripRow
	}
	// The outer "Preview" wrap (rendered by the caller, using the real/applied
	// context) uses a large title bar under the same rule it always does --
	// title non-empty, not RAW/submenu -- whenever LargeTitleBars is on.
	// RenderBorderedBoxCtx auto-downgrades to a small title bar when the
	// content is too tall to leave room for the extra 2 rows; reserving that
	// overhead here keeps the mockup's title bar large/small in sync with the
	// setting instead of silently falling back to small under a tight budget.
	if displayengine.GetActiveContext().LargeTitleBars {
		fixedLines += displayengine.LargeTitleBarOverhead
	}
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

	dialogBox := displayengine.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, true, false, previewCtx.DialogTitleAlign, "Title", previewCtx, displayengine.TitleBarState{Show: true})
	dialogBox = displayengine.AddShadowCtx(dialogBox, previewCtx)

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
	backdropBlock = displayengine.Overlay(dialogBox, backdropBlock, displayengine.OverlayCenter, displayengine.OverlayCenter, 0, 0)

	// --- 3. Help Line ---
	helpStyle := displayengine.SemanticRawStyle("Preview_Helpline")
	helpCtx := previewCtx
	helpCtx.Dialog = helpStyle // Ensure snapping back to Helpline background

	// Help line is full width (no side borders)
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ", helpCtx)

	// --- 4. Console Toggle Strip ---
	// Both strip and label: ConsoleTitle fg on ConsoleBox bg.
	consoleTitleStyle := displayengine.SemanticRawStyle("Preview_ConsoleTitle")
	consoleBorderStyle := displayengine.SemanticRawStyle("Preview_ConsoleBorder")

	marker := "^"
	titleText := "Console"
	switch pMode {
	case "log":
		titleText = "Log"
	case "system":
		titleText = "System Console"
	}
	label := consoleTitleStyle.Render(" " + marker + " " + titleText + " " + marker + " ")

	var leftT, rightT, borderTop, topLeftC, topRightC string
	if s.config.UI.LineCharacters {
		leftT = "┤"
		rightT = "├"
		borderTop = "─"
		topLeftC = "┌"
		topRightC = "┐"
	} else {
		leftT = "|"
		rightT = "|"
		borderTop = "-"
		topLeftC = "+"
		topRightC = "+"
	}

	labelW := lipgloss.Width(displayengine.GetPlainText(label))
	titleSectionLen := 1 + 1 + labelW + 1 + 1

	// Strip has side borders/corners in the mockup
	innerWidthStrip := width - 2
	var leftPad int
	if s.config.UI.PanelTitleAlign == "left" {
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
	}
	if showStrip {
		mockupParts = append(mockupParts, logStripRow)
	}
	for i, p := range mockupParts {
		mockupParts[i] = strings.TrimRight(p, "\n")
	}
	mockup := lipgloss.JoinVertical(lipgloss.Left, mockupParts...)

	// Wrap in a standard dialog using the staged preview context, not the
	// active/applied one -- only the mockup's own inner content (built above
	// with previewCtx) reflects staged options; the outer "Preview" frame is
	// part of the real Appearance Settings dialog chrome and stays on the
	// currently active theme, same as the rest of that dialog.
	mockupWidth := lipgloss.Width(mockup)
	// Synchronize preview height with settings dialog
	return displayengine.RenderBorderedBoxCtx("Preview", mockup, mockupWidth, targetHeight, false, true, false, displayengine.GetActiveContext().DialogTitleAlign, "Title", displayengine.GetActiveContext())
}

// getPreviewShadowColor extracts the shadow color from the preview theme
// Prefers foreground (for shade chars), falls back to background
func getPreviewShadowColor() color.Color {
	shadowStyle := displayengine.SemanticRawStyle("Preview_Shadow")
	if fg := shadowStyle.GetForeground(); fg != nil {
		return fg
	}
	return shadowStyle.GetBackground()
}
