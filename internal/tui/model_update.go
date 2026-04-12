package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update implements tea.Model
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// Suppress further panics during recovery
			defer func() { _ = recover() }()

			// Restore terminal
			Shutdown()
			console.SetTUIEnabled(false)

			// Check if it's already a FatalError
			if _, ok := r.(logger.FatalError); ok {
				return
			}

			// Report as fatal
			logger.FatalWithStackSkip(m.ctx, 2, "TUI Update Panic: %v", r)
		}
	}()

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case toggleLogPanelMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		// Sync focus with expansion state
		m.setLogPanelFocus(m.logPanel.expanded)
		// Resize backdrop, screen, and dialog to match new panel height
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
		}
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				dW, dH := m.getDialogArea(m.dialog)
				sizable.SetSize(dW, dH)
			}
		}
		return m, m.wrap(cmd)

	case logLineMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		return m, m.wrap(cmd)

	case DragDoneMsg:
		if msg.ID == logResizeZoneID {
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			m.backdrop.SetSize(m.width, m.backdropHeight())
			return m, m.wrap(cmd)
		}

	case tea.KeyMsg:
		if model, cmd, handled := m.handleKeyMsg(msg); handled {
			return model, m.wrap(cmd)
		}

	case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		// Convert to tea.MouseMsg interface for the handler
		resModel, resCmd, handled := m.handleMouseMsg(msg.(tea.MouseMsg))
		m = resModel.(*AppModel) // Preserve any state changes from handleMouseMsg
		if handled {
			// Update helpline on click/wheel/release — selection can change.
			// Skip on motion: hover doesn't change the active item, and motion
			// events fire at high frequency, making this a hot path over SSH.
			if _, isMotion := msg.(tea.MouseMotionMsg); !isMotion {
				if m.dialog != nil {
					if h, ok := m.dialog.(interface{ HelpText() string }); ok {
						m.backdrop.SetHelpText(h.HelpText())
					}
				} else if m.activeScreen != nil {
					m.backdrop.SetHelpText(m.activeScreen.HelpText())
				}
			}
			return m, m.wrap(resCmd)
		}
		if resCmd != nil {
			cmds = append(cmds, resCmd)
		}
		// Fall through to common update logic (backdrop and activeScreen)

	case ShowGlobalFlagsMsg:
		return m, m.wrap(func() tea.Msg { return ShowDialogMsg{Dialog: NewFlagsToggleDialog()} })

	case TriggerHelpMsg:
		return m, m.showHelpCmd(msg.CapturedContext)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Apply screen-aware log panel ceiling first (may snap height down).
		m.applyLogPanelMax()
		// Update log panel with full dimensions (so Height() is correct for backdrop)
		m.logPanel.SetSize(m.width, m.height)

		// Update backdrop with adjusted height so helpline is visible above log panel
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		contentSizeMsg := tea.WindowSizeMsg{Width: caW, Height: caH}

		// Update dialog or active screen with content area dimensions
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				dW, dH := m.getDialogArea(m.dialog)
				sizable.SetSize(dW, dH)
			}
			var dialogCmd tea.Cmd
			m.dialog, dialogCmd = m.dialog.Update(contentSizeMsg)
			if dialogCmd != nil {
				cmds = append(cmds, dialogCmd)
			}
		} else if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
			updated, screenCmd := m.activeScreen.Update(contentSizeMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			if screenCmd != nil {
				cmds = append(cmds, screenCmd)
			}
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
		}
		return m, m.wrap(cmds...)

	case NavigateMsg:
		// Push current screen to stack and switch to new screen
		if m.activeScreen != nil {
			m.screenStack = append(m.screenStack, m.activeScreen)
		}
		m.activeScreen = msg.Screen
		if m.activeScreen != nil {
			CurrentPageName = m.activeScreen.MenuName()
			// Re-apply log panel ceiling for the new screen; snap if needed.
			if m.applyLogPanelMax() {
				m.logPanel.SetSize(m.width, m.height)
				m.backdrop.SetSize(m.width, m.backdropHeight())
			}
			caW, caH := m.getContentArea()
			m.activeScreen.SetSize(caW, caH)
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
			cmds = append(cmds, m.activeScreen.Init())
		}
		return m, m.wrap(cmds...)

	case NavigateBackMsg:
		// Pop from stack and return to previous screen
		if CurrentPageName == "tabbed_vars" {
			CurrentEditorApp = ""
		}
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			if m.activeScreen != nil {
				CurrentPageName = m.activeScreen.MenuName()
				// Re-apply log panel ceiling for the restored screen; snap if needed.
				if m.applyLogPanelMax() {
					m.logPanel.SetSize(m.width, m.height)
					m.backdrop.SetSize(m.width, m.backdropHeight())
				}
				caW, caH := m.getContentArea()
				m.activeScreen.SetSize(caW, caH)
				m.backdrop.SetHelpText(m.activeScreen.HelpText())
			}
		} else {
			// If stack is empty, we "go back" to nothing (which triggers quit at the bottom)
			m.activeScreen = nil
			CurrentPageName = ""
		}

		// Check for application exit immediately if we just cleared the last screen and have no dialog.
		// This avoids the "No active screen" splotch and ensures ESC works for standalone tools.
		if m.ready && m.activeScreen == nil && m.dialog == nil {
			return m, ConfirmExitAction()
		}
		if msg.Refresh && m.activeScreen != nil {
			return m, func() tea.Msg { return RefreshAppsListMsg{} }
		}
		return m, nil
	case ShowDialogMsg:
		// Push current dialog to stack if one exists
		if m.dialog != nil {
			// Never push context menus to the stack; they should always be discarded when a new dialog opens.
			if _, ok := m.dialog.(*ContextMenuModel); !ok {
				m.dialogStack = append(m.dialogStack, m.dialog)
			}
		}

		// Safeguard: Prevent pushing the active screen as its own dialog
		if msg.Dialog == m.activeScreen {
			return m, nil
		}

		m.dialog = msg.Dialog
		if m.dialog != nil {
			// Ensure MenuModels are marked as dialogs so they render with shadows
			if menu, ok := m.dialog.(*MenuModel); ok {
				menu.SetIsDialog(true)
			}
			dW, dH := m.getDialogArea(m.dialog)
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(dW, dH)
			}
			m.updateComponentFocus()
			cmds = append(cmds, m.dialog.Init())
		}
		return m, m.wrap(cmds...)

	case ShowConfirmDialogMsg:
		// If a dialog is already open, push it to stack and show the confirm dialog as the new top
		if m.dialog != nil {
			// Never push context menus to the stack
			if _, ok := m.dialog.(*ContextMenuModel); !ok {
				m.dialogStack = append(m.dialogStack, m.dialog)
			}
		}

		// Show it as the main dialog (top of stack)
		dialog := newConfirmDialog(msg.Title, msg.Question, msg.DefaultYes)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		m.pendingConfirm = msg.ResultChan
		return m, m.wrap(m.dialog.Init())

	case ShowMessageDialogMsg:
		// If a dialog is already open, push it to stack
		if m.dialog != nil {
			// Never push context menus to the stack
			if _, ok := m.dialog.(*ContextMenuModel); !ok {
				m.dialogStack = append(m.dialogStack, m.dialog)
			}
		}

		// Show message dialog as the main dialog
		dialog := newMessageDialog(msg.Title, msg.Message, msg.Type)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		return m, m.dialog.Init()

	case ShowPromptDialogMsg:
		// If a dialog is already open, push it to stack
		if m.dialog != nil {
			// Never push context menus to the stack
			if _, ok := m.dialog.(*ContextMenuModel); !ok {
				m.dialogStack = append(m.dialogStack, m.dialog)
			}
		}

		// Show prompt dialog as the main dialog
		dialog := newPromptDialog(msg.Title, msg.Question, msg.Sensitive)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		m.pendingPrompt = msg.ResultChan
		return m, m.wrap(m.dialog.Init())

	case FinalizeSelectionMsg:
		// Atomically clear/navigate and show dialog to avoid race conditions in batches
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
		} else {
			m.activeScreen = nil
		}

		// Show the new dialog
		if msg.Dialog != nil && msg.Dialog != m.activeScreen {
			m.dialog = msg.Dialog
			dW, dH := m.getDialogArea(m.dialog)
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(dW, dH)
			}
			m.updateComponentFocus()
			cmds = append(cmds, m.dialog.Init())
		}
		return m, m.wrap(cmds...)

	case CloseDialogMsg:
		// If we're waiting for a confirmation, send the result
		if m.pendingConfirm != nil {
			if b, ok := msg.Result.(bool); ok {
				m.pendingConfirm <- b
			} else {
				m.pendingConfirm <- false
			}
			m.pendingConfirm = nil
		}

		// If we're waiting for a prompt, send the result
		if m.pendingPrompt != nil {
			if r, ok := msg.Result.(promptResultMsg); ok {
				m.pendingPrompt <- r
			} else {
				m.pendingPrompt <- promptResultMsg{confirmed: false}
			}
			m.pendingPrompt = nil
		}

		// Clear current dialog and try to pop from stack
		m.dialog = nil
		if len(m.dialogStack) > 0 {
			if shouldForwardResult(msg.Result) && !msg.ForwardToParent {
				// Result targets the active screen (e.g. ApplyVarValueMsg from a context
				// submenu): drain the entire stack so all menus close, then forward.
				m.dialogStack = nil
				m.updateComponentFocus()
				if m.activeScreen != nil {
					fwd := msg.Result
					return m, m.wrap(fwd)
				}
				return m, nil
			}

			// Pop parent dialog from stack and restore it.
			m.dialog = m.dialogStack[len(m.dialogStack)-1]
			m.dialogStack = m.dialogStack[:len(m.dialogStack)-1]

			// Re-focus and re-size the popped dialog
			if m.dialog != nil {
				dW, dH := m.getDialogArea(m.dialog)
				if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
					sizable.SetSize(dW, dH)
				}
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			}

			// ForwardToParent: deliver Result to the restored parent dialog.
			if msg.ForwardToParent && shouldForwardResult(msg.Result) {
				fwd := msg.Result
				return m, m.wrap(fwd)
			}
			return m, nil
		}

		// Fallback: if stack is empty and no screen, we quit.
		if m.activeScreen == nil {
			// If the dialog signaled success (e.g. "OK" button pressed), skip confirmation and quit immediately.
			// Do NOT set m.dialog = nil yet, so the caller can still inspect the final state (errors, etc).
			if result, ok := msg.Result.(bool); ok && result {
				return m, tea.Quit
			}
			return m, ConfirmExitAction()
		}

		// When returning to screen: restore focus to the active screen.
		m.updateComponentFocus()
		// If header was already focused (e.g. status bar flags), KEEP it focused.
		// Only clear header focus if it wasn't already focused.
		if m.backdrop.header.GetFocus() == HeaderFocusNone {
			m.backdrop.header.SetFocus(HeaderFocusNone) // No-op, but for clarity
		}
		// Forward any followup message or command piggybacked on Result (e.g. from context menus).
		// Skip the known confirm (bool) and prompt (promptResultMsg) types already handled above.
		if msg.Result != nil && m.activeScreen != nil {
			fwd := msg.Result
			if cmd, ok := fwd.(tea.Cmd); ok {
				return m, cmd
			}
			if _, isBool := fwd.(bool); !isBool {
				if _, isPrompt := fwd.(promptResultMsg); !isPrompt {
					return m, m.wrap(fwd)
				}
			}
		}
		return m, nil

	case UpdateHeaderMsg:
		m.backdrop.header.SyncFlags()
		return m, nil

	case ConfigChangedMsg:
		m.config = msg.Config
		InitStyles(m.config)
		m.backdrop.header.SyncFlags()

		// Manually trigger sizing to avoid the complexities of tea.WindowSizeMsg re-triggering
		m.backdrop.SetSize(m.width, m.backdropHeight())
		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
			// Forward so screens like DisplayOptionsScreen can reload preview-namespace styles
			// that were cleared by InitStyles → ClearSemanticCache above.
			return m, m.wrap(cmd)
		}
		return m, nil

	case QuitMsg:
		return m, tea.Quit
	}

	// Update backdrop
	// If a dialog is open, filter out key events to prevent background interaction (like re-triggering header actions)
	backdropMsg := msg
	if m.dialog != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			backdropMsg = nil
		}
	}

	if backdropMsg != nil {
		var backdropCmd tea.Cmd
		backdropModel, backdropCmd := m.backdrop.Update(backdropMsg)
		m.backdrop = backdropModel.(*BackdropModel)
		if backdropCmd != nil {
			cmds = append(cmds, backdropCmd)
		}
	}

	// Update dialog if present
	if m.dialog != nil {
		var dialogCmd tea.Cmd
		m.dialog, dialogCmd = m.dialog.Update(msg)
		if dialogCmd != nil {
			cmds = append(cmds, dialogCmd)
		}
		// If dialog supports help text, update backdrop
		if h, ok := m.dialog.(interface{ HelpText() string }); ok {
			m.backdrop.SetHelpText(h.HelpText())
		}
	} else if m.activeScreen != nil {
		var screenCmd tea.Cmd
		updated, screenCmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		if screenCmd != nil {
			cmds = append(cmds, screenCmd)
		}
		// Update helpline from screen
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
	}

	// Check for application exit when both activeScreen and dialog are nil
	// This happens when NavigateBack is used on the "root" screen.
	// We wait until the end of Update to handle batches (e.g. ShowDialog + NavigateBack)
	if m.ready && m.activeScreen == nil && m.dialog == nil {
		allCmds := make([]any, 0, len(cmds)+1)
		allCmds = append(allCmds, ConfirmExitAction())
		for _, c := range cmds {
			allCmds = append(allCmds, c)
		}
		return m, m.wrap(allCmds...)
	}

	return m, m.wrap(cmds...)
}

