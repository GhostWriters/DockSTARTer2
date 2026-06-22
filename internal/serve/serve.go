// Package serve implements the optional SSH and web servers that allow remote
// access to the DS2 TUI. All server functionality is disabled by default and
// must be explicitly enabled in dockstarter2.toml.
package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/sessionlocks"

	"charm.land/wish/v2"
	"charm.land/wish/v2/logging"
	"github.com/charmbracelet/ssh"
)

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
	console.ServerDisconnect = func() { _ = sessionlocks.Sessions.RequestDisconnect() }
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
			tuiMiddleware(startMenu),
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
	if err := sessionlocks.Sessions.AcquireServer(cfg.SSH.Port, webPort); err != nil {
		logger.Warn(ctx, "Could not write server PID file: %v", err)
	}
	defer sessionlocks.Sessions.ReleaseServer()

	// Update proc registration with server port info so other instances
	// can display it in startup warnings. Only meaningful in the daemon process.
	if console.IsDaemon {
		connInfo := fmt.Sprintf("SSH:%d", cfg.SSH.Port)
		if webPort > 0 {
			connInfo += fmt.Sprintf(" Web:%d", webPort)
		}
		sessionlocks.Sessions.UpdateProcConnInfo(connInfo)
	}

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
				if sessionlocks.Sessions.IsStopRequested() {
					sessionlocks.Sessions.ClearStopRequest()
					_ = sessionlocks.Sessions.RequestDisconnect()
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

// StopServer signals server daemon(s) to shut down gracefully.
// If targetPID is non-zero, only that instance is stopped via SIGTERM.
// If targetPID is zero, all instances are stopped via the stop-request file.
// If force is true, processes are killed after the timeout.
func StopServer(ctx context.Context, force bool, targetPID int) error {
	servers := sessionlocks.Sessions.ListServerInfos()
	if len(servers) == 0 {
		logger.Info(ctx, "Server is not running.")
		return nil
	}

	// Filter to target if specified.
	if targetPID != 0 {
		found := false
		for _, s := range servers {
			if s.PID == targetPID {
				found = true
				break
			}
		}
		if !found {
			logger.Warn(ctx, "No running server found with PID %d.", targetPID)
			return nil
		}
		servers = []sessionlocks.ServerInfo{{PID: targetPID}}
	}

	timeout := 10 * time.Second
	if force {
		timeout = 5 * time.Second
	}

	type procEntry struct {
		pid  int
		proc *os.Process
	}
	entries := make([]procEntry, 0, len(servers))
	for _, s := range servers {
		logger.Info(ctx, "Requesting graceful server stop (PID %d).", s.PID)
		e := procEntry{pid: s.PID}
		if p, err := os.FindProcess(s.PID); err == nil {
			e.proc = p
		}
		entries = append(entries, e)
	}

	if targetPID != 0 {
		// Targeted stop — write a PID-specific request file.
		if err := sessionlocks.Sessions.RequestStopPID(targetPID); err != nil {
			return fmt.Errorf("writing stop request: %w", err)
		}
	} else {
		// Broadcast stop via request file — all daemons pick it up.
		if err := sessionlocks.Sessions.RequestStop(); err != nil {
			return fmt.Errorf("writing stop request: %w", err)
		}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		allDead := true
		for _, e := range entries {
			if sessionlocks.ProcessExists(e.pid) {
				allDead = false
				break
			}
		}
		if allDead {
			logger.Notice(ctx, "Server stopped.")
			return nil
		}
	}

	if !force {
		logger.Warn(ctx, "Server did not stop within 10s. Use '--force --server stop' to force.")
		return nil
	}

	for _, e := range entries {
		if e.proc != nil && sessionlocks.ProcessExists(e.pid) {
			logger.Warn(ctx, "Server did not stop gracefully — forcing stop (PID %d).", e.pid)
			_ = e.proc.Kill()
		}
	}
	if targetPID == 0 {
		sessionlocks.Sessions.ForceRelease()
		sessionlocks.Sessions.ClearDisconnectRequest()
	}
	logger.Notice(ctx, "Server stopped.")
	return nil
}

// sessionLabel returns a display string for a connected session.
func sessionLabel(cs sessionlocks.ConnectedSession) string {
	s := "{{|IPAddress|}}" + cs.ClientIP + "{{[-]}}"
	if cs.Terminal != "" {
		s += " ({{|RunningCommand|}}" + cs.Terminal + "{{[-]}})"
	}
	return cs.ConnType + " Server session " + s
}

// Disconnect requests a graceful disconnect of the active editor session.
// It writes a disconnect request file that the session handler watches for,
// then waits up to 10 seconds for the session to close cleanly.
// If force is true, it skips the graceful path and kills immediately.
func Disconnect(ctx context.Context, force bool) error {
	pid := sessionlocks.Sessions.EditLockPID()
	if pid == 0 {
		logger.Warn(ctx, "No active editor session to disconnect.")
		return nil
	}

	err := sessionlocks.Sessions.Disconnect(ctx, force)
	if err != nil {
		return err
	}

	if sessionlocks.Sessions.EditLockPID() == 0 {
		logger.Notice(ctx, "Editor session disconnected.")
	} else if force {
		logger.Warn(ctx, "Editor session could not be forcibly disconnected.")
	} else {
		logger.Warn(ctx, "Editor session did not close within 10s. Use '--force --server disconnect' to forcibly disconnect.")
	}

	return nil
}

// DisconnectSessions disconnects connected sessions matching target.
// target may be "all", "web", "ssh", or an "ip:port" string.
func DisconnectSessions(ctx context.Context, target string, force bool) error {
	sessions := sessionlocks.Sessions.ListConnectedSessions()
	var matched []sessionlocks.ConnectedSession
	for _, cs := range sessions {
		switch target {
		case "all":
			matched = append(matched, cs)
		case "web":
			if cs.ConnType == "Web" {
				matched = append(matched, cs)
			}
		case "ssh":
			if cs.ConnType == "SSH" {
				matched = append(matched, cs)
			}
		default:
			if cs.ClientIP == target {
				matched = append(matched, cs)
			}
		}
	}

	if len(matched) == 0 {
		logger.Warn(ctx, "No matching sessions found for target %q.", target)
		return nil
	}

	deadline := time.Now().Add(10 * time.Second)
	for _, cs := range matched {
		label := sessionLabel(cs)
		if force {
			_ = sessionlocks.Sessions.RequestSessionDisconnect(cs.ID)
		} else {
			_ = sessionlocks.Sessions.RequestSessionDisconnect(cs.ID)
			// Wait up to deadline for this session to unregister.
			for time.Now().Before(deadline) {
				time.Sleep(250 * time.Millisecond)
				found := false
				for _, s := range sessionlocks.Sessions.ListConnectedSessions() {
					if s.ID == cs.ID {
						found = true
						break
					}
				}
				if !found {
					break
				}
			}
		}
		// Check if it's gone.
		gone := true
		for _, s := range sessionlocks.Sessions.ListConnectedSessions() {
			if s.ID == cs.ID {
				gone = false
				break
			}
		}
		if gone {
			logger.Notice(ctx, "Disconnected session: %s.", label)
		} else {
			logger.Warn(ctx, "Failed to disconnect session: %s.", label)
		}
	}
	return nil
}
