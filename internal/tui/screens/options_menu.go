package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// OptionsMenuScreen is the options menu screen
type OptionsMenuScreen struct {
	menu tui.MenuModel
}

// NewOptionsMenuScreen creates the options menu
func NewOptionsMenuScreen() *OptionsMenuScreen {
	items := []tui.MenuItem{
		{
			Tag:    "Choose Theme",
			Desc:   "Select color theme",
			Help:   "Change the TUI color scheme",
			Action: nil, // Not implemented yet - will navigate to theme_select
		},
		{
			Tag:    "Display Options",
			Desc:   "Configure display settings",
			Help:   "Borders, shadows, and line characters",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Package Manager",
			Desc:   "Select package manager",
			Help:   "Choose apt, yum, dnf, or other",
			Action: nil, // Not implemented yet
		},
	}

	menu := tui.NewMenuModel(
		"options_menu",
		"Options",
		"Customize settings",
		items,
		navigateBack(),
	)

	return &OptionsMenuScreen{menu: menu}
}

// Init implements tea.Model
func (s *OptionsMenuScreen) Init() tea.Cmd {
	return s.menu.Init()
}

// Update implements tea.Model
func (s *OptionsMenuScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.menu.Update(msg)
	if menu, ok := updated.(tui.MenuModel); ok {
		s.menu = menu
	}
	return s, cmd
}

// ViewString returns the screen content as a string (for compositing)
func (s *OptionsMenuScreen) ViewString() string {
	return s.menu.ViewString()
}

// View implements tea.Model
func (s *OptionsMenuScreen) View() tea.View {
	return s.menu.View()
}

// Title implements ScreenModel
func (s *OptionsMenuScreen) Title() string {
	return s.menu.Title()
}

// HelpText implements ScreenModel
func (s *OptionsMenuScreen) HelpText() string {
	return s.menu.HelpText()
}

// SetSize implements ScreenModel
func (s *OptionsMenuScreen) SetSize(width, height int) {
	s.menu.SetSize(width, height)
}

// SetFocused propagates focus state to the inner menu (used by log panel focus)
func (s *OptionsMenuScreen) SetFocused(f bool) {
	s.menu.SetFocused(f)
}
