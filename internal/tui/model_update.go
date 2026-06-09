package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui/components/streamvp"

	tea "charm.land/bubbletea/v2"
)

// Update implements tea.Model
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// Restore terminal immediately
			EmergencyShutdown()

			// Log and exit
			logger.FatalWithStackSkip(m.ctx, 2, "TUI Update Panic: %v", r)
		}
	}()

	var cmds []tea.Cmd

	if !m.ready {
		switch msg.(type) {
		case tea.WindowSizeMsg:
			// Allow resizing to proceed
		case EnvLoadDoneMsg, RefreshAppsListMsg, SubDialogMsg, SubDialogResultMsg,
			UniversalPromptMsg, ShowConfirmDialogMsg, ShowPromptDialogMsg, ShowMessageDialogMsg:
			// Allow data-loading, dialog triggers, and results to pass through
			// even if terminal size isn't yet synced.
		default:
			return m, nil
		}
	}

	if _, ok := msg.(widgetClearPressMsg); ok {
		m.panel.ClearPress()
		// Forward to dialog and screen so their title bar pressed states also clear,
		// even if a different dialog (e.g. help) was open when the tick fired.
		if m.dialog != nil {
			m.dialog, _ = m.dialog.Update(msg)
		}
		if m.activeScreen != nil {
			updated, _ := m.activeScreen.Update(msg)
			if sm, ok := updated.(ScreenModel); ok {
				m.activeScreen = sm
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case PanelCommandLockChangedMsg:
		m.updateExitLockedState(msg.Locked)
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

	case LockStateChangedMsg:
		// Broadcast lock changes to both the active screen and any open dialog
		// to ensure background items update even if a dialog has focus.
		if m.activeScreen != nil {
			_, cmd := m.activeScreen.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

	case togglePanelMsg:
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		// Sync focus with expansion state
		m.setPanelFocus(m.panel.expanded)
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
		return m, logger.BatchRecoverTUI(m.ctx, cmd)

	case menuSpinnerTickMsg, menuDeferredActionMsg:
		// Always route to the active screen — these are scoped by instanceID so a
		// dialog being open must not swallow them (dialogs don't own menu spinners).
		if m.activeScreen != nil {
			updated, cmd := m.activeScreen.Update(msg)
			if sm, ok := updated.(ScreenModel); ok {
				m.activeScreen = sm
			}
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}
		return m, nil

	case panelSpinnerTickMsg:
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		return m, logger.BatchRecoverTUI(m.ctx, cmd)

	case streamvp.SpinnerTickMsg:
		// Route by tag: panel gets its own tick, dialog gets its own — don't steal across components.
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		var dialogCmd tea.Cmd
		if m.dialog != nil {
			m.dialog, dialogCmd = m.dialog.Update(msg)
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmd, dialogCmd)

	case panelLineMsg:
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		return m, logger.BatchRecoverTUI(m.ctx, cmd)

	case consoleLinesMsg:
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		m.updateExitLocked()
		return m, logger.BatchRecoverTUI(m.ctx, cmd)

	case consoleDoneMsg:
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		if msg.configChanged {
			conf := config.LoadAppConfig()
			cmd = tea.Batch(cmd, func() tea.Msg { return ConfigChangedMsg{Config: conf} })
		}
		if msg.appsChanged {
			cmd = tea.Batch(cmd, func() tea.Msg { return RefreshAppsListMsg{} })
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmd)

	case ScrollDoneMsg:
		if m.activeScreen != nil {
			updated, cmd := m.activeScreen.Update(msg)
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}

	case DragDoneMsg:
		if msg.ID == resizeZoneID {
			updated, cmd := m.panel.Update(msg)
			m.panel = updated.(PanelModel)
			m.refreshPanelLayout()
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}

	case tea.KeyMsg:
		if model, cmd, handled := m.handleKeyMsg(msg); handled {
			return model, logger.BatchRecoverTUI(m.ctx, cmd)
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
			return m, logger.BatchRecoverTUI(m.ctx, resCmd)
		}
		if resCmd != nil {
			cmds = append(cmds, resCmd)
		}
		// Fall through to common update logic (backdrop and activeScreen)

	case ShowGlobalFlagsMsg:
		return m, func() tea.Msg { return ShowDialogMsg{Dialog: NewFlagsToggleDialog()} }

	case ShowPendingRestartMsg:
		return m, showPendingRestartDialog(m.ctx)


	case TriggerHelpMsg:
		return m, m.showHelpCmd(msg.CapturedContext, msg.ScreenLevelOnly)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Size the active screen first so calculateLayout populates SubtitleHeight (needed by MinHeight).
		{
			caW, caH := m.getContentArea()
			if m.activeScreen != nil {
				m.activeScreen.SetSize(caW, caH)
			}
		}
		// Apply screen-aware log panel ceiling (may snap height down).
		m.applyPanelMax()
		// Update log panel with full dimensions (so Height() is correct for backdrop)
		m.panel.SetSize(m.width, m.height)

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
				cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, dialogCmd))
			}
		} else if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
			updated, screenCmd := m.activeScreen.Update(contentSizeMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			if screenCmd != nil {
				cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, screenCmd))
			}
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

	case NavigateMsg:
		// Block navigation if any screen in the stack is still loading.
		type loader interface{ IsLoading() bool }
		if l, ok := m.activeScreen.(loader); ok && l.IsLoading() {
			return m, logger.BatchRecoverTUI(m.ctx, cmds...)
		}
		for _, s := range m.screenStack {
			if l, ok := s.(loader); ok && l.IsLoading() {
				return m, logger.BatchRecoverTUI(m.ctx, cmds...)
			}
		}
		if msg.Screen != nil && msg.Screen.IsDestructive() {
			if !sessionlocks.Sessions.AcquireEditLock(m.clientIP, msg.Screen.Title(), "menu", m.connType) {
				info := sessionlocks.Sessions.ReadEditInfo()
				busyMsg := editLockBusyMsg(info, msg.Screen.Title())
				return m, func() tea.Msg {
					return ShowMessageDialogMsg{
						Title:   "Resource Busy",
						Message: busyMsg,
						Type:    MessageError,
					}
				}
			}
			// Lock acquired or already held — update ConnType to reflect current screen.
			sessionlocks.Sessions.UpdateEditLockConnType(msg.Screen.Title())
		}

		// Push current screen to stack and switch to new screen.
		// Clear its spinner first — it kept spinning while the cmd built the new screen.
		if m.activeScreen != nil {
			if cp, ok := m.activeScreen.(interface{ ClearProcessingState() }); ok {
				cp.ClearProcessingState()
			}
			m.screenStack = append(m.screenStack, m.activeScreen)
		}
		m.activeScreen = msg.Screen
		if m.activeScreen != nil {
			CurrentPageName = m.activeScreen.MenuName()
			// Size the screen first so calculateLayout populates SubtitleHeight (needed by MinHeight).
			caW, caH := m.getContentArea()
			m.activeScreen.SetSize(caW, caH)
			// Re-apply log panel ceiling with accurate MinHeight data; snap if needed.
			if m.applyPanelMax() {
				m.panel.SetSize(m.width, m.height)
				m.backdrop.SetSize(m.width, m.backdropHeight())
				// Re-size with updated content area after panel snap.
				caW, caH = m.getContentArea()
				m.activeScreen.SetSize(caW, caH)
			}
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
			cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, m.activeScreen.Init()))
			m.updateComponentFocus()

			// Sync current lock state to the new screen immediately
			if m.lockedByOthers {
				cmds = append(cmds, func() tea.Msg { return LockStateChangedMsg{LockedByOthers: true} })
			}
		}
		updateRestartSafeMarker()
		checkPendingRestart(m.ctx)
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

	case NavigateBackMsg:
		if m.activeScreen != nil && m.activeScreen.IsDestructive() {
			// Check if the previous screen is also destructive — if so, update
			// the lock to reflect it rather than releasing.
			var prevScreen ScreenModel
			if len(m.screenStack) > 0 {
				prevScreen = m.screenStack[len(m.screenStack)-1]
			}
			if prevScreen != nil && prevScreen.IsDestructive() {
				sessionlocks.Sessions.UpdateEditLockConnType(prevScreen.Title())
			} else {
				sessionlocks.Sessions.ReleaseEditLock()
			}
		}

		// Pop from stack and return to previous screen
		if CurrentPageName == "tabbed_vars" {
			CurrentEditorApp = ""
		}
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			if m.activeScreen != nil {
				if cp, ok := m.activeScreen.(interface{ ClearProcessingState() }); ok {
					cp.ClearProcessingState()
				}
				CurrentPageName = m.activeScreen.MenuName()
				// Size the screen first so calculateLayout populates SubtitleHeight (needed by MinHeight).
				caW, caH := m.getContentArea()
				m.activeScreen.SetSize(caW, caH)
				// Re-apply log panel ceiling with accurate MinHeight data; snap if needed.
				if m.applyPanelMax() {
					m.panel.SetSize(m.width, m.height)
					m.backdrop.SetSize(m.width, m.backdropHeight())
					// Re-size with updated content area after panel snap.
					caW, caH = m.getContentArea()
					m.activeScreen.SetSize(caW, caH)
				}
				m.backdrop.SetHelpText(m.activeScreen.HelpText())

				// Sync current lock state to the restored screen immediately
				if m.lockedByOthers {
					cmds = append(cmds, func() tea.Msg { return LockStateChangedMsg{LockedByOthers: true} })
				}
			}
		} else {
			// If stack is empty, we "go back" to nothing (which triggers quit at the bottom)
			m.activeScreen = nil
			CurrentPageName = ""
		}

		// Check for application exit immediately if we just cleared the last screen and have no dialog.
		// This avoids the "No active screen" splotch and ensures ESC works for standalone tools.
		if m.ready && m.activeScreen == nil && m.dialog == nil {
			return m, m.exitOrWarn()
		}
		updateRestartSafeMarker()
		checkPendingRestart(m.ctx)
		if msg.Refresh && m.activeScreen != nil {
			return m, func() tea.Msg { return RefreshAppsListMsg{} }
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)
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
			cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, m.dialog.Init()))
			// Update lock ConnType to show the active screen + dialog context.
			if m.activeScreen != nil && m.activeScreen.IsDestructive() {
				connType := m.activeScreen.Title()
				if titled, ok := m.dialog.(interface{ Title() string }); ok {
					if t := titled.Title(); t != "" {
						connType += "|" + t
					}
				}
				sessionlocks.Sessions.UpdateEditLockConnType(connType)
			}
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

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
		return m, logger.RecoverTUI(m.ctx, m.dialog.Init())

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
		return m, logger.BatchRecoverTUI(m.ctx, m.dialog.Init())

	case ShowPromptDialogMsg:
		// If a dialog is already open, push it to stack
		if m.dialog != nil {
			// Never push context menus to the stack
			if _, ok := m.dialog.(*ContextMenuModel); !ok {
				m.dialogStack = append(m.dialogStack, m.dialog)
			}
		}

		// Show prompt dialog as the main dialog
		dialog := newPromptDialog(msg.Title, msg.Question, msg.Sensitive, msg.InitialValue)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		m.pendingPrompt = msg.ResultChan
		return m, logger.RecoverTUI(m.ctx, m.dialog.Init())

	case UniversalPromptMsg:
		// Smart routing for prompts
		// If we have an active ProgramBox, forward as a sub-dialog to keep the box visible and responsive.
		if _, ok := m.dialog.(*ProgramBoxModel); ok {
			var subModel tea.Model
			if msg.Type == PromptTypeConfirm {
				confirm := newConfirmDialog(msg.Title, msg.Question, msg.DefaultYes)
				confirm.onResult = func(r bool) tea.Msg { return SubDialogResultMsg{Result: r} }
				subModel = confirm
			} else {
				prompt := newPromptDialog(msg.Title, msg.Question, msg.Sensitive, msg.InitialValue)
				prompt.onResult = func(r string, c bool) tea.Msg {
					return SubDialogResultMsg{Result: promptResultMsg{result: r, confirmed: c}}
				}
				subModel = prompt
			}
			return m, func() tea.Msg {
				return SubDialogMsg{
					Model: subModel,
					Chan:  msg.ResultChan,
				}
			}
		}

		// Otherwise, show as a normal global dialog
		if msg.Type == PromptTypeConfirm {
			return m, func() tea.Msg {
				return ShowConfirmDialogMsg{
					Title:      msg.Title,
					Question:   msg.Question,
					DefaultYes: msg.DefaultYes,
					ResultChan: msg.ResultChan.(chan bool),
				}
			}
		}
		return m, func() tea.Msg {
			return ShowPromptDialogMsg{
				Title:        msg.Title,
				Question:     msg.Question,
				Sensitive:    msg.Sensitive,
				InitialValue: msg.InitialValue,
				ResultChan:   msg.ResultChan.(chan promptResultMsg),
			}
		}

	case SubDialogResultMsg:
		// Forward result to the active dialog (likely a ProgramBox)
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}
		return m, nil

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
			cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, m.dialog.Init()))
		}
		return m, logger.BatchRecoverTUI(m.ctx, cmds...)

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

		// If the dialog handles its own sub-dialogs (e.g. ProgramBox), forward it
		// and return instead of clearing the main dialog.
		if pb, ok := m.dialog.(*ProgramBoxModel); ok && pb.subDialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}

		// Revert lock ConnType to the parent dialog+screen context (or just screen if no parent dialog).
		if m.activeScreen != nil && m.activeScreen.IsDestructive() {
			connType := m.activeScreen.Title()
			if len(m.dialogStack) > 0 {
				parent := m.dialogStack[len(m.dialogStack)-1]
				if titled, ok := parent.(interface{ Title() string }); ok {
					if t := titled.Title(); t != "" {
						connType += "|" + t
					}
				}
			}
			sessionlocks.Sessions.UpdateEditLockConnType(connType)
		}

		// Capture whether the closing dialog was a context menu before clearing
		_, closingContextMenu := m.dialog.(*ContextMenuModel)

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
					return m, func() tea.Msg { return fwd }
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
				// Blur the title bar of the restored dialog so widgets don't stay focused.
				if tb, ok := m.dialog.(TitleBarFocusable); ok {
					tb.BlurTitleBar()
				}
			}

			// ForwardToParent: deliver Result to the restored parent dialog.
			if msg.ForwardToParent && shouldForwardResult(msg.Result) {
				fwd := msg.Result
				return m, (func() tea.Msg { return fwd })
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
			return m, m.exitOrWarn()
		}

		// When returning to screen: restore focus to the active screen.
		// Clear header focus so version numbers don't stay selected after a dialog closes.
		// Exception: context menu opened from a header widget — preserve focus so ESC
		// closes the menu first; a second ESC then unfocuses the widget.
		// Blur any title bar that may have been focused before the dialog opened.
		if !closingContextMenu {
			m.setHeaderFocus(HeaderFocusNone)
			if m.activeScreen != nil {
				if tb, ok := m.activeScreen.(TitleBarFocusable); ok {
					tb.BlurTitleBar()
				}
			}
			m.updateComponentFocus()
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
					return m, func() tea.Msg { return fwd }
				}
			}
		}
		return m, nil

	case UpdateHeaderMsg:
		m.backdrop.header.SyncFlags()
		return m, nil

	case ConfigChangedMsg:
		m.config = msg.Config
		console.SpinnerEnabled = msg.Config.UI.Spinner
		console.SpinnerSpeed = msg.Config.UI.SpinnerSpeed
		console.LineCharacters = msg.Config.UI.LineCharacters
		_, _ = theme.Load(m.config.UI.Theme, "")
		m.invalidateAllCaches()
		m.backdrop.header.SyncFlags()
		updated, _ := m.panel.Update(msg)
		m.panel = updated.(PanelModel)

		// Manually trigger sizing to avoid the complexities of tea.WindowSizeMsg re-triggering
		m.backdrop.SetSize(m.width, m.backdropHeight())
		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
			// Forward so screens like DisplayOptionsScreen can reload preview-namespace styles
			// that were cleared by InitStyles → ClearSemanticCache above.
			_, cmd := m.activeScreen.Update(msg)
			return m, logger.BatchRecoverTUI(m.ctx, cmd)
		}
		return m, nil

	case QuitMsg:
		return m, tea.Quit

	case ConsoleLockMsg:
		// remote.lock never locks the local console bar (it just indicates session presence)
		if msg.ID == "remote.lock" {
			return m, nil
		}

		// edit.lock state sync
		if msg.ID == "edit.lock" {
			// Only lock menu items when a genuinely external session holds the lock.
			lockedByOthers := msg.Locked && !sessionlocks.Sessions.HoldEditLockLocal()
			if lockedByOthers != m.lockedByOthers {
				m.lockedByOthers = lockedByOthers
				cmds = append(cmds, func() tea.Msg { return LockStateChangedMsg{LockedByOthers: lockedByOthers} })
			}
			return m, logger.BatchRecoverTUI(m.ctx, cmds...)
		}

		return m, nil
	}

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
			cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, dialogCmd))
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
			cmds = append(cmds, logger.BatchRecoverTUI(m.ctx, screenCmd))
		}
		// Update helpline from screen
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
	}

	// Check for application exit when both activeScreen and dialog are nil
	// This happens when NavigateBack is used on the "root" screen.
	// We wait until the end of Update to handle batches (e.g. ShowDialog + NavigateBack)
	if m.ready && m.activeScreen == nil && m.dialog == nil {
		allCmds := make([]tea.Cmd, 0, len(cmds)+1)
		allCmds = append(allCmds, m.exitOrWarn())
		allCmds = append(allCmds, cmds...)
		return m, logger.BatchRecoverTUI(m.ctx, allCmds...)
	}

	m.updateExitLocked()
	return m, logger.BatchRecoverTUI(m.ctx, cmds...)
}

