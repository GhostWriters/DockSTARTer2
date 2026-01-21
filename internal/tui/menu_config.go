package tui

func showConfigMenu(backAction func()) {
	items := []MenuItem{
		{
			Tag:      "Full Setup",
			Desc:     "This goes through selecting apps and editing variables. Recommended for first run",
			Help:     "Select and configure all applications",
			Shortcut: 'F',
			Action:   nil,
		},
		{
			Tag:      "Edit Global Variables",
			Desc:     "Review and adjust global variables",
			Help:     "Modify shared application settings",
			Shortcut: 'G',
			Action:   nil,
		},
		{
			Tag:      "Select Applications",
			Desc:     "Select which apps to run. Previously installed apps are remembered",
			Help:     "Choose which services to enable",
			Shortcut: 'S',
			Action:   nil,
		},
		{
			Tag:      "Configure Applications",
			Desc:     "Review and adjust variables for installed apps",
			Help:     "Modify settings for specific apps",
			Shortcut: 'C',
			Action:   nil,
		},
		{
			Tag:      "Start All Applications",
			Desc:     "Run Docker Compose to start all applications",
			Help:     "Bring your services online",
			Shortcut: 'A',
			Action:   nil,
		},
		{
			Tag:      "Stop All Applications",
			Desc:     "Run Docker Compose to stop all applications",
			Help:     "Take your services offline",
			Shortcut: 'O',
			Action:   nil,
		},
		{
			Tag:      "Prune Docker System",
			Desc:     "Remove all unused containers, networks, volumes, images and build cache",
			Help:     "Clean up Docker storage",
			Shortcut: 'P',
			Action:   nil,
		},
	}

	dialog, list := NewMenuDialog("Configuration Menu", "What would you like to do?", items, backAction)

	// Update Panels
	panels.AddPanel("menu", dialog, true, true)
	panels.ShowPanel("menu")
	app.SetFocus(list)
}
