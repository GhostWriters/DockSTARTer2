package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *TabbedVarsEditorModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	if len(m.tabs) == 0 {
		return "No tabs loaded"
	}

	ctx := displayengine.GetActiveContext()

	var body string
	if m.splitMode {
		pane0 := m.renderPane(0, m.activeTab == 0)
		pane1 := m.renderPane(1, m.activeTab == 1)
		if m.layoutMode == envLayoutSideBySide {
			gutter := m.gutterFill(ctx, true, lipgloss.Height(pane0))
			body = lipgloss.JoinHorizontal(lipgloss.Top, pane0, gutter, pane1)
		} else {
			gutter := m.gutterFill(ctx, false, m.fullContentWidth)
			body = lipgloss.JoinVertical(lipgloss.Left, pane0, gutter, pane1)
		}
	} else {
		body = m.renderPane(m.activeTab, m.focus == envFocusEditor)
	}

	// Render buttons (shared row below both panes, spans the full width)
	buttons := m.renderButtons(m.fullContentWidth)

	// One shared heading above both panes instead of each pane repeating its
	// own (redundant when, as usual, both tabs belong to the same app).
	parts := []string{}
	if subtitle := m.renderSubtitleForTab(&m.tabs[m.activeTab], m.fullContentWidth, m.subtitleHeight); subtitle != "" {
		parts = append(parts, strings.TrimRight(subtitle, "\n"))
	}
	parts = append(parts, strings.TrimRight(body, "\n"), strings.TrimRight(buttons, "\n"))
	innerContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Apply 1-char side margin so inner components are inset from the outer border (matching menu dialogs).
	layout := displayengine.GetLayout()
	outerContentWidth := m.fullContentWidth + layout.ContentMarginWidth()
	fullContent := lipgloss.NewStyle().
		Background(ctx.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin).
		Render(innerContent)

	// Wrap in the outer dialog border
	// outerContentWidth = m.fullContentWidth + margin = m.width - BorderWidth, so total = m.width.
	return displayengine.RenderBorderedBoxCtx(
		m.title,
		fullContent,
		outerContentWidth,
		m.height,
		m.focused,
		true, // Show indicators in the main title
		false,
		ctx.DialogTitleAlign,
		"Title",
		ctx,
		func() displayengine.TitleBarState {
			tbs := m.State()
			tbs.SpinnerIndicator, tbs.SpinnerIndicatorRight = m.currentSpinnerIndicators()
			return tbs
		}(),
	)
}

// gutterFill renders the split gutter's content: the normal blank strip, or
// (while an active drag or keyboard resize mode is in progress) a line with
// an arrow at each end, styled with the ResizeLine tag (falls back to
// ItemFocused's reverse-video block in themes that don't set it), so
// "actively resizing" reads differently from the idle divider. Unicode
// double-arrows
// (↔/↕) were considered and dropped: both are East-Asian-Width "Ambiguous",
// so a terminal can legitimately render them as 1 or 2 columns, which would
// misalign the gutter's exact column math. Instead this reuses the small-
// triangle glyphs (▸/◂/▾, Geometric Shapes block -- unambiguous width)
// already used elsewhere for expand/focus indicators (dialog_border_box.go,
// dialog_render.go), gated behind ctx.LineCharacters like every other
// Unicode/ASCII glyph pair in this codebase, with a plain-ASCII fallback.
// vertical is true for the side-by-side layout's column gutter (length =
// its height in rows -- only 1 column wide, so the line runs top-to-bottom
// with a ▴/▲ or ▾/▼-style cap at each end); false for the stacked layout's
// row gutter (length = its width in columns, line runs left-to-right with
// ◂/▸ caps).
func (m *TabbedVarsEditorModel) gutterFill(ctx displayengine.StyleContext, vertical bool, length int) string {
	if !m.gutterDrag.Dragging && !m.resizingGutter {
		bgStyle := lipgloss.NewStyle().Background(ctx.Dialog.GetBackground())
		if vertical {
			return bgStyle.Width(1).Height(length).Render("")
		}
		return bgStyle.Width(length).Height(1).Render("")
	}
	var start, end, fill string
	if vertical {
		if ctx.LineCharacters {
			start, end, fill = "▴", "▾", "│"
		} else {
			start, end, fill = "^", "v", "|"
		}
		// Styled per row, then joined by "\n": the ANSI codes RenderThemeText
		// emits only open/close once per call, but lipgloss.JoinHorizontal
		// (ViewString) splits multi-line content by "\n" before rejoining it
		// with the panes -- a single style wrapped around the whole block
		// would leave every row but the first with no active SGR code.
		rows := endCappedLine(length, start, end, fill)
		for i, r := range rows {
			rows[i] = displayengine.RenderThemeText("{{|ResizeLine|}}"+r+"{{[-]}}", ctx.Dialog)
		}
		return strings.Join(rows, "\n")
	}
	if ctx.LineCharacters {
		start, end, fill = "◂", "▸", "─"
	} else {
		start, end, fill = "<", ">", "-"
	}
	content := strings.Join(endCappedLine(length, start, end, fill), "")
	return displayengine.RenderThemeText("{{|ResizeLine|}}"+content+"{{[-]}}", ctx.Dialog)
}

