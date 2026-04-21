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

	"github.com/charmbracelet/ssh"
	"charm.land/wish/v2"
)

// sessionBusyMsg is kept for potential future use or legacy compatibility.
const sessionBusyMsg = "\r\nA DockSTARTer2 session is already active.\r\n" +
	"Use 'ds2 --disconnect' on the host to force-release the session.\r\n\r\n"

// tuiMiddleware returns a wish middleware that runs the DS2 TUI for each
// incoming SSH session.
func tuiMiddleware(startMenu string) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()

			clientIP := s.RemoteAddr().String()
			if s.User() == "web" {
				// The web proxy connects from loopback; read the real browser IP
				// forwarded via the DS2_CLIENT_IP environment variable.
				for _, env := range s.Environ() {
					if strings.HasPrefix(env, "DS2_CLIENT_IP=") {
						clientIP = strings.TrimPrefix(env, "DS2_CLIENT_IP=")
						break
					}
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
			opts := tui.ProgramOptions{
				Input:         s,
				Output:        s,
				WindowSize:    makeWindowSizeChan(ptyReq, windowCh, sessCtx),
				Environ:       envs,
				InitialWidth:  ptyReq.Window.Width,
				InitialHeight: ptyReq.Window.Height,
			}

			logger.Info(ctx, "SSH session started from %s", s.RemoteAddr())

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
