package serve

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"

	"github.com/coder/websocket"
	gossh "golang.org/x/crypto/ssh"
)

//go:embed web_static
var webStaticFS embed.FS

// StartWebServer starts the HTTP/WebSocket server for browser-based TUI access.
// It blocks until ctx is cancelled. The signer is the ephemeral internal key
// used by the web proxy to authenticate with the local SSH server.
func StartWebServer(ctx context.Context, cfg config.ServerConfig, signer gossh.Signer) error {
	if cfg.Web.Port == 0 {
		return fmt.Errorf("server.web.port is not set in dockstarter2.toml")
	}

	staticRoot, err := fs.Sub(webStaticFS, "web_static")
	if err != nil {
		return fmt.Errorf("loading embedded static files: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))
	mux.HandleFunc("/ws", authMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocketHTTP(ctx, w, r, cfg, signer)
	})).ServeHTTP)

	addr := fmt.Sprintf(":%d", cfg.Web.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logger.Notice(ctx, "Web server started on port %d", cfg.Web.Port)

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
// mode is "password" or "pubkey". For "none" it passes through.
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
		return next
	}
}

// handleWebSocketHTTP upgrades an HTTP request to a WebSocket connection and
// hands it off to the SSH-proxy handler.
func handleWebSocketHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, cfg config.ServerConfig, signer gossh.Signer) {
	clientAddr := r.RemoteAddr
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		logger.Error(ctx, "WebSocket upgrade failed: %v", err)
		return
	}
	conn.SetReadLimit(1 << 20) // 1 MiB
	handleWebSocket(ctx, conn, clientAddr, cfg, signer)
}