// endCappedLine returns length characters: start at the first, end at the
// last, fill in between. Falls back to a single repeated fill character
// when length is too short to fit both end caps.
func endCappedLine(length int, start, end, fill string) []string {
	if length <= 0 {
		return nil
	}
	if length == 1 {
		return []string{fill}
	}
	line := make([]string, length)
	line[0] = start
	line[length-1] = end
	for i := 1; i < length-1; i++ {
		line[i] = fill
	}
	return line
}

// renderPane renders one tab's tab-row/title + bordered editor box +
// INS/OVR bottom label, at pane idx's own paneContentWidth x
// paneEditorHeight (see SetSize). The shared subtitle
// heading is rendered in ViewString, not here. In split mode there's no
// multi-tab strip within a pane -- just this pane's own title segment;
// clicking the OTHER pane's background switches which tab is active.
func (m *TabbedVarsEditorModel) renderPane(idx int, focused bool) string {
	tab := m.tabs[idx]
	editor := tab.editor
	editorView := editor.View()
	ctx := displayengine.GetActiveContext()

	var tabRow string
	if m.splitMode {
		styleTag := "TitleSubMenu"
		if focused {
			styleTag = "TitleSubMenuFocused"
		}
		tabRow = displayengine.RenderTitleSegmentCtx(tab.spec.Title, focused, focused, true, styleTag, ctx)
	} else {
		tabRow = m.renderTabs()
	}

	// Each pane gets its own layout-control widgets on its border, via the
	// same TitleBarState/rightWidget mechanism the outer dialog's [?]/[x]
	// use (see paneLayoutWidgets). Keyboard focus only ever applies to the
	// active pane.
	widgets := m.paneLayoutWidgets()
	paneTBS := displayengine.TitleBarState{Show: len(widgets) > 0, Widgets: widgets}
	if focused && m.paneTitleFocused {
		paneTBS.Focused = true
		paneTBS.ActiveWidget = m.paneActiveWidget
	}

	innerBox := displayengine.RenderBorderedBoxCtx(
		tabRow,
		editorView,
		m.paneContentWidth[idx]-2,
		m.paneEditorHeight[idx]+2,
		focused,
		false, // No focus indicators here
		true,  // Rounded corners to match submenu style
		ctx.SubmenuTitleAlign,
		"RAW", // Use the pre-rendered tabRow exactly
		ctx,
		paneTBS,
	)

	// Replace the bottom border with INS/OVR label (left) and scroll % (right, if scrolling).
	modeLabel := "INS"
	if editor.IsOverwrite() {
		modeLabel = "OVR"
	}
	scrollLabel := ""
	if editor.TotalDisplayLines() > editor.Height() {
		scrollLabel = fmt.Sprintf("%d%%", int(editor.ScrollPercent()*100))
	}
	lines := strings.Split(innerBox, "\n")
	if len(lines) > 0 {
		lines[len(lines)-1] = displayengine.BuildDualLabelBottomBorderCtx(m.paneContentWidth[idx], modeLabel, scrollLabel, focused, ctx)
		innerBox = strings.Join(lines, "\n")
	}

	return strings.TrimRight(innerBox, "\n")
}

