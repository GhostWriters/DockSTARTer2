package screens

import (
	"DockSTARTer2/internal/tui"
)

// NewConfigMenuScreen creates the configuration menu as a standalone screen
func NewConfigMenuScreen() tui.ScreenModel {
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
			Action: navigateToGlobalVarsEditor(),
		},
		{
			Tag:    "Select Applications",
			Desc:   "Choose which apps to enable",
			Help:   "Enable or disable applications",
			Action: navigateToAppSelection(),
		},
		{
			Tag:    "Configure Applications",
			Desc:   "Edit application settings",
			Help:   "Configure ports, volumes, and environment variables",
			Action: navigateToConfigApps(),
		},
		{
			Tag:    "Start All Applications",
			Desc:   "Start enabled applications",
			Help:   "Run docker compose up for all enabled apps",
			Action: tui.TriggerComposeUpdate(),
		},
		{
			Tag:    "Stop All Applications",
			Desc:   "Stop all running applications",
			Help:   "Run docker compose stop or down",
			Action: tui.TriggerComposeStop(),
		},
		{
			Tag:    "Prune Docker System",
			Desc:   "Clean up unused Docker resources",
			Help:   "Remove unused images, containers, and volumes",
			Action: tui.TriggerDockerPrune(),
		},
	}

	menu := tui.NewMenuModel(
		tui.IDListPanel,
		"Configuration",
		"Select a configuration task",
		items,
		navigateBack(),
	)

	menu.SetMenuName("config")
	return menu
}
