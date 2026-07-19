package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *TabbedVarsEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if tickCmd, ok := m.btnRow.Update(msg); ok {
		return m, tickCmd
	}

	if m.HandleWidgetClearPress(msg) {
		return m, nil
	}

	switch msg := msg.(type) {
	case displayengine.LockStateChangedMsg:
		m.lockedByOthers = msg.LockedByOthers
		return m, nil

	case displayengine.LayerHitMsg:
		if strings.HasPrefix(msg.ID, "tabbed_vars.tab-") {
			// On right-click, do nothing (allows through hit-testing to global context menu)
			if msg.Button == tea.MouseRight {
				return m, nil
			}

			// Left click (or other) switches tabs
			tabIdxStr := strings.TrimPrefix(msg.ID, "tabbed_vars.tab-")
			if idx, err := strconv.Atoi(tabIdxStr); err == nil && idx >= 0 && idx < len(m.tabs) {
				m.focus = envFocusEditor
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Blur()
				}
				m.activeTab = idx
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Focus()
				}
				return m, nil
			}
		}

		if msg.ID == "tabbed_vars.editor" {
			// Right-click opens the context menu for the clicked variable row WITHOUT moving focus/cursor
			if msg.Button == tea.MouseRight {
				if len(m.tabs) > 0 {
					return m, m.showContextMenuForClick(msg.X, msg.Y)
				}
				return m, nil
			}

			// Left click moves focus and cursor
			m.focus = envFocusEditor
			if len(m.tabs) > 0 {
				m.tabs[m.activeTab].editor.Focus()

				// Calculate relative coordinates for the editor click
				// Hit region is at NestedLeftOffset, NestedTopOffset + subtitleHeight
				layout := displayengine.GetLayout()
				relX := msg.X - (m.lastOffsetX + layout.NestedLeftOffset())
				relY := msg.Y - (m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight)

				var cmd tea.Cmd
				m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(tea.MouseClickMsg{
					X:      relX,
					Y:      relY,
					Button: msg.Button,
				})
				return m, cmd
			}
			return m, nil
		}

		// Button clicks
		if displayengine.ButtonIDMatches(msg.ID, displayengine.IDSaveButton) {
			if msg.Button == tea.MouseLeft {
				m.focus = envFocusButtons
				m.btnIdx = 0
				if m.lockedByOthers {
					return m, nil
				}
				if m.hasErrors() {
					return m, func() tea.Msg {
						return tui.ShowMessageDialogMsg{
							Title:   "Validation Error",
							Message: "Cannot save while there are invalid variable names or incomplete lines.",
							Type:    tui.MessageError,
						}
					}
				}
				return m, m.btnRow.SetProcessing(displayengine.IDSaveButton, m.saveEnv())
			}
		} else if displayengine.ButtonIDMatches(msg.ID, displayengine.IDRefreshButton) {
			if msg.Button == tea.MouseLeft {
				m.focus = envFocusButtons
				m.btnIdx = m.buttonIndex("Refresh")
				return m, m.btnRow.SetProcessing(displayengine.IDRefreshButton, func() tea.Msg { return envRefreshMsg{} })
			}
		} else if displayengine.ButtonIDMatches(msg.ID, displayengine.IDBackButton) {
			if msg.Button == tea.MouseLeft {
				m.focus = envFocusButtons
				m.btnIdx = m.buttonIndex("Back")
				if m.hasChanges() {
					return m, m.btnRow.SetProcessing(displayengine.IDBackButton, m.promptUnsavedChanges(m.onClose))
				}
				return m, m.btnRow.SetProcessing(displayengine.IDBackButton, m.onClose)
			}
		} else if displayengine.ButtonIDMatches(msg.ID, displayengine.IDExitButton) {
			if msg.Button == tea.MouseLeft {
				m.focus = envFocusButtons
				m.btnIdx = m.buttonIndex("Exit")
				return m, m.btnRow.SetProcessing(displayengine.IDExitButton, m.confirmExitAction())
			}
		} else if msg.ID == "tabbed_vars."+displayengine.IDTitleWidgetClose {
			if msg.Button == tea.MouseLeft {
				m.BlurTitleBar()
				pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetClose, msg.ID)
				m.focus = envFocusButtons
				m.btnIdx = m.buttonIndex("Back")
				closeAction := m.onClose
				if m.hasChanges() {
					closeAction = m.promptUnsavedChanges(m.onClose)
				}
				return m, tea.Batch(pressCmd, m.btnRow.SetProcessing(displayengine.IDBackButton, closeAction))
			}
		} else if msg.ID == "tabbed_vars."+displayengine.IDTitleWidgetHelp {
			if msg.Button == tea.MouseLeft {
				m.BlurTitleBar()
				pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetHelp, msg.ID)
				return m, tea.Batch(pressCmd, func() tea.Msg { return displayengine.TriggerHelpMsg{ScreenLevelOnly: true} })
			}
		} else if msg.ID == "tabbed_vars."+displayengine.IDTitleWidgetRefresh {
			if msg.Button == tea.MouseLeft {
				m.BlurTitleBar()
				pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetRefresh, msg.ID)
				m.focus = envFocusButtons
				m.btnIdx = m.buttonIndex("Refresh")
				return m, tea.Batch(pressCmd, m.btnRow.SetProcessing(displayengine.IDRefreshButton, func() tea.Msg { return envRefreshMsg{} }))
			}
		} else if msg.ID == "tabbed_vars."+displayengine.IDInsOvr {
			if msg.Button == tea.MouseLeft && len(m.tabs) > 0 {
				m.tabs[m.activeTab].editor.ToggleOverwrite()
			}
			return m, nil
		}
		return m, nil

	case tea.MouseClickMsg:
		// Scrollbar thumb drag initiation routed by model_mouse.go section B0.
		if msg.Button == tea.MouseLeft && len(m.tabs) > 0 {
			// Translate coordinates to editor-relative and only forward if within editor bounds.
			layout := displayengine.GetLayout()
			relX := msg.X - (m.lastOffsetX + layout.NestedLeftOffset())
			relY := msg.Y - (m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight)
			editorW := m.contentWidth - layout.BorderWidth()
			if relX >= 0 && relY >= 0 && relY < m.editorHeight && relX < editorW {
				var cmd tea.Cmd
				m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(tea.MouseClickMsg{
					X:      relX,
					Y:      relY,
					Button: msg.Button,
				})
				return m, cmd
			}
		}

	case displayengine.LayerWheelMsg, tea.MouseWheelMsg:
		var wheelBtn tea.MouseButton
		if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
			wheelBtn = mwMsg.Button
		} else if lwMsg, ok := msg.(displayengine.LayerWheelMsg); ok {
			wheelBtn = lwMsg.Button
		}

		if (wheelBtn == tea.MouseWheelUp || wheelBtn == tea.MouseWheelDown) && len(m.tabs) > 0 {
			var cmd tea.Cmd
			m.focus = envFocusEditor
			m.tabs[m.activeTab].editor.Focus()

			// Translate wheel to up/down arrows for enveditor
			var keyMsg tea.KeyPressMsg
			switch wheelBtn {
			case tea.MouseWheelUp:
				keyMsg = tea.KeyPressMsg{Code: tea.KeyUp}
			case tea.MouseWheelDown:
				keyMsg = tea.KeyPressMsg{Code: tea.KeyDown}
			}
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(keyMsg)
			return m, cmd
		}

	case tea.KeyMsg:
		// When titlebar has focus, handle navigation between widgets.
		if m.TitleBarFocused() {
			switch msg.String() {
			case "left":
				m.CycleWidget(-1)
				return m, nil
			case "right":
				m.CycleWidget(+1)
				return m, nil
			case "enter", " ":
				switch m.ActiveWidget() {
				case displayengine.IDTitleWidgetHelp:
					pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetHelp, "key")
					return m, tea.Batch(pressCmd, func() tea.Msg { return displayengine.TriggerHelpMsg{ScreenLevelOnly: true} })
				case displayengine.IDTitleWidgetClose:
					pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetClose, "key")
					if m.hasChanges() {
						return m, tea.Batch(pressCmd, m.promptUnsavedChanges(m.onClose))
					}
					return m, tea.Batch(pressCmd, m.onClose)
				case displayengine.IDTitleWidgetRefresh:
					pressCmd := m.PressWidgetID(displayengine.IDTitleWidgetRefresh, "key")
					m.BlurTitleBar()
					m.focus = envFocusButtons
					m.btnIdx = m.buttonIndex("Refresh")
					return m, tea.Batch(pressCmd, m.btnRow.SetProcessing(displayengine.IDRefreshButton, func() tea.Msg { return envRefreshMsg{} }))
				}
			case "esc":
				m.BlurTitleBar()
				return m, nil
			}
			return m, nil
		}

		switch {
		case msg.String() == "esc":
			m.focus = envFocusButtons
			m.btnIdx = m.buttonIndex("Back")
			return m, m.btnRow.SetProcessing(displayengine.IDBackButton, m.EscapeAction())
		case key.Matches(msg, displayengine.Keys.EnvNextTab): // Next Tab
			if m.focus == envFocusEditor && len(m.tabs) > 1 {
				m.tabs[m.activeTab].editor.Blur()
				m.activeTab = (m.activeTab + 1) % len(m.tabs)
				m.tabs[m.activeTab].editor.Focus()
				m.SetSize(m.width, m.height)
				return m, nil
			}
		case key.Matches(msg, displayengine.Keys.EnvPrevTab): // Prev Tab
			if m.focus == envFocusEditor && len(m.tabs) > 1 {
				m.tabs[m.activeTab].editor.Blur()
				m.activeTab--
				if m.activeTab < 0 {
					m.activeTab = len(m.tabs) - 1
				}
				m.tabs[m.activeTab].editor.Focus()
				m.SetSize(m.width, m.height)
				return m, nil
			}
		case msg.String() == "tab" || msg.String() == "shift+tab":
			if m.focus == envFocusEditor {
				m.focus = envFocusButtons
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Blur()
				}
			} else {
				m.focus = envFocusEditor
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Focus()
				}
			}
			return m, nil
		case key.Matches(msg, displayengine.Keys.EnvRefresh):
			m.focus = envFocusButtons
			m.btnIdx = m.buttonIndex("Refresh")
			return m, m.btnRow.SetProcessing(displayengine.IDRefreshButton, func() tea.Msg { return envRefreshMsg{} })
		case key.Matches(msg, displayengine.Keys.ContextMenu):
			if m.focus == envFocusEditor && len(m.tabs) > 0 {
				editor := m.tabs[m.activeTab].editor
				layout := displayengine.GetLayout()
				y := m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight + editor.CursorVisualRow() - editor.YOffset()
				x := m.lastOffsetX + layout.NestedLeftOffset() + editor.CursorVisualCol() + 1
				return m, m.showContextMenuForClick(x, y)
			}
		}

		if m.focus == envFocusButtons {
			switch msg.String() {
			case "left":
				m.btnIdx--
				if m.btnIdx < 0 {
					m.btnIdx = len(m.buttons) - 1
				}
			case "right":
				m.btnIdx++
				if m.btnIdx >= len(m.buttons) {
					m.btnIdx = 0
				}
			case "enter":
				if m.btnIdx >= 0 && m.btnIdx < len(m.buttons) {
					zoneByName := map[string]string{"Save": displayengine.IDSaveButton, "Refresh": displayengine.IDRefreshButton, "Back": displayengine.IDBackButton, "Exit": displayengine.IDExitButton}
					btnName := m.buttons[m.btnIdx]
					switch btnName {
					case "Save":
						if m.hasErrors() {
							return m, func() tea.Msg {
								return tui.ShowMessageDialogMsg{
									Title:   "Validation Error",
									Message: "Cannot save while there are invalid variable names or incomplete lines.",
									Type:    tui.MessageError,
								}
							}
						}
						return m, m.btnRow.SetProcessing(zoneByName[btnName], m.saveEnv())
					case "Refresh":
						return m, m.btnRow.SetProcessing(zoneByName[btnName], func() tea.Msg { return envRefreshMsg{} })
					case "Back":
						if m.hasChanges() {
							return m, m.btnRow.SetProcessing(zoneByName[btnName], m.promptUnsavedChanges(m.onClose))
						}
						return m, m.btnRow.SetProcessing(zoneByName[btnName], m.onClose)
					case "Exit":
						return m, m.btnRow.SetProcessing(zoneByName[btnName], m.confirmExitAction())
					}
				}
			}
		} else {
			// Specific editor hotkeys
			switch {
			case key.Matches(msg, displayengine.Keys.EnvDelete):
				if len(m.tabs) > 0 {
					varName := m.tabs[m.activeTab].editor.CurrentVariableName()
					m.tabs[m.activeTab].editor.DeleteCurrentVariable()
					return m, m.checkEnabledChangedForKey(varName)
				}
				return m, nil
			case msg.String() == "ctrl+u":
				if len(m.tabs) > 0 {
					varName := m.tabs[m.activeTab].editor.CurrentVariableName()
					m.tabs[m.activeTab].editor.UndeleteCurrentVariable()
					return m, m.checkEnabledChangedForKey(varName)
				}
				return m, nil
			case key.Matches(msg, displayengine.Keys.EnvAddVar):
				return m, m.showAddVarDialog()
			case msg.String() == "f2":
				return m, m.showSetValueDialog()
			case key.Matches(msg, displayengine.Keys.EnvReorderU):
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.MoveVariableUp()
				}
				return m, nil
			case key.Matches(msg, displayengine.Keys.EnvReorderD):
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.MoveVariableDown()
				}
				return m, nil
			}
		}
	case tea.MouseMotionMsg:
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			editor := m.tabs[m.activeTab].editor
			if editor.IsDragging() {
				layout := displayengine.GetLayout()
				relX := msg.X - (m.lastOffsetX + layout.NestedLeftOffset())
				relY := msg.Y - (m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight)
				var cmd tea.Cmd
				m.tabs[m.activeTab].editor, cmd = editor.Update(tea.MouseMotionMsg{
					X: relX,
					Y: relY,
				})
				return m, cmd
			}
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			layout := displayengine.GetLayout()
			relX := msg.X - (m.lastOffsetX + layout.NestedLeftOffset())
			relY := msg.Y - (m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight)
			var cmd tea.Cmd
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(tea.MouseReleaseMsg{
				X:      relX,
				Y:      relY,
				Button: msg.Button,
			})
			return m, cmd
		}
	case envSaveSuccessMsg:
		// Reload from disk — ParseEnv will fully reset editor state (clears all
		// gutter markers, removes pending-delete lines, updates InitialLine).
		// Also refresh the app list so user-defined status reflects the new file.
		m.loading = true
		m.titleSpinner.Start()
		return m, tea.Batch(
			func() tea.Msg { return tui.RefreshAppsListMsg{} },
			m.loadEnv,
		)
	case envAddVarMsg:
		if len(m.tabs) > 0 {
			tab := &m.tabs[m.activeTab]
			defVal := ""
			if tab.editor.DefaultValueFunc != nil {
				defVal = tab.editor.DefaultValueFunc(msg.key)
			}
			tab.editor.AddVariable(msg.key, defVal)
		}
		return m, nil
	case envAddVarTemplateMsg:
		prefix := msg.prefix
		return m, func() tea.Msg {
			keyName, err := tui.PromptText("Add Variable", "Enter variable name:", false)
			if err == nil && keyName != "" {
				keyName = strings.TrimSpace(keyName)
				if !strings.HasPrefix(strings.ToUpper(keyName), strings.ToUpper(prefix)) {
					keyName = prefix + keyName
				}
				return envAddVarMsg{key: keyName}
			}
			return nil
		}
	case envAddAllStockMsg:
		if len(m.tabs) > 0 {
			for _, key := range msg.vars {
				m.tabs[m.activeTab].editor.AddVariable(key, msg.defaults[key])
			}
		}
		return m, nil
	case ApplyVarValueMsg:
		if len(m.tabs) > 0 {
			// If the variable is pending deletion, restore it first — editing implies intent to keep it.
			m.tabs[m.activeTab].editor.UndeleteVariableByName(msg.VarName)
			m.tabs[m.activeTab].editor.SetVariableValue(msg.VarName, msg.Value)
		}
		return m, m.checkEnabledChangedForKey(msg.VarName)
	case deleteVarMsg:
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.DeleteVariableByName(msg.VarName)
		}
		return m, m.checkEnabledChangedForKey(msg.VarName)
	case restoreVarMsg:
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.UndeleteVariableByName(msg.VarName)
		}
		return m, m.checkEnabledChangedForKey(msg.VarName)
	case envRefreshMsg:
		ctx := context.Background()
		globalLines := make(map[string][]string)
		for i := range m.tabs {
			if m.tabs[i].spec.IsGlobal {
				globalLines[strings.ToUpper(m.tabs[i].spec.App)] = m.tabs[i].editor.ActiveLines()
			}
		}
		for i := range m.tabs {
			tab := &m.tabs[i]
			capturedComposeEnvPath := tab.composeEnvPath
			capturedApp := tab.spec.App
			appUpper := strings.ToUpper(capturedApp)
			envLines := globalLines[appUpper]
			if envLines == nil {
				envLines = globalLines[""]
			}
			capturedEnvLines := envLines

			// Re-derive defaultLines using staged envLines so a newly-typed APPNAME__ENABLED
			// causes the template to be loaded on refresh (mirrors loadEnv logic but uses
			// IsAppUserDefinedFromLines instead of the disk-based IsAppUserDefined).
			var capturedDefaultLines []string
			if capturedApp != "" && !appenv.IsAppUserDefinedFromLines(ctx, capturedApp, capturedEnvLines) {
				var fileSuffix string
				if tab.spec.IsGlobal {
					fileSuffix = ".env"
				} else {
					fileSuffix = ".env.app.*"
				}
				if defaultFilePath, err := appenv.AppInstanceFile(ctx, capturedApp, fileSuffix); err == nil {
					capturedDefaultLines = appenv.ReadDefaultLines(defaultFilePath)
				}
			} else if capturedApp == "" {
				// Global .env tab: use cached template lines so variables defined in the
				// template are not incorrectly classified as user-defined on refresh.
				capturedDefaultLines = tab.defaultLines
			}

			// Snapshot pre-refresh values to detect which vars the user actually changed.
			preRefresh, _ := appenv.ListVarsLiteralsData(tab.editor.GetContent())
			tab.editor.ReformatEnv(tab.editor.DefaultValueFunc, tab.readOnlyVars, msg.preservePendingDeletes, func(currentLines []string) []string {
				return appenv.FormatLinesCore(ctx, currentLines, capturedDefaultLines, capturedEnvLines, capturedApp, capturedComposeEnvPath)
			})
			// Update initialVars only for variables the user had not changed before refresh,
			// so formatting-only changes don't appear as unsaved edits, but real user edits
			// remain dirty.
			refreshed, _ := appenv.ListVarsLiteralsData(tab.editor.GetContent())
			for k, refreshedVal := range refreshed {
				preVal, existedBefore := preRefresh[k]
				initVal, existedInit := tab.initialVars[k]
				if !existedBefore || !existedInit || preVal == initVal {
					// Key is new, or user hadn't changed it — absorb the refreshed value.
					tab.initialVars[k] = refreshedVal
				}
				// Otherwise user had edited this var — leave initialVars[k] unchanged so
				// hasChanges() still detects the diff.
			}
			// Remove keys that no longer exist after refresh.
			for k := range tab.initialVars {
				if _, exists := refreshed[k]; !exists {
					delete(tab.initialVars, k)
				}
			}
		}
		m.SetSize(m.width, m.height)
		// envRefreshMsg resolves synchronously within this Update call, unlike
		// Save/Back/Exit whose spinners clear when a sub-dialog they open
		// later closes (see ClearProcessingState) -- nothing else would ever
		// stop the Refresh button's spinner otherwise.
		m.btnRow.Clear()
		return m, nil
	case tui.EnvLoadDoneMsg:
		m.loading = false
		m.titleSpinner.Stop()
		for _, data := range msg.Tabs {
			i := data.Index
			if i < 0 || i >= len(m.tabs) {
				continue
			}
			// Configure editor settings before parsing
			m.tabs[i].editor.DefaultValueFunc = data.DefaultFunc
			m.tabs[i].editor.AddPrefix = data.AddPrefix
			m.tabs[i].editor.ValidationType = data.ValidationType
			m.tabs[i].editor.ValidationAppName = data.ValidationApp
			m.tabs[i].editor.ValidationIsGlobal = data.IsGlobal
			m.tabs[i].editor.ValidateFunc = appenv.VarNameIsValid
			// Parse content into editor (resets value + lineMeta, invalidates cache)
			m.tabs[i].editor.ParseEnv(data.Content, data.DefaultFunc, data.ReadOnlyVars)
			if m.focused && m.activeTab == i && m.focus == envFocusEditor {
				m.tabs[i].editor.Focus()
			} else {
				m.tabs[i].editor.Blur()
			}
			m.tabs[i].editor.ScrollbarFunc = func(content string, total, visible, offset int, lineChars bool) string {
				return displayengine.ApplyScrollbarColumn(content, total, visible, offset, lineChars, displayengine.GetActiveContext())
			}
			m.tabs[i].editor.SetLineCharacters(displayengine.GetActiveContext().LineCharacters)
			m.tabs[i].editor.LineNumberBrackets = displayengine.GetActiveContext().LineNumberBrackets
			m.tabs[i].editor.LineNumberBracketOpen, m.tabs[i].editor.LineNumberBracketClose = displayengine.TagBracketGlyphs()

			// Apply theme-aware env-specific styles
			editorStyles := m.tabs[i].editor.Styles()
			editorStyles.Focused.LineNumber = displayengine.SemanticRawStyle("LineNumber")
			editorStyles.Focused.LineNumberFocused = displayengine.SemanticRawStyle("LineNumberFocused")
			editorStyles.Focused.LineNumberModified = displayengine.SemanticRawStyle("LineNumberModified")
			editorStyles.Focused.LineNumberModifiedFocused = displayengine.SemanticRawStyle("LineNumberModifiedFocused")
			editorStyles.Focused.LineNumberBrackets = displayengine.SemanticRawStyle("LineNumberBrackets")
			editorStyles.Focused.InvalidText = displayengine.SemanticRawStyle("EnvInvalid")
			editorStyles.Focused.DuplicateText = displayengine.SemanticRawStyle("EnvDuplicate")
			editorStyles.Focused.BuiltinText = displayengine.SemanticRawStyle("EnvBuiltin")
			editorStyles.Focused.CommentText = displayengine.SemanticRawStyle("LineComment")
			editorStyles.Focused.ModifiedText = displayengine.SemanticRawStyle("ModifiedText")
			editorStyles.Focused.ReadOnlyText = displayengine.SemanticRawStyle("EnvReadOnly")
			editorStyles.Focused.PendingDeleteText = displayengine.SemanticRawStyle("EnvPendingDelete")
			editorStyles.Focused.GutterAdded = displayengine.SemanticRawStyle("MarkerAdded")
			editorStyles.Focused.GutterDeleted = displayengine.SemanticRawStyle("MarkerDeleted")
			editorStyles.Focused.GutterModified = displayengine.SemanticRawStyle("MarkerModified")
			editorStyles.Focused.GutterInvalid = displayengine.SemanticRawStyle("MarkerInvalid")
			editorStyles.Cursor.Color = displayengine.TextCursorColor()

			editorStyles.Blurred.LineNumber = displayengine.SemanticRawStyle("LineNumber")
			editorStyles.Blurred.LineNumberFocused = displayengine.SemanticRawStyle("LineNumberFocused")
			editorStyles.Blurred.LineNumberModified = displayengine.SemanticRawStyle("LineNumberModified")
			editorStyles.Blurred.LineNumberModifiedFocused = displayengine.SemanticRawStyle("LineNumberModifiedFocused")
			editorStyles.Blurred.LineNumberBrackets = displayengine.SemanticRawStyle("LineNumberBrackets")
			editorStyles.Blurred.InvalidText = displayengine.SemanticRawStyle("EnvInvalid")
			editorStyles.Blurred.DuplicateText = displayengine.SemanticRawStyle("EnvDuplicate")
			editorStyles.Blurred.BuiltinText = displayengine.SemanticRawStyle("EnvBuiltin")
			editorStyles.Blurred.CommentText = displayengine.SemanticRawStyle("LineComment")
			editorStyles.Blurred.ModifiedText = displayengine.SemanticRawStyle("ModifiedText")
			editorStyles.Blurred.ReadOnlyText = displayengine.SemanticRawStyle("EnvReadOnly")
			editorStyles.Blurred.PendingDeleteText = displayengine.SemanticRawStyle("EnvPendingDelete")
			editorStyles.Blurred.GutterAdded = displayengine.SemanticRawStyle("MarkerAdded")
			editorStyles.Blurred.GutterDeleted = displayengine.SemanticRawStyle("MarkerDeleted")
			editorStyles.Blurred.GutterModified = displayengine.SemanticRawStyle("MarkerModified")
			editorStyles.Blurred.GutterInvalid = displayengine.SemanticRawStyle("MarkerInvalid")
			m.tabs[i].editor.SetStyles(editorStyles)
			// Update tab metadata used by saveEnv and heading display
			m.tabs[i].initialVars = data.InitialVars
			m.tabs[i].defaultFilePath = data.DefaultFilePath
			m.tabs[i].defaultLines = data.DefaultLines
			m.tabs[i].composeEnvPath = data.ComposeEnvPath
			m.tabs[i].readOnlyVars = data.ReadOnlyVars
			m.tabs[i].niceName = data.NiceName
			m.tabs[i].description = data.Description
			m.tabs[i].envFilePath = data.EnvFilePath
			m.tabs[i].appMeta = data.AppMeta
			// Clear undo — content has been reloaded so prior edits are irrelevant
			m.tabs[i].editor.ClearUndo()
			// Seed lastEnabledState so the first edit is compared against the loaded state.
			if m.tabs[i].spec.IsGlobal && m.tabs[i].spec.App != "" {
				appUpper := strings.ToUpper(m.tabs[i].spec.App)
				m.tabs[i].lastEnabledState = m.enabledStateForApp(appUpper)
			}
		}
		m.SetSize(m.width, m.height)
		return m, nil
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		// Filter out raw mouse messages that fall through (unhandled clicks).
		// These shouldn't reach the editor as they would trigger unwanted scrolling.
		// Mouse interaction is handled via LayerHitMsg (clicks) or explicit MouseMotionMsg (dragging).
		isMouse := false
		switch msg.(type) {
		case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			isMouse = true
		}

		if !isMouse {
			// Before passing the key to the editor, snapshot the cursor row and the
			// line content so we can detect when the cursor leaves an ENABLED line.
			tab := &m.tabs[m.activeTab]
			prevRow := tab.editor.Line()
			prevLine := ""
			if tab.spec.IsGlobal && tab.spec.App != "" && appenv.IsAppBuiltIn(strings.ToUpper(tab.spec.App)) {
				if lm, ok := tab.editor.LineMetaAt(prevRow); ok && lm.IsVariable {
					prevLine = tab.editor.LineAt(prevRow)
				}
			}

			var cmd tea.Cmd
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(msg)
			cmds = append(cmds, cmd)

			// If cursor moved off a line that contained APPNAME__ENABLED, check state.
			if prevLine != "" && tab.editor.Line() != prevRow {
				appUpper := strings.ToUpper(tab.spec.App)
				eqIdx := strings.Index(prevLine, "=")
				if eqIdx > 0 && strings.TrimSpace(prevLine[:eqIdx]) == appUpper+"__ENABLED" {
					if refreshCmd := m.checkEnabledChanged(m.activeTab); refreshCmd != nil {
						cmds = append(cmds, refreshCmd)
					}
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// HandleContextMenuKey implements the ContextMenuKeyHandler interface so that
// AppModel delegates the context-menu key to the editor rather than showing the
// generic global menu. Mirrors the case displayengine.Keys.ContextMenu branch in Update.
func (m *TabbedVarsEditorModel) HandleContextMenuKey() (tea.Model, tea.Cmd, bool) {
	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		editor := m.tabs[m.activeTab].editor
		layout := displayengine.GetLayout()
		y := m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight + editor.CursorVisualRow() - editor.YOffset()
		x := m.lastOffsetX + layout.NestedLeftOffset() + editor.CursorVisualCol() + 1
		cmd := m.showContextMenuForClick(x, y)
		if cmd != nil {
			return m, cmd, true
		}
	}
	return m, nil, false
}

// TitleBarFocusable implementation is promoted from the embedded tui.