func (m *TabbedVarsEditorModel) View() tea.View {
	v := tea.View{Content: m.ViewString()}

	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		c := m.tabs[m.activeTab].editor.Cursor()
		if c != nil {
			layout := displayengine.GetLayout()
			c.X += m.lastOffsetX + layout.NestedLeftOffset()
			c.Y += m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight
			v.Cursor = c
		}
	}

	return v
}

func (m *TabbedVarsEditorModel) getButtonSpecs() []displayengine.ButtonSpec {
	zoneByName := map[string]string{
		"Save":    displayengine.IDSaveButton,
		"Refresh": displayengine.IDRefreshButton,
		"Cancel":  displayengine.IDBackButton,
		"Exit":    displayengine.IDExitButton,
	}
	helpByName := map[string]string{
		"Save":    "Save all changes in all tabs to the environment file.",
		"Refresh": "Reformat and re-stage all tabs (same as pressing F5).",
		"Cancel":  "Discard all changes and return (prompts if unsaved changes exist).",
		"Exit":    "Discard all changes and exit the application.",
	}
	var specs []displayengine.ButtonSpec
	for i, btn := range m.buttons {
		zoneID := zoneByName[btn]
		specs = append(specs, displayengine.ButtonSpec{
			Text:   envButtonLabel(btn),
			Active: (m.focus == envFocusButtons && m.btnIdx == i) || m.btnRow.IsProcessingID(zoneID),
			ZoneID: zoneID,
			Help:   helpByName[btn],
		})
	}
	return specs
}

func (m *TabbedVarsEditorModel) renderButtons(width int) string {
	specs := m.btnRow.ApplySpinner(m.getButtonSpecs())
	return displayengine.RenderCenteredButtonsExplicit(width, m.buttonHeight == displayengine.DialogButtonHeight, displayengine.GetActiveContext(), specs...)
}

func (m *TabbedVarsEditorModel) renderTabs() string {
	ctx := displayengine.GetActiveContext()
	editorFocused := m.focus == envFocusEditor
	var tabSegments []string
	for i, tab := range m.tabs {
		title := tab.spec.Title
		isActive := i == m.activeTab
		styleTag := "TitleSubMenu"
		if isActive {
			styleTag = "TitleSubMenuFocused"
		}
		// Pass editorFocused as borderFocused so the tab bar border dims when
		// buttons have focus, but always mark the active tab as contentFocused
		// so it remains visually distinguished regardless of which panel is active.
		seg := displayengine.RenderTitleSegmentCtx(title, editorFocused, isActive, true, styleTag, ctx)
		tabSegments = append(tabSegments, seg)
	}
	return strings.Join(tabSegments, "")
}

func (m *TabbedVarsEditorModel) ShortHelp() []key.Binding {
	if m.focus == envFocusEditor {
		b := []key.Binding{displayengine.Keys.EnvRefresh, displayengine.Keys.EnvAddVar, displayengine.Keys.EnvDelete, displayengine.Keys.Esc, displayengine.Keys.Help}
		if len(m.tabs) > 1 {
			b = append(b, displayengine.Keys.EnvNextTab)
		}
		return b
	}
	return []key.Binding{displayengine.Keys.Left, displayengine.Keys.Right, displayengine.Keys.Enter, displayengine.Keys.CycleTab, displayengine.Keys.Esc}
}

