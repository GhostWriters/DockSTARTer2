package tui

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
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

	// Context Menu Blockade
	// If a context menu is open, ALL keys go to it and we return immediately to
	// prevent keys (especially ESC) from leaking through to the underlying screen.
	if m.dialog != nil {
		if _, ok := m.dialog.(*displayengine.ContextMenuModel); ok {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, logger.BatchRecoverTUI(m.ctx, cmd), true
		}
	}

	// Global Priority Actions (always work, regardless of focus)
	if key.Matches(msg, displayengine.Keys.ToggleLog) {
		return m, func() tea.Msg { return displayengine.TogglePanelMsg{} }, true
	}
	if key.Matches(msg, displayengine.Keys.FocusPanelTitle) {
		// Focus the title bar of whatever currently has focus — never silently fall through.
		if m.dialog != nil {
			// A dialog is open: toggle its title bar.
			if tb, ok := m.dialog.(displayengine.TitleBarFocusable); ok {
				if tb.TitleBarFocused() {
					tb.BlurTitleBar()
				} else {
					tb.FocusTitleBar()
				}
				return m, nil, true
			}
		} else if (m.panelFocused || m.panelTitleFocused) && m.panel.PanelMode != "none" {
			// Panel is focused (or its title bar is) — toggle panel title bar.
			if m.panelTitleFocused {
				m.setPanelTitleFocus(false)
				if m.panelInputWasFocused {
					m.panelInputWasFocused = false
					return m, m.panel.FocusInput(), true
				}
				m.setPanelFocus(true)
			} else {
				m.panelInputWasFocused = m.panel.InputFocused
				m.setPanelTitleFocus(true)
			}
			return m, nil, true
		} else if m.activeScreen != nil && m.backdrop.Header.GetFocus() == displayengine.HeaderFocusNone {
			// No dialog, no focused panel, no header focus: toggle active screen title bar.
			if tb, ok := m.activeScreen.(displayengine.TitleBarFocusable); ok {
				if tb.TitleBarFocused() {
					tb.BlurTitleBar()
				} else {
					tb.FocusTitleBar()
				}
				return m, nil, true
			}
		}
		// No displayengine.TitleBarFocusable target found — ignore.
	}
	if key.Matches(msg, displayengine.Keys.Help) || msg.String() == "?" {
		return m, m.showHelpCmd(m.focusedPanelHelpContext(), false), true
	}
	if key.Matches(msg, displayengine.Keys.ContextMenu) {
		return m, m.showContextMenuCmd(), true
	}
	if key.Matches(msg, displayengine.Keys.ForceQuit) {
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
	if key.Matches(msg, displayengine.Keys.Tab) {
		if m.panelFocused {
			// If panel is expanded and input not yet focused, Tab → input bar.
			if m.panel.Expanded && !m.panel.InputFocused && !m.panel.SessionActive() {
				return m, m.panel.FocusInput(), true
			}
			// Viewport focused (inputFocused already handled above): Tab → exit panel to header / dialog.
			if m.dialog != nil {
				m.setPanelFocus(false)
				return m, nil, true
			}
			m.setHeaderFocus(displayengine.HeaderFocusFlags)
			return m, nil, true
		} else if m.dialog != nil {
			// Dialog open: pass Tab through to the dialog (not handled here).
			return m, nil, false
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusFlags {
			m.setHeaderFocus(displayengine.HeaderFocusApp)
			return m, nil, true
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusApp {
			m.setHeaderFocus(displayengine.HeaderFocusTmpl)
			return m, nil, true
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusTmpl {
			m.setHeaderFocus(displayengine.HeaderFocusNone)
			return m, nil, true
		} else {
			// From screen to panel viewport
			m.setPanelFocus(true)
			return m, nil, true
		}
	}

	if key.Matches(msg, displayengine.Keys.ShiftTab) {
		if m.panelFocused {
			m.setPanelFocus(false)
			return m, nil, true
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusFlags {
			m.setPanelFocus(true)
			return m, nil, true
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusApp {
			m.setHeaderFocus(displayengine.HeaderFocusFlags)
			return m, nil, true
		} else if m.backdrop.Header.GetFocus() == displayengine.HeaderFocusTmpl {
			m.setHeaderFocus(displayengine.HeaderFocusApp)
			return m, nil, true
		} else {
			// From screen to header (tmpl)
			if m.dialog != nil {
				// Dialog open: pass ShiftTab through to the dialog (not handled here).
				return m, nil, false
			}
			m.setHeaderFocus(displayengine.HeaderFocusTmpl)
			return m, nil, true
		}
	}

	// Arrow Key Navigation within Header
	// We handle this regardless of m.dialog != nil because the header should trap its keys if focused
	if m.backdrop.Header.GetFocus() != displayengine.HeaderFocusNone {
		if key.Matches(msg, displayengine.Keys.Right) {
			switch m.backdrop.Header.GetFocus() {
			case displayengine.HeaderFocusFlags:
				m.setHeaderFocus(displayengine.HeaderFocusApp)
			case displayengine.HeaderFocusApp:
				m.setHeaderFocus(displayengine.HeaderFocusTmpl)
			}
			return m, nil, true
		}
		if key.Matches(msg, displayengine.Keys.Left) {
			switch m.backdrop.Header.GetFocus() {
			case displayengine.HeaderFocusTmpl:
				m.setHeaderFocus(displayengine.HeaderFocusApp)
			case displayengine.HeaderFocusApp:
				m.setHeaderFocus(displayengine.HeaderFocusFlags)
			}
			return m, nil, true
		}
		// Trap Up/Down keys so they don't leak to underlying screens/dialogs
		if key.Matches(msg, displayengine.Keys.Up) || key.Matches(msg, displayengine.Keys.Down) {
			return m, nil, true
		}
		// Escape to return to screen
		if key.Matches(msg, displayengine.Keys.Esc) {
			m.setHeaderFocus(displayengine.HeaderFocusNone)
			return m, nil, true
		}
	}

	// Handle Enter on focused header items
	if key.Matches(msg, displayengine.Keys.Enter) && m.backdrop.Header.GetFocus() != displayengine.HeaderFocusNone {
		switch m.backdrop.Header.GetFocus() {
		case displayengine.HeaderFocusFlags:
			return m, func() tea.Msg { return ShowGlobalFlagsMsg{} }, true
		case displayengine.HeaderFocusApp:
			if update.RestartPending {
				return m, func() tea.Msg { return ShowPendingRestartMsg{} }, true
			}
			return m, TriggerAppUpdate(), true
		case displayengine.HeaderFocusTmpl:
			return m, TriggerTemplateUpdate(), true
		}
	}

	// Panel Title Bar Focus: keyboard resize actions.
	if m.panelTitleFocused {
		if key.Matches(msg, displayengine.Keys.Esc) {
			m.setPanelTitleFocus(false)
			return m, nil, true
		}
		if key.Matches(msg, displayengine.Keys.Left) {
			m.panel.SetWidget(displayengine.PanelWidgetUp)
			return m, nil, true
		}
		if key.Matches(msg, displayengine.Keys.Right) {
			m.panel.SetWidget(displayengine.PanelWidgetDn)
			return m, nil, true
		}
		if key.Matches(msg, displayengine.Keys.Enter) || msg.String() == " " {
			if m.panel.ActiveWidget() == displayengine.PanelWidgetDn {
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
		case key.Matches(msg, displayengine.Keys.EnvReorderU):
			m.panel.ResizeBy(coarseDelta)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, displayengine.Keys.EnvReorderD):
			m.panel.ResizeBy(-coarseDelta)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, displayengine.Keys.Up):
			m.panel.ResizeBy(1)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, displayengine.Keys.Down):
			m.panel.ResizeBy(-1)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, displayengine.Keys.PageUp):
			m.panel.ResizeBy(coarseDelta)
			m.applyPanelMax()
			m.refreshPanelLayout()
			return m, nil, true
		case key.Matches(msg, displayengine.Keys.PageDown):
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
		if m.panel.InputFocused {
			// Physical Tab/Shift+Tab only (not "."/","): cycle back to viewport.
			if kp, ok := msg.(tea.KeyPressMsg); ok && (kp.String() == "tab" || kp.String() == "shift+tab") {
				m.panel.Input.Blur()
				m.panel.InputFocused = false
				return m, nil, true
			}
			// Input bar has focus: forward all keys (Esc handled inside the panel).
			updated, cmd := m.panel.Update(msg)
			m.panel = updated.(displayengine.PanelModel)
			return m, logger.BatchRecoverTUI(m.ctx, cmd), true
		}
		// Viewport-focused: Esc unfocuses the panel.
		if key.Matches(msg, displayengine.Keys.Esc) {
			m.setPanelFocus(false)
			return m, nil, true
		}
		// Tab/Shift+Tab: cycle to input bar (two-section dialog cycle).
		if key.Matches(msg, displayengine.Keys.CycleTab) || key.Matches(msg, displayengine.Keys.CycleShiftTab) {
			if m.panel.Expanded && !m.panel.SessionActive() {
				return m, m.panel.FocusInput(), true
			}
		}
		// Enter focuses the input bar (if panel is expanded and not session-locked).
		if key.Matches(msg, displayengine.Keys.Enter) {
			if m.panel.Expanded && !m.panel.SessionActive() {
				return m, m.panel.FocusInput(), true
			}
			return m, func() tea.Msg { return displayengine.TogglePanelMsg{} }, true
		}
		// Space toggles the panel open/closed.
		if msg.String() == " " {
			return m, func() tea.Msg { return displayengine.TogglePanelMsg{} }, true
		}
		// All other keys go to the panel viewport.
		updated, cmd := m.panel.Update(msg)
		m.panel = updated.(displayengine.PanelModel)
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

// shouldForwardResult reports whether a displayengine.CloseDialogMsg.Result needs to be
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

// focusedPanelHelpContext returns the displayengine.HelpContext for a focused non-screen panel
// (log panel or header element), or nil so showHelpCmd falls through to the screen/dialog.
func (m *AppModel) focusedPanelHelpContext() *displayengine.HelpContext {
	// Check title bar widget focus on active dialog or screen first.
	// Find the matching displayengine.HitRegion by ID suffix so we get the screen-specific help text.
	widgetHelpFromHits := func(model interface{}) *displayengine.HelpContext {
		wh, ok := model.(displayengine.TitleBarWidgetHelper)
		if !ok {
			return nil
		}
		suffix := wh.FocusedWidgetID()
		if suffix == "" {
			return nil
		}
		hrp, ok := model.(displayengine.HitRegionProvider)
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
	focus := m.backdrop.Header.GetFocus()
	if focus == displayengine.HeaderFocusNone {
		return nil
	}
	var targetID string
	switch focus {
	case displayengine.HeaderFocusFlags:
		targetID = displayengine.IDHeaderFlags
	case displayengine.HeaderFocusApp:
		targetID = displayengine.IDAppVersion
	case displayengine.HeaderFocusTmpl:
		targetID = displayengine.IDTmplVersion
	}
	for _, r := range m.backdrop.Header.GetHitRegions(0, 0) {
		if r.ID == targetID && r.Help != nil {
			return r.Help
		}
	}
	return nil
}

// showHelpCmd returns a command that builds and shows the context-sensitive help dialog.
func (m *AppModel) showHelpCmd(capturedCtx *displayengine.HelpContext, screenLevelOnly bool) tea.Cmd {
	var km help.KeyMap = displayengine.Keys
	var contextInfo displayengine.HelpContext
	availW, availH := displayengine.GetAvailableDialogSize(m.width, m.height, true)
	if availW < 40 || availH < 10 {
		// Terminal too small for help dialog
		return nil
	}
	helpContentWidth := displayengine.HelpContextWidth(m.width, m.height)

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
		if cp, ok := m.dialog.(displayengine.HelpContextProvider); ok {
			contextInfo = cp.HelpContext(helpContentWidth)
		}
	} else if m.activeScreen != nil {
		if h, ok := m.activeScreen.(help.KeyMap); ok {
			km = h
		}
		if cp, ok := m.activeScreen.(displayengine.HelpContextProvider); ok {
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
		return displayengine.ShowDialogMsg{Dialog: NewHelpDialogWithContext(km, contextInfo)}
	}
}

// showContextMenuCmd builds a context menu for the currently focused element,
// using the same focus detection as showHelpCmd so both F1 and the context menu
// key always reflect the same focused item.
func (m *AppModel) showContextMenuCmd() tea.Cmd {
	x, y := m.width/2, m.height/2
	helpContentWidth := displayengine.HelpContextWidth(m.width, m.height)

	// Panel/header element has focus — use its displayengine.HelpContext, position near the element
	if panelCtx := m.focusedPanelHelpContext(); panelCtx != nil {
		r := displayengine.HitRegion{Help: panelCtx, Label: panelCtx.ScreenName}
		// Try to find the hit region to get its screen position
		for _, hr := range m.hitRegions {
			if hr.Help != nil && hr.Help.ScreenName == panelCtx.ScreenName && hr.Help.PageTitle == panelCtx.PageTitle {
				rx, ry := hr.X, hr.Y+hr.Height
				return m.showGlobalContextMenu(rx, ry, &r)
			}
		}
		return m.showGlobalContextMenu(x, y, &r)
	}

	// Dialog is open — use dialog's displayengine.HelpContext
	if m.dialog != nil {
		if cp, ok := m.dialog.(displayengine.HelpContextProvider); ok {
			ctx := cp.HelpContext(helpContentWidth)
			r := displayengine.HitRegion{Help: &ctx, Label: ctx.ScreenName}
			return m.showGlobalContextMenu(x, y, &r)
		}
		return m.showGlobalContextMenu(x, y, nil)
	}

	// Active screen — let it handle via HandleContextMenuKey if it can
	if m.activeScreen != nil {
		if ckh, ok := m.activeScreen.(interface {
			HandleContextMenuKey() (tea.Model, tea.Cmd, bool)
		}); ok {
			if updated, cmd, handled := ckh.HandleContextMenuKey(); handled {
				m.activeScreen = updated.(ScreenModel)
				return cmd
			}
		}
		// Fallback: use screen's displayengine.HelpContext for the global menu header
		if cp, ok := m.activeScreen.(displayengine.HelpContextProvider); ok {
			ctx := cp.HelpContext(helpContentWidth)
			r := displayengine.HitRegion{Help: &ctx, Label: ctx.ScreenName}
			return m.showGlobalContextMenu(x, y, &r)
		}
	}

	return m.showGlobalContextMenu(x, y, nil)
}

// showGlobalContextMenu shows a context menu with global actions like Help.
func (m *AppModel) showGlobalContextMenu(x, y int, hit *displayengine.HitRegion) tea.Cmd {
	var items []displayengine.ContextMenuItem

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
			case displayengine.IDAppVersion:
				header = "App Version"
			case displayengine.IDTmplVersion:
				header = "Template Version"
			case displayengine.IDHeaderFlags:
				header = "Global Flags"
			case displayengine.IDStatusBar:
				header = "Status Bar"
			case displayengine.IDPanel, displayengine.IDPanelViewport, displayengine.IDPanelToggle, displayengine.IDPanelResize:
				header = "Log Panel"
			}
		}
	}

	// For now, global menu is primarily for Help.
	items = append(items, displayengine.ContextMenuItem{IsHeader: true, Label: header})
	items = append(items, displayengine.ContextMenuItem{IsSeparator: true})
	// You could add "Refresh" or "App Version" here if they are useful as menu items.

	// Use the tail helper to add Clipboard and Help
	// (clipItems nil for now as global clipboard actions like Paste need a target)
	var hCtx *displayengine.HelpContext
	if hit != nil {
		hCtx = hit.Help
	}
	items = displayengine.AppendContextMenuTail(items, nil, hCtx)

	return func() tea.Msg {
		return displayengine.ShowDialogMsg{Dialog: displayengine.NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}
