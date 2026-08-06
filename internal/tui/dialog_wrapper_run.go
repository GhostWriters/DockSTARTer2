package tui

import (
	"fmt"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
)

// RunDialogWithBackdrop bootstraps a standalone Bubble Tea program running
// dialog wrapped in classic's generic DialogWithBackdrop, for CLI-invoked
// dialogs shown before/outside a full TUI session (e.g. a bare Confirm/Message/
// Prompt call). The DialogWithBackdrop rendering itself lives in classic;
// this function is app-wiring (loads config, starts a tea.Program) so it
// stays in internal/tui rather than classic.
func RunDialogWithBackdrop[T displayengine.DialogModel](dialog T, helpText string, position displayengine.DialogPosition) (T, error) {

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if _, err := theme.Load(cfg.UI.Theme, ""); err == nil {
		displayengine.InitStyles(cfg)
	}

	wrapper := displayengine.NewDialogWithBackdrop(dialog, helpText).WithPosition(position)

	p := NewProgram(wrapper, ProgramOptions{})
	finalModel, err := p.Run()

	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")

	if err != nil {
		return dialog, err
	}

	if m, ok := finalModel.(displayengine.DialogWithBackdrop[T]); ok {
		return m.Dialog(), nil
	}

	return dialog, nil
}
