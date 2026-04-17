package cmd

import (
	"context"
	"fmt"

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

// handleServerStart spawns the server as a background process.
// It validates the config before attempting to start.
func handleServerStart(ctx context.Context, conf *config.AppConfig) error {
	if !conf.Server.Enabled {
		logger.Warn(ctx, "Server is disabled in dockstarter2.toml — set [server] enabled = true to enable.")
		return nil
	}
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

	logger.Notice(ctx, "Starting server in the background...")
	logger.Warn(ctx, "Background server start is not yet implemented — coming in Stage 3.")
	return nil
}

// handleServerStop requests a graceful shutdown of the running server.
func handleServerStop(ctx context.Context, state *CmdState) error {
	info := serve.Sessions.ReadServerInfo()
	if info.PID == 0 {
		logger.Notice(ctx, "Server is not running.")
		return nil
	}
	return serve.Disconnect(ctx, state.Force)
}

// handleServerDisconnect requests a graceful disconnect of the active session.
func handleServerDisconnect(ctx context.Context, state *CmdState) error {
	return serve.Disconnect(ctx, state.Force)
}
