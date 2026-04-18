package serve

import (
	"context"
	"fmt"
	"net"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
)

// CheckStartupStatus logs warnings if an SSH server or active session is
// detected from a previous or concurrent process. Called during DS2 startup
// so the user is aware of any running server/session before executing commands.
func CheckStartupStatus(ctx context.Context) {
	serverInfo := Sessions.ReadServerInfo()
	if serverInfo.PID != 0 && ProcessExists(serverInfo.PID) {
		if serverInfo.Port > 0 {
			logger.Warn(ctx, "SSH server is running on port {{|Highlight|}}%d{{[-]}} (PID %d).", serverInfo.Port, serverInfo.PID)
		} else {
			logger.Warn(ctx, "SSH server is running (PID %d).", serverInfo.PID)
		}

		sessionInfo := Sessions.ReadSessionInfo()
		if sessionInfo.PID != 0 && ProcessExists(sessionInfo.PID) {
			ip := formatIP(sessionInfo.ClientIP)
			logger.Warn(ctx, "Active SSH session connected from {{|Highlight|}}%s{{[-]}}.", ip)
		}
	}
}

// PrintServerStatus prints the current server and session state to stdout.
func PrintServerStatus(_ context.Context, cfg config.ServerConfig) {
	// ── Service ──────────────────────────────────────────────────────────────
	installed, _ := ServiceInstalled()
	if installed {
		enabled, _ := ServiceEnabled()
		if enabled {
			fmt.Println("Service:  installed, enabled (starts at boot)")
		} else {
			fmt.Println("Service:  installed, disabled (won't start at boot)")
		}
	} else {
		fmt.Println("Service:  not installed")
	}

	// ── Server ───────────────────────────────────────────────────────────────
	serverInfo := Sessions.ReadServerInfo()
	serverRunning := serverInfo.PID != 0 && ProcessExists(serverInfo.PID)

	sshPort := serverInfo.Port
	if sshPort == 0 {
		sshPort = cfg.SSH.Port
	}
	if sshPort > 0 {
		if serverRunning {
			fmt.Printf("SSH Server:  port %d (running, PID %d)\n", sshPort, serverInfo.PID)
		} else {
			fmt.Printf("SSH Server:  port %d (stopped)\n", sshPort)
		}
	} else {
		fmt.Println("SSH Server:  not configured")
	}

	webPort := serverInfo.WebPort
	if webPort == 0 {
		webPort = cfg.Web.Port
	}
	if webPort > 0 {
		if serverRunning && serverInfo.WebPort > 0 {
			fmt.Printf("Web Server:  port %d (running)\n", webPort)
		} else {
			fmt.Printf("Web Server:  port %d (stopped)\n", webPort)
		}
	}

	// ── Session ───────────────────────────────────────────────────────────────
	if !serverRunning {
		fmt.Println("Session:  none")
		return
	}
	sessionInfo := Sessions.ReadSessionInfo()
	if sessionInfo.PID != 0 && ProcessExists(sessionInfo.PID) {
		ip := formatIP(sessionInfo.ClientIP)
		connType := sessionInfo.ConnType
		if connType == "" {
			connType = "ssh"
		}
		fmt.Printf("Session:  active — %s from %s (PID %d)\n", connType, ip, sessionInfo.PID)
	} else {
		fmt.Println("Session:  none")
	}
}

// formatIP strips the port from an addr string like "192.168.1.10:54321"
// and returns just the IP. Falls back to the raw string on parse failure.
func formatIP(addr string) string {
	if addr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
