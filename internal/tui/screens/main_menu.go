package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// MainMenuScreen is the main menu screen
type MainMenuScreen struct {
	menu tui.MenuModel
}

// NewMainMenuScreen creates the main menu
func NewMainMenuScreen() *MainMenuScreen {
	items := []tui.MenuItem{
		{
			Tag:    "Configuration",
			Desc:   "Setup and start applications",
			Help:   "Configure applications and services",
			Action: navigateToConfigMenu(),
		},
		{
			Tag:    "Update",
			Desc:   "Update DockSTARTer2",
			Help:   "Check for and install updates",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Options",
			Desc:   "Customize settings",
			Help:   "Theme, display options, and more",
			Action: navigateToOptionsMenu(),
		},
	}

	menu := tui.NewMenuModel(
		"main_menu",
		"Main Menu",
		"What would you like to do?",
		items,
		nil, // No back action for main menu
	)

	return &MainMenuScreen{menu: menu}
}

// Init implements tea.Model
func (s *MainMenuScreen) Init() tea.Cmd {
	return s.menu.Init()
}

// Update implements tea.Model
func (s *MainMenuScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.menu.Update(msg)
	if menu, ok := updated.(tui.MenuModel); ok {
		s.menu = menu
	}
	return s, cmd
}

// View implements tea.Model
func (s *MainMenuScreen) View() tea.View {
	return s.menu.View()
}

// Title implements ScreenModel
func (s *MainMenuScreen) Title() string {
	return s.menu.Title()
}

// HelpText implements ScreenModel
func (s *MainMenuScreen) HelpText() string {
	return s.menu.HelpText()
}

// SetSize implements ScreenModel
func (s *MainMenuScreen) SetSize(width, height int) {
	s.menu.SetSize(width, height)
}

// SetFocused propagates focus state to the inner menu (used by log panel focus)
func (s *MainMenuScreen) SetFocused(f bool) {
	s.menu.SetFocused(f)
}

// Navigation commands
func navigateToConfigMenu() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewConfigMenuScreen()}
	}
}

func navigateToOptionsMenu() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewOptionsMenuScreen()}
	}
}
