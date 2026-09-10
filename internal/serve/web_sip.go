package serve

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui"

	sip "github.com/Gaurav-Gosain/sip"

	tea "charm.land/bubbletea/v2"
)

// webUserAgentKey is the context key sip's User-Agent-capturing
// ConnectMiddleware stores under -- sip only exposes the client's remote
// address via context (sip.RemoteAddrFromContext), not its User-Agent, so
// this carries the same value the old loopback-SSH proxy forwarded via a
// DS2_USER_AGENT env var.
type webUserAgentKey struct{}

// StartSipWebServer starts sip's browser-terminal server for web-based TUI
// access. Unlike the loopback-SSH proxy it replaces, a sip session runs the
// DS2 TUI directly -- no local SSH server, ephemeral keypair, or PTY-over-SSH
// hop involved. It blocks until ctx is cancelled.
func StartSipWebServer(ctx context.Context, cfg config.ServerConfig, startMenu string) error {
	if cfg.Web.Port == 0 {
		return fmt.Errorf("server.web.port is not set in dockstarter2.toml")
	}

	switch cfg.Auth.Mode {
	case "pubkey":
		// HTTP Basic Auth has no public-key equivalent -- refuse rather than
		// silently exposing the web server with no auth when the operator
		// explicitly locked SSH down to pubkey-only.
		return fmt.Errorf("server.auth.mode %q has no web equivalent -- use \"password\" or \"none\" for web access, or leave server.web.port unset", cfg.Auth.Mode)
	case "none":
		logger.Warn(ctx, "Web server is running with no authentication (server.auth.mode = \"none\"). "+
			"This is insecure — anyone on the network can connect.")
	}

	sipCfg := sip.DefaultConfig()
	sipCfg.Host = "0.0.0.0"
	sipCfg.Port = strconv.Itoa(cfg.Web.Port)
	// Binding a non-loopback address without TLS is refused by sip itself;
	// AutoTLS has it generate and reuse a self-signed keypair rather than
	// requiring an operator-supplied certificate for a LAN home-server tool.
	sipCfg.AutoTLS = true
	sipCfg.ConnectMiddleware = append(sipCfg.ConnectMiddleware, captureUserAgentMiddleware)
	if cfg.Auth.Mode == "password" {
		if cfg.Auth.Password == "" {
			return fmt.Errorf("server.auth.password must be set when auth mode is \"password\"")
		}
		sipCfg.ConnectMiddleware = append(sipCfg.ConnectMiddleware, sipPasswordAuthMiddleware(cfg.Auth.Password))
	}

	srv := sip.NewServer(sipCfg)
	logger.Notice(ctx, "Web server started on port %d", cfg.Web.Port)
	return srv.ServeWithProgram(ctx, newSipProgramHandler(ctx, startMenu))
}

// captureUserAgentMiddleware stashes the connecting browser's User-Agent
// into the request context, the same pattern sip's own WithRemoteAddr uses,
// so it's readable later via sess.Context().
func captureUserAgentMiddleware(next sip.ConnectHandler) sip.ConnectHandler {
	return func(r *http.Request) error {
		r = r.WithContext(context.WithValue(r.Context(), webUserAgentKey{}, r.UserAgent()))
		return next(r)
	}
}

// sipPasswordAuthMiddleware returns a sip ConnectMiddleware performing HTTP
// Basic Auth against passwordHash (a bcrypt hash, matching the SSH server's
// own password auth) -- sip's built-in BasicUsername/BasicPassword config
// fields do a plain string compare, which can't be used against a bcrypt
// hash directly.
func sipPasswordAuthMiddleware(passwordHash string) sip.ConnectMiddleware {
	return func(next sip.ConnectHandler) sip.ConnectHandler {
		return func(r *http.Request) error {
			_, pw, ok := r.BasicAuth()
			if !ok || !checkPassword(pw, passwordHash) {
				headers := make(http.Header)
				headers.Add("WWW-Authenticate", `Basic realm="DockSTARTer2"`)
				return &sip.ConnectError{
					Status:  http.StatusUnauthorized,
					Headers: headers,
					Body:    "Unauthorized",
				}
			}
			return next(r)
		}
	}
}

