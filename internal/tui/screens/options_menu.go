package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// NewOptionsMenuScreen creates the options menu as a standalone screen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewOptionsMenuScreen(isRoot bool) tui.ScreenModel {
	items := []tui.MenuItem{
		{
			Tag:    "Appearance",
			Desc:   "Themes and Display Options",
			Help:   "Configure color scheme, borders, and effects",
			Action: func() tea.Msg { return tui.NavigateMsg{Screen: NewDisplayOptionsScreen(false)} },
			IsDestructive: true,
		},
		{
			Tag:    "Server",
			Desc:   "SSH and Web Server Settings",
			Help:   "Configure remote access to the DS2 TUI via SSH or browser",
			Action: func() tea.Msg { return tui.NavigateMsg{Screen: NewServerOptionsScreen(false)} },
			IsDestructive: true,
		},
	}

	var backAction tea.Cmd
	if !isRoot {
		backAction = navigateBack()
	}
	menu := tui.NewMenuModel(
		tui.IDListPanel,
		"Options",
		"Customize settings",
		items,
		backAction,
	)

	menu.SetMenuName("options")
	menu.SetHelpPageText("Application settings and preferences. Configure the visual theme, UI display options, and other tool behaviors.")
	menu.SetHelpItemPrefix("Action")
	return menu
}
