package logger

import (
	"DockSTARTer2/internal/console"
	"context"

	tea "charm.land/bubbletea/v2"
)

// Recover traps panics and displays them using FatalWithStack.
// Usage: defer logger.Recover(ctx)
func Recover(ctx context.Context) {
	if r := recover(); r != nil {
		// Suppress further panics during recovery
		defer func() {
			if recover() != nil {
				// If we panic again during recovery, just exit
				// This prevents infinite loops
			}
		}()

		// Restore terminal if TUI was running
		if console.TUIShutdown != nil {
			console.TUIShutdown()
		}

		// Ensure TUI flag is off so we print directly to terminal
		console.SetTUIEnabled(false)

		// Check if it's already a FatalError (intentional panic)
		if _, ok := r.(FatalError); ok {
			return
		}

		// Display as a fatal error with stack trace
		// We skip 2 frames: Recover + runtime.panic
		FatalWithStackSkip(ctx, 2, "panic: %v", r)
	}
}

// RecoverTUI wraps a tea.Cmd in a recovery block that uses FatalWithStack.
func RecoverTUI(ctx context.Context, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		defer func() {
			if r := recover(); r != nil {
				// Suppress further panics during recovery
				defer func() { recover() }()

				// Restore terminal and log the panic
				if console.TUIShutdown != nil {
					console.TUIShutdown()
				}
				console.SetTUIEnabled(false)

				// Check if it's already a FatalError
				if _, ok := r.(FatalError); ok {
					return
				}

				// We skip 2 frames: closure + runtime.panic
				FatalWithStackSkip(ctx, 2, "TUI Panic: %v", r)
			}
		}()
		return cmd()
	}
}
