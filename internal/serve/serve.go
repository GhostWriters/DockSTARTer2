// Package serve implements the optional SSH and web servers that allow remote
// access to the DS2 TUI. All server functionality is disabled by default and
// must be explicitly enabled in dockstarter2.toml.
package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui"

	"charm.land/wish/v2"
	"charm.land/wish/v2/logging"
	"charm.land/ssh"
)

func init() {
	config.ServerTLSDefaultHook = applyServerTLSDefault
}

// applyServerTLSDefault backfills server.web.tls for a config file saved
// before that field existed (see config.ServerTLSDefaultHook's doc comment).
// The field is new with the switch to sip for the web frontend -- an
// existing user who was already running a plain-HTTP web server under the
// old frontend must not be silently switched to requiring HTTPS, since that
// breaks any bookmark/script hitting "http://" outright. present["TLS"]
// being false means this config predates the field entirely (a file saved
// after this shipped always has it, once merged back to disk on any load or
// save -- see LoadAppConfig), so this only ever fires once per such file.
//
// conf.Server.Web.Port alone can't answer "was a server already in active
// use" -- the embedded default config ships with it already non-zero
// (40080), so a install that has never touched server settings at all looks
// identical to one that has. The two real signals are: another instance of
// this program is right now registered as a running server with a web port
// (sessionlocks tracks this across processes), or this one is installed/
// enabled as a system service (systemd/launchd) -- both mean this user
// deliberately set the web server up before, as opposed to just inheriting
// an untouched default.
//
// Accepted gap: a server that was stopped and never installed/enabled as a
// system service leaves no trace anywhere (not in config, not on disk), so
// it looks identical to a fresh install and gets the secure default too --
// there is no remaining signal to check.
func applyServerTLSDefault(conf *config.AppConfig, present map[string]bool) {
	if present["TLS"] {
		return
	}
	if serverAlreadyInUse() {
		conf.Server.Web.TLS = "none"
	}
	// Else: leave it at the compiled-in default ("self-signed") -- this is
	// either a genuinely fresh install, or an existing config whose server
	// was never actually enabled/run, which should get the secure default.
}

// serverAlreadyInUse reports whether the web server appears to already be
// in active use by this installation, via either signal described in
// applyServerTLSDefault's doc comment.
func serverAlreadyInUse() bool {
	for _, p := range sessionlocks.Sessions.ListProcInfos() {
		if p.IsServer && p.WebPort > 0 {
			return true
		}
	}
	if installed, err := ServiceInstalled(); err == nil && installed {
		return true
	}
	if enabled, err := ServiceEnabled(); err == nil && enabled {
		return true
	}
	return false
}