// wrap is a helper to apply BatchRecoverTUI to all commands returned to Bubble Tea.
func (m *AppModel) wrap(cmds ...any) tea.Cmd {
	var finalCmds []tea.Cmd
	for _, c := range cmds {
		if c == nil {
			continue
		}
		switch v := c.(type) {
		case tea.Cmd:
			finalCmds = append(finalCmds, v)
		case tea.Msg:
			msg := v
			finalCmds = append(finalCmds, func() tea.Msg { return msg })
		}
	}
	if len(finalCmds) == 0 {
		return nil
	}
	return logger.BatchRecoverTUI(m.ctx, finalCmds...)
}

// setLogPanelFocus updates logPanelFocused and tells the active screen to
// unfocus/refocus its border accordingly (if it supports the interface).
func (m *AppModel) setLogPanelFocus(focused bool) {
	m.logPanelFocused = focused
	m.logPanel.focused = focused
	if focused {
		m.backdrop.header.SetFocus(HeaderFocusNone)
	}
	m.updateComponentFocus()
}

func (m *AppModel) setHeaderFocus(focus HeaderFocus) {
	m.backdrop.header.SetFocus(focus)
	if focus != HeaderFocusNone {
		m.logPanelFocused = false
		m.logPanel.focused = false
	}
	m.updateComponentFocus()
}

