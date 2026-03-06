package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// navigateToConfigMenu returns a command to navigate to the config menu
func navigateToConfigMenu() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewConfigMenuScreen()}
	}
}

// navigateToOptionsMenu returns a command to navigate to the options menu
func navigateToOptionsMenu() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewOptionsMenuScreen(false)}
	}
}

// navigateToAppSelection returns a command to navigate to app selection
func navigateToAppSelection() tea.Cmd {
	return func() tea.Msg {
		cfg := config.LoadAppConfig()
		return tui.NavigateMsg{Screen: NewAppSelectionScreen(cfg, false)}
	}
}

// navigateBack returns a command to navigate back
func navigateBack() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateBackMsg{}
	}
}
