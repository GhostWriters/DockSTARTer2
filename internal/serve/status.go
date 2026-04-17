package serve

import (
	"context"
	"net"

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

// PrintServerStatus prints the current server and session state to the logger.
func PrintServerStatus(ctx context.Context) {
	// ── Service ──────────────────────────────────────────────────────────────
	installed, _ := ServiceInstalled()
	if installed {
		enabled, _ := ServiceEnabled()
		if enabled {
			logger.Notice(ctx, "Service:  {{|Success|}}installed{{[-]}} and {{|Success|}}enabled{{[-]}} (starts at login)")
		} else {
			logger.Notice(ctx, "Service:  {{|Highlight|}}installed{{[-]}} but {{|Highlight|}}disabled{{[-]}} (won't start at login)")
		}
	} else {
		logger.Notice(ctx, "Service:  not installed")
	}

	// ── Server ───────────────────────────────────────────────────────────────
	serverInfo := Sessions.ReadServerInfo()
	if serverInfo.PID == 0 || !ProcessExists(serverInfo.PID) {
		logger.Notice(ctx, "Server:   {{|Highlight|}}not running{{[-]}}")
		logger.Notice(ctx, "Session:  none")
		return
	}

	if serverInfo.Port > 0 {
		logger.Notice(ctx, "Server:   {{|Success|}}running{{[-]}} — SSH port {{|Highlight|}}%d{{[-]}} (PID %d)", serverInfo.Port, serverInfo.PID)
	} else {
		logger.Notice(ctx, "Server:   {{|Success|}}running{{[-]}} (PID %d)", serverInfo.PID)
	}
	if serverInfo.WebPort > 0 {
		logger.Notice(ctx, "Web:      {{|Success|}}running{{[-]}} — port {{|Highlight|}}%d{{[-]}}", serverInfo.WebPort)
	}

	// ── Session ───────────────────────────────────────────────────────────────
	sessionInfo := Sessions.ReadSessionInfo()
	if sessionInfo.PID != 0 && ProcessExists(sessionInfo.PID) {
		ip := formatIP(sessionInfo.ClientIP)
		connType := sessionInfo.ConnType
		if connType == "" {
			connType = "ssh"
		}
		logger.Notice(ctx, "Session:  {{|Highlight|}}active{{[-]}} — %s connection from {{|Highlight|}}%s{{[-]}} (PID %d)", connType, ip, sessionInfo.PID)
	} else {
		logger.Notice(ctx, "Session:  none")
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