// setPanelFocus updates panelFocused and tells the active screen to
// unfocus/refocus its border accordingly (if it supports the interface).
func (m *AppModel) setPanelFocus(focused bool) {
	m.panelFocused = focused
	m.panelTitleFocused = false
	m.panel.focused = focused
	m.panel.BlurTitleBar()
	if focused {
		m.backdrop.header.SetFocus(HeaderFocusNone)
	} else {
		m.panel.input.Blur()
		m.panel.inputFocused = false
	}
	m.updateComponentFocus()
}

func (m *AppModel) setPanelTitleFocus(focused bool) {
	m.panelTitleFocused = focused
	if focused {
		m.panel.FocusTitleBar()
		m.panel.SetWidget(panelWidgetUp) // default to [▲]
		m.panelFocused = false
		m.panel.focused = false
		m.panel.input.Blur()
		m.panel.inputFocused = false
		m.backdrop.header.SetFocus(HeaderFocusNone)
	} else {
		m.panel.BlurTitleBar()
	}
	m.updateComponentFocus()
}

func (m *AppModel) setHeaderFocus(focus HeaderFocus) {
	m.backdrop.header.SetFocus(focus)
	m.backdrop.InvalidateBackdropCache()
	if focus != HeaderFocusNone {
		m.panelFocused = false
		m.panel.focused = false
		m.panelTitleFocused = false
		m.panel.BlurTitleBar()
	}
	m.updateComponentFocus()
}

