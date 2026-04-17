package cmd

import (
	"context"
	"fmt"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/serve"
)

// handleServe starts the SSH server using settings from dockstarter2.toml.
// Blocks until the context is cancelled (e.g. Ctrl-C).
func handleServe(ctx context.Context, conf *config.AppConfig) error {
	if !conf.Server.Enabled {
		return fmt.Errorf(
			"server is disabled in dockstarter2.toml — set [server] enabled = true and [server.ssh] port = <port> to enable",
		)
	}
	return serve.StartSSHServer(ctx, conf.Server)
}

// handleDisconnect requests a graceful disconnect of the active SSH session.
// With --force, it kills the session immediately instead of waiting.
func handleDisconnect(ctx context.Context, state *CmdState) error {
	if err := serve.Disconnect(ctx, state.Force); err != nil {
		logger.Error(ctx, "Failed to disconnect session: %v", err)
		return err
	}
	return nil
}
