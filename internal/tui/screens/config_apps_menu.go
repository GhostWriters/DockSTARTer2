package screens

import (
	"context"
	"path/filepath"
	"strings"

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
			Tag:     niceName,
			Desc:    descText,
			Help:    "Configure environment variables for " + niceName,
			Action:  navigateToAppConfigEditorWithRefresh(appName),
			BaseApp: appenv.AppNameToBaseAppName(appName),
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

func (m *configAppsMenuModel) HelpContext(maxWidth int) tui.HelpContext {
	inner := m.MenuModel.HelpContext(maxWidth)
	itemText := inner.ItemText

	items := m.MenuModel.GetItems()
	idx := m.MenuModel.Index()
	if idx >= 0 && idx < len(items) {
		item := items[idx]
		if item.BaseApp != "" {
			ctx := context.Background()
			appMeta, _ := appenv.LoadAppMeta(ctx, item.BaseApp)
			var parts []string
			if appMeta != nil && appMeta.App.HelpText != "" {
				parts = append(parts, appMeta.App.HelpText)
			} else {
				if desc := appenv.GetDescriptionFromTemplate(ctx, item.BaseApp, ""); desc != "" {
					parts = append(parts, desc)
				}
			}
			if appMeta != nil && appMeta.App.Website != "" {
				parts = append(parts, "Website: "+appMeta.App.Website)
			}
			if appenv.IsAppDeprecated(ctx, item.BaseApp) {
				parts = append(parts, "{{|TitleError|}}⚠ This app is deprecated.{{[-]}}")
			}
			if len(parts) > 0 {
				itemText = strings.Join(parts, "\n\n")
			}
		}
	}

	return tui.HelpContext{
		ScreenName: inner.ScreenName,
		PageTitle:  inner.PageTitle,
		PageText:   inner.PageText,
		ItemTitle:  inner.ItemTitle,
		ItemText:   itemText,
	}
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
	menu.SetHelpPageText("Select an application to browse and edit its environment variables. Each application's settings are stored in your .env file.")
	menu.SetHelpItemPrefix("App")
	return &configAppsMenuModel{MenuModel: menu}
}
