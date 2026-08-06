package cmd

import (
	"context"
	"errors"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"
)

// handleUpdate calls the shared update logic.
// Other registered processes (daemons, TUI sessions) are signalled via restart
// signal files written by SelfUpdate and handle their own re-exec when safe.
func handleUpdate(ctx context.Context, group *CommandGroup, state *CmdState, restArgs []string) error {
	return commands.HandleUpdate(ctx, group, state, restArgs)
}

func handleMenu(ctx context.Context, group *CommandGroup) error {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	// Normalize targets that mean "app select"
	switch target {
	case "config-app-select", "app-select", "select":
		target = "app-select"
	}
	if err := tui.Start(ctx, target); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}

func handleEditVars(ctx context.Context, group *CommandGroup) error {
	appName := ""
	if group.Command == "--edit-app" || group.Command == "--start-edit-app" {
		if len(group.Args) > 0 {
			appName = group.Args[0]
		}
	}
	if appName != "" {
		appName = strings.ToUpper(appName)
	}
	isRoot := group.Command == "--edit-global" || group.Command == "--edit-app"
	if err := tui.StartEditor(ctx, appName, isRoot); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}

func handleAppSelect(ctx context.Context, _ *CommandGroup) error {
	// -S / --select always opens the app selection menu
	if err := tui.Start(ctx, "app-select"); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}
