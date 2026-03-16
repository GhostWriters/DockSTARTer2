package screens

import (
	"strings"

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

// navigateToGlobalVarsEditor returns a command to navigate to the global variables editor
func navigateToGlobalVarsEditor() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewEnvEditorGlobal(navigateBack())}
	}
}

// navigateToConfigApps returns a command to navigate to configure applications
func navigateToConfigApps() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewConfigAppsMenuScreen()}
	}
}

// navigateToAppConfigEditor returns a command to navigate to a specific app's config editor
func navigateToAppConfigEditor(appName string) tea.Cmd {
	return func() tea.Msg {
		specs := []EnvTabSpec{
			{Title: ".env", App: appName, IsGlobal: true},
			{Title: ".env.app." + strings.ToLower(appName), App: appName, IsGlobal: false},
		}
		return tui.NavigateMsg{Screen: NewTabbedVarsEditorScreen(navigateBack(), "Configure "+appName, specs)}
	}
}

// navigateToAppConfigEditorWithRefresh returns a command to navigate to a specific app's config editor
// and refresh the apps list when returning.
func navigateToAppConfigEditorWithRefresh(appName string) tea.Cmd {
	return func() tea.Msg {
		specs := []EnvTabSpec{
			{Title: ".env", App: appName, IsGlobal: true},
			{Title: ".env.app." + strings.ToLower(appName), App: appName, IsGlobal: false},
		}
		return tui.NavigateMsg{Screen: NewTabbedVarsEditorScreen(navigateBackWithRefresh(), "Configure "+appName, specs)}
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

// navigateBackWithRefresh returns a command to navigate back and refresh the apps list.
// The refresh is dispatched by the NavigateBackMsg handler after the screen swap,
// avoiding a race condition where RefreshAppsListMsg could arrive before back
// navigation completes and be routed to the wrong screen.
func navigateBackWithRefresh() tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateBackMsg{Refresh: true}
	}
}
