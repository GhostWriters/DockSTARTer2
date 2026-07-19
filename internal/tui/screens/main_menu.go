package screens

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/version"

	tea "charm.land/bubbletea/v2"
)

// NewMainMenuScreen creates the main menu as a standalone screen.
// Built as an outer container MenuModel (title, buttons) with the item list
// as a single submenu-mode content section, matching the pattern used by
// multi-section screens like DisplayOptionsScreen -- see the Content/
// ContentRow migration plan for why a MenuModel can't be its own section.
func NewMainMenuScreen(connType string) tui.ScreenModel {
	items := []displayengine.MenuItem{
		{
			Tag:    "Configuration",
			Desc:   "Setup and start applications",
			Help:   "Configure applications and services",
			Action: navigateToConfigMenu(connType),
		},
		{
			Tag:           "Update",
			Desc:          "Update DockSTARTer2",
			Help:          "Check for and install updates",
			Action:        tui.TriggerUpdate(),
			IsDestructive: true,
		},
		{
			Tag:    "Options",
			Desc:   "Customize settings",
			Help:   "Theme, display options, and more",
			Action: navigateToOptionsMenu(connType),
		},
	}

	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", items)
	// Unique key for cursor-position persistence, distinct from other
	// screens' lists that share the IDListPanel id.
	list.SetMenuName("main_menu")
	list.SetConnType(connType)
	list.SetHelpPageText("The main navigation menu for " + version.ApplicationName + ". Select an action to configure your Docker application stack, apply updates, or adjust settings.")
	list.SetHelpItemPrefix("Action")
	list.SetSubMenuMode(true)
	list.SetVariableHeight(true)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding, so the section's own internal left margin
	// (which was correct for the original single-MenuModel, unwrapped
	// layout) would double up to 2 margin columns instead of 1 -- suppress
	// it here to match themeMenu/optionsMenu's identical convention for
	// sections nested inside an outer sectioned dialog.
	list.SetNoLeftMargin(true)

	outer := displayengine.NewMenuModel("main_menu_outer", "Main Menu", "", nil)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "Select", ZoneID: "btn-select", Action: func() tea.Msg {
			item := list.SelectedItem()
			if item.Action != nil {
				return item.Action()
			}
			return nil
		}, Help: "Execute the selected action."},
		{Label: "Exit", ZoneID: displayengine.IDExitButton, Action: tui.ConfirmExitAction(), Help: "Exit the application."},
	})
	outer.AddContentSection(displayengine.NewPlainTextSection("main_menu_subtitle", "What would you like to do?"))
	outer.AddContentSection(list)

	return outer
}
