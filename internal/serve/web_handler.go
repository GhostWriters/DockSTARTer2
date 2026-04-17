package serve

import (
	"context"
	"fmt"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"

	"github.com/coder/websocket"
)

// sessionBusyWebMsg is sent to a browser when a primary session is already active.
const sessionBusyWebMsg = "A DockSTARTer2 session is already active.\r\nUse 'ds2 --disconnect' on the host to force-release the session.\r\n"

// handleWebSocket is called for each accepted WebSocket connection.
// It enforces session locking and then runs the DS2 TUI over the WebSocket.
func handleWebSocket(ctx context.Context, conn *websocket.Conn, clientAddr string, cfg config.ServerConfig) {
	defer conn.CloseNow()

	if err := Sessions.AcquirePrimary(clientAddr); err != nil {
		logger.Info(ctx, "Web connection rejected: session already active")
		_ = conn.Write(ctx, websocket.MessageText, []byte(sessionBusyWebMsg))
		return
	}
	defer Sessions.ReleasePrimary()

	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Watch for graceful disconnect requests (same mechanism as SSH sessions).
	go func() {
		for {
			select {
			case <-sessCtx.Done():
				return
			default:
			}
			if Sessions.IsDisconnectRequested() {
				Sessions.ClearDisconnectRequest()
				logger.Info(ctx, "Graceful disconnect requested — closing web session from %s", clientAddr)
				cancel()
				return
			}
			select {
			case <-sessCtx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()

	rw := newWSReadWriter(conn)
	go rw.readLoop(sessCtx)

	// Wait for the browser's initial resize so we know terminal dimensions
	// before the TUI renders its first frame.
	var initialSize tui.WindowSizeEvent
	select {
	case sz := <-rw.resizeCh:
		initialSize = sz
	case <-time.After(2 * time.Second):
		initialSize = tui.WindowSizeEvent{Width: 80, Height: 24}
	case <-sessCtx.Done():
		return
	}

	opts := tui.ProgramOptions{
		Input:         rw,
		Output:        rw,
		WindowSize:    rw.resizeCh,
		Environ:       []string{"TERM=xterm-256color", "COLORTERM=truecolor"},
		ForceColors:   true,
		InitialWidth:  initialSize.Width,
		InitialHeight: initialSize.Height,
	}

	logger.Info(ctx, "Web session started from %s (%dx%d)", clientAddr, initialSize.Width, initialSize.Height)

	if err := tui.Start(sessCtx, "", opts); err != nil {
		logger.Error(ctx, "Web TUI session error: %v", err)
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("\r\nSession error: %v\r\n", err)))
		return
	}

	logger.Info(ctx, "Web session ended from %s", clientAddr)
}
