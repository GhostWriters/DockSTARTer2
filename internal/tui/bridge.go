package tui

import (
	"context"
	"fmt"
	"DockSTARTer2/internal/commands"
	tea "charm.land/bubbletea/v2"
)

// TUIBridge implements commands.UIProvider to allow the TUI Console
// to trigger real TUI dialogs and screens.
type TUIBridge struct{}

// Prompt shows a TUI confirmation dialog and waits for the result.
func (b *TUIBridge) Prompt(ctx context.Context, title, message string, defaultVal bool) (bool, error) {
	if program == nil {
		return defaultVal, nil
	}

	resultChan := make(chan bool)
	program.Send(UniversalPromptMsg{
		Title:      title,
		Question:   message,
		DefaultYes: defaultVal,
		ResultChan: resultChan,
		Type:       PromptTypeConfirm,
	})

	select {
	case res := <-resultChan:
		return res, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// AppSelect jumps to the interactive App Selection screen.
func (b *TUIBridge) AppSelect(ctx context.Context) error {
	if program == nil {
		return fmt.Errorf("TUI program is not running")
	}
	// Normal Start() inside TUI loop doesn't work as it tries to run a new tea.Program.
	// Instead we send a navigation message.
	return b.Navigate(ctx, "app-select")
}

// ValueEdit jumps to the interactive Variable Editor screen.
func (b *TUIBridge) ValueEdit(ctx context.Context, appName, varName, file, mode string) error {
	if program == nil {
		return fmt.Errorf("TUI program is not running")
	}

	onClose := func() tea.Msg { return NavigateBackMsg{} }
	
	// If mode is "global", jump to global editor.
	// If appName is set, jump to that app's editor.
	if mode == "global" {
		appName = ""
	}

	// We use the editorFactory which is registered by the screens package
	screen := editorFactory(appName, onClose, true, GetConnType())
	program.Send(NavigateMsg{Screen: screen})
	return nil
}

// RunCommand wraps a task in a TUI Program Box.
func (b *TUIBridge) RunCommand(ctx context.Context, title, subtitle string, task func(context.Context) error) error {
	return RunCommand(ctx, title, subtitle, task)
}

// Navigate switches the active TUI screen to the specified target.
func (b *TUIBridge) Navigate(ctx context.Context, target string) error {
	if program == nil {
		return fmt.Errorf("TUI program is not running")
	}

	pageName, _ := resolveMenuTarget(target)
	if pageName == CurrentPageName {
		// Already on this screen, do nothing
		return nil
	}

	entry, ok := screenRegistry[pageName]
	if !ok {
		return fmt.Errorf("screen '%s' not found", target)
	}

	// create(false) ensures we get a Back button. 
	// NavigateMsg ensures the current screen is pushed to the stack.
	program.Send(NavigateMsg{Screen: entry.create(false, GetConnType())})
	return nil
}

func init() {
	commands.SetUIProvider(&TUIBridge{})
}