func (m *AppModel) updateComponentFocus() {
	// Screen/Dialog is focused ONLY if neither log panel nor header have focus
	mainFocused := !m.logPanelFocused && m.backdrop.header.GetFocus() == HeaderFocusNone
	dialogOpen := m.dialog != nil

	if m.activeScreen != nil {
		if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
			// Screen is focused only if main area is focused AND no dialog is open
			focusable.SetFocused(mainFocused && !dialogOpen)
		}
	}
	if m.dialog != nil {
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(mainFocused)
		}
	}
}

// applyLogPanelMax computes the maximum log panel height for the current active screen,
// updates the log panel's ceiling, and snaps the log panel down if it now exceeds the new max.
// Returns true if the log panel height changed (caller should resize the active screen/dialog).
func (m *AppModel) applyLogPanelMax() bool {
	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow
	headerH := 1
	if m.backdrop != nil {
		headerH = m.backdrop.header.Height()
	}
	shadowH := 0
	if hasShadow {
		shadowH = layout.ShadowHeight
	}

	// Ask the active screen for its minimum height requirement (optional interface).
	minContentH := MinDialogHeight
	if m.activeScreen != nil {
		if mh, ok := m.activeScreen.(interface{ MinHeight() int }); ok {
			if h := mh.MinHeight(); h > minContentH {
				minContentH = h
			}
		}
	}

	maxLogH := m.height - layout.ChromeHeight(headerH) - layout.BottomChrome(layout.HelplineHeight) - shadowH - minContentH
	if maxLogH < 2 {
		maxLogH = 2
	}

	m.logPanel.SetMaxHeight(maxLogH)

	// Snap down if the current height exceeds the new ceiling.
	if m.logPanel.expanded && m.logPanel.height > maxLogH {
		m.logPanel.height = maxLogH
		m.logPanel.SetSize(m.width, m.height)
		return true
	}
	return false
}

