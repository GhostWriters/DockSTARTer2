package tui

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/update"

	tea "charm.land/bubbletea/v2"
)

// ShowWebDisplaySettingsMsg requests the web display settings dialog
type ShowWebDisplaySettingsMsg struct{}

// ShowGlobalFlagsMsg requests the flags toggle dialog
type ShowGlobalFlagsMsg struct{}

// ShowPendingRestartMsg requests the pending restart confirmation dialog
type ShowPendingRestartMsg struct{}

// headerUpdate handles header interaction messages, dispatching app-level
// actions (navigation, updates) in response to clicks/hits on displayengine.HeaderModel's
// rendered regions. displayengine.HeaderModel itself (rendering, hit regions, focus state)
// lives in classic; this is the app-wiring half that reacts to those hits.
func headerUpdate(h *displayengine.HeaderModel, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case displayengine.LayerHitMsg:
		if msg.ID == displayengine.IDStatusBar {
			// Clicking the status bar background → focus App version if nothing is focused.
			if h.GetFocus() == displayengine.HeaderFocusNone {
				h.SetFocus(displayengine.HeaderFocusFlags)
			}
			return h, nil
		}
		if msg.Button == displayengine.HoverButton {
			// Focus-only hit (e.g. click blocked by open dialog) — update focus without triggering action.
			switch msg.ID {
			case displayengine.IDAppVersion:
				h.SetFocus(displayengine.HeaderFocusApp)
			case displayengine.IDTmplVersion:
				h.SetFocus(displayengine.HeaderFocusTmpl)
			case displayengine.IDHeaderFlags:
				h.SetFocus(displayengine.HeaderFocusFlags)
			case displayengine.IDHeaderWebDisplay:
				h.SetFocus(displayengine.HeaderFocusWebDisplay)
			}
			return h, nil
		}
		_, cmd := headerHandleHit(h, msg.ID)
		return h, cmd

	case displayengine.LayerWheelMsg:
		if msg.ID == displayengine.IDStatusBar {
			// Scroll wheel cycles between Flags, Center (web only), App version and Tmpl version.
			isWeb := h.ConnType == "web"
			focus := h.GetFocus()
			switch msg.Button {
			case tea.MouseWheelUp:
				switch focus {
				case displayengine.HeaderFocusNone, displayengine.HeaderFocusApp:
					focus = displayengine.HeaderFocusFlags
				case displayengine.HeaderFocusTmpl:
					focus = displayengine.HeaderFocusApp
				case displayengine.HeaderFocusWebDisplay:
					focus = displayengine.HeaderFocusFlags
				}
			case tea.MouseWheelDown:
				switch focus {
				case displayengine.HeaderFocusNone, displayengine.HeaderFocusFlags:
					if isWeb {
						focus = displayengine.HeaderFocusWebDisplay
					} else {
						focus = displayengine.HeaderFocusApp
					}
				case displayengine.HeaderFocusWebDisplay:
					focus = displayengine.HeaderFocusApp
				case displayengine.HeaderFocusApp:
					focus = displayengine.HeaderFocusTmpl
				}
			}
			h.SetFocus(focus)
			return h, nil
		}
	}

	if _, ok := msg.(displayengine.RefreshHeaderMsg); ok {
		h.SyncFlags()
		return h, nil
	}

	// Middle-click (displayengine.ToggleFocusedMsg) activates the currently focused item.
	if _, ok := msg.(displayengine.ToggleFocusedMsg); ok {
		switch h.GetFocus() {
		case displayengine.HeaderFocusFlags:
			return h, func() tea.Msg { return ShowGlobalFlagsMsg{} }
		case displayengine.HeaderFocusWebDisplay:
			return h, func() tea.Msg { return ShowWebDisplaySettingsMsg{} }
		case displayengine.HeaderFocusApp:
			if update.RestartPending {
				return h, func() tea.Msg { return ShowPendingRestartMsg{} }
			}
			return h, TriggerAppUpdate()
		case displayengine.HeaderFocusTmpl:
			return h, TriggerTemplateUpdate()
		}
		return h, nil
	}

	return h, nil
}

// headerHandleHit handles a hit result from the compositor
func headerHandleHit(h *displayengine.HeaderModel, id string) (bool, tea.Cmd) {
	switch id {
	case displayengine.IDAppVersion:
		h.SetFocus(displayengine.HeaderFocusApp)
		if update.RestartPending {
			return true, func() tea.Msg { return ShowPendingRestartMsg{} }
		}
		return true, TriggerAppUpdate()
	case displayengine.IDTmplVersion:
		h.SetFocus(displayengine.HeaderFocusTmpl)
		return true, TriggerTemplateUpdate()
	case displayengine.IDHeaderFlags:
		h.SetFocus(displayengine.HeaderFocusFlags)
		return true, func() tea.Msg { return ShowGlobalFlagsMsg{} }
	case displayengine.IDHeaderWebDisplay:
		h.SetFocus(displayengine.HeaderFocusWebDisplay)
		return true, func() tea.Msg { return ShowWebDisplaySettingsMsg{} }
	}
	return false, nil
}
