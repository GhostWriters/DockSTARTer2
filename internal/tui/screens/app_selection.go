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
	"charm.land/lipgloss/v2"
)

// AppSelectionScreen is the screen for selecting applications to enable/disable
type AppSelectionScreen struct {
	menu *tui.MenuModel
	conf config.AppConfig
}

type TriggerSaveMsg struct{}

// NewAppSelectionScreen creates a new AppSelectionScreen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewAppSelectionScreen(conf config.AppConfig, isRoot bool) *AppSelectionScreen {
	var backAction tea.Cmd
	if !isRoot {
		backAction = navigateBack()
	}
	menu := tui.NewMenuModel(
		"app_selection",
		"Select Applications",
		"Choose which apps you would like to install:\nUse {{|Theme_KeyCap|}}[up]{{[-]}}, {{|Theme_KeyCap|}}[down]{{[-]}}, and {{|Theme_KeyCap|}}[space]{{[-]}} to select apps, and {{|Theme_KeyCap|}}[tab]{{[-]}} to switch to the buttons at the bottom.",
		nil, // items will be set by refreshItems
		backAction,
	)
	menu.SetMaximized(true)
	menu.SetButtonLabels("Done", "Cancel", "Exit")
	menu.SetShowExit(true)
	menu.SetEnterAction(func() tea.Msg { return TriggerSaveMsg{} })
	menu.SetCheckboxMode(true) // Enable checkboxes for app selection

	s := &AppSelectionScreen{
		menu: &menu,
		conf: conf,
	}
	s.refreshItems()
	return s
}

// refreshItems re-fetches applications and updates the menu items
func (s *AppSelectionScreen) refreshItems() {
	ctx := context.Background()
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)

	// Fetch applications
	nonDeprecated, _ := appenv.ListNonDeprecatedApps(ctx)
	added, _ := appenv.ListAddedApps(ctx, envFile)

	// Merge lists and remove duplicates
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

	enabledApps, _ := appenv.ListEnabledApps(s.conf)
	enabledMap := make(map[string]bool)
	for _, app := range enabledApps {
		enabledMap[app] = true
	}

	// Build menu items with alphabetical separators
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
		desc := appenv.GetDescription(ctx, app, envFile)

		// Prepend theme tags for correct description coloring
		if appenv.AppNameToInstanceName(app) != "" {
			desc = "{{|Theme_ListAppUserDefined|}}" + desc
		} else {
			desc = "{{|Theme_ListApp|}}" + desc
		}

		items = append(items, tui.MenuItem{
			Tag:           niceName,
			Desc:          desc,
			Help:          fmt.Sprintf("Toggle %s", niceName),
			Selectable:    true,
			Selected:      enabledMap[app],
			IsUserDefined: appenv.AppNameToInstanceName(app) != "",
			Metadata:      map[string]string{"appName": app},
		})
	}

	s.menu.SetItems(items)
}

// Init implements tea.Model
func (s *AppSelectionScreen) Init() tea.Cmd {
	return s.menu.Init()
}

// Update implements tea.Model
func (s *AppSelectionScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tui.TemplateUpdateSuccessMsg:
		s.refreshItems()
		return s, nil
	case TriggerSaveMsg:
		return s, s.handleSave()
	}

	// Handle internal menu updates
	var cmd tea.Cmd
	var newMenu tea.Model
	newMenu, cmd = s.menu.Update(msg)
	if menu, ok := newMenu.(*tui.MenuModel); ok {
		s.menu = menu
	}

	return s, cmd
}

