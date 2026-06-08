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
	"github.com/charmbracelet/ssh"
)

// tuiMiddleware returns a wish middleware that runs the DS2 TUI for each
// incoming SSH session.
func tuiMiddleware(startMenu string) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()

			clientIP := formatIP(s.RemoteAddr().String())
			userAgent := ""
			termProgram := ""
			for _, env := range s.Environ() {
				switch {
				case strings.HasPrefix(env, "DS2_CLIENT_IP="):
					clientIP = strings.TrimPrefix(env, "DS2_CLIENT_IP=")
				case strings.HasPrefix(env, "DS2_USER_AGENT="):
					userAgent = strings.TrimPrefix(env, "DS2_USER_AGENT=")
				case strings.HasPrefix(env, "TERM_PROGRAM="):
					termProgram = strings.TrimPrefix(env, "TERM_PROGRAM=")
				}
			}

			// We no longer block connections at the SSH level.
			// Multiple sessions (local and remote) can coexist.
			// We only use the remote lock to signal activity to the local TUI.
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

			// Watch for a graceful disconnect request from the local user.
			// When detected, cancel the session context so the TUI exits cleanly.
			go func() {
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-sessCtx.Done():
						return
					case <-ticker.C:
						if sessionlocks.Sessions.IsDisconnectRequested() {
							sessionlocks.Sessions.ClearDisconnectRequest()
							logger.Info(ctx, "Graceful disconnect requested — closing SSH session from %s", s.RemoteAddr())
							cancel()
							return
						}
					}
				}
			}()

			envs := s.Environ()
			envs = append(envs, "TERM="+ptyReq.Term)
			envs = append(envs, "DS2_CLIENT_IP="+clientIP)
			if s.User() != "web" {
				envs = append(envs, "DS2_CONN_TYPE=ssh-server")
			}
			opts := tui.ProgramOptions{
				Input:         s,
				Output:        s,
				WindowSize:    makeWindowSizeChan(ptyReq, windowCh, sessCtx),
				Environ:       envs,
				InitialWidth:  ptyReq.Window.Width,
				InitialHeight: ptyReq.Window.Height,
			}

			logger.Info(ctx, "SSH session started from %s", s.RemoteAddr())

			// Register the active connection so startup warnings can show it.
			connType := "SSH"
			var terminal string
			if s.User() == "web" {
				connType = "Web"
				terminal = simplifyUserAgent(userAgent)
			} else {
				// For SSH: "TERM_PROGRAM/TERM" or just "TERM"
				if termProgram != "" {
					terminal = termProgram + "/" + ptyReq.Term
				} else {
					terminal = ptyReq.Term
				}
			}
			sessionID := sessionlocks.Sessions.RegisterSession(clientIP, connType, terminal)
			defer sessionlocks.Sessions.UnregisterSession(sessionID)

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