// backdropHeight returns the height available for the backdrop (terminal minus log panel).
func (m AppModel) backdropHeight() int {
	return m.height - m.logPanel.Height()
}

// getContentArea returns the dimensions available for screens and dialogs.
func (m AppModel) getContentArea() (int, int) {
	// Use backdropHeight() to account for log panel
	bh := m.backdropHeight()
	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow
	headerH := 1
	helplineH := layout.HelplineHeight
	if m.backdrop != nil {
		headerH = m.backdrop.header.Height()
		helplineH = m.backdrop.HelplineActualHeight()
	}

	return layout.ContentArea(m.width, bh, hasShadow, headerH, helplineH)
}

// getDialogArea returns the dimensions available for a specific dialog.
// Help dialog uses the full terminal height, other dialogs use the backdrop's content area.
func (m AppModel) getDialogArea(d tea.Model) (int, int) {
	if _, isHelp := d.(*HelpDialogModel); isHelp {
		// Use Layout helpers directly to get full-screen content area for help
		layout := GetLayout()
		headerH := 1
		helplineH := layout.HelplineHeight
		if m.backdrop != nil {
			headerH = m.backdrop.ChromeHeight() - 1
			helplineH = m.backdrop.HelplineActualHeight()
		}
		return layout.ContentArea(m.width, m.height, m.config.UI.Shadow, headerH, helplineH)
	}
	return m.getContentArea()
}

