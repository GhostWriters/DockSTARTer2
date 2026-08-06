package tui

import (
	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// bridgeSend delivers msg via ctx's session-scoped callback (see
// console.WithSendFunc) if present, falling back to the global program var --
// the same reason TextPrompt/QuestionPrompt prefer their own ctx-scoped
// callbacks over TUIPrompt/TUIConfirm.
func bridgeSend(ctx context.Context, msg tea.Msg) bool {
	if fn := console.SendFuncFromContext(ctx); fn != nil {
		fn(msg)
		return true
	}
	if program != nil {
		program.Send(msg)
		return true
	}
	return false
}

// TUIBridge implements commands.UIProvider to allow the TUI Console
// to trigger real TUI dialogs and screens.
type TUIBridge struct{}

// Prompt shows a TUI confirmation dialog and waits for the result. Prefers
// ctx's session-scoped confirm callback (see console.WithConfirmFunc) over
// the global program var, for the same reason bridgeSend does.
func (b *TUIBridge) Prompt(ctx context.Context, title, message string, defaultVal bool) (bool, error) {
	if fn := console.ConfirmFuncFromContext(ctx); fn != nil {
		return fn(title, message, defaultVal), nil
	}
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
	// Normal Start() inside TUI loop doesn't work as it tries to run a new tea.Program.
	// Instead we send a navigation message.
	return b.Navigate(ctx, "app-select")
}

// ValueEdit jumps to the interactive Variable Editor screen.
func (b *TUIBridge) ValueEdit(ctx context.Context, appName, varName, file, mode string) error {
	onClose := func() tea.Msg { return NavigateBackMsg{} }

	// If mode is "global", jump to global editor.
	// If appName is set, jump to that app's editor.
	if mode == "global" {
		appName = ""
	}

	// We use the editorFactory which is registered by the screens package
	screen := editorFactory(appName, onClose, true, GetConnType())
	if !bridgeSend(ctx, NavigateMsg{Screen: screen}) {
		return fmt.Errorf("TUI program is not running")
	}
	return nil
}

// RunCommand wraps a task in a TUI Program Box.
func (b *TUIBridge) RunCommand(ctx context.Context, title, subtitle, command string, task func(context.Context) error) error {
	return RunCommand(ctx, title, subtitle, command, task)
}

// Navigate switches the active TUI screen to the specified target.
func (b *TUIBridge) Navigate(ctx context.Context, target string) error {
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
	if !bridgeSend(ctx, NavigateMsg{Screen: entry.create(false, GetConnType())}) {
		return fmt.Errorf("TUI program is not running")
	}
	return nil
}

func init() {
	commands.SetUIProvider(&TUIBridge{})
}