func (m *AppModel) updateComponentFocus() {
	dialogOpen := m.dialog != nil
	headerFocused := m.backdrop.header.GetFocus() != HeaderFocusNone

	_, dialogIsProgramBox := m.dialog.(*ProgramBoxModel)
	panelBlockedByDialog := dialogOpen && !dialogIsProgramBox

	// Log panel only keeps its "internal" focus state if no modal dialog is blocking it.
	// Program boxes are non-modal — the panel can be focused alongside them.
	m.panel.focused = m.panelFocused && !panelBlockedByDialog
	if m.panelTitleFocused && !panelBlockedByDialog {
		if !m.panel.TitleBarFocused() {
			m.panel.FocusTitleBar()
		}
	} else if !m.panelTitleFocused || panelBlockedByDialog {
		if m.panel.TitleBarFocused() {
			m.panel.BlurTitleBar()
		}
	}

	// Screen is focused only if no dialog is open AND neither panel nor header have focus.
	// Exception: context menus are lightweight overlays — keep the screen focused so the
	// selected item stays highlighted while the context menu is visible.
	_, dialogIsContextMenu := m.dialog.(*ContextMenuModel)
	if m.activeScreen != nil {
		if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused((!dialogOpen || dialogIsContextMenu) && !m.panelFocused && !m.panelTitleFocused && !headerFocused)
		}
	}

	// Dialog focus: modal dialogs are always focused; program boxes yield to the panel.
	if m.dialog != nil {
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			_, isProgramBox := m.dialog.(*ProgramBoxModel)
			if isProgramBox {
				focusable.SetFocused(!m.panelFocused && !m.panelTitleFocused)
			} else {
				focusable.SetFocused(true)
			}
		}
	}
}

