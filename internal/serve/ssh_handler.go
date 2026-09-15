package serve

import (
	"context"
	"fmt"
	"strings"
	"time"

	"DockSTARTer2/internal/lockfile"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui"

	"charm.land/wish/v2"
	"charm.land/ssh"
)

// ds2TrustEnvPrefixes are the env vars DS2 itself sets to convey trusted
// facts about a session (connection type, client IP, session/token
// identity) -- never values a client should be able to supply itself via
// SSH env forwarding.
var ds2TrustEnvPrefixes = []string{
	"DS2_CONN_TYPE=",
	"DS2_CLIENT_IP=",
	"DS2_SESSION_ID=",
}

// stripDS2TrustEnv drops any entries matching ds2TrustEnvPrefixes from a
// client-supplied environment, so the real values DS2 appends afterward
// can't be shadowed or duplicated by something the client requested.
func stripDS2TrustEnv(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, env := range environ {
		trusted := false
		for _, prefix := range ds2TrustEnvPrefixes {
			if strings.HasPrefix(env, prefix) {
				trusted = true
				break
			}
		}
		if !trusted {
			filtered = append(filtered, env)
		}
	}
	return filtered
}

// tuiMiddleware returns a wish middleware that runs the DS2 TUI for each
// incoming SSH session.
func tuiMiddleware(startMenu string) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()

			clientIP := formatIP(s.RemoteAddr().String())
			termProgram := ""
			for _, env := range s.Environ() {
				if strings.HasPrefix(env, "TERM_PROGRAM=") {
					termProgram = strings.TrimPrefix(env, "TERM_PROGRAM=")
				}
			}

			// Multiple sessions (local and remote) can coexist; this lock
			// only signals activity to the local TUI, it doesn't block
			// connections at the SSH level.
			rlock, _ := lockfile.AcquireShared(paths.GetRemoteLockPath())
			if rlock != nil {
				defer rlock.Release()
			}

			ptyReq, windowCh, isPTY := s.Pty()
			if !isPTY {
				fmt.Fprint(s, "\r\nDS2 requires an interactive terminal (PTY). "+
					"Connect with: ssh -t ...\r\n\r\n")
				_ = s.Exit(1)
				return
			}

			// Build a cancelable context tied to the SSH session lifetime.
			sessCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Register the active connection so startup warnings can show it,
			// and so its sessionID can be threaded into the TUI as the
			// identity edit-lock re-entry checks against (see AcquireEditLock)
			// -- computed before opts/envs so it can ride along in Environ,
			// same as DS2_CLIENT_IP below.
			// For SSH: "TERM_PROGRAM/TERM" or just "TERM"
			var terminal string
			if termProgram != "" {
				terminal = termProgram + "/" + ptyReq.Term
			} else {
				terminal = ptyReq.Term
			}
			sessionID := sessionlocks.Sessions.RegisterSession(clientIP, "SSH", terminal)
			defer sessionlocks.Sessions.UnregisterSession(sessionID)

			// DS2's own trust markers (connection type/identity, consumed by
			// parseClientInfo to decide local vs. remote and gate
			// remote-only features like System Console) must never be
			// satisfiable by whatever the client itself requested via SSH
			// env forwarding -- discard any client-supplied copies before
			// appending the real ones below, rather than relying solely on
			// DS2's own entries coming last (append order happens to make
			// this safe today, but that's incidental, not guaranteed).
			envs := stripDS2TrustEnv(s.Environ())
			envs = append(envs, "TERM="+ptyReq.Term)
			envs = append(envs, "DS2_CLIENT_IP="+clientIP)
			envs = append(envs, "DS2_SESSION_ID="+sessionID)
			envs = append(envs, "DS2_CONN_TYPE=ssh-server")
			opts := tui.ProgramOptions{
				Input:         s,
				Output:        s,
				WindowSize:    makeWindowSizeChan(ptyReq, windowCh, sessCtx),
				Environ:       envs,
				InitialWidth:  ptyReq.Window.Width,
				InitialHeight: ptyReq.Window.Height,
			}

			logger.Info(ctx, "SSH session started from %s", s.RemoteAddr())

			// Watch for graceful disconnect requests (global or per-session).
			go func() {
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-sessCtx.Done():
						return
					case <-ticker.C:
						if sessionlocks.Sessions.IsDisconnectRequested() {
							if err := sessionlocks.Sessions.ClearDisconnectRequest(); err != nil {
								logger.Warn(ctx, "Failed to clear disconnect-request file: %v", err)
							}
							logger.Info(ctx, "Graceful disconnect requested — closing SSH session from %s", s.RemoteAddr())
							cancel()
							return
						}
						if sessionlocks.Sessions.IsSessionDisconnectRequested(sessionID) {
							sessionlocks.Sessions.ClearSessionDisconnectRequest(sessionID)
							logger.Info(ctx, "Per-session disconnect requested — closing SSH session from %s", s.RemoteAddr())
							cancel()
							return
						}
					}
				}
			}()

			if err := tui.Start(sessCtx, startMenu, opts); err != nil {
				logger.Error(ctx, "SSH TUI session error: %v", err)
				_ = s.Exit(1)
				return
			}

			logger.Info(ctx, "SSH session ended from %s", s.RemoteAddr())
			_ = s.Exit(0)
		}
	}
}

// simplifyUserAgent returns a short browser name from a User-Agent string.
// Falls back to the raw string if no known browser is detected.
func simplifyUserAgent(ua string) string {
	if ua == "" {
		return ""
	}
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "Edge/"):
		return "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Safari/") && strings.Contains(ua, "Version/"):
		return "Safari"
	case strings.Contains(ua, "Firefox/"):
		return "Firefox"
	default:
		// Return first token as a best-effort name
		if idx := strings.IndexAny(ua, " /"); idx > 0 {
			return ua[:idx]
		}
		return ua
	}
}
