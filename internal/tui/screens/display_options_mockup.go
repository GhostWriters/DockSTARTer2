package screens

import (
	"image/color"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// previewContentWidth is the fixed inner content width every piece of the
// mockup below assumes.
const previewContentWidth = 44

// previewContent holds the preview mockup's rendered pieces for the screen's
// current previewTheme/config. Returned by computePreviewContent, which is
// called fresh on every render (from the persistent preview section's
// ContentRenderer/SectionHeightOverride closures in buildPreviewSection) --
// nothing here is cached across renders.
type previewContent struct {
	invalid       bool // true if the staged theme failed to load; only invalidLabel is meaningful then
	invalidLabel  string
	headerBlock   string
	buildBackdrop func(h int) string
	helpRow       string
	logStripRow   string
	showStrip     bool
	fixedOverhead int
	naturalHeight int
}

// computePreviewContent renders the fake preview dialog's pieces from the
// screen's current previewTheme/config. Kept separate from buildPreviewSection
// so the persistent preview section's closures can call it fresh every
// render instead of the section itself needing to be reconstructed to stay
// theme-live -- rebuilding the whole MenuModel would also discard its
// interaction state (focus, in particular), which content changes have no
// business affecting.
func (s *DisplayOptionsScreen) computePreviewContent() previewContent {
	for _, t := range s.themes {
		if t.ConfigValue == s.previewTheme && t.IsInvalid {
			return previewContent{invalid: true, invalidLabel: "Invalid theme"}
		}
	}

	width := previewContentWidth

	// Resolve the Preview_Border/Preview_Border2 tags based on the staged
	// Border Color setting, so the mockup reflects the same merge every
	// other border consumer gets for free -- without touching the shared
	// theme registry.
	borderOverrides := displayengine.ResolveThemeOverrides(s.config.UI.BorderColor, "Preview_")

	bgStyle := displayengine.SemanticRawStyleWithPrefix("Screen", "Preview_")
	dContent := displayengine.SemanticRawStyleWithPrefix("Dialog", "Preview_")
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
		LargeTitleArea:      displayengine.SemanticRawStyleWithPrefix("LargeTitleArea", "Preview_"),
		Screen:              bgStyle,
		Dialog:              dContent,
		ContentBackground:   dContent,
		DialogTitle:         displayengine.SemanticRawStyleWithPrefix("Title", "Preview_"),
		DialogTitleHelp:     displayengine.SemanticRawStyleWithPrefix("TitleHelp", "Preview_"),
		SubmenuTitle:        displayengine.SemanticRawStyleWithPrefix("TitleSubMenu", "Preview_"),
		SubmenuTitleFocused: displayengine.SemanticRawStyleWithPrefix("TitleSubMenuFocused", "Preview_"),
		Border:              b,
		BorderColor:         dBorder1.GetForeground(),
		Border2Color:        dBorder2.GetForeground(),
		ButtonActive:        displayengine.SemanticRawStyleWithPrefix("ButtonActive", "Preview_"),
		ButtonInactive:      displayengine.SemanticRawStyleWithPrefix("ButtonInactive", "Preview_"),
		ButtonKeyActive:     displayengine.SemanticRawStyleWithPrefix("ButtonKeyActive", "Preview_"),
		ButtonKeyInactive:   displayengine.SemanticRawStyleWithPrefix("ButtonKeyInactive", "Preview_"),
		IconFocused:         displayengine.SemanticRawStyleWithPrefix("IconFocused", "Preview_"),
		IconPressed:         displayengine.SemanticRawStyleWithPrefix("IconPressed", "Preview_"),
		IconInactive:        displayengine.SemanticRawStyleWithPrefix("IconInactive", "Preview_"),
		IconHelpInactive:    displayengine.SemanticRawStyleWithPrefix("IconHelpInactive", "Preview_"),
		IconRefreshInactive: displayengine.SemanticRawStyleWithPrefix("IconRefreshInactive", "Preview_"),
		IconExitInactive:    displayengine.SemanticRawStyleWithPrefix("IconExitInactive", "Preview_"),
		ItemNormal:          displayengine.SemanticRawStyleWithPrefix("Item", "Preview_"),
		ItemFocused:         displayengine.SemanticRawStyleWithPrefix("ItemFocused", "Preview_"),
		TagNormal:           displayengine.SemanticRawStyleWithPrefix("Tag", "Preview_"),
		TagFocused:          displayengine.SemanticRawStyleWithPrefix("TagFocused", "Preview_"),
		TagKey:              displayengine.SemanticRawStyleWithPrefix("TagKey", "Preview_"),
		TagKeyFocused:       displayengine.SemanticRawStyleWithPrefix("TagKeyFocused", "Preview_"),
		Shadow:              displayengine.SemanticRawStyleWithPrefix("Shadow", "Preview_"),
		ShadowColor:         getPreviewShadowColor(),
		ShadowLevel:         s.config.UI.ShadowLevel,
		HelpLine:            displayengine.SemanticRawStyleWithPrefix("Helpline", "Preview_"),
		StatusSuccess:       displayengine.SemanticRawStyleWithPrefix("TitleNotice", "Preview_"),
		StatusWarn:          displayengine.SemanticRawStyleWithPrefix("TitleWarn", "Preview_"),
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
	hStyle := displayengine.SemanticRawStyleWithPrefix("StatusBar", "Preview_")
	headerCtx := previewCtx
	headerCtx.Dialog = hStyle // Ensure snapping back to StatusBar background

	themeName := s.previewTheme

	// Border style for the status bar frame (falls back to StatusBar if undefined).
	// lipgloss v2 returns NoColor{} (never nil) for unset colors, so use type assertion
	// to detect truly unset properties and fall back to hStyle colors.
	// Background is ALWAYS forced to hStyle so the │ chars blend with the status bar
	// (StatusBarBorder in the theme uses Screen/white bg for the real header's bottom
	// corners, which would create a visible seam here).
	bStyle := displayengine.SemanticRawStyleWithPrefix("StatusBarBorder", "Preview_")
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
		" {{|Item|}}Item 2      {{|ItemListUserDefined|}}User Description{{[-]}}",
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
	helpStyle := displayengine.SemanticRawStyleWithPrefix("Helpline", "Preview_")
	helpCtx := previewCtx
	helpCtx.Dialog = helpStyle // Ensure snapping back to Helpline background

	// Help line is full width (no side borders)
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ", helpCtx)

	// --- 4. Console Toggle Strip ---
	// Both strip and label: PanelTitle fg on ConsoleBox bg.
	panelTitleStyle := displayengine.SemanticRawStyleWithPrefix("PanelTitle", "Preview_")
	panelBorderStyle := displayengine.SemanticRawStyleWithPrefix("PanelBorder", "Preview_")

	marker := "^"
	titleText := "Console"
	switch pMode {
	case "log":
		titleText = "Log"
	case "system":
		titleText = "System Console"
	}
	label := panelTitleStyle.Render(" " + marker + " " + titleText + " " + marker + " ")

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
	logStripRow := panelBorderStyle.Render(topLeftC+leftDashes+leftT+" ") +
		label +
		panelBorderStyle.Render(" "+rightT+rightDashes+topRightC)

	// Backdrop + fake dialog: built at buildBackdrop(h) call time (inside
	// the ContentRenderer closure below), not baked in here -- it stretches
	// to fill any extra height the submenu is given beyond what's needed
	// (centering the fake dialog within the extra room), but never shrinks
	// below backdropNaturalHeight, so scrolling (not squeezing) is what
	// handles genuinely cramped space.
	backdropNaturalHeight := lipgloss.Height(dialogBox)
	if backdropNaturalHeight < 10 {
		backdropNaturalHeight = 10
	}
	buildBackdrop := func(h int) string {
		if h < backdropNaturalHeight {
			h = backdropNaturalHeight
		}
		backdropLines := make([]string, h)
		filler := bgStyle.Render(strutil.Repeat(" ", width))
		for i := range backdropLines {
			backdropLines[i] = filler
		}
		backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)
		return displayengine.Overlay(dialogBox, backdropBlock, displayengine.OverlayCenter, displayengine.OverlayCenter, 0, 0)
	}

	headerBlock := headerRow + "\n" + bottomBorderRow
	headerHeight := lipgloss.Height(headerBlock)
	helpHeight := lipgloss.Height(helpRow)
	stripHeight := 0
	if showStrip {
		stripHeight = lipgloss.Height(logStripRow)
	}
	fixedOverhead := headerHeight + helpHeight + stripHeight
	naturalHeight := fixedOverhead + backdropNaturalHeight

	return previewContent{
		headerBlock:   headerBlock,
		buildBackdrop: buildBackdrop,
		helpRow:       helpRow,
		logStripRow:   logStripRow,
		showStrip:     showStrip,
		fixedOverhead: fixedOverhead,
		naturalHeight: naturalHeight,
	}
}

