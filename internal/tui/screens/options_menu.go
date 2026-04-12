package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"
	"context"
	"strings"

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
			Action: func() tea.Msg { return tea.Msg(strings.Repeat("runtime-panic-test", -1)) },
		},
		{
			Tag:    "Trigger View Panic",
			Desc:   "{{|TitleError|}}Test low-level View panic{{[-]}}",
			Help:   "Verify recovery for crashes in the main UI thread",
			Action: func() tea.Msg { return tui.TriggerViewPanicMsg{} },
		},
		{
			Tag:  "Simulate Sudo Prompt",
			Desc: "Test universal text prompt",
			Help: "Show what a sudo password request looks like",
			Action: func() tea.Msg {
				go func() {
					// Simulate executing a background task that requires a password
					_, _ = console.TextPrompt(context.Background(), func(context.Context, any, ...any) {}, "{{|TitleQuestion|}}Sudo Password Required{{[-]}}", "sudo run simulated/command.sh", true)
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
	menu.SetHelpPageText("Application settings and preferences. Configure the visual theme, UI display options, and other tool behaviors.")
	menu.SetHelpItemPrefix("Action")
	return menu
}
