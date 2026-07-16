package screens

import (
	"context"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// configAppsMenuModel wraps the outer container MenuModel (title, buttons)
// built by NewConfigAppsMenuScreen; the actual app list lives in a nested
// variable-height content section (m.list), following the outer-container +
// inner-submenu-section pattern used by Main Menu/Config Menu/Options Menu.
type configAppsMenuModel struct {
	*displayengine.MenuModel
	list     *displayengine.MenuModel
	connType string
}

func (m *configAppsMenuModel) refreshItems() {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	apps, err := appenv.ListReferencedApps(ctx, cfg)
	if err != nil {
		return
	}
	envFile := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	m.list.SetItems(buildConfigAppItems(ctx, apps, envFile, m.connType))
	// The item count may have changed (apps added/removed elsewhere);
	// re-trigger the outer's grow-then-scroll height calculation against the
	// new count -- SetItems alone only invalidates the render cache, it
	// doesn't recompute layout (same as it never did before this screen was
	// sectioned; the difference now is the outer container's height needs
	// this explicit nudge to stay consistent with calculateSectionLayout's
	// natural-height shrink).
	w, h := m.Width(), m.Height()
	m.SetSize(w, h)
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
	m.MenuModel = updated.(*displayengine.MenuModel)
	return m, cmd
}

func buildConfigAppItems(ctx context.Context, apps []string, envFile string, connType string) []displayengine.MenuItem {
	var items []displayengine.MenuItem
	for _, appName := range apps {
		niceName := appenv.GetNiceName(ctx, appName)
		desc := appenv.GetDescription(ctx, appName, envFile)
		isUserDefined := appenv.IsAppUserDefined(ctx, appName, envFile)

		descText := displayengine.GetPlainText(desc)
		switch {
		case isUserDefined:
			descText = "{{|ListItemUserDefined|}}" + descText
		case appenv.IsAppDeprecated(ctx, appenv.AppNameToBaseAppName(appName)):
			descText = "{{|ListItemDeprecated|}}" + descText
		default:
			descText = "{{|ListItem|}}" + descText
		}

		items = append(items, displayengine.MenuItem{
			Tag:           niceName,
			Desc:          descText,
			Help:          "Configure environment variables for " + niceName,
			Action:        navigateToAppConfigEditorWithRefresh(appName, connType),
			BaseApp:       appenv.AppNameToBaseAppName(appName),
			IsUserDefined: isUserDefined,
			IsDestructive: true,
			Metadata:      map[string]string{"rawDesc": desc},
		})
	}

	// Add an item to add a new application
	items = append(items, displayengine.MenuItem{
		Tag:           "<ADD APPLICATION>",
		Desc:          "Add a new application to configure",
		Help:          "Add a new application",
		IsDestructive: true,
		Action:        nil,
	})
	return items
}

// configAppItemHelp returns enriched (itemTitle, itemText) for a config app menu item.
// Returns ("", "") for items without a base app.
func configAppItemHelp(item displayengine.MenuItem) (itemTitle, itemText string) {
	if item.BaseApp == "" {
		return "", ""
	}
	if item.IsUserDefined {
		if rawDesc := item.Metadata["rawDesc"]; rawDesc != "" {
			return item.Tag, rawDesc
		}
		return item.Tag, "{{|App|}}" + item.Tag + "{{[-]}} is a user defined application"
	}
	ctx := context.Background()
	appMeta, _ := appenv.LoadAppMeta(ctx, item.BaseApp)
	var parts []string
	if appMeta != nil && appMeta.App.HelpText != "" {
		parts = append(parts, appMeta.App.HelpText)
	} else if desc := appenv.GetDescriptionFromTemplate(ctx, item.BaseApp, ""); desc != "" {
		parts = append(parts, desc)
	}
	if appMeta != nil && appMeta.App.Website != "" {
		parts = append(parts, "Website: {{|URL|}}"+appMeta.App.Website+"{{[-]}}")
	}
	if appenv.IsAppDeprecated(ctx, item.BaseApp) {
		parts = append(parts, "{{|TitleError|}}⚠ This app is deprecated.{{[-]}}")
	}
	if len(parts) == 0 {
		return "", ""
	}
	return item.Tag, strings.Join(parts, "\n\n")
}

func (m *configAppsMenuModel) HelpContext(maxWidth int) displayengine.HelpContext {
	inner := m.list.HelpContext(maxWidth)
	items := m.list.GetItems()
	idx := m.list.Index()
	if idx >= 0 && idx < len(items) {
		if t, txt := configAppItemHelp(items[idx]); txt != "" {
			if t != "" {
				inner.ItemTitle = t
			}
			inner.ItemText = txt
		}
	}

	return inner
}

// NewConfigAppsMenuScreen creates the "Configure Applications" menu.
// Built as an outer container MenuModel (title, buttons) with the app list
// as a single submenu-mode, variable-height content section, matching the
// pattern used by Main Menu/Config Menu/Options Menu -- except this list is
// non-maximized (grows to fit the app list, capped at available terminal
// height, then scrolls), preserving the screen's original behavior.
func NewConfigAppsMenuScreen(connType string) tui.ScreenModel {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	apps, err := appenv.ListReferencedApps(ctx, cfg)
	if err != nil {
		apps = []string{}
	}
	envFile := filepath.Join(cfg.ComposeDir, constants.EnvFileName)

	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", buildConfigAppItems(ctx, apps, envFile, connType))
	list.SetMenuName("config_apps")
	list.SetConnType(connType)
	list.SetVariableHeight(true)
	list.SetHelpPageText("Select an application to browse and edit its environment variables. Each application's settings are stored in your .env file.")
	list.SetHelpItemPrefix("App")
	list.SetItemHelpFunc(configAppItemHelp)
	list.SetItemDocFunc(func(item displayengine.MenuItem) (docMarkdown, docAppName string) {
		if item.BaseApp == "" || item.IsUserDefined {
			return "", ""
		}
		doc, err := appenv.GetAppMarkdown(context.Background(), item.BaseApp)
		if err != nil {
			return "", ""
		}
		return doc, item.Tag
	})
	list.SetSubMenuMode(true)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	list.SetNoLeftMargin(true)

	outer := displayengine.NewMenuModel("config_apps_outer", "Configure Applications", "", nil)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "Select", ZoneID: "btn-select", Action: func() tea.Msg {
			item := list.SelectedItem()
			if item.Action != nil {
				return item.Action()
			}
			return nil
		}, Help: "Execute the selected action."},
		{Label: "Back", ZoneID: "btn-back", Action: navigateBack(), Help: "Return to the previous screen."},
		{Label: "Exit", ZoneID: "btn-exit", Action: tui.ConfirmExitAction(), Help: "Exit the application."},
	})
	outer.AddContentSection(displayengine.NewPlainTextSection("config_apps_subtitle", "Select the application to configure"))
	outer.AddContentSection(list)
	// Outer intentionally never calls SetMaximized -- defaults false, so
	// calculateSectionLayout uses the grow-then-scroll natural-height path.

	return &configAppsMenuModel{MenuModel: outer, list: list, connType: connType}
}