func (m *TabbedVarsEditorModel) FullHelp() [][]key.Binding {
	editorActions := []key.Binding{
		displayengine.Keys.EnvRefresh,
		displayengine.Keys.EnvAddVar,
		displayengine.Keys.EnvInsert,
		displayengine.Keys.EnvSplitLine,
		displayengine.Keys.EnvDelete,
		key.NewBinding(key.WithKeys("ctrl+up"), key.WithHelp("alt+↑/↓", "reorder row")),
		displayengine.Keys.EnvEditValue,
	}
	if len(m.tabs) > 1 {
		editorActions = append(editorActions, displayengine.Keys.EnvNextTab, displayengine.Keys.EnvPrevTab)
	}
	if len(m.tabs) == 2 {
		editorActions = append(editorActions, displayengine.Keys.EnvCycleLayout)
		if m.splitMode {
			editorActions = append(editorActions, displayengine.Keys.EnvResizeSplit)
		}
	}

	return [][]key.Binding{
		{
			displayengine.Keys.Help,
			displayengine.Keys.Esc,
			displayengine.Keys.Tab,
			displayengine.Keys.Enter,
			displayengine.Keys.MouseRight,
			displayengine.Keys.ContextMenu,
			key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓/←/→", "move cursor")),
			key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup/pgdn", "page up/down")),
			key.NewBinding(key.WithKeys("home"), key.WithHelp("home/end", "top/bottom")),
		},
		editorActions,
		{
			key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("alt+z/y", "undo/redo")),
			key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("alt+c", "copy value/selection")),
			key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("shift+←/→/home/end", "select text")),
			displayengine.Keys.ToggleLog,
			displayengine.Keys.FocusPanelTitle,
			displayengine.Keys.ForceQuit,
		},
	}
}

func (m *TabbedVarsEditorModel) HelpText() string {
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return ""
	}
	tab := m.tabs[m.activeTab]
	meta, ok := tab.editor.CurrentLineMeta()
	if !ok || !meta.IsVariable {
		return ""
	}
	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	// meta.toml takes precedence — allows semantic styles and app-specific overrides.
	if vm, ok := tab.appMeta.GetVarMeta(varName, tab.spec.App); ok && vm.HelpLine != "" {
		return vm.HelpLine
	}
	if line := appenv.GetVarHelpLine(varName); line != "" {
		return line
	}
	return ""
}

// splitGutter is the blank space between the two panes when split -- 1
// column (side by side) or 1 row (stacked).
const splitGutter = 1

