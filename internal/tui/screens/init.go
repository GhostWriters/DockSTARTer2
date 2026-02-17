package screens

import (
	"DockSTARTer2/internal/tui"
)

func init() {
	// Register screen creators to avoid circular imports
	tui.RegisterScreenCreators(
		func() tui.ScreenModel { return NewMainMenuScreen() },
		func() tui.ScreenModel { return NewConfigMenuScreen() },
		func() tui.ScreenModel { return NewOptionsMenuScreen() },
		func() tui.ScreenModel { return nil }, // App selection screen - TODO: implement
	)
}