// handleKeyMsg processes keyboard input.
// Returns (model, cmd, handled) where handled indicates if the key was consumed.
func (m *AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	// Specialized Help Blockade
	// If help is open, ANY key closes it and we return immediately to prevent leaks.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, m.wrap(cmd), true
		}
	}

	// Global Priority Actions (always work, regardless of focus)
	if key.Matches(msg, Keys.ToggleLog) {
		return m, m.wrap(func() tea.Msg { return toggleLogPanelMsg{} }), true
	}
	if key.Matches(msg, Keys.Help) || msg.String() == "?" {
		return m, m.wrap(m.showHelpCmd(m.focusedPanelHelpContext())), true
	}
	if key.Matches(msg, Keys.ForceQuit) {
		m.Fatal = true
		return m, m.wrap(tea.Quit), true
	}

	// Cycle: Screen -> LogPanel -> Header(Flags) -> Header(App) -> Header(Tmpl) -> Screen
	if key.Matches(msg, Keys.Tab) {
		if m.logPanelFocused {
			if m.dialog != nil {
				// Dialog open: Skip header, return focus to dialog
				m.setLogPanelFocus(false)
				return m, nil, true
			}
			m.setHeaderFocus(HeaderFocusFlags)
			return m, nil, true
		} else if m.dialog != nil {
			// Dialog open: pass Tab through to the dialog (not handled here).
			return m, nil, false
		} else if m.backdrop.header.GetFocus() == HeaderFocusFlags {
			m.setHeaderFocus(HeaderFocusApp)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.setHeaderFocus(HeaderFocusTmpl)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.setHeaderFocus(HeaderFocusNone)
			return m, nil, true
		} else {
			// From screen to log panel
			m.setLogPanelFocus(true)
			return m, nil, true
		}
	}

	if key.Matches(msg, Keys.ShiftTab) {
		if m.logPanelFocused {
			m.setLogPanelFocus(false)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusFlags {
			m.setLogPanelFocus(true)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.setHeaderFocus(HeaderFocusFlags)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.setHeaderFocus(HeaderFocusApp)
			return m, nil, true
		} else {
			// From screen to header (tmpl)
			if m.dialog != nil {
				// Dialog open: pass ShiftTab through to the dialog (not handled here).
				return m, nil, false
			}
			m.setHeaderFocus(HeaderFocusTmpl)
			return m, nil, true
		}
	}

	// Arrow Key Navigation within Header
	// We handle this regardless of m.dialog != nil because the header should trap its keys if focused
	if m.backdrop.header.GetFocus() != HeaderFocusNone {
		if key.Matches(msg, Keys.Right) {
			switch m.backdrop.header.GetFocus() {
			case HeaderFocusFlags:
				m.setHeaderFocus(HeaderFocusApp)
			case HeaderFocusApp:
				m.setHeaderFocus(HeaderFocusTmpl)
			}
			return m, nil, true
		}
		if key.Matches(msg, Keys.Left) {
			switch m.backdrop.header.GetFocus() {
			case HeaderFocusTmpl:
				m.setHeaderFocus(HeaderFocusApp)
			case HeaderFocusApp:
				m.setHeaderFocus(HeaderFocusFlags)
			}
			return m, nil, true
		}
		// Trap Up/Down keys so they don't leak to underlying screens/dialogs
		if key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) {
			return m, nil, true
		}
		// Escape to return to screen
		if key.Matches(msg, Keys.Esc) {
			m.setHeaderFocus(HeaderFocusNone)
			return m, nil, true
		}
	}

	// Handle Enter on focused header items
	if key.Matches(msg, Keys.Enter) && m.backdrop.header.GetFocus() != HeaderFocusNone {
		switch m.backdrop.header.GetFocus() {
		case HeaderFocusFlags:
			return m, m.wrap(ShowGlobalFlagsMsg{}), true
		case HeaderFocusApp:
			return m, m.wrap(TriggerAppUpdate()), true
		case HeaderFocusTmpl:
			return m, m.wrap(TriggerTemplateUpdate()), true
		}
	}

	// Focused Log Panel Actions
	// When log panel is focused, it gets all scroll/navigation keys exclusively
	// We handle this AFTER global cycling (Tab/ShiftTab) so we don't trap those keys.
	if m.logPanelFocused {
		// Esc unfocuses the panel and returns focus to the screen/dialog
		if key.Matches(msg, Keys.Esc) {
			m.setLogPanelFocus(false)
			return m, nil, true
		}
		// Enter or Space toggles the panel open/closed
		if key.Matches(msg, Keys.Enter) || msg.String() == " " {
			return m, m.wrap(toggleLogPanelMsg{}), true
		}
		// All other keys go to the panel viewport
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		return m, m.wrap(cmd), true
	}

	// Modal Dialog Support
	if m.dialog != nil {
		var cmd tea.Cmd
		m.dialog, cmd = m.dialog.Update(msg)
		// Ensure helpline reflects current state after update
		if h, ok := m.dialog.(interface{ HelpText() string }); ok {
			m.backdrop.SetHelpText(h.HelpText())
		}
		return m, m.wrap(cmd), true
	}

	// Active Screen Support (fallback)
	if m.activeScreen != nil {
		updated, cmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		// Ensure helpline reflects current state after update
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
		return m, m.wrap(cmd), true
	}

	return m, nil, false
}

