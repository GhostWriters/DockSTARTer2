package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// ConfigMenuScreen is the configuration menu screen
type ConfigMenuScreen struct {
	menu tui.MenuModel
}

// NewConfigMenuScreen creates the configuration menu
func NewConfigMenuScreen() *ConfigMenuScreen {
	items := []tui.MenuItem{
		{
			Tag:    "Full Setup",
			Desc:   "Run complete setup wizard",
			Help:   "Guided setup for all applications",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Edit Global Variables",
			Desc:   "Configure global settings",
			Help:   "Edit PUID, PGID, TZ, and other global variables",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Select Applications",
			Desc:   "Choose which apps to enable",
			Help:   "Enable or disable applications",
			Action: nil, // Not implemented yet - will navigate to app_select
		},
		{
			Tag:    "Configure Applications",
			Desc:   "Edit application settings",
			Help:   "Configure ports, volumes, and environment variables",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Start All Applications",
			Desc:   "Start enabled applications",
			Help:   "Run docker compose up for all enabled apps",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Stop All Applications",
			Desc:   "Stop all running applications",
			Help:   "Run docker compose down",
			Action: nil, // Not implemented yet
		},
		{
			Tag:    "Prune Docker System",
			Desc:   "Clean up unused Docker resources",
			Help:   "Remove unused images, containers, and volumes",
			Action: nil, // Not implemented yet
		},
	}

	menu := tui.NewMenuModel(
		"config_menu",
		"Configuration",
		"Setup and configure applications",
		items,
		navigateBack(),
	)

	return &ConfigMenuScreen{menu: menu}
}

// Init implements tea.Model
func (s *ConfigMenuScreen) Init() tea.Cmd {
	return s.menu.Init()
}

// Update implements tea.Model
func (s *ConfigMenuScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.menu.Update(msg)
	if menu, ok := updated.(tui.MenuModel); ok {
		s.menu = menu
	}
	return s, cmd
}

// ViewString returns the screen content as a string (for compositing)
func (s *ConfigMenuScreen) ViewString() string {
	return s.menu.ViewString()
}

// View implements tea.Model
func (s *ConfigMenuScreen) View() tea.View {
	return s.menu.View()
}

// Title implements ScreenModel
func (s *ConfigMenuScreen) Title() string {
	return s.menu.Title()
}

// HelpText implements ScreenModel
func (s *ConfigMenuScreen) HelpText() string {
	return s.menu.HelpText()
}

// SetSize implements ScreenModel
func (s *ConfigMenuScreen) SetSize(width, height int) {
	s.menu.SetSize(width, height)
}

// SetFocused propagates focus state to the inner menu (used by log panel focus)
func (s *ConfigMenuScreen) SetFocused(f bool) {
	s.menu.SetFocused(f)
}

// navigateBack returns a command to go back to the previous screen
func navigateBack() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateBackMsg{}
	}
}