func (m *TabbedVarsEditorModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// width and height are the already-computed content area dimensions passed by AppModel.
	// Use them directly as dialog bounds, just like MenuModel does.
	// fullContentWidth is shared by both panes when split -- only
	// side-by-side reduces contentWidth, the per-pane width, below it.
	layout := displayengine.GetLayout()
	m.fullContentWidth = m.width - layout.BorderWidth() - layout.ContentMarginWidth()
	if m.fullContentWidth < 1 {
		m.fullContentWidth = 1
	}

	wantSplit := len(m.tabs) == 2 && m.layoutMode != envLayoutMaximized
	m.splitMode = false
	m.paneContentWidth[0] = m.fullContentWidth
	m.paneContentWidth[1] = m.fullContentWidth
	m.pane1OffsetX, m.pane1OffsetY = 0, 0

	// Stacked's height fit is checked later, once subtitleHeight/buttonHeight/
	// largeTitleOverhead are known. Auto-collapses to maximized rendering
	// without touching m.layoutMode, so it springs back once there's room.
	if wantSplit && m.layoutMode == envLayoutSideBySide {
		budget := m.fullContentWidth - splitGutter
		if budget >= 2*minPaneContentWidth {
			paneW := int(float64(budget) * m.sideBySideRatio)
			// Clamp so the ratio can never push either pane below its own
			// floor -- an off-center ratio means pane 2's width
			// (budget-paneW) isn't automatically >= pane 1's, so both
			// directions need an explicit check.
			if paneW < minPaneContentWidth {
				paneW = minPaneContentWidth
			} else if paneW > budget-minPaneContentWidth {
				paneW = budget - minPaneContentWidth
			}
			m.splitMode = true
			m.paneContentWidth[0] = paneW
			m.paneContentWidth[1] = budget - paneW
			m.pane1OffsetX = paneW + splitGutter
		}
	}

	specs := m.getButtonSpecs()
	// Determine button height based on width availability (bordered=3, flat=1)
	m.buttonHeight = displayengine.ButtonRowHeight(m.fullContentWidth, 0, specs...)

	// One shared heading above both panes (see ViewString), at the active
	// tab's content and the full dialog width.
	m.subtitleHeight = calcSubtitleHeightForTab(&m.tabs[m.activeTab], m.fullContentWidth)

	ctx := displayengine.GetActiveContext()
	titleBudget := m.height - layout.BorderHeight() - m.buttonHeight - m.subtitleHeight - layout.BorderHeight()
	useLarge, _ := displayengine.DecideLargeTitleBar(ctx.LargeTitleBars, titleBudget, 3)
	largeTitleOverhead := 0
	if useLarge {
		largeTitleOverhead = displayengine.LargeTitleBarOverhead
	}
	m.largeTitleOverhead = largeTitleOverhead

	// Height available to both panes combined -- button row, large titlebar,
	// and the shared subtitle are all consumed once, not per-pane.
	paneBudget := m.height - layout.BorderHeight() - largeTitleOverhead - m.buttonHeight - m.subtitleHeight

	if wantSplit && m.layoutMode == envLayoutStacked {
		budget := paneBudget - splitGutter
		minPerPane := minPaneEditorHeight + layout.BorderHeight()
		if budget >= 2*minPerPane {
			perPane := int(float64(budget) * m.stackedRatio)
			if perPane < minPerPane {
				perPane = minPerPane
			} else if perPane > budget-minPerPane {
				perPane = budget - minPerPane
			}
			m.splitMode = true
			m.paneEditorHeight[0] = perPane - layout.BorderHeight()
			m.paneEditorHeight[1] = (budget - perPane) - layout.BorderHeight()
			m.pane1OffsetY = perPane + splitGutter
		}
	}
	if !m.splitMode || m.layoutMode != envLayoutStacked {
		// Maximized or side-by-side: one pane's worth of height, using the
		// full budget (side-by-side shares height across both panes).
		m.paneEditorHeight[0] = paneBudget - layout.BorderHeight()
		m.paneEditorHeight[1] = m.paneEditorHeight[0]
	}
	for i := range m.paneEditorHeight {
		if m.paneEditorHeight[i] < 1 {
			m.paneEditorHeight[i] = 1
		}
	}
	if m.paneEditorHeight[m.activeTab] < 3 && m.buttonHeight == 3 && !m.splitMode {
		// Fallback: force buttons flat to save 2 lines if editor would be too small
		m.buttonHeight = 1
		overhead := layout.BorderHeight() + largeTitleOverhead + 1 + m.subtitleHeight + layout.BorderHeight()
		h := m.height - overhead
		if h < 1 {
			h = 1
		}
		m.paneEditorHeight[0] = h
		m.paneEditorHeight[1] = h
	}

	m.activePaneOffsetX, m.activePaneOffsetY = 0, 0
	if m.splitMode && m.activeTab == 1 {
		m.activePaneOffsetX, m.activePaneOffsetY = m.pane1OffsetX, m.pane1OffsetY
	}
	if !m.splitMode {
		// No gutter to resize once collapsed (window shrunk below the
		// split floor) -- exit both resize states so they don't get stuck
		// active with nothing visible to act on.
		m.resizingGutter = false
		m.gutterDrag.StopDrag()
	}

	for i := range m.tabs {
		// Editor content width accounts for inner box borders.
		editorWidth := m.paneContentWidth[i] - layout.BorderWidth()
		if editorWidth < 10 {
			editorWidth = 10
		}
		m.tabs[i].editor.SetWidth(editorWidth)
		m.tabs[i].editor.SetHeight(m.paneEditorHeight[i])
	}
}

