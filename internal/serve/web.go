package serve

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"

	"golang.org/x/net/websocket"
)

//go:embed web_static
var webStaticFS embed.FS

// StartWebServer starts the HTTP/WebSocket server for browser-based TUI access.
// It blocks until ctx is cancelled. Requires the SSH server's ServerConfig for
// auth settings (the web server shares the [server.auth] config section).
func StartWebServer(ctx context.Context, cfg config.ServerConfig) error {
	if !cfg.Web.Enabled {
		return fmt.Errorf("web server is not enabled in dockstarter2.toml")
	}
	if cfg.Web.Port == 0 {
		return fmt.Errorf("server.web.port is not set in dockstarter2.toml")
	}

	staticRoot, err := fs.Sub(webStaticFS, "web_static")
	if err != nil {
		return fmt.Errorf("loading embedded static files: %w", err)
	}

	mux := http.NewServeMux()

	// Serve static files (index.html, etc.)
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))

	// WebSocket endpoint
	mux.Handle("/ws", authMiddleware(cfg, websocket.Handler(func(ws *websocket.Conn) {
		handleWebSocket(ctx, ws, cfg)
	})))

	addr := fmt.Sprintf(":%d", cfg.Web.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logger.Info(ctx, "Web server listening on http://localhost%s", addr)

	// Shut down when context is cancelled.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server: %w", err)
	}
	return nil
}

// authMiddleware wraps a handler with HTTP Basic Auth when the server auth
// mode is "password". For "pubkey" mode it also uses password Basic Auth
// (the stored bcrypt hash). For "none" it passes through.
func authMiddleware(cfg config.ServerConfig, next http.Handler) http.Handler {
	switch cfg.Auth.Mode {
	case "password", "pubkey":
		hash := cfg.Auth.Password
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, pw, ok := r.BasicAuth()
			if !ok || hash == "" || !checkPassword(pw, hash) {
				w.Header().Set("WWW-Authenticate", `Basic realm="DockSTARTer2"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	default:
		// "none" — no auth
		return next
	}
}

// resizeMsg is the JSON structure the browser sends for terminal resize events.
type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// wsReadWriter wraps a websocket.Conn as an io.ReadWriter for the TUI.
// Reads come from an internal pipe fed by the WebSocket read loop.
// Writes go directly to the WebSocket as binary frames.
type wsReadWriter struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	pr      *io.PipeReader
	pw      *io.PipeWriter
	resizeCh chan tui.WindowSizeEvent
}

func newWSReadWriter(ws *websocket.Conn) *wsReadWriter {
	pr, pw := io.Pipe()
	return &wsReadWriter{
		ws:       ws,
		pr:       pr,
		pw:       pw,
		resizeCh: make(chan tui.WindowSizeEvent, 4),
	}
}

func (w *wsReadWriter) Read(p []byte) (int, error) {
	return w.pr.Read(p)
}

func (w *wsReadWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := websocket.Message.Send(w.ws, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// readLoop pumps WebSocket messages into the pipe (terminal input) or resize channel.
// Runs until the WebSocket closes or ctx is cancelled.
func (w *wsReadWriter) readLoop(ctx context.Context) {
	defer w.pw.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg string
		if err := websocket.Message.Receive(w.ws, &msg); err != nil {
			return
		}

		// Detect resize JSON message
		if len(msg) > 0 && msg[0] == '{' {
			var rm resizeMsg
			if json.Unmarshal([]byte(msg), &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
				select {
				case w.resizeCh <- tui.WindowSizeEvent{Width: rm.Cols, Height: rm.Rows}:
				default:
					// Drop if channel is full — next resize will arrive shortly
				}
				continue
			}
		}

		// Terminal input — write to pipe
		if _, err := w.pw.Write([]byte(msg)); err != nil {
			return
		}
	}
}