func (s *AppSelectionScreen) handleSave() tea.Cmd {
	var toEnable []string
	var toDisable []string
	niceNames := make(map[string]string)
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)

	// Get original enabled apps
	originalEnabled, _ := appenv.ListEnabledApps(s.conf)
	originalMap := make(map[string]bool)
	for _, app := range originalEnabled {
		originalMap[app] = true
	}

	// Calculate diff
	for _, item := range s.menu.GetItems() {
		if !item.Selectable {
			continue
		}
		appName := item.Metadata["appName"]
		niceNames[appName] = item.Tag // item.Tag stores the NiceName
		if item.Selected && !originalMap[appName] {
			toEnable = append(toEnable, appName)
		} else if !item.Selected && originalMap[appName] {
			toDisable = append(toDisable, appName)
		}
	}

	// If no changes, just go back
	if len(toEnable) == 0 && len(toDisable) == 0 {
		return func() tea.Msg { return tui.NavigateBackMsg{} }
	}

	// Map to nice names for display in the progress dialog
	var toEnableNice []string
	for _, app := range toEnable {
		toEnableNice = append(toEnableNice, niceNames[app])
	}
	var toDisableNice []string
	for _, app := range toDisable {
		toDisableNice = append(toDisableNice, niceNames[app])
	}

	// Create and configure the program box dialog
	dialog := tui.NewProgramBoxModel("{{|Theme_TitleSuccess|}}Enabling Selected Applications", "", "")
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

	// Task to execute inside the program box
	task := func(ctx context.Context, w io.Writer) error {
		ctx = console.WithTUIWriter(ctx, w)
		totalSteps := len(toEnable) + len(toDisable) + 1 // +1 for final update
		completedSteps := 0

		updateProgress := func() {
			tui.Send(tui.UpdatePercentMsg{Percent: float64(completedSteps) / float64(totalSteps)})
		}

		if len(toDisable) > 0 {
			tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: ""})
			for _, app := range toDisable {
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
				_ = appenv.Disable(ctx, []string{app}, s.conf)
				completedSteps++
				updateProgress()
			}
			tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusCompleted, ActiveApp: ""})
		}

		if len(toEnable) > 0 {
			tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: ""})
			for _, app := range toEnable {
				tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
				_ = appenv.Enable(ctx, []string{app}, s.conf)
				completedSteps++
				updateProgress()
			}
			tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusCompleted, ActiveApp: ""})
		}

		// Synchronize .env
		tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusInProgress, ActiveApp: ""})
		_ = appenv.Update(ctx, console.Force(), envFile)
		completedSteps++
		tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusCompleted, ActiveApp: ""})
		updateProgress()

		return nil
	}
	dialog.SetTask(task)

	return func() tea.Msg {
		return tui.FinalizeSelectionMsg{Dialog: dialog}
	}
}

// View implements tea.Model
func (s *AppSelectionScreen) View() tea.View {
	return s.menu.View()
}

// ViewString implements ViewStringer for overlay compositing
func (s *AppSelectionScreen) ViewString() string {
	return s.menu.ViewString()
}

// Title implements ScreenModel
func (s *AppSelectionScreen) Title() string {
	return s.menu.Title()
}

// HelpText implements ScreenModel
func (s *AppSelectionScreen) HelpText() string {
	return ""
}

// SetSize implements ScreenModel
func (s *AppSelectionScreen) SetSize(width, height int) {
	// app_selection leaves 1 blank line before the helpline.
	s.menu.SetSize(width, height)
}

// HasDialog implements ScreenModel
func (s *AppSelectionScreen) HasDialog() bool {
	return s.menu.HasDialog()
}

// SetFocused propagates focus state
func (s *AppSelectionScreen) SetFocused(f bool) {
	s.menu.SetFocused(f)
}

// IsMaximized implements ScreenModel
func (s *AppSelectionScreen) IsMaximized() bool {
	return s.menu.IsMaximized()
}

// MenuName implements ScreenModel
func (s *AppSelectionScreen) MenuName() string {
	return "app-select"
}

// Layers implements LayeredView for compositing
func (s *AppSelectionScreen) Layers() []*lipgloss.Layer {
	return s.menu.Layers()
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (s *AppSelectionScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	return s.menu.GetHitRegions(offsetX, offsetY)
}