// splitBudgetAndFloor returns the total space available to split between
// the two panes (width side-by-side, height stacked) and the minimum
// either pane may shrink to, in that same unit. Shared by applyGutterDrag
// and nudgeSplitRatio so both convert mouse/key deltas into a split-ratio
// clamp identically to SetSize's own collapse-threshold math.
func (m *TabbedVarsEditorModel) splitBudgetAndFloor() (budget, minPane int) {
	layout := displayengine.GetLayout()
	if m.layoutMode == envLayoutSideBySide {
		return m.fullContentWidth - splitGutter, minPaneContentWidth
	}
	paneBudget := m.height - layout.BorderHeight() - m.largeTitleOverhead - m.buttonHeight - m.subtitleHeight
	return paneBudget - splitGutter, minPaneEditorHeight + layout.BorderHeight()
}

// clampSplitRatio clamps ratio so neither pane can go below minPane out of
// budget.
func clampSplitRatio(ratio float64, budget, minPane int) float64 {
	minRatio := float64(minPane) / float64(budget)
	maxRatio := 1 - minRatio
	if ratio < minRatio {
		return minRatio
	}
	if ratio > maxRatio {
		return maxRatio
	}
	return ratio
}

// applyGutterDrag updates the active layout's split ratio from the current
// absolute mouse position during an active gutter drag (mouse is msg.X for
// side-by-side,
// msg.Y for stacked), clamped to the same per-pane floor SetSize enforces,
// then re-lays-out via SetSize.
func (m *TabbedVarsEditorModel) applyGutterDrag(mouse int) {
	budget, minPane := m.splitBudgetAndFloor()
	if budget < 1 {
		return
	}

	delta := mouse - m.gutterDrag.StartMouse
	ratio := clampSplitRatio(m.gutterDrag.StartRatio+float64(delta)/float64(budget), budget, minPane)

	*m.activeSplitRatio() = ratio
	m.SetSize(m.width, m.height)
}

// nudgeSplitRatio adjusts the active layout's split ratio by a fixed step (2
// columns/rows worth) in the given direction (-1 or +1), clamped to the
// same per-pane floor SetSize enforces, then re-lays-out via SetSize. Used
// by the keyboard resize mode (EnvResizeSplit + arrow keys).
func (m *TabbedVarsEditorModel) nudgeSplitRatio(dir int) {
	const stepUnits = 2
	budget, minPane := m.splitBudgetAndFloor()
	if budget < 1 {
		return
	}
	ratioPtr := m.activeSplitRatio()
	*ratioPtr = clampSplitRatio(*ratioPtr+float64(dir*stepUnits)/float64(budget), budget, minPane)
	m.SetSize(m.width, m.height)
}

// calcSubtitleHeightForTab returns the number of subtitle lines for one tab
// at the given width. Global tabs: 1 line (file path). App tabs: 1 line (app
// name) + wrapped description lines.
//
// TODO(refactor): this predicts renderSubtitleForTab's word-wrap ahead of
// render time only because the tabbed vars editor is hand-rolled rather than
// built on the Content/ContentColumn section system, which auto-measures via
// SectionHeight. Rebuilding this screen on that system (subtitle as its own
// Content section) would let this go away entirely.
func calcSubtitleHeightForTab(tab *envTab, width int) int {
	if width < 4 {
		return 0
	}
	if tab.niceName == "" {
		// Global tab: just the file path, 1 line
		return 1
	}
	// App tab: "Application: AppName" (1 line) + word-wrapped description.
	// Must measure the same plain text renderSubtitleForTab actually wraps
	// (theme markup stripped) -- counting raw markup as text overestimates
	// word lengths, wrapping to more lines than are actually rendered.
	h := 1
	if tab.description != "" {
		valueW := width - headingLabelW
		if valueW < 10 {
			valueW = 10
		}
		h += subtitleWrapLines(displayengine.GetPlainText(tab.description), valueW)
	}
	return h
}

// subtitleWrapLines returns how many lines the text occupies when word-wrapped to maxWidth.
func subtitleWrapLines(text string, maxWidth int) int {
	if maxWidth <= 0 || text == "" {
		return 0
	}
	words := strings.Fields(text)
	lines, lineLen := 1, 0
	for _, w := range words {
		wl := len(w)
		if lineLen == 0 {
			lineLen = wl
		} else if lineLen+1+wl > maxWidth {
			lines++
			lineLen = wl
		} else {
			lineLen += 1 + wl
		}
	}
	return lines
}

