package cmd

import (
	"context"
	"fmt"
	"os"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/serve"
)

// handleServer routes --server [subcommand] to the appropriate handler.
// No subcommand defaults to "status". All subcommands print a notice and
// return, allowing further command-line options to continue executing.
func handleServer(ctx context.Context, group *CommandGroup, state *CmdState, conf *config.AppConfig) error {
	sub := "status"
	if len(group.Args) > 0 {
		sub = group.Args[0]
	}

	switch sub {
	case "status", "":
		return handleServerStatus(ctx, conf)
	case "start":
		return handleServerStart(ctx, conf)
	case "stop":
		return handleServerStop(ctx, state)
	case "restart":
		if err := handleServerStop(ctx, state); err != nil {
			return err
		}
		return handleServerStart(ctx, conf)
	case "disconnect":
		return handleServerDisconnect(ctx, state)
	case "install":
		logger.Warn(ctx, "Server service installation is not yet implemented.")
		return nil
	case "uninstall":
		logger.Warn(ctx, "Server service uninstallation is not yet implemented.")
		return nil
	case "enable":
		logger.Warn(ctx, "Server service enable is not yet implemented.")
		return nil
	case "disable":
		logger.Warn(ctx, "Server service disable is not yet implemented.")
		return nil
	default:
		return fmt.Errorf("unknown server subcommand %q", sub)
	}
}

// handleServerStatus prints the current server and session state.
func handleServerStatus(ctx context.Context, _ *config.AppConfig) error {
	serve.PrintServerStatus(ctx)
	return nil
}

// handleServerStart spawns the server as a background daemon process.
// It validates the config before attempting to start.
func handleServerStart(ctx context.Context, conf *config.AppConfig) error {
	if conf.Server.SSH.Port == 0 {
		logger.Warn(ctx, "server.ssh.port is not set in dockstarter2.toml — cannot start server.")
		return nil
	}

	// Check if already running.
	info := serve.Sessions.ReadServerInfo()
	if info.PID != 0 {
		logger.Notice(ctx, "Server is already running (PID %d, port %d).", info.PID, info.Port)
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	logger.Notice(ctx, "Starting server in the background...")
	proc, err := serve.SpawnDaemon(execPath, nil)
	if err != nil {
		return fmt.Errorf("spawning server daemon: %w", err)
	}
	logger.Notice(ctx, "Server started (PID %d).", proc.Pid)
	return nil
}

// handleServeDaemon is the internal handler for --server-daemon.
// It runs the blocking SSH (and web) server loop. Never called directly by
// the user — only invoked when --server start re-execs the binary.
//
// group.Args may contain nav args such as ["--menu", "start-options"] or
// ["--start-edit-app", "plex"] that were appended by SpawnDaemon so the
// daemon can restore navigation state for reconnecting web/SSH sessions.
func handleServeDaemon(ctx context.Context, group *CommandGroup, conf *config.AppConfig) error {
	startMenu := extractNavArg(group.Args)
	return serve.StartSSHServer(ctx, conf.Server, startMenu)
}

// extractNavArg parses nav args appended after --server-daemon and returns
// the startMenu string to pass to the TUI (e.g. "start-options", "plex").
// Returns "" if no nav arg is found.
func extractNavArg(args []string) string {
	for i, arg := range args {
		switch arg {
		case "--menu", "-M":
			if i+1 < len(args) {
				return args[i+1]
			}
		case "--start-edit-global":
			return "edit-global"
		case "--start-edit-app":
			if i+1 < len(args) {
				return "edit-app:" + args[i+1]
			}
		}
	}
	return ""
}

// handleServerStop signals the server daemon to shut down.
func handleServerStop(ctx context.Context, state *CmdState) error {
	return serve.StopServer(ctx, state.Force)
}

// handleServerDisconnect requests a graceful disconnect of the active session.
func handleServerDisconnect(ctx context.Context, state *CmdState) error {
	return serve.Disconnect(ctx, state.Force)
}
