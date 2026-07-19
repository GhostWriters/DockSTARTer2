package screens

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// NewOptionsMenuScreen creates the options menu as a standalone screen.
// isRoot suppresses the Back button when this screen is the entry point.
// Built as an outer container MenuModel (title, buttons) with the item list
// as a single submenu-mode content section, matching the pattern used by
// Main Menu and other multi-section screens.
func NewOptionsMenuScreen(isRoot bool, connType string) tui.ScreenModel {
	items := []displayengine.MenuItem{
		{
			Tag:    "Appearance",
			Desc:   "Themes and Display Options",
			Help:   "Configure color scheme, borders, and effects",
			Action: func() tea.Msg { return tui.NavigateMsg{Screen: NewDisplayOptionsScreen(false, connType)} },
		},
		{
			Tag:           "Server",
			Desc:          "SSH and Web Server Settings",
			Help:          "Configure remote access to the DS2 TUI via SSH or browser",
			Action:        func() tea.Msg { return tui.NavigateMsg{Screen: NewServerOptionsScreen(false, connType)} },
			IsDestructive: true,
		},
	}

	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", items)
	// Unique key for cursor-position persistence, distinct from other
	// screens' lists that share the IDListPanel id.
	list.SetMenuName("options_menu")
	list.SetConnType(connType)
	list.SetHelpPageText("Application settings and preferences. Configure the visual theme, UI display options, and other tool behaviors.")
	list.SetHelpItemPrefix("Action")
	list.SetSubMenuMode(true)
	list.SetVariableHeight(true)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding; suppress the section's own internal left
	// margin to avoid doubling up (matches themeMenu/optionsMenu's identical
	// convention for sections nested inside an outer sectioned dialog).
	list.SetNoLeftMargin(true)

	outer := displayengine.NewMenuModel("options_menu_outer", "Options", "", nil)
	selectAction := func() tea.Msg {
		item := list.SelectedItem()
		if item.Action != nil {
			return item.Action()
		}
		return nil
	}
	if !isRoot {
		outer.SetShowButtons(true)
		outer.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Action: selectAction, Help: "Confirm and execute the selected action."},
			{Label: "Back", ZoneID: "btn-back", Action: navigateBack(), Help: "Return to the previous screen."},
			{Label: "Exit", ZoneID: "btn-exit", Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	}
	outer.AddContentSection(displayengine.NewPlainTextSection("options_menu_subtitle", "Customize settings"))
	outer.AddContentSection(list)

	return outer
}