// buildPreviewSection builds the fake preview dialog as a submenu-mode
// Content section (title "Preview"), for nesting inside the outer Appearance
// Settings dialog's own section tree instead of rendering as an independent
// top-level dialog. Built once, like the settings sections, and reused for
// the screen's lifetime: its ContentRenderer/SectionHeightOverride closures
// call computePreviewContent fresh every render instead of this MenuModel
// being torn down and rebuilt to stay theme-live -- reconstructing it would
// also discard its interaction state (focus in particular), which a content
// change has no business affecting.
func (s *DisplayOptionsScreen) buildPreviewSection() *displayengine.MenuModel {
	// ID matches mockupMenu's own ID (and previewScroll.ID below) rather than
	// a distinct "_scroll" suffix -- MatchesID is substring-based, and both
	// appearanceLayoutRow's outer redirect and mockupMenu's own
	// updateSections section-routing need the scrollbar's hit-region IDs
	// (built from previewScroll.ID) to match, or a thumb-drag click never
	// reaches this section at all.
	scrollSection := displayengine.NewMenuModel("appearance_preview_mockup", "", "", nil)
	scrollSection.SetSubMenuMode(true)
	scrollSection.SetBorderless(true)
	scrollSection.SetButtons(nil)
	scrollSection.SetMaximized(true)
	scrollSection.SetVariableHeight(true)
	scrollSection.SetWantsAllMessages(true)
	scrollSection.SectionHeightOverride = func(_ int) int {
		pc := s.computePreviewContent()
		if pc.invalid {
			return 3
		}
		return pc.naturalHeight
	}
	scrollSection.ContentRenderer = func(contentWidth int) string {
		pc := s.computePreviewContent()
		h := scrollSection.Height()
		if h < 1 {
			h = 1
		}
		if pc.invalid {
			leftPad := (previewContentWidth - len(pc.invalidLabel)) / 2
			rightPad := previewContentWidth - len(pc.invalidLabel) - leftPad
			centeredLine := strutil.Repeat(" ", leftPad) + pc.invalidLabel + strutil.Repeat(" ", rightPad)
			lines := make([]string, h)
			for i := range lines {
				lines[i] = strutil.Repeat(" ", previewContentWidth)
			}
			lines[(h-1)/2] = centeredLine
			return strings.Join(lines, "\n")
		}
		parts := []string{pc.headerBlock, pc.buildBackdrop(h - pc.fixedOverhead), pc.helpRow}
		if pc.showStrip {
			parts = append(parts, pc.logStripRow)
		}
		content := lipgloss.JoinVertical(lipgloss.Left, parts...)
		ctx := displayengine.GetActiveContext()
		s.previewViewport.SetWidth(contentWidth)
		s.previewViewport.SetHeight(h)
		s.previewViewport.SetContent(content)
		viewportOutput := s.previewViewport.View()
		// Pad any shortfall ourselves rather than relying on
		// viewport.FillHeight -- that pads with truly empty (unstyled)
		// rows, which shows as a mismatched-background gap instead of a
		// themed one. This also covers the near-bottom-of-scroll case,
		// where the remaining slice of content is shorter than h even
		// though the total content isn't.
		if short := h - lipgloss.Height(viewportOutput); short > 0 {
			bgStyle := displayengine.SemanticRawStyleWithPrefix("Screen", "Preview_")
			filler := bgStyle.Render(strutil.Repeat(" ", previewContentWidth))
			fillLines := make([]string, short)
			for i := range fillLines {
				fillLines[i] = filler
			}
			viewportOutput = viewportOutput + "\n" + strings.Join(fillLines, "\n")
		}
		return displayengine.ApplyScrollbar(&s.previewScroll, viewportOutput,
			s.previewViewport.TotalLineCount(), s.previewViewport.VisibleLineCount(),
			s.previewViewport.YOffset(), ctx.LineCharacters, ctx)
	}
	scrollSection.ExtraHitRegions = func(offsetX, offsetY, baseZ int) []displayengine.HitRegion {
		if !s.previewScroll.Info.Needed {
			return nil
		}
		sbX := offsetX + s.previewViewport.Width()
		return s.previewScroll.HitRegions(sbX, offsetY, baseZ+20, "Preview") // no left border to offset past (SetBorderless)
	}
	scrollSection.SetUpdateInterceptor(func(msg tea.Msg, _ *displayengine.MenuModel) (tea.Cmd, bool) {
		if newOff, cmd, changed := s.previewScroll.Update(msg, s.previewViewport.YOffset(), s.previewViewport.TotalLineCount(), s.previewViewport.VisibleLineCount()); changed {
			s.previewViewport.SetYOffset(newOff)
			return cmd, true
		}
		switch m := msg.(type) {
		case tea.KeyPressMsg:
			switch m.String() {
			case "up":
				s.previewViewport.ScrollUp(1)
				return nil, true
			case "down":
				s.previewViewport.ScrollDown(1)
				return nil, true
			case "pgup":
				s.previewViewport.PageUp()
				return nil, true
			case "pgdown":
				s.previewViewport.PageDown()
				return nil, true
			case "home":
				s.previewViewport.SetYOffset(0)
				return nil, true
			case "end":
				s.previewViewport.SetYOffset(s.previewViewport.TotalLineCount())
				return nil, true
			}
		case displayengine.LayerHitMsg, displayengine.LayerWheelMsg,
			tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			return nil, true
		}
		return nil, false
	})

	mockupMenu := displayengine.NewMenuModel("appearance_preview_mockup", "Preview", "", nil)
	mockupMenu.SetButtons(nil)
	mockupMenu.AddContentSection(scrollSection)

	// Rendered as a nested submenu section (title "Preview") instead of a
	// top-level dialog -- the caller (the row pairing this with the settings
	// column) owns sizing, same as any other Content.
	mockupMenu.SetSubMenuMode(true)
	mockupMenu.SetIsDialog(false)
	mockupMenu.SetMaximized(true)

	// A Close widget on the preview's own border lets the user hide it --
	// dispatches appearancePreviewHideMsg, intercepted directly by
	// appearanceLayoutRow.Update (this MenuModel has no idea the row can
	// hide it). Opt-in since submenus don't render widgets by default.
	mockupMenu.SetSubmenuWidgetsEnabled(true)
	closeWidget := displayengine.WidgetClose
	closeWidget.HelpText = "Hide the preview panel."
	closeWidget.Action = func() tea.Cmd {
		return func() tea.Msg { return appearancePreviewHideMsg{} }
	}
	mockupMenu.ConfigureWidgets(closeWidget)
	return mockupMenu
}

