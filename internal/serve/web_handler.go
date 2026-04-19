package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"

	"github.com/coder/websocket"
	gossh "golang.org/x/crypto/ssh"
)

// sessionBusyWebMsg is sent to a browser when a primary session is already active.
const sessionBusyWebMsg = "A DockSTARTer2 session is already active.\r\nUse 'ds2 --disconnect' on the host to force-release the session.\r\n"

// resizeMsg is the JSON structure the browser sends for terminal resize events.
type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// handleWebSocket proxies a WebSocket connection through to the local wish SSH
// server. The browser's xterm.js instance drives the session; resize events
// are translated into SSH window-change requests so bubbletea receives proper
// tea.WindowSizeMsg messages via the normal SSH path.
func handleWebSocket(ctx context.Context, conn *websocket.Conn, clientAddr string, cfg config.ServerConfig, signer gossh.Signer) {
	defer conn.CloseNow()

	// Wait for the browser's initial resize so we can set the PTY size before
	// the first frame is rendered.
	var initialCols, initialRows int
	initialCols, initialRows = 80, 24

	// Use a short-lived context just for the resize wait.
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	_, data, err := conn.Read(waitCtx)
	waitCancel()
	if err == nil {
		var rm resizeMsg
		if json.Unmarshal(data, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
			initialCols, initialRows = rm.Cols, rm.Rows
		}
	}

	// Connect to the local wish SSH server using the ephemeral internal key.
	sshAddr := fmt.Sprintf("127.0.0.1:%d", cfg.SSH.Port)
	sshCfg := &gossh.ClientConfig{
		User: "web",
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		// We are connecting to our own server on loopback; accept any host key.
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         5 * time.Second,
	}

	sshClient, err := gossh.Dial("tcp", sshAddr, sshCfg)
	if err != nil {
		logger.Error(ctx, "Web proxy: SSH dial failed: %v", err)
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("\r\nFailed to connect to SSH server: %v\r\n", err)))
		return
	}
	defer sshClient.Close()

	sshSess, err := sshClient.NewSession()
	if err != nil {
		logger.Error(ctx, "Web proxy: SSH new session failed: %v", err)
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("\r\nFailed to open SSH session: %v\r\n", err)))
		return
	}
	defer sshSess.Close()

	// Forward the real browser IP so the SSH handler can record it instead of 127.0.0.1.
	_ = sshSess.Setenv("DS2_CLIENT_IP", clientAddr)

	// Request a PTY with the dimensions the browser reported.
	if err := sshSess.RequestPty("xterm-256color", initialRows, initialCols, gossh.TerminalModes{}); err != nil {
		logger.Error(ctx, "Web proxy: PTY request failed: %v", err)
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("\r\nFailed to request PTY: %v\r\n", err)))
		return
	}

	// Wire up stdin/stdout pipes.
	sshIn, err := sshSess.StdinPipe()
	if err != nil {
		logger.Error(ctx, "Web proxy: stdin pipe failed: %v", err)
		return
	}
	sshOut, err := sshSess.StdoutPipe()
	if err != nil {
		logger.Error(ctx, "Web proxy: stdout pipe failed: %v", err)
		return
	}

	if err := sshSess.Shell(); err != nil {
		logger.Error(ctx, "Web proxy: shell start failed: %v", err)
		_ = conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf("\r\nFailed to start shell: %v\r\n", err)))
		return
	}

	logger.Info(ctx, "Web session proxied via SSH from %s (%dx%d)", clientAddr, initialCols, initialRows)

	proxyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// SSH stdout → WebSocket (binary frames).
	go func() {
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := sshOut.Read(buf)
			if n > 0 {
				if werr := conn.Write(proxyCtx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → SSH stdin (input) or window-change (resize).
	go func() {
		defer cancel()
		for {
			_, data, err := conn.Read(proxyCtx)
			if err != nil {
				return
			}

			// Text frames starting with '{' may be resize messages.
			if len(data) > 0 && data[0] == '{' {
				var rm resizeMsg
				if json.Unmarshal(data, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
					_ = sshSess.WindowChange(rm.Rows, rm.Cols)
					continue
				}
			}

			// Everything else is terminal input.
			if _, err := sshIn.Write(data); err != nil {
				return
			}
		}
	}()

	// Watch for graceful disconnect requests.
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-proxyCtx.Done():
				return
			case <-ticker.C:
				if sessionlocks.Sessions.IsDisconnectRequested() {
					sessionlocks.Sessions.ClearDisconnectRequest()
					logger.Info(ctx, "Graceful disconnect requested — closing web session from %s", clientAddr)
					cancel()
					return
				}
			}
		}
	}()

	// Wait for the proxy to finish (either side closed).
	<-proxyCtx.Done()

	logger.Info(ctx, "Web session ended from %s", clientAddr)
}
