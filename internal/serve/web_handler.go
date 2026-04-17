package serve

import (
	"context"
	"fmt"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"

	"golang.org/x/net/websocket"
)

// sessionBusyWebMsg is sent to a browser when a primary session is already active.
const sessionBusyWebMsg = "A DockSTARTer2 session is already active.\r\nUse 'ds2 --disconnect' on the host to force-release the session.\r\n"

// handleWebSocket is called for each incoming WebSocket connection.
// It enforces session locking and then runs the DS2 TUI over the WebSocket.
func handleWebSocket(ctx context.Context, ws *websocket.Conn, cfg config.ServerConfig) {
	clientAddr := ws.Request().RemoteAddr

	if err := Sessions.AcquirePrimary(clientAddr); err != nil {
		logger.Info(ctx, "Web connection rejected: session already active")
		_ = websocket.Message.Send(ws, sessionBusyWebMsg)
		ws.Close()
		return
	}
	defer Sessions.ReleasePrimary()

	// Build a cancelable context for this session's lifetime.
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

	rw := newWSReadWriter(ws)

	// Start reading from the WebSocket in the background.
	// This feeds terminal input into rw's pipe and resize events into rw.resizeCh.
	go rw.readLoop(sessCtx)

	// Wire the WebSocket I/O into ProgramOptions.
	opts := tui.ProgramOptions{
		Input:      rw,
		Output:     rw,
		WindowSize: rw.resizeCh,
	}

	logger.Info(ctx, "Web session started from %s", clientAddr)

	if err := tui.Start(sessCtx, "", opts); err != nil {
		logger.Error(ctx, "Web TUI session error: %v", err)
		_ = websocket.Message.Send(ws, fmt.Sprintf("\r\nSession error: %v\r\n", err))
		return
	}

	logger.Info(ctx, "Web session ended from %s", clientAddr)
}
