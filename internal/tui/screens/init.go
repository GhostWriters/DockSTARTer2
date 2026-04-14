package screens

import (
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

func init() {
	// Register the standalone var editor factory so tui.StartVarEditor can create
	// a Set Value screen without importing the screens package (circular import).
	tui.RegisterVarEditorFactory(func(
		varName, appName, appDesc, filePath, origVal string,
		opts []appenv.VarOption,
		helpText, docMarkdown, docAppName string,
		onSave func(string) tea.Cmd,
		onCancel tea.Cmd,
	) tui.ScreenModel {
		return newSetValueDialog(varName, appName, appDesc, filePath, origVal, opts, helpText, docMarkdown, docAppName, onSave, onCancel)
	})

	// Register the editor factory so tui.StartEditor can create editor screens
	// without importing the screens package (which would be circular).
	tui.RegisterEditorFactory(func(appName string, onClose tea.Cmd, showBack bool) tui.ScreenModel {
		if appName == "" {
			return NewEnvEditorGlobal(onClose, showBack)
		}
		specs := []EnvTabSpec{
			{Title: ".env", App: appName, IsGlobal: true},
			{Title: ".env.app." + strings.ToLower(appName), App: appName, IsGlobal: false},
		}
		return NewTabbedVarsEditorScreen(onClose, "Configure "+appName, specs, showBack)
	})
	// Register each screen with its canonical page name, creator, and parent stack.
	// Parents are ordered outermost-first and define the navigation stack that is
	// pre-populated when the screen is started via "-M start-<name>".
	tui.RegisterScreen("main",
		func(isRoot bool) tui.ScreenModel { return NewMainMenuScreen() },
		nil)

	tui.RegisterScreen("config",
		func(isRoot bool) tui.ScreenModel { return NewConfigMenuScreen() },
		[]string{"main"})

	tui.RegisterScreen("options",
		func(isRoot bool) tui.ScreenModel { return NewOptionsMenuScreen(isRoot) },
		[]string{"main"})

	tui.RegisterScreen("appearance",
		func(isRoot bool) tui.ScreenModel { return NewDisplayOptionsScreen(isRoot) },
		[]string{"main", "options"})

	tui.RegisterScreen("app-select",
		func(isRoot bool) tui.ScreenModel { return NewAppSelectionScreen(config.LoadAppConfig(), isRoot) },
		[]string{"main", "config"})
}
