package screens

import (
	"image/color"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// renderMockup renders the fake preview dialog at targetHeight (matching the
// settings dialog) but never taller than maxHeight -- the real available
// content-area height, which can shrink out from under a screen's own
// natural content need (e.g. the log console panel expanding eats rows out
// of the terminal that were never reserved for this screen at all).
func (s *DisplayOptionsScreen) renderMockup(targetHeight, maxHeight int) string {
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

	// --- 5. Assemble as real Content sections instead of hand-computed
	// height arithmetic ---
	//
	// Each piece becomes a borderless, non-focusable leaf section (matching
	// the NewPlainTextSection convention: SubMenuMode+Borderless+
	// NonFocusable+no buttons+Maximized) stacked inside one outer sectioned
	// MenuModel. The outer MenuModel's own calculateSectionLayout/
	// viewWithSections machinery -- the exact same code path outerMenu (the
	// real Appearance Settings dialog) uses -- then owns the height/large-
	// title-bar/border decisions, so this mockup's title bar and total
	// height can no longer silently drift out of sync with the real dialog
	// the way the old hand-computed fixedLines/backdropHeight arithmetic did.
	layout := displayengine.GetLayout()

	newLeafSection := func(id string) *displayengine.MenuModel {
		leaf := displayengine.NewMenuModel(id, "", "", nil)
		leaf.SetSubMenuMode(true)
		leaf.SetBorderless(true)
		leaf.SetNonFocusable(true)
		leaf.SetButtons(nil)
		leaf.SetMaximized(true)
		return leaf
	}

	// Header: status bar + separator, stacked as one borderless block.
	// Height is measured directly from the rendered content instead of
	// re-deriving the 1/2/3-line decision separately -- the one true source
	// of truth is what actually got rendered above.
	headerBlock := headerRow + "\n" + bottomBorderRow
	headerHeight := lipgloss.Height(headerBlock)
	headerSection := newLeafSection("appearance_preview_header")
	headerSection.ContentRenderer = func(_ int) string { return headerBlock }
	headerSection.SectionHeightOverride = func(_ int) int { return headerHeight }

	// Backdrop + fake dialog: the sole expandable section, absorbing
	// whatever height is left over -- same role themeMenu plays in the real
	// settings dialog. Reads its own assigned Height() at render time
	// (available once calculateSectionLayout has already sized it) instead
	// of a separately hand-computed backdropHeight.
	backdropSection := newLeafSection("appearance_preview_backdrop")
	backdropSection.SetVariableHeight(true)
	// Its natural need is whatever the fake dialog itself actually requires
	// (e.g. 2 taller when staged Large Title Bars makes it switch to a large
	// title) -- feeding this into the natural-height measurement pass below
	// is what lets the whole mockup grow when the inner dialog needs more
	// room than the settings dialog's own height provides.
	backdropNaturalHeight := lipgloss.Height(dialogBox)
	if backdropNaturalHeight < 10 {
		backdropNaturalHeight = 10
	}
	backdropSection.SectionHeightOverride = func(_ int) int { return backdropNaturalHeight }
	backdropSection.ContentRenderer = func(_ int) string {
		h := backdropSection.Height()
		if h < 10 {
			h = 10
		}
		backdropLines := make([]string, h)
		filler := bgStyle.Render(strutil.Repeat(" ", width))
		for i := range backdropLines {
			backdropLines[i] = filler
		}
		backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)
		return displayengine.Overlay(dialogBox, backdropBlock, displayengine.OverlayCenter, displayengine.OverlayCenter, 0, 0)
	}

	// Help line.
	helpHeight := lipgloss.Height(helpRow)
	helpSection := newLeafSection("appearance_preview_help")
	helpSection.ContentRenderer = func(_ int) string { return helpRow }
	helpSection.SectionHeightOverride = func(_ int) int { return helpHeight }

	mockupMenu := displayengine.NewMenuModel("appearance_preview_mockup", "Preview", "", nil)
	mockupMenu.SetButtons(nil)
	mockupMenu.AddContentSection(headerSection)
	mockupMenu.AddContentSection(backdropSection)
	mockupMenu.AddContentSection(helpSection)
	if showStrip {
		stripHeight := lipgloss.Height(logStripRow)
		stripSection := newLeafSection("appearance_preview_strip")
		stripSection.ContentRenderer = func(_ int) string { return logStripRow }
		stripSection.SectionHeightOverride = func(_ int) int { return stripHeight }
		mockupMenu.AddContentSection(stripSection)
	}

	// Size it so each section's own inset content width comes out to the
	// same fixed `width` (44) every render above has always used --
	// BorderWidth + ContentMarginWidth is exactly what calculateSectionLayout
	// subtracts before assigning section width.
	mockupTotalWidth := layout.BorderWidth() + layout.ContentMarginWidth() + width

	// Measure the mockup's own natural required height first (reusing
	// calculateSectionLayout's existing shrink-to-natural-height logic via
	// !maximized, rather than re-deriving the border/large-title-bar/section
	// overhead formula by hand): size it generously large, read back the
	// height it actually settled on, then lock it at max(targetHeight,
	// naturalHeight) so it matches the settings dialog when there's room to,
	// but grows past that when its own content (e.g. the fake dialog
	// switching to a large title bar) genuinely needs more.
	mockupMenu.SetMaximized(false)
	mockupMenu.SetSize(mockupTotalWidth, 200)
	naturalHeight := mockupMenu.Height()
	effectiveHeight := targetHeight
	if naturalHeight > effectiveHeight {
		effectiveHeight = naturalHeight
	}
	// Never claim more than what's actually available -- growing to fit our
	// own content is only correct up to the real ceiling; past that we must
	// shrink back down like every other dialog does; the backdrop section
	// (the sole expandable one) absorbs the squeeze first.
	if maxHeight > 0 && effectiveHeight > maxHeight {
		effectiveHeight = maxHeight
	}
	mockupMenu.SetMaximized(true)
	mockupMenu.SetSize(mockupTotalWidth, effectiveHeight)

	// The outer "Preview" frame renders through viewWithSections, which
	// (like every other dialog) reads the real/applied theme via
	// GetActiveContext() internally for its border/title/large-title-bar
	// decision -- only the sections' own ContentRenderer closures above use
	// previewCtx (staged options) for their internal painting. This is the
	// same staged-vs-applied split the manual RenderBorderedBoxCtx call used
	// to enforce by hand, now structural instead of a call-site convention.
	return mockupMenu.ViewString()
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
