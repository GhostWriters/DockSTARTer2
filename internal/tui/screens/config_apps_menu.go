package screens

import (
	"context"
	"path/filepath"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"
)

// NewConfigAppsMenuScreen creates the "Configure Applications" menu
func NewConfigAppsMenuScreen() tui.ScreenModel {
	ctx := context.Background()
	cfg := config.LoadAppConfig()

	// Get referenced apps like the legacy script
	apps, err := appenv.ListReferencedApps(ctx, cfg)
	if err != nil {
		apps = []string{}
	}

	envFile := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	var items []tui.MenuItem
	for _, appName := range apps {
		desc := appenv.GetDescription(ctx, appName, envFile)
		isUserDefined := appenv.IsAppUserDefined(ctx, appName, envFile)

		descText := desc
		if isUserDefined {
			// Using the same format as bash legacy
			descText = tui.RenderThemeText("{{|Theme_ListAppUserDefined|}}" + descText + "{{[-]}}")
		} else {
			descText = tui.RenderThemeText("{{|Theme_ListApp|}}" + descText + "{{[-]}}")
		}

		items = append(items, tui.MenuItem{
			Tag:    appName,
			Desc:   descText,
			Help:   "Configure environment variables for " + appName,
			Action: navigateToAppConfigEditor(appName),
		})
	}

	// Add an item to add a new application
	items = append(items, tui.MenuItem{
		Tag:    "<ADD APPLICATION>",
		Desc:   "Add a new application to configure",
		Help:   "Add a new application",
		Action: nil, // Not implemented yet
	})

	menu := tui.NewMenuModel(
		tui.IDListPanel,
		"Configure Applications",
		"Select the application to configure",
		items,
		navigateBack(),
	)

	menu.SetMenuName("config_apps")
	return menu
}
