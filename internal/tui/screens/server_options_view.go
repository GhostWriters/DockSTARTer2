package screens

import (
	"DockSTARTer2/internal/tui"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (s *ServerOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Every inner menu must see its own deferred-action messages (button
	// clicks on outerMenu, item Action clicks like the SSH Port/Auth Mode
	// dropdowns on settingsMenu) before any early-return branch below can
	// drop them -- each menu's menuDeferredActionMsg is scoped to its own
	// instanceID, so only that menu can absorb it.
	if s.outerMenu != nil {
		if action := s.outerMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}
	if s.settingsMenu != nil {
		if action := s.settingsMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}
	if s.statusMenu != nil {
		if action := s.statusMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}

	var cmd tea.Cmd

	// Forward drag/scroll done messages to inner menus.
	switch dmsg := msg.(type) {
	case tui.DragDoneMsg:
		updated, c1 := s.settingsMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.settingsMenu = m
		}
		updated, c2 := s.statusMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.statusMenu = m
		}
		return s, tea.Batch(c1, c2)
	case tui.ScrollDoneMsg:
		updated, c1 := s.settingsMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.settingsMenu = m
		}
		updated, c2 := s.statusMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.statusMenu = m
		}
		return s, tea.Batch(c1, c2)
	}

	// Forward raw mouse drag/release to the dragging sub-menu.
	if s.IsScrollbarDragging() {
		target := s.settingsMenu
		if s.statusMenu.IsScrollbarDragging() {
			target = s.statusMenu
		}
		if _, ok := msg.(tea.MouseMotionMsg); ok {
			updated, uCmd := target.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				if target == s.settingsMenu {
					s.settingsMenu = m
				} else {
					s.statusMenu = m
				}
			}
			return s, uCmd
		}
		if _, ok := msg.(tea.MouseReleaseMsg); ok {
			updated, uCmd := target.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				if target == s.settingsMenu {
					s.settingsMenu = m
				} else {
					s.statusMenu = m
				}
			}
			return s, uCmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case tea.MouseClickMsg:
		return s, nil

	case tea.MouseWheelMsg:
		switch s.focusedPanel {
		case FocusServerSettings:
			updated, uCmd := s.settingsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.settingsMenu = m
			}
			return s, uCmd
		case FocusServerStatus:
			updated, uCmd := s.statusMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.statusMenu = m
			}
			return s, uCmd
		case FocusServerButtons:
			maxBtn := s.maxFocusedButton()
			switch msg.Button {
			case tea.MouseWheelUp:
				if s.focusedButton > 0 {
					s.focusedButton--
				}
			case tea.MouseWheelDown:
				if s.focusedButton < maxBtn {
					s.focusedButton++
				}
			}
			return s, nil
		}

	case tui.LayerHitMsg:
		// Panel focus routing
		switch msg.ID {
		case "server_settings":
			s.focusedPanel = FocusServerSettings
			s.updateFocusStates()
			return s, nil
		case "server_status":
			s.focusedPanel = FocusServerStatus
			s.updateFocusStates()
			return s, nil
		case tui.IDButtonPanel:
			s.focusedPanel = FocusServerButtons
			s.updateFocusStates()
			return s, nil
		}

		// Global button actions
		if tui.ButtonIDMatches(msg.ID, tui.IDApplyButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, s.outerMenu.SetProcessingBtnDeferred(tui.IDApplyButton, s.handleApply())
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, nil
			}
		} else if tui.ButtonIDMatches(msg.ID, tui.IDBackButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = 1
				s.updateFocusStates()
				if s.isRoot {
					return s, nil
				}
				return s, s.outerMenu.SetProcessingBtnDeferred(tui.IDBackButton, navigateBack())
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = 1
				s.updateFocusStates()
				return s, nil
			}
		} else if tui.ButtonIDMatches(msg.ID, tui.IDExitButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				return s, s.outerMenu.SetProcessingBtnDeferred(tui.IDExitButton, tui.ConfirmExitAction())
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusServerButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				return s, nil
			}
		}

		// Delegate to sub-menus
		if strings.Contains(msg.ID, "server_settings") {
			s.focusedPanel = FocusServerSettings
			s.updateFocusStates()
			updated, uCmd := s.settingsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.settingsMenu = m
			}
			return s, uCmd
		} else if strings.Contains(msg.ID, "server_status") {
			s.focusedPanel = FocusServerStatus
			s.updateFocusStates()
			updated, uCmd := s.statusMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.statusMenu = m
			}
			return s, uCmd
		}

		// Title widget clicks — delegate to outerMenu
		if s.outerMenu != nil && tui.IsTitleWidgetID(msg.ID) {
			updated, uCmd := s.outerMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.outerMenu = m
			}
			return s, uCmd
		}

	case tui.ToggleFocusedMsg:
		switch s.focusedPanel {
		case FocusServerSettings:
			updated, uCmd := s.settingsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.settingsMenu = m
			}
			return s, uCmd
		case FocusServerStatus:
			updated, uCmd := s.statusMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.statusMenu = m
			}
			return s, uCmd
		case FocusServerButtons:
			return s.execFocusedButton()
		}
		return s, nil

	case tea.KeyPressMsg:
		// Title bar focus: delegate all keys to outer menu when its title bar is focused.
		if s.outerMenu != nil && s.outerMenu.TitleBarFocused() {
			updated, uCmd := s.outerMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.outerMenu = m
			}
			return s, uCmd
		}

		// Tab / Shift-Tab: cycle between panels
		if key.Matches(msg, tui.Keys.CycleTab) || key.Matches(msg, tui.Keys.CycleShiftTab) {
			switch s.focusedPanel {
			case FocusServerSettings:
				s.focusedPanel = FocusServerStatus
			default:
				s.focusedPanel = FocusServerSettings
			}
			s.updateFocusStates()
			return s, nil
		}

		// Left/Right: cycle buttons
		if key.Matches(msg, tui.Keys.Left) {
			s.focusedPanel = FocusServerButtons
			s.focusedButton--
			if s.focusedButton < 0 {
				s.focusedButton = s.maxFocusedButton()
			}
			s.updateFocusStates()
			return s, nil
		}
		if key.Matches(msg, tui.Keys.Right) {
			s.focusedPanel = FocusServerButtons
			s.focusedButton++
			if s.focusedButton > s.maxFocusedButton() {
				s.focusedButton = 0
			}
			s.updateFocusStates()
			return s, nil
		}

		if key.Matches(msg, tui.Keys.Enter) {
			return s.execFocusedButton()
		}

		if key.Matches(msg, tui.Keys.Esc) {
			return s, s.EscapeAction()
		}

		// Up/Down/Space: route to focused panel
		switch s.focusedPanel {
		case FocusServerSettings:
			updated, uCmd := s.settingsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.settingsMenu = m
			}
			return s, uCmd
		case FocusServerStatus:
			updated, uCmd := s.statusMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.statusMenu = m
			}
			return s, uCmd
		}

	case updateServerOptionMsg:
		msg.update(&s.config)
		s.syncSettingsMenu()
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case serverStatusRefreshMsg:
		s.refreshStatus()
		return s, nil

	case tui.ConfigChangedMsg:
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case tui.LockStateChangedMsg:
		updated, c1 := s.settingsMenu.Update(msg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.settingsMenu = m
		}
		updated, c2 := s.statusMenu.Update(msg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.statusMenu = m
		}
		return s, tea.Batch(c1, c2)
	}

	return s, cmd
}

