package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// NewAppSelectionScreen creates a new AppSelectionScreen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewAppSelectionScreen(conf config.AppConfig, isRoot bool) *tui.MenuModel {
	var backAction tea.Cmd
	if !isRoot {
		backAction = func() tea.Msg { return tui.NavigateBackMsg{} }
	}

	menu := tui.NewMenuModel(
		"app-select",
		"Select Applications",
		"Choose which apps you would like to install:\nUse {{|KeyCap|}}[up]{{[-]}}, {{|KeyCap|}}[down]{{[-]}}, and {{|KeyCap|}}[space]{{[-]}} to select apps, and {{|KeyCap|}}[tab]{{[-]}} to switch to the buttons at the bottom.",
		nil, // items will be set by refreshItems
		backAction,
	)

	menu.SetMenuName("app-select")
	menu.SetButtonLabels("Done", "Back", "Exit")
	menu.SetShowExit(true)
	menu.SetCheckboxMode(true) // Enable checkboxes for app selection
	menu.SetVariableHeight(true)
	menu.SetMaximized(true)
	menu.SetSubMenuMode(false) // Render natively as a standard standalone screen

	refreshItems := func() {
		ctx := context.Background()
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

		nonDeprecated, _ := appenv.ListNonDeprecatedApps(ctx)
		added, _ := appenv.ListAddedApps(ctx, envFile)

		allAppsMap := make(map[string]bool)
		for _, app := range nonDeprecated {
			allAppsMap[app] = true
		}
		for _, app := range added {
			allAppsMap[app] = true
		}

		var allApps []string
		for app := range allAppsMap {
			allApps = append(allApps, app)
		}
		slices.Sort(allApps)

		enabledApps, _ := appenv.ListEnabledApps(conf)
		enabledMap := make(map[string]bool)
		for _, app := range enabledApps {
			enabledMap[app] = true
		}

		var items []tui.MenuItem
		var lastLetter string

		for _, app := range allApps {
			letter := strings.ToUpper(app[:1])
			if letter != lastLetter {
				if lastLetter != "" {
					items = append(items, tui.MenuItem{
						Tag:         "", // Blank line separator
						IsSeparator: true,
					})
				}
				lastLetter = letter
			}

			niceName := appenv.GetNiceName(ctx, app)
			desc := appenv.GetDescriptionFromTemplate(ctx, app, envFile)

			if appenv.AppNameToInstanceName(app) != "" {
				desc = "{{|ListAppUserDefined|}}" + desc
			} else {
				desc = "{{|ListApp|}}" + desc
			}

			items = append(items, tui.MenuItem{
				Tag:           niceName,
				Desc:          desc,
				Help:          fmt.Sprintf("Toggle %s", niceName),
				Selectable:    true,
				Selected:      enabledMap[app],
				Checked:       enabledMap[app],
				IsCheckbox:    true,
				IsUserDefined: appenv.AppNameToInstanceName(app) != "",
				Metadata:      map[string]string{"appName": app},
			})
		}

		menu.SetItems(items)
	}

	handleSave := func() tea.Msg {
		var toEnable []string
		var toDisable []string
		niceNames := make(map[string]string)
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

		originalEnabled, _ := appenv.ListEnabledApps(conf)
		originalMap := make(map[string]bool)
		for _, app := range originalEnabled {
			originalMap[app] = true
		}

		for _, item := range menu.GetItems() {
			if !item.Selectable {
				continue
			}
			appName := item.Metadata["appName"]
			niceNames[appName] = item.Tag
			if item.Selected && !originalMap[appName] {
				toEnable = append(toEnable, appName)
			} else if !item.Selected && originalMap[appName] {
				toDisable = append(toDisable, appName)
			}
		}

		if len(toEnable) == 0 && len(toDisable) == 0 {
			return tui.NavigateBackMsg{}
		}

		var toEnableNice []string
		for _, app := range toEnable {
			toEnableNice = append(toEnableNice, niceNames[app])
		}
		var toDisableNice []string
		for _, app := range toDisable {
			toDisableNice = append(toDisableNice, niceNames[app])
		}

		dialog := tui.NewProgramBoxModel("{{|TitleSuccess|}}Enabling Selected Applications", "", "")
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		dialog.SetAutoClose(false, 0)

		if len(toDisable) > 0 {
			dialog.AddTask("Removing applications", "ds --remove", toDisableNice)
		}
		if len(toEnable) > 0 {
			dialog.AddTask("Adding applications", "ds --add", toEnableNice)
		}
		dialog.AddTask("Updating variable files", "", nil)

		task := func(ctx context.Context, w io.Writer) error {
			ctx = console.WithTUIWriter(ctx, w)
			totalSteps := len(toEnable) + len(toDisable) + 1
			completedSteps := 0

			updateProgress := func() {
				tui.Send(tui.UpdatePercentMsg{Percent: float64(completedSteps) / float64(totalSteps)})
			}

			if len(toDisable) > 0 {
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: ""})
				for _, app := range toDisable {
					tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
					_ = appenv.Disable(ctx, []string{app}, conf)
					completedSteps++
					updateProgress()
				}
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusCompleted, ActiveApp: ""})
			}

			if len(toEnable) > 0 {
				tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: ""})
				for _, app := range toEnable {
					tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
					_ = appenv.Enable(ctx, []string{app}, conf)
					completedSteps++
					updateProgress()
				}
				tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusCompleted, ActiveApp: ""})
			}

			tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusInProgress, ActiveApp: ""})
			_ = appenv.Update(ctx, console.Force(), envFile)
			completedSteps++
			tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusCompleted, ActiveApp: ""})
			updateProgress()

			return nil
		}
		dialog.SetTask(task)

		return tui.FinalizeSelectionMsg{Dialog: dialog}
	}

	menu.SetEnterAction(handleSave)
	menu.SetUpdateInterceptor(func(msg tea.Msg, m *tui.MenuModel) (tea.Cmd, bool) {
		switch msg.(type) {
		case tui.TemplateUpdateSuccessMsg:
			refreshItems()
			return nil, true
		}
		return nil, false
	})

	refreshItems()
	return menu
}