// shouldForwardResult reports whether a CloseDialogMsg.Result needs to be
// forwarded to the active screen rather than silently dropped.
// Bool and promptResultMsg are consumed internally by pending channel listeners;
// everything else (e.g. ApplyVarValueMsg from a context menu) must reach the screen.
func shouldForwardResult(result any) bool {
	if result == nil {
		return false
	}
	if _, ok := result.(bool); ok {
		return false
	}
	if _, ok := result.(promptResultMsg); ok {
		return false
	}
	return true
}

// showHelpCmd returns a command that builds and shows the context-sensitive help dialog.
// focusedPanelHelpContext returns the HelpContext for a focused non-screen panel
// (log panel or header element), or nil so showHelpCmd falls through to the screen/dialog.
func (m *AppModel) focusedPanelHelpContext() *HelpContext {
	if m.logPanelFocused {
		for _, r := range m.logPanel.GetHitRegions(0, 0) {
			if r.Help != nil {
				return r.Help
			}
		}
	}
	focus := m.backdrop.header.GetFocus()
	if focus == HeaderFocusNone {
		return nil
	}
	var targetID string
	switch focus {
	case HeaderFocusFlags:
		targetID = IDHeaderFlags
	case HeaderFocusApp:
		targetID = IDAppVersion
	case HeaderFocusTmpl:
		targetID = IDTmplVersion
	}
	for _, r := range m.backdrop.header.GetHitRegions(0, 0) {
		if r.ID == targetID && r.Help != nil {
			return r.Help
		}
	}
	return nil
}

