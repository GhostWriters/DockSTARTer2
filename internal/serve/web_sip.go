package serve

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/version"

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
	sipCfg.Appearance.Title = webPageTitle()
	for _, site := range cfg.Web.FrameAncestors {
		if err := config.ValidFrameAncestor(site); err != nil {
			return fmt.Errorf("server.web.frame_ancestors in dockstarter2.toml: %w", err)
		}
	}
	sipCfg.FrameAncestors = cfg.Web.FrameAncestors
	for _, host := range cfg.Web.AllowedHosts {
		if err := config.ValidAllowedHost(host); err != nil {
			return fmt.Errorf("server.web.allowed_hosts in dockstarter2.toml: %w", err)
		}
	}
	// DS2 checks the Host header itself (webHostCheckMiddleware), so a
	// refusal reaches DS2's log; sip's own check logs only to stderr.
	sipCfg.AllowedHosts = []string{"*"}

	switch cfg.Web.TLS {
	case "cert":
		if cfg.Web.TLSCert == "" || cfg.Web.TLSKey == "" {
			return fmt.Errorf("server.web.tls_cert and server.web.tls_key must both be set when server.web.tls = \"cert\"")
		}
		sipCfg.TLSCert = cfg.Web.TLSCert
		sipCfg.TLSKey = cfg.Web.TLSKey
	case "none":
		// Binding a non-loopback address without TLS is refused by sip
		// unless explicitly permitted -- this is that explicit opt-in.
		sipCfg.AllowInsecureNoTLS = true
		logger.Warn(ctx, "Web server is running over plain HTTP (server.web.tls = \"none\"). "+
			"Traffic, including any auth.mode = \"password\" credentials, is unencrypted.")
	case "self-signed", "":
		// sip generates and manages its own self-signed keypair rather than
		// requiring an operator-supplied certificate for a LAN home-server
		// tool. Browsers show a one-time warning on first visit.
		sipCfg.AutoTLS = true
	default:
		return fmt.Errorf("unknown server.web.tls %q (valid: self-signed, cert, none)", cfg.Web.TLS)
	}

	if hosts, ok := webAllowedHosts(cfg); ok {
		sipCfg.ConnectMiddleware = append(sipCfg.ConnectMiddleware, webHostCheckMiddleware(ctx, hosts, cfg.Web.AllowedHosts))
	}
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

// webAllowedHosts returns the host names a session's Host header may carry,
// and false when no Host check applies. The check guards against DNS
// rebinding, so it applies only with no TLS and no password: with TLS the
// browser rejects the certificate, which doesn't cover a rebinding page's
// name, and such a page can't supply the password. "*" in
// server.web.allowed_hosts turns it off. Otherwise it is
// server.web.allowed_hosts plus the machine's hostname.
func webAllowedHosts(cfg config.ServerConfig) ([]string, bool) {
	if cfg.Web.TLS != "none" || cfg.Auth.Mode == "password" || slices.Contains(cfg.Web.AllowedHosts, "*") {
		return nil, false
	}
	hosts := append([]string{}, cfg.Web.AllowedHosts...)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		hosts = append(hosts, hostname)
	}
	return hosts, true
}

// webHostRefusalLogInterval is the least time between logged Host refusals,
// so a client sending refused names can't grow the log without bound.
const webHostRefusalLogInterval = time.Minute

// webHostCheckMiddleware refuses a session whose Host header isn't an IP
// address, localhost, a name under .localhost, or one of hosts. A
// rebinding page always sends its own DNS name, so IP addresses are safe.
// Refusals are logged with the command that allows the name alongside the
// configured ones, since the browser can't show why the terminal didn't
// connect: the first at once, then at most one per
// webHostRefusalLogInterval, counting those in between.
func webHostCheckMiddleware(ctx context.Context, hosts, configured []string) sip.ConnectMiddleware {
	var limit logLimiter
	return func(next sip.ConnectHandler) sip.ConnectHandler {
		return func(r *http.Request) error {
			host := normalizeHost(r.Host)
			if !webHostAllowed(host, hosts) {
				if logIt, skipped := limit.allow(time.Now(), webHostRefusalLogInterval); logIt {
					more := ""
					if skipped > 0 {
						more = fmt.Sprintf(" (%d more refused since the last report)", skipped)
					}
					allow := strings.Join(append(slices.Clone(configured), host), " ")
					logger.Warn(ctx, "Refused a web session for host name '{{|Var|}}%s{{[-]}}' from %s%s. Allow it with '{{|UserCommand|}}%s --config-web-hosts %s{{[-]}}'.",
						host, r.RemoteAddr, more, version.CommandName, allow)
				}
				return &sip.ConnectError{
					Status: http.StatusForbidden,
					Body:   "This server does not answer to this host name.",
				}
			}
			return next(r)
		}
	}
}

// logLimiter spaces out repeated log lines. The zero value is ready to use.
type logLimiter struct {
	mu       sync.Mutex
	last     time.Time
	unlogged int
}

// allow reports whether an event at now should be logged, at most once per
// interval, and how many events went unlogged since the last one that was.
func (l *logLimiter) allow(now time.Time, interval time.Duration) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.last.IsZero() && now.Sub(l.last) < interval {
		l.unlogged++
		return false, 0
	}
	skipped := l.unlogged
	l.last, l.unlogged = now, 0
	return true, skipped
}

// webHostAllowed reports whether a normalized host name may reach the web
// server: an IP address, localhost or a name under .localhost (browsers
// resolve those to loopback themselves), or one of hosts.
func webHostAllowed(host string, hosts []string) bool {
	if host == "" {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	return slices.ContainsFunc(hosts, func(h string) bool { return normalizeHost(h) == host })
}

// normalizeHost drops the port, IPv6 brackets and a trailing dot from a host
// name, and lowers its case.
func normalizeHost(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
}

// webPageTitle is the browser tab title: the OS hostname distinguishes
// instances sharing one reverse-proxy domain, where location.hostname alone
// can't.
func webPageTitle() string {
	hostname, err := os.Hostname()
	if err != nil {
		return version.ApplicationName
	}
	return fmt.Sprintf("%s (%s)", version.ApplicationName, hostname)
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
						if err := sessionlocks.Sessions.ClearDisconnectRequest(); err != nil {
							logger.Warn(parentCtx, "Failed to clear disconnect-request file: %v", err)
						}
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
