package screens

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/version"

	tea "charm.land/bubbletea/v2"
)

// NewConfigMenuScreen creates the configuration menu as a standalone screen.
// isRoot suppresses the Back button when this screen is the entry point.
// Built as an outer container MenuModel (title, buttons) with the item list
// as a single submenu-mode content section, matching the pattern used by
// Main Menu and other multi-section screens.
func NewConfigMenuScreen(isRoot bool, connType string) tui.ScreenModel {
	items := []displayengine.MenuItem{
		{
			Tag:           "Full Setup",
			Desc:          "Run complete setup wizard",
			Help:          "Guided setup for all applications",
			Action:        navigateToFullSetup(connType),
			IsDestructive: true,
		},
		{
			Tag:           "Edit Global Variables",
			Desc:          "Configure global settings",
			Help:          "Edit PUID, PGID, TZ, and other global variables",
			Action:        navigateToGlobalVarsEditor(connType),
			IsDestructive: true,
		},
		{
			Tag:           "Select Applications",
			Desc:          "Choose which apps to enable",
			Help:          "Enable or disable applications",
			Action:        navigateToAppSelection(connType),
			IsDestructive: true,
		},
		{
			Tag:    "Configure Applications",
			Desc:   "Edit application settings",
			Help:   "Configure ports, volumes, and environment variables",
			Action: navigateToConfigApps(connType),
		},
		{
			Tag:           "Start All Applications",
			Desc:          "Start enabled applications",
			Help:          "Run docker compose up for all enabled apps",
			Action:        tui.TriggerComposeUpdate(),
			IsDestructive: true,
		},
		{
			Tag:           "Stop All Applications",
			Desc:          "Stop all running applications",
			Help:          "Run docker compose stop or down",
			Action:        tui.TriggerComposeStop(),
			IsDestructive: true,
		},
		{
			Tag:           "Prune Docker System",
			Desc:          "Clean up unused Docker resources",
			Help:          "Remove unused images, containers, and volumes",
			Action:        tui.TriggerDockerPrune(),
			IsDestructive: true,
		},
	}

	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", items)
	list.SetMenuName("")
	list.SetConnType(connType)
	list.SetHelpPageText("Docker and " + version.ApplicationName + " configuration tasks. Run the full setup wizard, edit environment variables, enable or disable applications, and manage your running containers.")
	list.SetHelpItemPrefix("Action")
	list.SetSubMenuMode(true)
	list.SetVariableHeight(false)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding; suppress the section's own internal left
	// margin to avoid doubling up (matches themeMenu/optionsMenu's identical
	// convention for sections nested inside an outer sectioned dialog).
	list.SetNoLeftMargin(true)

	outer := displayengine.NewMenuModel("config_menu_outer", "Configuration", "", nil)
	outer.SetShowButtons(true)
	selectAction := func() tea.Msg {
		item := list.SelectedItem()
		if item.Action != nil {
			return item.Action()
		}
		return nil
	}
	buttons := []displayengine.ButtonDef{
		{Label: "Select", ZoneID: "btn-select", Action: selectAction, Help: "Confirm and execute the selected action."},
	}
	if !isRoot {
		buttons = append(buttons, displayengine.ButtonDef{Label: "Back", ZoneID: "btn-back", Action: navigateBack(), Help: "Return to the previous screen."})
	}
	buttons = append(buttons, displayengine.ButtonDef{Label: "Exit", ZoneID: "btn-exit", Action: tui.ConfirmExitAction(), Help: "Exit the application."})
	outer.SetButtons(buttons)
	outer.AddContentSection(displayengine.NewPlainTextSection("config_menu_subtitle", "Select a configuration task"))
	outer.AddContentSection(list)

	return outer
}