func (m *AppModel) showHelpCmd(capturedCtx *HelpContext) tea.Cmd {
	var km help.KeyMap = Keys
	var contextInfo HelpContext
	availW, availH := GetAvailableDialogSize(m.width, m.height)
	if availW < 40 || availH < 10 {
		// Terminal too small for help dialog
		return nil
	}
	helpContentWidth := HelpContextWidth(m.width, m.height)

	if capturedCtx != nil {
		contextInfo = *capturedCtx
		// Try to find a keymap from focus anyway, as context captured might not include km
		if m.dialog != nil {
			if h, ok := m.dialog.(help.KeyMap); ok {
				km = h
			}
		} else if m.activeScreen != nil {
			if h, ok := m.activeScreen.(help.KeyMap); ok {
				km = h
			}
		}
	} else if m.dialog != nil {
		if h, ok := m.dialog.(help.KeyMap); ok {
			km = h
		}
		if cp, ok := m.dialog.(HelpContextProvider); ok {
			contextInfo = cp.HelpContext(helpContentWidth)
		}
	} else if m.activeScreen != nil {
		if h, ok := m.activeScreen.(help.KeyMap); ok {
			km = h
		}
		if cp, ok := m.activeScreen.(HelpContextProvider); ok {
			contextInfo = cp.HelpContext(helpContentWidth)
		}
	}

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewHelpDialogWithContext(km, contextInfo)}
	}
}

// showGlobalContextMenu shows a context menu with global actions like Help.
func (m *AppModel) showGlobalContextMenu(x, y int, hit *HitRegion) tea.Cmd {
	var items []ContextMenuItem

	header := "Main Menu"
	if hit != nil {
		if hit.Help != nil && hit.Help.ItemTitle != "" {
			header = hit.Help.ItemTitle
		} else if hit.Help != nil && hit.Help.ScreenName != "" {
			header = hit.Help.ScreenName
		} else if hit.Label != "" {
			header = hit.Label
		} else {
			// Fallback to ID-based labels for regions not yet fully converted to metadata
			switch hit.ID {
			case IDAppVersion:
				header = "App Version"
			case IDTmplVersion:
				header = "Template Version"
			case IDHeaderFlags:
				header = "Global Flags"
			case IDStatusBar:
				header = "Status Bar"
			case IDLogPanel, IDLogViewport, IDLogToggle, IDLogResize:
				header = "Log Panel"
			}
		}
	}

	// For now, global menu is primarily for Help.
	items = append(items, ContextMenuItem{IsHeader: true, Label: header})
	items = append(items, ContextMenuItem{IsSeparator: true})
	// You could add "Refresh" or "App Version" here if they are useful as menu items.

	// Use the tail helper to add Clipboard and Help
	// (clipItems nil for now as global clipboard actions like Paste need a target)
	var hCtx *HelpContext
	if hit != nil {
		hCtx = hit.Help
	}
	items = AppendContextMenuTail(items, nil, hCtx)

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}