// invalidateAllCaches clears every render cache in the TUI so a full redraw
// occurs on the next frame. Call this before InitStyles on any config change.
func (m *AppModel) invalidateAllCaches() {
	InitStyles(m.config)
	invalidateShadowCache()
	m.backdrop.InvalidateBackdropCache()
}

// applyPanelMax computes the maximum log panel height for the current active screen,
// updates the log panel's ceiling, and snaps the log panel down if it now exceeds the new max.
// Returns true if the log panel height changed (caller should resize the active screen/dialog).
func (m *AppModel) applyPanelMax() bool {
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

	m.panel.SetMaxHeight(maxLogH)

	// Snap down if the current height exceeds the new ceiling.
	if m.panel.expanded && m.panel.height > maxLogH {
		m.panel.height = maxLogH
		m.panel.SetSize(m.width, m.height)
		return true
	}
	return false
}

// backdropHeight returns the height available for the backdrop (terminal minus log panel).
func (m AppModel) backdropHeight() int {
	return m.height - m.panel.Height()
}

// updateExitLocked pushes the current panel command-in-progress state to the active screen's menu.
func (m *AppModel) updateExitLocked() {
	m.updateExitLockedState(m.panel.CommandInProgress())
}

// updateExitLockedState pushes an explicit locked state to the active screen's menu.
// Use this when you have the authoritative state from the message rather than re-querying
// the panel (which may have already been updated by a subsequent unlock).
func (m *AppModel) updateExitLockedState(locked bool) {
	if screen, ok := m.activeScreen.(interface{ SetCommandLocked(bool) }); ok {
		screen.SetCommandLocked(locked)
	} else if screen, ok := m.activeScreen.(interface{ SetExitLocked(bool) }); ok {
		screen.SetExitLocked(locked)
	}
	if screen, ok := m.activeScreen.(interface{ SetExitAction(func() tea.Cmd) }); ok {
		screen.SetExitAction(m.exitOrWarn)
	}
}