// previewSectionWidth returns the outer width to assign the preview section
// via SetSize so its inner content area works out to the fixed 44-column
// mockup width every leaf section's ContentRenderer closure above assumes.
func previewSectionWidth() int {
	layout := displayengine.GetLayout()
	return layout.BorderWidth() + layout.ContentMarginWidth() + previewContentWidth
}

// previewNaturalHeight measures mockupMenu's natural (unmaximized) height at
// previewSectionWidth() -- mirrors calculateSectionLayout's shrink-to-natural
// pass for expandable sections in the real settings dialog, since a nested
// sectioned MenuModel isn't handled by the generic Sizer path (SectionHeight
// has no case for "a MenuModel with its own content sections"). Leaves
// mockupMenu maximized afterward; callers must still SetSize it to the real
// target height before rendering.
func previewNaturalHeight(mockupMenu *displayengine.MenuModel) int {
	mockupMenu.SetMaximized(false)
	mockupMenu.SetSize(previewSectionWidth(), 200)
	h := mockupMenu.Height()
	mockupMenu.SetMaximized(true)
	return h
}

// getPreviewShadowColor extracts the shadow color from the preview theme
// Prefers foreground (for shade chars), falls back to background
func getPreviewShadowColor() color.Color {
	shadowStyle := displayengine.SemanticRawStyleWithPrefix("Shadow", "Preview_")
	if fg := shadowStyle.GetForeground(); fg != (lipgloss.NoColor{}) {
		return fg
	}
	return shadowStyle.GetBackground()
}
