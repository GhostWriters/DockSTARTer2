package screens

import (
	"context"
	"path/filepath"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"
	tea "charm.land/bubbletea/v2"
)

type configAppsMenuModel struct {
	*tui.MenuModel
}

func (m *configAppsMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.MenuModel == nil {
		return m, nil
	}

	switch msg.(type) {
	case tui.RefreshAppsListMsg:
		m.refreshItems()
		return m, nil
	}

	updated, cmd := m.MenuModel.Update(msg)
	m.MenuModel = updated.(*tui.MenuModel)
	return m, cmd
}

func buildConfigAppItems(ctx context.Context, apps []string, envFile string) []tui.MenuItem {
	var items []tui.MenuItem
	for _, appName := range apps {
		niceName := appenv.GetNiceName(ctx, appName)
		desc := appenv.GetDescription(ctx, appName, envFile)
		isUserDefined := appenv.IsAppUserDefined(ctx, appName, envFile)

		descText := desc
		if isUserDefined {
			descText = "{{|ListAppUserDefined|}}" + descText
		} else {
			descText = "{{|ListApp|}}" + descText
		}

		items = append(items, tui.MenuItem{
			Tag:    niceName,
			Desc:   descText,
			Help:   "Configure environment variables for " + niceName,
			Action: navigateToAppConfigEditorWithRefresh(appName),
		})
	}

	// Add an item to add a new application
	items = append(items, tui.MenuItem{
		Tag:    "<ADD APPLICATION>",
		Desc:   "Add a new application to configure",
		Help:   "Add a new application",
		Action: nil,
	})
	return items
}

func (m *configAppsMenuModel) refreshItems() {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	apps, err := appenv.ListReferencedApps(ctx, cfg)
	if err != nil {
		return
	}
	envFile := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	m.MenuModel.SetItems(buildConfigAppItems(ctx, apps, envFile))
}

// NewConfigAppsMenuScreen creates the "Configure Applications" menu
func NewConfigAppsMenuScreen() tui.ScreenModel {
	ctx := context.Background()
	cfg := config.LoadAppConfig()

	apps, err := appenv.ListReferencedApps(ctx, cfg)
	if err != nil {
		apps = []string{}
	}

	envFile := filepath.Join(cfg.ComposeDir, constants.EnvFileName)

	menu := tui.NewMenuModel(
		tui.IDListPanel,
		"Configure Applications",
		"Select the application to configure",
		buildConfigAppItems(ctx, apps, envFile),
		navigateBack(),
	)

	menu.SetMenuName("config_apps")
	menu.SetVariableHeight(true)
	return &configAppsMenuModel{MenuModel: menu}
}
