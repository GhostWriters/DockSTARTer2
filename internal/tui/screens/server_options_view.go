package screens

import (
	"DockSTARTer2/internal/tui"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (s *ServerOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				return s, s.handleApply()
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
				return s, navigateBack()
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
				return s, tui.ConfirmExitAction()
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
			if s.isRoot {
				return s, tui.ConfirmExitAction()
			}
			return s, navigateBack()
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

func (s *ServerOptionsScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	layout := tui.GetLayout()
	contentX := (layout.BorderWidth() / 2) + layout.ContentSideMargin
	const contentY = 1

	// Settings menu regions
	settingsRegions := s.settingsMenu.GetHitRegions(offsetX+contentX, offsetY+contentY)
	regions = append(regions, settingsRegions...)
	regions = append(regions, tui.HitRegion{
		ID:     "server_settings",
		X:      offsetX + contentX,
		Y:      offsetY + contentY,
		Width:  s.settingsMenu.Width(),
		Height: s.settingsMenu.Height(),
		ZOrder: tui.ZScreen + 1,
		Label:  "Server Configuration",
	})

	// Status menu regions (below settings)
	statusY := contentY + s.settingsMenu.Height()
	statusRegions := s.statusMenu.GetHitRegions(offsetX+contentX, offsetY+statusY)
	regions = append(regions, statusRegions...)
	regions = append(regions, tui.HitRegion{
		ID:     "server_status",
		X:      offsetX + contentX,
		Y:      offsetY + statusY,
		Width:  s.statusMenu.Width(),
		Height: s.statusMenu.Height(),
		ZOrder: tui.ZScreen + 1,
		Label:  "Server Status",
		Help: &tui.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Live status of the SSH server and any active remote session.",
		},
	})

	// Button row
	menuWidth := s.width - layout.BorderWidth()
	buttonY := 1 + s.settingsMenu.Height() + s.statusMenu.Height()
	btnRowWidth := menuWidth - layout.ContentMarginWidth()

	regions = append(regions, tui.HitRegion{
		ID:     tui.IDButtonPanel,
		X:      offsetX + contentX,
		Y:      offsetY + buttonY,
		Width:  btnRowWidth,
		Height: s.outerMenu.GetButtonHeight(),
		ZOrder: tui.ZScreen + 1,
		Label:  "Actions",
	})

	btnSpecs := []tui.ButtonSpec{
		{Text: "Apply", ZoneID: tui.IDApplyButton, Help: "Save server settings to dockstarter2.toml."},
	}
	if s.isRoot {
		btnSpecs = append(btnSpecs, tui.ButtonSpec{Text: "Exit", ZoneID: tui.IDExitButton, Help: "Exit the application."})
	} else {
		btnSpecs = append(btnSpecs,
			tui.ButtonSpec{Text: "Back", ZoneID: tui.IDBackButton, Help: "Return to the previous screen."},
			tui.ButtonSpec{Text: "Exit", ZoneID: tui.IDExitButton, Help: "Exit the application."},
		)
	}
	helpCtx := tui.HelpContext{
		ScreenName: s.outerMenu.Title(),
		PageTitle:  "Description",
		PageText:   "Configure remote access to the DS2 TUI.",
	}
	regions = append(regions, tui.GetButtonHitRegions(
		helpCtx,
		s.outerMenu.ID(), offsetX+contentX, offsetY+buttonY, btnRowWidth, tui.ZScreen+25,
		btnSpecs...,
	)...)

	return regions
}
