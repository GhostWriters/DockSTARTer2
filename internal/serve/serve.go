// Package serve implements the optional SSH and web servers that allow remote
// access to the DS2 TUI. All server functionality is disabled by default and
// must be explicitly enabled in dockstarter2.toml.
package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
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
func StartSSHServer(ctx context.Context, cfg config.ServerConfig, startMenu string) error { //nolint:cyclop
	// Register a shutdown hook so that when an update is applied from within a
	// TUI session running inside this daemon, ReExec can cancel the server
	// context and allow main() to pick up PendingReExec and exec the new binary.
	innerCtx, cancelInner := context.WithCancel(ctx)
	defer cancelInner()
	console.DaemonShutdown = cancelInner
	defer func() { console.DaemonShutdown = nil }()
	console.ServerDisconnect = func() { _ = Sessions.RequestDisconnect() }
	defer func() { console.ServerDisconnect = nil }()
	ctx = innerCtx
	if cfg.SSH.Port == 0 {
		return fmt.Errorf("server.ssh.port is not set in dockstarter2.toml")
	}

	if cfg.Auth.Mode == "none" {
		logger.Warn(ctx, "SSH server is running with no authentication (server.auth.mode = \"none\"). "+
			"This is insecure — anyone on the network can connect.")
	}

	// Generate an ephemeral key pair for the internal web proxy client.
	// This key is never written to disk; it is regenerated each startup.
	internalKey, err := generateInternalKey()
	if err != nil {
		return fmt.Errorf("generating internal key: %w", err)
	}

	hostKeyPath := cfg.HostKey
	if hostKeyPath == "" {
		hostKeyPath = filepath.Join(paths.GetStateDir(), "server_host_key")
	}
	if err := os.MkdirAll(filepath.Dir(hostKeyPath), 0700); err != nil {
		return fmt.Errorf("creating host key directory: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.SSH.Port)

	opts := []ssh.Option{
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			tuiMiddleware(Sessions, startMenu),
			logging.Middleware(),
		),
	}

	// isInternalKey checks whether a presented public key is our ephemeral
	// web-proxy key. Used as the fast-path in combined public-key handlers.
	isInternalKey := func(key ssh.PublicKey) bool {
		return ssh.KeysEqual(key, internalKey.PublicKey)
	}

	// Configure authentication. Every branch must register exactly one
	// PublicKeyHandler (wish.WithPublicKeyAuth / wish.WithAuthorizedKeys each
	// set the same underlying field, so two calls would overwrite each other).
	// We therefore build a combined handler wherever needed.
	switch cfg.Auth.Mode {
	case "pubkey":
		if cfg.Auth.AuthKeysFile == "" {
			return fmt.Errorf("server.auth.auth_keys_file must be set when auth mode is \"pubkey\"")
		}
		authKeysFile := cfg.Auth.AuthKeysFile
		opts = append(opts, wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			// Accept the internal web-proxy key OR any key in the authorized_keys file.
			if isInternalKey(key) {
				return true
			}
			return authorizedKeysContains(authKeysFile, key)
		}))
	case "password":
		if cfg.Auth.Password == "" {
			return fmt.Errorf("server.auth.password must be set when auth mode is \"password\"")
		}
		opts = append(opts,
			wish.WithPasswordAuth(func(_ ssh.Context, password string) bool {
				return checkPassword(password, cfg.Auth.Password)
			}),
			// Also accept the internal key via public-key auth so the web proxy
			// can connect without a password.
			wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
				return isInternalKey(key)
			}),
		)
	case "none", "":
		// No auth — allow all connections (warning already logged above).
		// The internal key is implicitly accepted since we allow everything.
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

	logger.Notice(ctx, "SSH server started on port %d", cfg.SSH.Port)

	webPort := 0
	if cfg.Web.Port > 0 {
		webPort = cfg.Web.Port
	}
	if err := Sessions.AcquireServer(cfg.SSH.Port, webPort); err != nil {
		logger.Warn(ctx, "Could not write server PID file: %v", err)
	}
	defer Sessions.ReleaseServer()

	// Start web server alongside SSH if configured.
	if cfg.Web.Port > 0 {
		go func() {
			if err := StartWebServer(ctx, cfg, internalKey.Signer); err != nil {
				logger.Error(ctx, "Web server stopped: %v", err)
			}
		}()
	}

	// Watch for a file-based stop request (written by `ds2 --server stop`).
	// On receipt: close any active session, then cancel the server context.
	// This is more reliable than SIGTERM, which wish may intercept.
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if Sessions.IsStopRequested() {
					Sessions.ClearStopRequest()
					Sessions.RequestDisconnect()
					cancelInner()
					return
				}
			}
		}
	}()

	// Shut down gracefully when context is cancelled.
	go func() {
		<-ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
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

// StopServer signals the running server daemon to shut down gracefully.
// If force is true it kills the process immediately and clears the PID file.
func StopServer(ctx context.Context, force bool) error {
	info := Sessions.ReadServerInfo()
	if info.PID == 0 || !ProcessExists(info.PID) {
		Sessions.ReleaseServer()
		logger.Info(ctx, "Server is not running.")
		return nil
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return fmt.Errorf("finding server process (PID %d): %w", info.PID, err)
	}

	logger.Info(ctx, "Requesting graceful server stop (PID %d)...", info.PID)
	if err := Sessions.RequestStop(); err != nil {
		return fmt.Errorf("writing stop request: %w", err)
	}

	timeout := 10 * time.Second
	if force {
		timeout = 5 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if !ProcessExists(info.PID) {
			logger.Notice(ctx, "Server stopped.")
			return nil
		}
	}

	if !force {
		logger.Warn(ctx, "Server did not stop within 10s. Use '--force --server stop' to force.")
		return nil
	}

	logger.Warn(ctx, "Server did not stop gracefully — forcing stop (PID %d).", info.PID)
	_ = proc.Kill()
	Sessions.ReleaseServer()
	Sessions.ForceRelease()
	Sessions.ClearDisconnectRequest()
	logger.Notice(ctx, "Server stopped.")
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