// renderSubtitleForTab renders the heading subtitle for one tab, padded to
// width. height == 0 means no subtitle to render (see calcSubtitleHeightForTab).
// Delegates the actual heading content (labels, tags, word-wrap) to
// FormatMenuHeading, the same helper the F1 help panel uses, so a tag like
// (User Template) only needs adding in one place, not duplicated per screen.
func (m *TabbedVarsEditorModel) renderSubtitleForTab(tab *envTab, width, height int) string {
	if height == 0 {
		return ""
	}
	dCtx := displayengine.GetActiveContext()
	bgStyle := dCtx.Dialog

	renderLine := func(raw string) string {
		processed := theme.ToANSI(raw, "")
		w := lipgloss.Width(processed)
		padded := processed + strutil.Repeat(" ", width-w)
		return displayengine.MaintainBackground(bgStyle.Render(padded), bgStyle)
	}

	params := MenuHeadingParams{
		AppName:        tab.niceName,
		AppDescription: tab.description,
		FilePath:       tab.envFilePath, // only shown by FormatMenuHeading when AppName == "" (global tab)
	}
	if tab.niceName != "" {
		if tab.spec.App != "" {
			ctx := context.Background()
			appUpper := strings.ToUpper(tab.spec.App)
			// Read state from the live editor buffer (same source as
			// checkEnabledChanged), not disk, so the tags reflect an
			// unsaved __ENABLED add/edit/delete immediately after a refresh.
			// "absent" (no __ENABLED key) means User Defined, matching
			// IsAppUserDefinedFromLines; only "disabled" means Disabled.
			state := m.enabledStateForApp(appUpper)
			params.AppIsUserDefined = state == "absent"
			params.AppIsUserTemplate = appenv.IsUserTemplate(tab.spec.App)
			params.AppIsDeprecated = appenv.IsAppDeprecated(ctx, appenv.AppNameToBaseAppName(tab.spec.App))
			params.AppIsDisabled = state == "disabled"
		}
		params.FilePath = "" // app tabs show "Application:", not "File:"
	}

	var lines []string
	for _, l := range strings.Split(FormatMenuHeading(params, width), "\n") {
		lines = append(lines, renderLine(l))
	}
	return strings.Join(lines, "\n")
}

// HelpContext implements displayengine.HelpContextProvider.
// Returns heading-style info about the variable under the cursor shown at the top of the help dialog.
// contentWidth is the available display width (used to word-wrap descriptions).
func (m *TabbedVarsEditorModel) HelpContext(contentWidth int) displayengine.HelpContext {
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return displayengine.HelpContext{}
	}

	tab := m.tabs[m.activeTab]
	legend := "| " +
		"{{|MarkerAdded|}}+{{[-]}} Added | " +
		"{{|MarkerDeleted|}}-{{[-]}} Deleted | " +
		"{{|MarkerModified|}}~{{[-]}} Changed | " +
		"{{|MarkerModified|}}M{{[-]}} Moved | " +
		"{{|MarkerInvalid|}}!{{[-]}} Invalid |"

	meta, ok := tab.editor.CurrentLineMeta()
	if !ok || !meta.IsVariable {
		hctx := displayengine.HelpContext{
			ScreenName: m.title,
			Legend:     legend,
		}
		if tab.spec.App != "" {
			base := appenv.AppNameToBaseAppName(tab.spec.App)
			var parts []string
			if tab.description != "" {
				parts = append(parts, tab.description)
			}
			if tab.appMeta != nil && tab.appMeta.App.Website != "" {
				parts = append(parts, "Website: {{|URL|}}"+tab.appMeta.App.Website+"{{[-]}}")
			}
			if appenv.IsAppDeprecated(context.Background(), base) {
				parts = append(parts, "{{|TitleError|}}⚠ This app is deprecated.{{[-]}}")
			}
			if len(parts) > 0 {
				hctx.ItemTitle = tab.niceName
				hctx.ItemText = strings.Join(parts, "\n\n")
			}
			if tab.spec.App != "" {
				doc, err := appenv.GetAppMarkdown(context.Background(), tab.spec.App)
				if err == nil {
					hctx.DocMarkdown = doc
					hctx.DocAppName = tab.niceName
				}
			}
			return hctx
		}
		return hctx
	}

	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	if varName == "" {
		return displayengine.HelpContext{
			ScreenName: m.title,
			Legend:     legend,
		}
	}

	return *m.getVariableHelpContext(varName, &tab, contentWidth)
}

