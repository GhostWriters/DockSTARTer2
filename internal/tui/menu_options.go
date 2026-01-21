package tui

import (
	"DockSTARTer2/internal/version"
)

func showOptionsMenu() {
	items := []MenuItem{
		{
			Tag:      "Choose Theme",
			Desc:     "Choose a theme for " + version.ApplicationName,
			Help:     "Select visual appearance",
			Shortcut: 'C',
			Action:   nil,
		},
		{
			Tag:      "Display Options",
			Desc:     "Set display options",
			Help:     "Change UI layout and behavior",
			Shortcut: 'D',
			Action:   nil,
		},
		{
			Tag:      "Package Manager",
			Desc:     "Choose the package manager to use",
			Help:     "Select apt or other package tools",
			Shortcut: 'P',
			Action:   nil,
		},
	}

	dialog, list := NewMenuDialog("Options", "What would you like to do?", items, showMainMenu)

	// Update Panels
	panels.AddPanel("menu", dialog, true, true)
	panels.ShowPanel("menu")
	app.SetFocus(list)
}
