package serve

import (
	"context"
	"fmt"
	"time"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"

	"github.com/charmbracelet/ssh"
	"charm.land/wish/v2"
)

// sessionBusyMsg is sent to a connecting client when a primary session is
// already active and the server is not configured to allow observers.
const sessionBusyMsg = "\r\nA DockSTARTer2 session is already active.\r\n" +
	"Use 'ds2 --disconnect' on the host to force-release the session.\r\n\r\n"

// tuiMiddleware returns a wish middleware that runs the DS2 TUI for each
// incoming SSH session. If a session is already active the connection is
// rejected with a clear message.
func tuiMiddleware(mgr *SessionManager) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()

			clientIP := s.RemoteAddr().String()
			if err := mgr.AcquirePrimary(clientIP); err != nil {
				logger.Info(ctx, "SSH connection rejected: session already active")
				fmt.Fprint(s, sessionBusyMsg)
				_ = s.Exit(1)
				return
			}
			defer mgr.ReleasePrimary()

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
						if mgr.IsDisconnectRequested() {
							mgr.ClearDisconnectRequest()
							logger.Info(ctx, "Graceful disconnect requested — closing SSH session from %s", s.RemoteAddr())
							cancel()
							return
						}
					}
				}
			}()

			// Wire up window resize events: SSH sends window-change requests
			// rather than SIGWINCH, so we forward them to the TUI via a
			// goroutine that watches the channel wish provides.
			opts := tui.ProgramOptions{
				Input:      s,
				Output:     s,
				WindowSize: makeWindowSizeChan(ptyReq, windowCh, sessCtx),
			}

			logger.Info(ctx, "SSH session started from %s", s.RemoteAddr())

			if err := tui.Start(sessCtx, "", opts); err != nil {
				logger.Error(ctx, "SSH TUI session error: %v", err)
				_ = s.Exit(1)
				return
			}

			logger.Info(ctx, "SSH session ended from %s", s.RemoteAddr())
			_ = s.Exit(0)
		}
	}
}