// StartServer starts whichever of the SSH (wish) and web (sip) servers have
// a port configured -- each is independently optional, matching the "port
// is the intent signal" convention SSHConfig/WebConfig each already use on
// their own. It blocks until ctx is cancelled. Returns an error if neither
// is configured, or if the one(s) that are fail to start (e.g. port already
// in use, bad config).
func StartServer(ctx context.Context, cfg config.ServerConfig, startMenu string) error { //nolint:cyclop
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

	if cfg.SSH.Port == 0 && cfg.Web.Port == 0 {
		return fmt.Errorf("neither server.ssh.port nor server.web.port is set in dockstarter2.toml")
	}

	var sshServer *ssh.Server
	if cfg.SSH.Port > 0 {
		var err error
		sshServer, err = newSSHServer(cfg, startMenu)
		if err != nil {
			return err
		}
	}

	if err := sessionlocks.Sessions.AcquireServer(cfg.SSH.Port, cfg.Web.Port); err != nil {
		logger.Warn(ctx, "Could not write server PID file: %v", err)
	}
	defer sessionlocks.Sessions.ReleaseServer()

	// Update proc registration with server port info so other instances
	// can display it in startup warnings. Only meaningful in the daemon process.
	if console.IsDaemon {
		var connInfo string
		if cfg.SSH.Port > 0 {
			connInfo = fmt.Sprintf("SSH:%d", cfg.SSH.Port)
		}
		if cfg.Web.Port > 0 {
			if connInfo != "" {
				connInfo += " "
			}
			connInfo += fmt.Sprintf("Web:%d", cfg.Web.Port)
		}
		sessionlocks.Sessions.UpdateProcConnInfo(connInfo)
	}

	// errCh carries the first failure from either server; a nil send means
	// that server stopped cleanly (context cancellation).
	errCh := make(chan error, 2)
	running := 0

	if cfg.Web.Port > 0 {
		running++
		go func() {
			err := StartSipWebServer(ctx, cfg, startMenu)
			if err != nil {
				logger.Error(ctx, "Web server stopped: %v", err)
			}
			errCh <- err
		}()
	}

	if sshServer != nil {
		running++
		go func() {
			<-ctx.Done()
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = sshServer.Shutdown(shutCtx)
		}()
		go func() {
			err := sshServer.ListenAndServe()
			// ErrServerClosed is expected on clean shutdown.
			if err != nil && err.Error() == "ssh: Server closed" {
				err = nil
			}
			if err != nil {
				logger.Error(ctx, "SSH server stopped: %v", err)
			}
			errCh <- err
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

	// Poll for a binary update independent of any connected session -- a
	// per-session watcher only runs while someone is connected, so an idle
	// daemon would otherwise never notice an update until the next connection.
	tui.StartDaemonRestartWatcher(ctx)

	// Wait for every started server to stop; report the first real error.
	var firstErr error
	for range running {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			cancelInner()
		}
	}

	// sip's own session shutdown (see StartSipWebServer/web_sip.go) signals
	// and cancels each session but doesn't itself wait for that session's
	// cleanup goroutine -- edit-lock release included -- to actually finish.
	// A caller re-execing right after StartServer returns (see
	// update.ReExec) replaces the process image outright via syscall.Exec,
	// which would simply erase any such cleanup still in flight. Wait for
	// it here so that never happens -- bounded, same 5-second grace period
	// as the SSH server's own shutdown above, so one wedged session can't
	// block a restart forever.
	//
	// sync.WaitGroup has no cancellable wait, so the goroutine below is
	// abandoned (not killed) if the timeout fires -- accepted rather than
	// engineered around, since it's harmless here: StartServer has exactly
	// one call site (cmd/executor_serve.go) and is never called again in
	// the same process, whose very next step is always syscall.Exec
	// (destroying every goroutine along with the rest of the process
	// image) or exiting outright. Neither leaves anything for a stray
	// goroutine to affect.
	waited := make(chan struct{})
	go func() {
		tui.WaitForActiveSessions()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		logger.Warn(ctx, "Timed out waiting for active sessions to finish cleanup before restarting.")
	}

	return firstErr
}

// newSSHServer builds (but does not start) the wish SSH server using
// settings from cfg.
func newSSHServer(cfg config.ServerConfig, startMenu string) (*ssh.Server, error) {
	if cfg.Auth.Mode == "none" {
		logger.Warn(context.Background(), "SSH server is running with no authentication (server.auth.mode = \"none\"). "+
			"This is insecure — anyone on the network can connect.")
	}

	hostKeyPath := cfg.HostKey
	if hostKeyPath == "" {
		hostKeyPath = filepath.Join(paths.GetStateDir(), "server_host_key")
	}
	if err := os.MkdirAll(filepath.Dir(hostKeyPath), 0700); err != nil {
		return nil, fmt.Errorf("creating host key directory: %w", err)
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

	// Configure authentication. Every branch must register exactly one
	// PublicKeyHandler (wish.WithPublicKeyAuth / wish.WithAuthorizedKeys each
	// set the same underlying field, so two calls would overwrite each other).
	switch cfg.Auth.Mode {
	case "pubkey":
		if cfg.Auth.AuthKeysFile == "" {
			return nil, fmt.Errorf("server.auth.auth_keys_file must be set when auth mode is \"pubkey\"")
		}
		authKeysFile := cfg.Auth.AuthKeysFile
		opts = append(opts, wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			return authorizedKeysContains(authKeysFile, key)
		}))
	case "password":
		if cfg.Auth.Password == "" {
			return nil, fmt.Errorf("server.auth.password must be set when auth mode is \"password\"")
		}
		opts = append(opts, wish.WithPasswordAuth(func(_ ssh.Context, password string) bool {
			return checkPassword(password, cfg.Auth.Password)
		}))
	case "none", "":
		// No auth — allow all connections (warning already logged above).
		opts = append(opts, wish.WithPublicKeyAuth(func(_ ssh.Context, _ ssh.PublicKey) bool {
			return true
		}))
	default:
		return nil, fmt.Errorf("unknown server.auth.mode %q (valid: password, pubkey, none)", cfg.Auth.Mode)
	}

	srv, err := wish.NewServer(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating SSH server: %w", err)
	}

	// wish.WithHostKeyPath generates the key file itself (on first run) but
	// doesn't guarantee restrictive permissions on it -- pin them explicitly
	// rather than relying on the library's default or the OS umask.
	if err := os.Chmod(hostKeyPath, 0600); err != nil {
		logger.Warn(context.Background(), "Could not restrict host key file permissions: %v", err)
	}

	logger.Notice(context.Background(), "SSH server started on port %d", cfg.SSH.Port)
	return srv, nil
}

