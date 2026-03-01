package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/tui"
)

func init() {
	// Register each screen with its canonical page name, creator, and parent stack.
	// Parents are ordered outermost-first and define the navigation stack that is
	// pre-populated when the screen is started via "-M start-<name>".
	tui.RegisterScreen("main",
		func(isRoot bool) tui.ScreenModel { return NewMainMenuScreen() },
		nil)

	tui.RegisterScreen("config",
		func(isRoot bool) tui.ScreenModel { return NewConfigMenuScreen(isRoot) },
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
