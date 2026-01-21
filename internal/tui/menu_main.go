package tui

import (
	"DockSTARTer2/internal/version"
)

func showMainMenu() {
	items := []MenuItem{
		{
			Tag:      "Configuration",
			Desc:     "Setup and start applications",
			Help:     "Modify " + version.ApplicationName + " configuration",
			Shortcut: 'C',
			Action:   showConfigMenu,
		},
		{
			Tag:      "Install Dependencies",
			Desc:     "Install required components",
			Help:     "Install or update " + version.ApplicationName + " dependencies",
			Shortcut: 'I',
			Action:   nil, // TODO: Implement dependencies installation
		},
		{
			Tag:      "Update " + version.ApplicationName,
			Desc:     "Get the latest version of " + version.ApplicationName,
			Help:     "Check for updates to " + version.ApplicationName + " and Templates",
			Shortcut: 'U',
			Action:   nil, // TODO: Implement update
		},
		{
			Tag:      "Options",
			Desc:     "Adjust options for " + version.ApplicationName,
			Help:     "Set display options and other " + version.ApplicationName + " settings",
			Shortcut: 'O',
			Action:   showOptionsMenu,
		},
	}

	dialog, list := NewMenuDialog("Main Menu", "What would you like to do?", items, nil)

	// Replace the content in the panels
	panels.AddPanel("menu", dialog, true, true)
	panels.ShowPanel("menu")
	app.SetFocus(list)
}
