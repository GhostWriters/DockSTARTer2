package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// NewOptionsMenuScreen creates the options menu as a standalone screen.
// isRoot suppresses the Back button when this screen is the entry point.
// Built as an outer container MenuModel (title, buttons) with the item list
// as a single submenu-mode content section, matching the pattern used by
// Main Menu and other multi-section screens.
func NewOptionsMenuScreen(isRoot bool, connType string) tui.ScreenModel {
	items := []tui.MenuItem{
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

	list := tui.NewMenuModel(tui.IDListPanel, "", "", items)
	list.SetMenuName("")
	list.SetConnType(connType)
	list.SetHelpPageText("Application settings and preferences. Configure the visual theme, UI display options, and other tool behaviors.")
	list.SetHelpItemPrefix("Action")
	list.SetSubMenuMode(true)
	list.SetVariableHeight(false)
	list.SetIsDialog(false)
	list.SetButtons([]tui.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding; suppress the section's own internal left
	// margin to avoid doubling up (matches themeMenu/optionsMenu's identical
	// convention for sections nested inside an outer sectioned dialog).
	list.SetNoLeftMargin(true)

	outer := tui.NewMenuModel("options_menu_outer", "Options", "", nil)
	selectAction := func() tea.Msg {
		item := list.SelectedItem()
		if item.Action != nil {
			return item.Action()
		}
		return nil
	}
	if !isRoot {
		outer.SetShowButtons(true)
		outer.SetButtons([]tui.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Action: selectAction, Help: "Confirm and execute the selected action."},
			{Label: "Back", ZoneID: "btn-back", Action: navigateBack(), Help: "Return to the previous screen."},
			{Label: "Exit", ZoneID: "btn-exit", Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	}
	outer.AddContentSection(tui.NewPlainTextSection("options_menu_subtitle", "Customize settings"))
	outer.AddContentSection(list)

	return outer
}
