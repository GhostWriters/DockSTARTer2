package screens

import (
	"DockSTARTer2/internal/displayengine"

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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

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

	case displayengine.ConfigChangedMsg:
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil
	}

	if s.outerMenu == nil {
		return s, nil
	}
	newOuter, cmd := s.outerMenu.Update(msg)
	if m, ok := newOuter.(*displayengine.MenuModel); ok {
		s.outerMenu = m
	}
	// Keep the named settingsMenu/statusMenu fields in sync with outerMenu's
	// own content sections, since syncSettingsMenu/HelpText/HelpContext/etc.
	// read them directly.
	secs := s.outerMenu.GetContentSections()
	if len(secs) >= 1 {
		if mm, ok := secs[0].(*displayengine.MenuModel); ok {
			s.settingsMenu = mm
		}
	}
	if len(secs) >= 2 {
		if mm, ok := secs[1].(*displayengine.MenuModel); ok {
			s.statusMenu = mm
		}
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
		lipgloss.NewLayer(dialog).X(0).Y(0).Z(displayengine.ZScreen),
	}
}

// GetHitRegions delegates entirely to the outer container MenuModel, which
// already knows its own (possibly shrunk, non-maximized) rendered width and
// each content section's actual position -- hand-computing regions here
// from s.width (the full content area, before the outer's own width-shrink)
// went stale and desynced from the real rendered layout once Server Settings
// started shrinking to fit instead of always filling the screen.
func (s *ServerOptionsScreen) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	if s.outerMenu == nil {
		return nil
	}
	return s.outerMenu.GetHitRegions(offsetX, offsetY)
}