// exitOrWarn returns ConfirmExitAction normally, but blocks exit with a warning
// when a destructive console command is in progress to prevent data corruption.
func (m *AppModel) exitOrWarn() tea.Cmd {
	if m.panel.CommandInProgress() {
		return func() tea.Msg {
			return ShowMessageDialogMsg{
				Title:   "Cannot Exit",
				Message: "A command is currently running.\nPlease wait for it to finish before exiting.",
				Type:    MessageInfo,
			}
		}
	}
	return ConfirmExitAction()
}

// refreshPanelLayout updates the backdrop and active screen/dialog after the panel height changes.
// Call this after any ResizeBy() + applyPanelMax() sequence.
func (m *AppModel) refreshPanelLayout() {
	m.panel.SetSize(m.width, m.height)
	m.backdrop.SetSize(m.width, m.backdropHeight())
	caW, caH := m.getContentArea()
	if m.dialog != nil {
		if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
			dW, dH := m.getDialogArea(m.dialog)
			sizable.SetSize(dW, dH)
		}
	} else if m.activeScreen != nil {
		m.activeScreen.SetSize(caW, caH)
	}
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

	return layout.ContentArea(m.width, bh, hasShadow, false, headerH, helplineH)
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
		return layout.ContentArea(m.width, m.height, m.config.UI.Shadow, true, headerH, helplineH)
	}
	return m.getContentArea()
}

// handleKeyMsg processes keyboard input.
// Returns (model, cmd, handled) where handled indicates if the key was consumed.
