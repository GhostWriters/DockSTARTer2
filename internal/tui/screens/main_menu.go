package screens

import (
	"DockSTARTer2/internal/tui"
)

// NewMainMenuScreen creates the main menu as a standalone screen
func NewMainMenuScreen() tui.ScreenModel {
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
			Action: tui.TriggerUpdate(),
			IsDestructive: true,
		},
		{
			Tag:    "Options",
			Desc:   "Customize settings",
			Help:   "Theme, display options, and more",
			Action: navigateToOptionsMenu(),
		},
	}

	menu := tui.NewMenuModel(
		tui.IDListPanel,
		"Main Menu",
		"What would you like to do?",
		items,
		nil, // No back action for main menu
	)

	menu.SetMenuName("")
	menu.SetHelpPageText("The main navigation menu for DockSTARTer. Select an action to configure your Docker application stack, apply updates, or adjust settings.")
	menu.SetHelpItemPrefix("Action")
	return menu
}