// newSipProgramHandler returns the per-session handler sip calls for every
// new browser connection. It mirrors ssh_handler.go's tuiMiddleware, minus
// the trust-env plumbing that existed only to authenticate the old loopback
// web-proxy connection as a real SSH client -- sip gives each session its
// own real remote address directly, so there's nothing to spoof.
func newSipProgramHandler(parentCtx context.Context, startMenu string) sip.ProgramHandler {
	return func(sess sip.Session) *tea.Program {
		clientIP := formatIP(sip.RemoteAddrFromContext(sess.Context()))
		userAgent, _ := sess.Context().Value(webUserAgentKey{}).(string)
		terminal := simplifyUserAgent(userAgent)

		sessionID := sessionlocks.Sessions.RegisterSession(clientIP, "Web", terminal)

		pty := sess.Pty()
		envs := []string{
			"TERM=xterm-256color",
			"DS2_CLIENT_IP=" + clientIP,
			"DS2_SESSION_ID=" + sessionID,
		}

		opts := tui.ProgramOptions{
			Input:         sess,
			Output:        sess,
			WindowSize:    makeSipWindowSizeChan(pty, sess.WindowChanges(), sess.Context()),
			Environ:       envs,
			InitialWidth:  pty.Width,
			InitialHeight: pty.Height,
			// WebOutbound/WebToken are left unset: sip's Session has no
			// side channel for arbitrary JSON pushed to the browser outside
			// the terminal byte stream, so the old web-only features built
			// on it (live display-settings echo, mobile-keyboard-focus
			// hints, exit-triggered browser reload) have no equivalent here
			// yet. Worth revisiting once this is confirmed worth keeping.
		}

		p, finish, err := tui.StartForSession(sess.Context(), startMenu, opts)
		if err != nil {
			logger.Error(parentCtx, "Web session failed to start: %v", err)
			sessionlocks.Sessions.UnregisterSession(sessionID)
			return nil
		}

		logger.Info(parentCtx, "Web session started from %s", clientIP)

		// Watch for graceful disconnect requests (global or per-session).
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-sess.Context().Done():
					return
				case <-ticker.C:
					if sessionlocks.Sessions.IsDisconnectRequested() {
						sessionlocks.Sessions.ClearDisconnectRequest()
						logger.Info(parentCtx, "Graceful disconnect requested — closing web session from %s", clientIP)
						p.Quit()
						return
					}
					if sessionlocks.Sessions.IsSessionDisconnectRequested(sessionID) {
						sessionlocks.Sessions.ClearSessionDisconnectRequest(sessionID)
						logger.Info(parentCtx, "Per-session disconnect requested — closing web session from %s", clientIP)
						p.Quit()
						return
					}
				}
			}
		}()

		// sess.Context() is cancelled once the program's Run() call (which
		// sip itself invokes) returns, so waiting on it here is the correct
		// "this session is over" signal for the cleanup Start would
		// otherwise perform inline after its own p.Run() call.
		go func() {
			<-sess.Context().Done()
			finish()
			sessionlocks.Sessions.UnregisterSession(sessionID)
			logger.Info(parentCtx, "Web session ended from %s", clientIP)
		}()

		return p
	}
}

// makeSipWindowSizeChan converts sip's window-change channel into a
// tui.WindowSizeEvent channel, the same pattern makeWindowSizeChan uses for
// wish's SSH sessions.
func makeSipWindowSizeChan(initial sip.Pty, changes <-chan sip.WindowSize, ctx context.Context) <-chan tui.WindowSizeEvent {
	ch := make(chan tui.WindowSizeEvent, 8)
	go func() {
		defer close(ch)
		ch <- tui.WindowSizeEvent{Width: initial.Width, Height: initial.Height}
		for {
			select {
			case <-ctx.Done():
				return
			case w, ok := <-changes:
				if !ok {
					return
				}
				ch <- tui.WindowSizeEvent{Width: w.Width, Height: w.Height}
			}
		}
	}()
	return ch
}