// getVariableHelpContext builds a help context for a specific variable in a tab.
func (m *TabbedVarsEditorModel) getVariableHelpContext(varName string, tab *envTab, contentWidth int) *displayengine.HelpContext {
	ctx := context.Background()
	legend := "| " +
		"{{|MarkerAdded|}}+{{[-]}} Added | " +
		"{{|MarkerDeleted|}}-{{[-]}} Deleted | " +
		"{{|MarkerModified|}}~{{[-]}} Changed | " +
		"{{|MarkerModified|}}M{{[-]}} Moved | " +
		"{{|MarkerInvalid|}}!{{[-]}} Invalid |"

	meta, ok := tab.editor.GetVariableMeta(varName)

	currentValue := tab.editor.GetVariableValue(varName)
	originalValue := tab.editor.GetVariableInitialValue(varName)
	// VarIsUserDefined: for app vars the IsUserDefined flag covers the var level;
	// for global vars it means the var itself is user-defined (not in defaults).
	varIsUserDefined := false
	if ok && meta.IsVariable {
		varIsUserDefined = meta.IsUserDefined && tab.niceName == ""
	}

	// Read state from the live editor buffer, same as renderSubtitleForTab:
	// "absent" (no __ENABLED key) means User Defined, matching
	// IsAppUserDefinedFromLines; only "disabled" means Disabled.
	appEnabledState := ""
	if tab.spec.App != "" {
		appEnabledState = m.enabledStateForApp(strings.ToUpper(tab.spec.App))
	}

	params := MenuHeadingParams{
		AppName:           tab.niceName,
		AppDescription:    tab.description,
		AppIsUserDefined:  (ok && meta.IsUserDefined && tab.niceName != "") || appEnabledState == "absent",
		AppIsUserTemplate: tab.spec.App != "" && appenv.IsUserTemplate(tab.spec.App),
		AppIsDeprecated:   tab.spec.App != "" && appenv.IsAppDeprecated(ctx, appenv.AppNameToBaseAppName(tab.spec.App)),
		AppIsDisabled:     appEnabledState == "disabled",
		FilePath:          tab.envFilePath,
		VarName:           varName,
		VarIsUserDefined:  varIsUserDefined,
		OriginalValue:     originalValue,
		CurrentValue:      currentValue,
	}

	itemText := FormatMenuHeading(params, contentWidth)

	if desc := appenv.GetVarHelpText(varName); desc != "" {
		itemText += "\n\n" + desc
	} else if vm, ok := tab.appMeta.GetVarMeta(varName, tab.spec.App); ok && vm.HelpText != "" {
		itemText += "\n\n" + vm.HelpText
	}

	h := displayengine.HelpContext{
		ScreenName: m.title,
		Legend:     legend,
		ItemTitle: "Variable: " + func() string {
			if tab.niceName != "" {
				return tab.niceName + ":" + varName
			}
			return varName
		}(),
		ItemText: itemText,
	}

	if tab.spec.App != "" {
		ctx := context.Background()
		if !appenv.IsAppUserDefined(ctx, tab.spec.App, tab.composeEnvPath) {
			doc, err := appenv.GetAppMarkdown(ctx, tab.spec.App)
			if err == nil {
				h.DocMarkdown = doc
				h.DocAppName = tab.niceName
			}
		}
	}

	return &h
}
