package cmd

import (
	"context"
	"errors"
	"os"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/update"
)

// handleUpdate is a wrapper that calls the shared logic and then restarts the server if needed.
func handleUpdate(ctx context.Context, group *CommandGroup, state *CmdState, restArgs []string) error {
	// Capture server state and executable path before the update replaces the binary.
	serverInfo := sessionlocks.Sessions.ReadServerInfo()
	wasServerRunning := serverInfo.PID != 0 && sessionlocks.ProcessExists(serverInfo.PID)
	execPath, execErr := os.Executable()

	if err := commands.HandleUpdate(ctx, group, state, restArgs); err != nil {
		return err
	}

	// PendingReExec is only set when the binary was actually replaced.
	// Stop the external server process and spawn a new one with the updated binary.
	// (When updating from inside the daemon, ReExec already called ServerDisconnect
	// and DaemonShutdown — the daemon restarts itself via the re-exec mechanism.)
	if len(update.PendingReExec) > 0 && wasServerRunning && !console.IsDaemon && execErr == nil {
		logger.Notice(ctx, "Stopping server before restart...")
		if err := serve.StopServer(ctx, false); err != nil {
			logger.Warn(ctx, "Could not stop server: %v", err)
		}
		logger.Notice(ctx, "Restarting server with new binary...")
		proc, err := serve.SpawnDaemon(execPath, tui.GetNavArgs())
		if err != nil {
			logger.Warn(ctx, "Failed to restart server: %v — run 'ds2 --server start' manually.", err)
		} else {
			logger.Notice(ctx, "Server restarted (PID %d).", proc.Pid)
		}
	}

	return nil
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
