package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"
	"context"
	"io"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type changeSet struct {
	toRemove  []string
	toEnable  []string
	toDisable []string
	niceNames map[string]string
}

func (s *AppSelectionScreen) computeChanges() changeSet {
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)
	niceNames := make(map[string]string)
	originalAdded, _ := appenv.ListAddedApps(context.Background(), envFile)
	originalMap := make(map[string]bool)
	for _, a := range originalAdded {
		originalMap[a] = true
	}
	originalEnabled, _ := appenv.ListEnabledApps(s.conf)
	originalEnabledMap := make(map[string]bool)
	for _, a := range originalEnabled {
		originalEnabledMap[a] = true
	}
	var toRemove, toEnable, toDisable []string
	for _, item := range s.menu.GetItems() {
		if item.IsGroupHeader || item.IsSeparator || item.IsEditing {
			continue
		}
		appName := item.Metadata["appName"]
		if appName == "" {
			continue
		}
		niceNames[appName] = item.Tag
		// Remove logic: was added, now unchecked.
		if !item.Checked && originalMap[appName] {
			toRemove = append(toRemove, appName)
		}
		// Enable/Disable logic for apps that are "Added" (Checked):
		if item.Checked {
			isNew := !originalMap[appName]
			if item.Enabled != originalEnabledMap[appName] || isNew {
				if item.Enabled {
					toEnable = append(toEnable, appName)
				} else {
					toDisable = append(toDisable, appName)
				}
			}
		}
	}
	return changeSet{toRemove: toRemove, toEnable: toEnable, toDisable: toDisable, niceNames: niceNames}
}

func (s *AppSelectionScreen) buildChangeSummary(cs changeSet) string {
	if len(cs.toRemove) == 0 && len(cs.toEnable) == 0 && len(cs.toDisable) == 0 {
		return "No changes pending."
	}
	// Align app names under the longest label ("Disable: " = 9 chars).
	const indent = "         " // 9 spaces
	var lines []string
	if len(cs.toRemove) > 0 {
		for i, app := range cs.toRemove {
			name := "{{|ProgressWaiting|}}" + cs.niceNames[app] + "{{[-]}}"
			if i == 0 {
				lines = append(lines, "{{|ProgressWaiting|}}Remove:{{[-]}}  "+name)
			} else {
				lines = append(lines, indent+name)
			}
		}
	}
	if len(cs.toEnable) > 0 {
		for i, app := range cs.toEnable {
			name := "{{|ProgressWaiting|}}" + cs.niceNames[app] + "{{[-]}}"
			if i == 0 {
				lines = append(lines, "{{|ProgressWaiting|}}Enable:{{[-]}}  "+name)
			} else {
				lines = append(lines, indent+name)
			}
		}
	}
	if len(cs.toDisable) > 0 {
		for i, app := range cs.toDisable {
			name := "{{|ProgressWaiting|}}" + cs.niceNames[app] + "{{[-]}}"
			if i == 0 {
				lines = append(lines, "{{|ProgressWaiting|}}Disable:{{[-]}} "+name)
			} else {
				lines = append(lines, indent+name)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (s *AppSelectionScreen) handleSave() tea.Msg {
	cs := s.computeChanges()
	toRemove := cs.toRemove
	toEnable := cs.toEnable
	toDisable := cs.toDisable
	niceNames := cs.niceNames

	if len(toRemove) == 0 && len(toEnable) == 0 && len(toDisable) == 0 {
		return tui.NavigateBackMsg{}
	}

	var toRemoveNice []string
	for _, app := range toRemove {
		toRemoveNice = append(toRemoveNice, niceNames[app])
	}
	var toEnableNice []string
	for _, app := range toEnable {
		toEnableNice = append(toEnableNice, niceNames[app])
	}
	var toDisableNice []string
	for _, app := range toDisable {
		toDisableNice = append(toDisableNice, niceNames[app])
	}

	dialog := tui.NewProgramBoxModel("{{|TitleSuccess|}}Applying Changes", "", "")
	dialog.SetIsDialog(true)
	dialog.SetMaximized(true)
	dialog.SetAutoClose(false, 0)
	dialog.AutoExit = false
	dialog.SuccessMsg = tui.NavigateBackMsg{}

	if len(toRemove) > 0 {
		dialog.AddTask("Removing applications", "ds --remove", toRemoveNice)
	}
	if len(toDisable) > 0 {
		dialog.AddTask("Disabling applications", "ds --disable", toDisableNice)
	}
	if len(toEnable) > 0 {
		dialog.AddTask("Enabling applications", "ds --enable", toEnableNice)
	}
	dialog.AddTask("Updating variable files", "", nil)

	task := func(ctx context.Context, w io.Writer) error {
		ctx = console.WithTUIWriter(ctx, w)
		totalSteps := len(toRemove) + len(toEnable) + len(toDisable) + 1
		completedSteps := 0

		updateProgress := func() {
			tui.Send(tui.UpdatePercentMsg{Percent: float64(completedSteps) / float64(totalSteps)})
		}

		if len(toRemove) > 0 {
			tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: ""})
			for _, app := range toRemove {
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
				_ = appenv.Remove(ctx, []string{app}, s.conf, true)
				completedSteps++
				updateProgress()
			}
			tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusCompleted, ActiveApp: ""})
		}

		if len(toDisable) > 0 {
			tui.Send(tui.UpdateTaskMsg{Label: "Disabling applications", Status: tui.StatusInProgress, ActiveApp: ""})
			for _, app := range toDisable {
				tui.Send(tui.UpdateTaskMsg{Label: "Disabling applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
				_ = appenv.Disable(ctx, []string{app}, s.conf)
				completedSteps++
				updateProgress()
			}
			tui.Send(tui.UpdateTaskMsg{Label: "Disabling applications", Status: tui.StatusCompleted, ActiveApp: ""})
		}

		if len(toEnable) > 0 {
			tui.Send(tui.UpdateTaskMsg{Label: "Enabling applications", Status: tui.StatusInProgress, ActiveApp: ""})
			for _, app := range toEnable {
				tui.Send(tui.UpdateTaskMsg{Label: "Enabling applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
				_ = appenv.Enable(ctx, []string{app}, s.conf)
				completedSteps++
				updateProgress()
			}
			tui.Send(tui.UpdateTaskMsg{Label: "Enabling applications", Status: tui.StatusCompleted, ActiveApp: ""})
		}

		tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusInProgress, ActiveApp: ""})
		_ = appenv.CreateAll(ctx, console.Force(), s.conf)
		completedSteps++
		tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusCompleted, ActiveApp: ""})
		updateProgress()

		return nil
	}
	dialog.SetTask(task)

	return tui.ShowDialogMsg{Dialog: dialog}
}
