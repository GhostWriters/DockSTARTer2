package screens

import (
	"context"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// navigateToConfigMenu returns a command to navigate to the config menu
func navigateToConfigMenu(connType string) tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewConfigMenuScreen(false, connType)}
	}
}

// navigateToGlobalVarsEditor returns a command to navigate to the global variables editor
func navigateToGlobalVarsEditor(connType string) tea.Cmd {
	return func() tea.Msg {
		tui.CurrentEditorApp = ""
		return tui.NavigateMsg{Screen: NewEnvEditorGlobal(navigateBack(), true, connType)}
	}
}

// navigateToConfigApps returns a command to navigate to configure applications
func navigateToConfigApps(connType string) tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewConfigAppsMenuScreen(connType)}
	}
}

// navigateToAppConfigEditorWithRefresh returns a command to navigate to a specific app's config editor
// and refresh the apps list when returning.
func navigateToAppConfigEditorWithRefresh(appName string, connType string) tea.Cmd {
	return func() tea.Msg {
		tui.CurrentEditorApp = appName
		specs := []EnvTabSpec{
			{Title: ".env", App: appName, IsGlobal: true},
			{Title: ".env.app." + strings.ToLower(appName), App: appName, IsGlobal: false},
		}
		return tui.NavigateMsg{Screen: NewTabbedVarsEditorScreen(navigateBackWithRefresh(), "Configure "+appenv.GetNiceName(context.Background(), appName), specs, true, connType)}
	}
}

// navigateToOptionsMenu returns a command to navigate to the options menu
func navigateToOptionsMenu(connType string) tea.Cmd {
	return func() tea.Msg {
		return tui.NavigateMsg{Screen: NewOptionsMenuScreen(false, connType)}
	}
}

// navigateToAppSelection returns a command to navigate to app selection
func navigateToAppSelection(connType string) tea.Cmd {
	return func() tea.Msg {
		cfg := config.LoadAppConfig()
		return tui.NavigateMsg{Screen: NewAppSelectionScreen(cfg, false, connType)}
	}
}

// navigateToFullSetup returns a command that starts the guided setup wizard:
// Edit Global Variables, then Select Applications, then Configure
// Applications. All three are navigated to in one NavigateMsg via
// PushStack, so each screen's own unmodified Back action (a stack pop)
// naturally reveals the next step, with no custom "on complete" callback
// needed. The final Back lands wherever Full Setup was launched from.
func navigateToFullSetup(connType string) tea.Cmd {
	return func() tea.Msg {
		tui.CurrentEditorApp = ""
		cfg := config.LoadAppConfig()
		configApps := NewConfigAppsMenuScreen(connType)
		appSelection := NewAppSelectionScreen(cfg, false, connType)
		globalVars := NewEnvEditorGlobal(navigateBack(), true, connType)
		return tui.NavigateMsg{
			Screen:    globalVars,
			PushStack: []tui.ScreenModel{configApps, appSelection},
		}
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
