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

	tab := m.tabs[m.activeTab]
	editor := tab.editor
	editorView := editor.View()

	ctx := displayengine.GetActiveContext()
	focused := m.focus == envFocusEditor

	// 1. Render the tabs row (to be used as the title of the inner border)
	tabRow := m.renderTabs()

	// 2. Wrap the editor in its own inner border (matching MenuModel list style)
	// We use m.contentWidth for the inner box width.
	// Height includes editor + 2 lines for borders.
	innerBox := displayengine.RenderBorderedBoxCtx(
		tabRow,
		editorView,
		m.contentWidth-2,
		m.editorHeight+2,
		focused,
		false, // No focus indicators here
		true,  // Rounded corners to match submenu style
		ctx.SubmenuTitleAlign,
		"RAW", // Use the pre-rendered tabRow exactly
		ctx,
	)

	// 3. Replace the bottom border with INS/OVR label (left) and scroll % (right, if scrolling).
	modeLabel := "INS"
	if editor.IsOverwrite() {
		modeLabel = "OVR"
	}
	scrollLabel := ""
	if editor.LineCount() > editor.Height() {
		scrollLabel = fmt.Sprintf("%d%%", int(editor.ScrollPercent()*100))
	}
	lines := strings.Split(innerBox, "\n")
	if len(lines) > 0 {
		lines[len(lines)-1] = displayengine.BuildDualLabelBottomBorderCtx(m.contentWidth, modeLabel, scrollLabel, focused, ctx)
		innerBox = strings.Join(lines, "\n")
	}

	// 4. Render buttons
	buttons := m.renderButtons(m.contentWidth)

	// 5. Join components vertically: subtitle (if any) → editor box → buttons
	var parts []string
	if subtitle := m.renderSubtitle(); subtitle != "" {
		parts = append(parts, subtitle)
	}
	parts = append(parts, innerBox, buttons)

	// Standardize TrimRight to prevent implicit gaps
	for i, part := range parts {
		parts[i] = strings.TrimRight(part, "\n")
	}
	innerContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Apply 1-char side margin so inner components are inset from the outer border (matching menu dialogs).
	layout := displayengine.GetLayout()
	outerContentWidth := m.contentWidth + layout.ContentMarginWidth()
	fullContent := lipgloss.NewStyle().
		Background(ctx.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin).
		Render(innerContent)

	// 6. Wrap in the outer dialog border
	// outerContentWidth = m.contentWidth + margin = m.width - BorderWidth, so total = m.width.
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
		"Back":    displayengine.IDBackButton,
		"Exit":    displayengine.IDExitButton,
	}
	helpByName := map[string]string{
		"Save":    "Save all changes in all tabs to the environment file.",
		"Refresh": "Reformat and re-stage all tabs (same as pressing F5).",
		"Back":    "Discard all changes and return (prompts if unsaved changes exist).",
		"Exit":    "Discard all changes and exit the application.",
	}
	var specs []displayengine.ButtonSpec
	for i, btn := range m.buttons {
		zoneID := zoneByName[btn]
		specs = append(specs, displayengine.ButtonSpec{
			Text:   btn,
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

func (m *TabbedVarsEditorModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// width and height are the already-computed content area dimensions passed by AppModel.
	// Use them directly as dialog bounds, just like MenuModel does.
	// contentWidth is the inner space inside the outer border (border takes 2 chars).
	// Inner components are further inset by 1-char margin each side (ContentMarginWidth).
	layout := displayengine.GetLayout()
	m.contentWidth = m.width - layout.BorderWidth() - layout.ContentMarginWidth()
	if m.contentWidth < 1 {
		m.contentWidth = 1
	}

	specs := m.getButtonSpecs()
	// Determine button height based on width availability (bordered=3, flat=1)
	m.buttonHeight = displayengine.ButtonRowHeight(m.contentWidth, 0, specs...)

	// Calculate subtitle height based on active tab heading content
	m.subtitleHeight = m.calcSubtitleHeight()

	ctx := displayengine.GetActiveContext()
	titleBudget := m.height - layout.BorderHeight() - m.buttonHeight - m.subtitleHeight - layout.BorderHeight()
	useLarge, _ := displayengine.DecideLargeTitleBar(ctx.LargeTitleBars, titleBudget, 3)
	largeTitleOverhead := 0
	if useLarge {
		largeTitleOverhead = displayengine.LargeTitleBarOverhead
	}
	m.largeTitleOverhead = largeTitleOverhead

	// Available for the editor: total inner height minus button row, subtitle, editor borders, and large titlebar rows
	m.editorHeight = m.height - layout.BorderHeight() - largeTitleOverhead - m.buttonHeight - m.subtitleHeight - layout.BorderHeight()
	if m.editorHeight < 1 {
		m.editorHeight = 1
	}
	if m.editorHeight < 3 && m.buttonHeight == 3 {
		// Fallback: force buttons flat to save 2 lines if editor would be too small
		m.buttonHeight = 1
		overhead := layout.BorderHeight() + largeTitleOverhead + 1 + m.subtitleHeight + layout.BorderHeight()
		m.editorHeight = m.height - overhead
	}

	if m.editorHeight < 1 {
		m.editorHeight = 1
	}

	editorWidth := m.contentWidth - layout.BorderWidth() // Editor content width accounts for inner box borders
	if editorWidth < 10 {
		editorWidth = 10
	}

	for i := range m.tabs {
		m.tabs[i].editor.SetWidth(editorWidth)
		m.tabs[i].editor.SetHeight(m.editorHeight)
	}
}

// calcSubtitleHeight returns the number of subtitle lines for the active tab.
// Global tabs: 1 line (file path). App tabs: 1 line (app name) + wrapped description lines.
func (m *TabbedVarsEditorModel) calcSubtitleHeight() int {
	if len(m.tabs) == 0 || m.contentWidth < 4 {
		return 0
	}
	tab := m.tabs[m.activeTab]
	if tab.niceName == "" {
		// Global tab: just the file path, 1 line
		return 1
	}
	// App tab: "Application: AppName" (1 line) + word-wrapped description.
	// Must measure the same plain text renderSubtitle actually wraps
	// (theme markup stripped) -- counting raw markup as text overestimates
	// word lengths, wrapping to more lines than are actually rendered.
	h := 1
	if tab.description != "" {
		valueW := m.contentWidth - headingLabelW
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

// renderSubtitle renders the heading subtitle for the active tab.
// Returns a slice of styled lines (each already padded to contentWidth).
func (m *TabbedVarsEditorModel) renderSubtitle() string {
	if m.subtitleHeight == 0 || len(m.tabs) == 0 {
		return ""
	}
	tab := m.tabs[m.activeTab]
	ctx := displayengine.GetActiveContext()
	bgStyle := ctx.Dialog

	renderLine := func(raw string) string {
		processed := theme.ToANSI(raw, "")
		w := lipgloss.Width(processed)
		padded := processed + strutil.Repeat(" ", m.contentWidth-w)
		return displayengine.MaintainBackground(bgStyle.Render(padded), bgStyle)
	}

	var lines []string

	if tab.niceName == "" {
		// Global: show file path
		lines = append(lines, renderLine(headingLabel("File: ")+"{{|HeadingValue|}}"+tab.envFilePath+"{{[-]}}"))
	} else {
		// App: "Application: AppName" on first line
		appLine := headingLabel("Application: ") + "{{|HeadingValue|}}" + tab.niceName + "{{[-]}}"
		lines = append(lines, renderLine(appLine))

		// Word-wrap description onto continuation lines, indented to align with value
		if tab.description != "" {
			indent := strutil.Repeat(" ", headingLabelW)
			valueW := m.contentWidth - headingLabelW
			if valueW < 10 {
				valueW = 10
			}
			for _, dl := range subtitleWrapText(displayengine.GetPlainText(tab.description), valueW) {
				lines = append(lines, renderLine(indent+"{{|HeadingAppDescription|}}"+dl+"{{[-]}}"))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// subtitleWrapText word-wraps text to maxWidth, returning a slice of lines.
func subtitleWrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 || text == "" {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
		} else if cur.Len()+1+len(w) > maxWidth {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
		} else {
			cur.WriteByte(' ')
			cur.WriteString(w)
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
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
	legend := "| " +
		"{{|MarkerAdded|}}+{{[-]}} Added | " +
		"{{|MarkerDeleted|}}-{{[-]}} Deleted | " +
		"{{|MarkerModified|}}~{{[-]}} Changed | " +
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

	params := MenuHeadingParams{
		AppName:          tab.niceName,
		AppDescription:   tab.description,
		AppIsUserDefined: ok && meta.IsUserDefined && tab.niceName != "",
		FilePath:         tab.envFilePath,
		VarName:          varName,
		VarIsUserDefined: varIsUserDefined,
		OriginalValue:    originalValue,
		CurrentValue:     currentValue,
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
