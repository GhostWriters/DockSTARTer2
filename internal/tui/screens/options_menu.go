package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"
	"context"

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
		},
		{
			Tag:    "Trigger Test Panic",
			Desc:   "{{|TitleError|}}Test error handling{{[-]}}",
			Help:   "Verify branded recovery and stack trace",
			Action: func() tea.Msg { panic("Manual verification panic (Test: 123)") },
		},
		{
			Tag:  "Simulate Sudo Prompt",
			Desc: "Test universal text prompt",
			Help: "Show what a sudo password request looks like",
			Action: func() tea.Msg {
				go func() {
					// Simulate executing a background task that requires a password
					console.TextPrompt(context.Background(), func(context.Context, any, ...any) {}, "{{|TitleQuestion|}}Sudo Password Required{{[-]}}", "sudo run simulated/command.sh", true)
				}()
				return nil
			},
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
	return menu
}
