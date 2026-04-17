// Package serve implements the optional SSH (and future web) server that
// allows remote access to the DS2 TUI. All server functionality is disabled
// by default and must be explicitly enabled in dockstarter2.toml.
package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	"time"

	"github.com/charmbracelet/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/logging"
)

// Global session manager — one per process lifetime.
var Sessions = NewSessionManager()

// StartSSHServer starts the wish SSH server using settings from cfg.
// It blocks until ctx is cancelled. Returns an error if the server cannot
// be started (e.g. port already in use, bad config).
func StartSSHServer(ctx context.Context, cfg config.ServerConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("server is disabled in dockstarter2.toml (set server.enabled = true to enable)")
	}
	if cfg.SSHPort == 0 {
		return fmt.Errorf("server.ssh_port is not set in dockstarter2.toml")
	}

	if cfg.Auth.Mode == "none" {
		logger.Warn(ctx, "SSH server is running with no authentication (server.auth.mode = \"none\"). "+
			"This is insecure — anyone on the network can connect.")
	}

	hostKeyPath := cfg.HostKey
	if hostKeyPath == "" {
		hostKeyPath = filepath.Join(paths.GetStateDir(), "server_host_key")
	}
	if err := os.MkdirAll(filepath.Dir(hostKeyPath), 0700); err != nil {
		return fmt.Errorf("creating host key directory: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.SSHPort)

	opts := []ssh.Option{
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			tuiMiddleware(Sessions),
			logging.Middleware(),
		),
	}

	// Configure authentication
	switch cfg.Auth.Mode {
	case "pubkey":
		if cfg.Auth.AuthKeysFile == "" {
			return fmt.Errorf("server.auth.auth_keys_file must be set when auth mode is \"pubkey\"")
		}
		opts = append(opts, wish.WithAuthorizedKeys(cfg.Auth.AuthKeysFile))
	case "password":
		if cfg.Auth.Password == "" {
			return fmt.Errorf("server.auth.password must be set when auth mode is \"password\"")
		}
		opts = append(opts, wish.WithPasswordAuth(func(_ ssh.Context, password string) bool {
			return checkPassword(password, cfg.Auth.Password)
		}))
	case "none", "":
		// No auth — allow all connections (warning already logged above)
		opts = append(opts, wish.WithPublicKeyAuth(func(_ ssh.Context, _ ssh.PublicKey) bool {
			return true
		}))
	default:
		return fmt.Errorf("unknown server.auth.mode %q (valid: password, pubkey, none)", cfg.Auth.Mode)
	}

	srv, err := wish.NewServer(opts...)
	if err != nil {
		return fmt.Errorf("creating SSH server: %w", err)
	}

	logger.Info(ctx, "SSH server listening on %s", addr)

	if err := Sessions.AcquireServer(cfg.SSHPort); err != nil {
		logger.Warn(ctx, "Could not write server PID file: %v", err)
	}
	defer Sessions.ReleaseServer()

	// Shut down gracefully when context is cancelled.
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != nil {
		// ErrServerClosed is expected on clean shutdown.
		if err.Error() == "ssh: Server closed" {
			return nil
		}
		return fmt.Errorf("SSH server: %w", err)
	}
	return nil
}

// Disconnect requests a graceful disconnect of the active SSH session.
// It writes a disconnect request file that the session handler watches for,
// then waits up to 10 seconds for the session to close cleanly.
// If force is true, it skips the graceful path and kills immediately.
func Disconnect(ctx context.Context, force bool) error {
	pid := Sessions.SessionLockPID()
	if pid == 0 {
		logger.Info(ctx, "No active session found.")
		return nil
	}

	if force {
		return forceDisconnect(ctx, pid)
	}

	logger.Info(ctx, "Requesting graceful disconnect (PID %d)...", pid)
	if err := Sessions.RequestDisconnect(); err != nil {
		return fmt.Errorf("writing disconnect request: %w", err)
	}

	// Wait up to 10 seconds for the session to release the lock.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if Sessions.SessionLockPID() == 0 {
			logger.Info(ctx, "Session disconnected successfully.")
			return nil
		}
	}

	logger.Warn(ctx, "Session did not close within 10s. Use '--force --disconnect' to forcibly disconnect.")
	return nil
}

// forceDisconnect immediately signals the session process and clears lock files.
func forceDisconnect(ctx context.Context, pid int) error {
	logger.Info(ctx, "Forcing disconnect of session (PID %d)...", pid)
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = signalProcess(proc)
	}
	Sessions.ForceRelease()
	Sessions.ClearDisconnectRequest()
	logger.Info(ctx, "Session forcibly disconnected.")
	return nil
}