func (s *ServerOptionsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height
	if s.outerMenu != nil {
		s.outerMenu.SetSize(width, height)
	}
}

func (s *ServerOptionsScreen) ViewString() string {
	if s.outerMenu == nil {
		return ""
	}
	if s.width != 0 && s.height != 0 {
		s.outerMenu.SetSize(s.width, s.height)
	}
	return s.outerMenu.ViewString()
}

func (s *ServerOptionsScreen) View() tea.View {
	v := tea.NewView(s.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (s *ServerOptionsScreen) Layers() []*lipgloss.Layer {
	if s.width == 0 || s.height == 0 {
		return nil
	}
	dialog := s.outerMenu.ViewString()
	return []*lipgloss.Layer{
		lipgloss.NewLayer(dialog).X(0).Y(0).Z(tui.ZScreen),
	}
}

// GetHitRegions delegates entirely to the outer container MenuModel, which
// already knows its own (possibly shrunk, non-maximized) rendered width and
// each content section's actual position -- hand-computing regions here
// from s.width (the full content area, before the outer's own width-shrink)
// went stale and desynced from the real rendered layout once Server Settings
// started shrinking to fit instead of always filling the screen.
func (s *ServerOptionsScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	if s.outerMenu == nil {
		return nil
	}
	return s.outerMenu.GetHitRegions(offsetX, offsetY)
}