// FindServersByPort returns all server instances that use targetPort as either
// their SSH or web port. Returns all servers if targetPort is 0.
func FindServersByPort(targetPort int) []sessionlocks.ServerInfo {
	servers := sessionlocks.Sessions.ListServerInfos()
	if targetPort == 0 {
		return servers
	}
	var matched []sessionlocks.ServerInfo
	for _, s := range servers {
		if s.Port == targetPort || s.WebPort == targetPort {
			matched = append(matched, s)
		}
	}
	return matched
}

// StopServer signals server daemon(s) to shut down gracefully.
// If targetPort is non-zero, only instances using that port are stopped.
// If targetPort is zero, all instances are stopped.
// If force is true, processes are killed after the timeout.
func StopServer(ctx context.Context, force bool, targetPort int) error {
	servers := FindServersByPort(targetPort)
	if len(servers) == 0 {
		if targetPort != 0 {
			logger.Warn(ctx, "No running server found on port %d.", targetPort)
		} else {
			logger.Info(ctx, "Server is not running.")
		}
		return nil
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
		logger.Info(ctx, "Requesting graceful server stop (PID %d, port %d).", s.PID, s.Port)
		e := procEntry{pid: s.PID}
		if p, err := os.FindProcess(s.PID); err == nil {
			e.proc = p
		}
		entries = append(entries, e)
	}

	if targetPort != 0 {
		// Targeted stop — write PID-specific request files for matched instances.
		for _, s := range servers {
			if err := sessionlocks.Sessions.RequestStopPID(s.PID); err != nil {
				return fmt.Errorf("writing stop request: %w", err)
			}
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
	if targetPort == 0 {
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
// target may be "all", "web", "ssh", an "ip:port" string, or a bare port number.
// A bare port number matches sessions on a server instance using that SSH or web port.
func DisconnectSessions(ctx context.Context, target string, force bool) error {
	sessions := sessionlocks.Sessions.ListConnectedSessions()

	// Resolve a bare port number to the set of server PIDs using that port.
	var portServerPIDs map[int]bool
	if targetPort, err := strconv.Atoi(target); err == nil && targetPort > 0 {
		portServerPIDs = make(map[int]bool)
		for _, s := range FindServersByPort(targetPort) {
			portServerPIDs[s.PID] = true
		}
	}

	var matched []sessionlocks.ConnectedSession
	for _, cs := range sessions {
		switch {
		case target == "all":
			matched = append(matched, cs)
		case target == "web":
			if cs.ConnType == "Web" {
				matched = append(matched, cs)
			}
		case target == "ssh":
			if cs.ConnType == "SSH" {
				matched = append(matched, cs)
			}
		case portServerPIDs != nil:
			if portServerPIDs[cs.ServerPID] {
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
