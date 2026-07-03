package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/webmsg"

	"github.com/coder/websocket"
	gossh "golang.org/x/crypto/ssh"
)

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
func handleWebSocket(ctx context.Context, conn *websocket.Conn, clientAddr, userAgent string, cfg config.ServerConfig, signer gossh.Signer) {
	defer func() { _ = conn.CloseNow() }()

	// Wait for the browser's initial resize AND display-settings-init,
	// sent back-to-back in ws.onopen (web_static/index.html), so the
	// session's real display settings are recorded before webmsg.Register
	// below -- otherwise the Browser Settings dialog can read the
	// fresh-session defaults if opened before the later WebSocket-read
	// goroutine gets to this same message.
	var initialCols, initialRows int
	initialCols, initialRows = 80, 24
	var pendingDisplay *webmsg.DisplaySettings

	// Two messages are expected; stop early once both arrive, but never
	// wait past the overall timeout for a straggler.
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	for haveResize, haveDisplay := false, false; !haveResize || !haveDisplay; {
		_, data, err := conn.Read(waitCtx)
		if err != nil {
			break
		}
		var rm resizeMsg
		if json.Unmarshal(data, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
			initialCols, initialRows = rm.Cols, rm.Rows
			haveResize = true
			continue
		}
		var cm struct {
			Type           string `json:"type"`
			FontFamily     string `json:"fontFamily"`
			FontSize       int    `json:"fontSize"`
			UseDefaultFont bool   `json:"useDefaultFont"`
		}
		if json.Unmarshal(data, &cm) == nil && cm.Type == "display-settings-init" {
			pendingDisplay = &webmsg.DisplaySettings{
				FontFamily:     cm.FontFamily,
				FontSize:       cm.FontSize,
				UseDefaultFont: cm.UseDefaultFont,
			}
			haveDisplay = true
			continue
		}
	}
	waitCancel()

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

	// Register the outbound channel before starting the SSH session so the
	// SSH handler can look it up via DS2_WEB_TOKEN before tui.Start() runs.
	webKey := fmt.Sprintf("%s-%p", formatIP(clientAddr), conn)
	outboundCh := webmsg.Register(webKey)
	defer webmsg.Unregister(webKey)
	if pendingDisplay != nil {
		webmsg.SetDisplaySettings(webKey, *pendingDisplay)
	}

	// Forward the real browser IP, User-Agent, and web token so the SSH handler
	// can record them and wire up the outbound channel.
	_ = sshSess.Setenv("DS2_CLIENT_IP", clientAddr)
	_ = sshSess.Setenv("DS2_WEB_TOKEN", webKey)
	if userAgent != "" {
		_ = sshSess.Setenv("DS2_USER_AGENT", userAgent)
	}

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

	// sshExited is set to true when the SSH session closes normally (TUI exited).
	// Used to send a reload signal to the browser so it reconnects immediately
	// rather than showing "Connection lost".
	var sshExited bool

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
				// EOF means the TUI exited normally.
				sshExited = true
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

			// Text frames starting with '{' may be control messages.
			if len(data) > 0 && data[0] == '{' {
				var rm resizeMsg
				if json.Unmarshal(data, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
					_ = sshSess.WindowChange(rm.Rows, rm.Cols)
					continue
				}
				var cm struct {
					Type           string `json:"type"`
					FontFamily     string `json:"fontFamily"`
					FontSize       int    `json:"fontSize"`
					UseDefaultFont bool   `json:"useDefaultFont"`
				}
				if json.Unmarshal(data, &cm) == nil && cm.Type == "display-settings-init" {
					webmsg.SetDisplaySettings(webKey, webmsg.DisplaySettings{
						FontFamily:     cm.FontFamily,
						FontSize:       cm.FontSize,
						UseDefaultFont: cm.UseDefaultFont,
					})
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

	// Forward TUI-initiated outbound messages (e.g. display-settings) to the browser.
	go func() {
		for {
			select {
			case <-proxyCtx.Done():
				return
			case msg, ok := <-outboundCh:
				if !ok {
					return
				}
				_ = conn.Write(proxyCtx, websocket.MessageText, msg)
			}
		}
	}()

	// Wait for the proxy to finish (either side closed).
	<-proxyCtx.Done()

	// If the TUI exited normally (user chose Exit), tell the browser to reload
	// so it reconnects to a fresh session immediately without a "Connection lost" message.
	if sshExited {
		_ = conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"reload"}`))
	}

	logger.Info(ctx, "Web session ended from %s", clientAddr)
}
