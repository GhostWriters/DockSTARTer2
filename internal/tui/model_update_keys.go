package tui

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/update"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	// Specialized Help Blockade
	// If help is open, ANY key closes it and we return immediately to prevent leaks.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, logger.BatchRecoverTUI(m.ctx, cmd), true
		}
	}

	// Global Priority Actions (always work, regardless of focus)
	if key.Matches(msg, Keys.ToggleLog) {
		return m, func() tea.Msg { return togglePanelMsg{} }, true
	}
	if key.Matches(msg, Keys.FocusPanelTitle) {
		// Focus the title bar of whatever currently has focus — never silently fall through.
		if m.dialog != nil {
			// A dialog is open: toggle its title bar.
			if tb, ok := m.dialog.(TitleBarFocusable); ok {
				if tb.TitleBarFocused() {
					tb.BlurTitleBar()
				} else {
					tb.FocusTitleBar()
				}
				return m, nil, true
			}
		} else if (m.panelFocused || m.panelTitleFocused) && m.panel.panelMode != "none" {
			// Panel is focused (or its title bar is) — toggle panel title bar.
			if m.panelTitleFocused {
				m.setPanelTitleFocus(false)
				if m.panelInputWasFocused {
					m.panelInputWasFocused = false
					return m, m.panel.FocusInput(), true
				}
				m.setPanelFocus(true)
			} else {
				m.panelInputWasFocused = m.panel.inputFocused
				m.setPanelTitleFocus(true)
			}
			return m, nil, true
		} else if m.activeScreen != nil && m.backdrop.header.GetFocus() == HeaderFocusNone {
			// No dialog, no focused panel, no header focus: toggle active screen title bar.
			if tb, ok := m.activeScreen.(TitleBarFocusable); ok {
				if tb.TitleBarFocused() {
					tb.BlurTitleBar()
				} else {
					tb.FocusTitleBar()
				}
				return m, nil, true
			}
		}
		// No TitleBarFocusable target found — ignore.
	}
	if key.Matches(msg, Keys.Help) || msg.String() == "?" {
		return m, m.showHelpCmd(m.focusedPanelHelpContext(), false), true
	}
	if key.Matches(msg, Keys.ForceQuit) {
		if console.IsDaemon {
			// In server mode Ctrl-\ restarts the TUI rather than killing the daemon.
			return m, func() tea.Msg {
				reExecArgs := []string{"--server-daemon"}
				_ = update.ReExec(m.ctx, registeredExePath, reExecArgs)
				return nil
			}, true
		}
		m.Fatal = true
		return m, tea.Quit, true
	}

	// Cycle: Screen -> panel viewport -> Input bar -> Header(Flags) -> Header(App) -> Header(Tmpl) -> Screen
	if key.Matches(msg, Keys.Tab) {
		if m.panelFocused {
			// If panel is expanded and input not yet focused, Tab → input bar.
			if m.panel.expanded && !m.panel.inputFocused && !m.panel.sessionActive() {
				return m, m.panel.FocusInput(), true
			}
			// Viewport focused (inputFocused already handled above): Tab → exit panel to header / dialog.
			if m.dialog != nil {
				m.setPanelFocus(false)
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
			// From screen to panel viewport
			m.setPanelFocus(true)
			return m, nil, true
		}
	}

	if key.Matches(msg, Keys.ShiftTab) {
		if m.panelFocused {
			m.setPanelFocus(false)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusFlags {
			m.setPanelFocus(true)
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
			return m, func() tea.Msg { return ShowGlobalFlagsMsg{} }, true
		case HeaderFocusApp:
			if update.RestartPending {
				return m, func() tea.Msg { return ShowPendingRestartMsg{} }, true
			}
			return m, TriggerAppUpdate(), true
		case HeaderFocusTmpl:
			return m, TriggerTemplateUpdate(), true
		}
	}

	// Panel Title Bar Focus: keyboard resize actions.
	if m.panelTitleFocused {
		if key.Matches(msg, Keys.Esc) {
			m.setPanelTitleFocus(false)
			return m, nil, true
		}
		if key.Matches(msg, Keys.Left) {
			m.panel.SetWidget(panelWidgetUp)
			return m, nil, true
		}
		if key.Matches(msg, Keys.Right) {
			m.panel.SetWidget(panelWidgetDn)
			return m, nil, true
		}
		if key.Matches(msg, Keys.Enter) || msg.String() == " " {
			if m.panel.ActiveWidget() == panelWidgetDn {
				m.panel.ResizeBy(-1)
			} else {
				m.panel.ResizeBy(1)
			}
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		}
		const coarseDelta = 5
		switch {
		case key.Matches(msg, Keys.Up):
			m.panel.ResizeBy(1)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, Keys.Down):
			m.panel.ResizeBy(-1)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, Keys.PageUp) || msg.String() == "alt+up":
			m.panel.ResizeBy(coarseDelta)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, Keys.PageDown) || msg.String() == "alt+down":
			m.panel.ResizeBy(-coarseDelta)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		}
		return m, nil, true
	}

	// Focused Log Panel Actions
	// When log panel is focused, it gets all scroll/navigation keys exclusively.
	// We handle this AFTER global cycling (Tab/ShiftTab) so we don't trap those keys.
	if m.panelFocused {
		if m.panel.inputFocused {
			// Physical Tab/Shift+Tab only (not "."/","): cycle back to viewport.
			if kp, ok := msg.(tea.KeyPressMsg); ok && (kp.String() == "tab" || kp.String() == "shift+tab") {
				m.panel.input.Blur()
				m.panel.inputFocused = false
				return m, nil, true
			}
			// Input bar has focus: forward all keys (Esc handled inside the panel).
			updated, cmd := m.panel.Update(msg)
			m.panel = updated.(PanelModel)
			return m, logger.BatchRecoverTUI(m.ctx, cmd), true
		}
		// Viewport-focused: Esc unfocuses the panel.
		if key.Matches(msg, Keys.Esc) {
			m.setPanelFocus(false)
			return m, nil, true
		}
		// Tab/Shift+Tab: cycle to input bar (two-section dialog cycle).
		if key.Matches(msg, Keys.CycleTab) || key.Matches(msg, Keys.CycleShiftTab) {
			if m.panel.expanded && !m.panel.sessionActive() {
				return m, m.panel.FocusInput(), true
			}
		}
		// Enter focuses the input bar (if panel is expanded and not session-locked).
		if key.Matches(msg, Keys.Enter) {
			if m.panel.expanded && !m.panel.sessionActive() {
				return m, m.panel.FocusInput(), true
			}
			return m, func() tea.Msg { return togglePanelMsg{} }, true
		}
		// Space toggles the panel open/closed.
		if msg.String() == " " {
			return m, func() tea.Msg { return togglePanelMsg{} }, true
		}
		// All other keys go to the panel viewport.
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(PanelModel)
		return m, logger.BatchRecoverTUI(m.ctx, cmd), true
	}

	// Modal Dialog Support
	// Key events only go to the dialog if it is focused; non-key messages always pass through.
	if m.dialog != nil {
		_, isKey := msg.(tea.KeyPressMsg)
		_, isKeyRelease := msg.(tea.KeyReleaseMsg)
		dialogFocused := true
		if focusable, ok := m.dialog.(interface{ IsFocused() bool }); ok {
			dialogFocused = focusable.IsFocused()
		}
		if !isKey && !isKeyRelease || dialogFocused {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			// Ensure helpline reflects current state after update
			if h, ok := m.dialog.(interface{ HelpText() string }); ok {
				m.backdrop.SetHelpText(h.HelpText())
			}
			return m, logger.BatchRecoverTUI(m.ctx, cmd), true
		}
	}

	// Active Screen Support (fallback)
	if m.activeScreen != nil {
		updated, cmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		// Ensure helpline reflects current state after update
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
		return m, logger.BatchRecoverTUI(m.ctx, cmd), true
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

// focusedPanelHelpContext returns the HelpContext for a focused non-screen panel
// (log panel or header element), or nil so showHelpCmd falls through to the screen/dialog.
func (m *AppModel) focusedPanelHelpContext() *HelpContext {
	// Check title bar widget focus on active dialog or screen first.
	// Find the matching HitRegion by ID suffix so we get the screen-specific help text.
	widgetHelpFromHits := func(model interface{}) *HelpContext {
		wh, ok := model.(TitleBarWidgetHelper)
		if !ok {
			return nil
		}
		suffix := wh.FocusedWidgetID()
		if suffix == "" {
			return nil
		}
		hrp, ok := model.(HitRegionProvider)
		if !ok {
			return nil
		}
		for _, r := range hrp.GetHitRegions(0, 0) {
			if strings.HasSuffix(r.ID, "."+suffix) && r.Help != nil {
				return r.Help
			}
		}
		return nil
	}
	if m.dialog != nil {
		if h := widgetHelpFromHits(m.dialog); h != nil {
			return h
		}
	} else if m.activeScreen != nil {
		if h := widgetHelpFromHits(m.activeScreen); h != nil {
			return h
		}
	}

	if m.panelFocused {
		for _, r := range m.panel.GetHitRegions(0, 0) {
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

// showHelpCmd returns a command that builds and shows the context-sensitive help dialog.
func (m *AppModel) showHelpCmd(capturedCtx *HelpContext, screenLevelOnly bool) tea.Cmd {
	var km help.KeyMap = Keys
	var contextInfo HelpContext
	availW, availH := GetAvailableDialogSize(m.width, m.height, true)
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

	if screenLevelOnly {
		contextInfo.ItemTitle = ""
		contextInfo.ItemText = ""
		contextInfo.DocMarkdown = ""
		contextInfo.DocAppName = ""
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
			case IDPanel, IDPanelViewport, IDPanelToggle, IDPanelResize:
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
