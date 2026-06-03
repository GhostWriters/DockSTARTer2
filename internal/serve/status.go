package serve

import (
	"context"
	"fmt"
	"net"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
)

// CheckStartupStatus logs information about other running instances of the
// application. Called during startup so the user is aware of concurrent
// sessions. Suppressed when no other instances are running.
func CheckStartupStatus(ctx context.Context) {
	procs := sessionlocks.Sessions.ListProcInfos()
	serverInfo := sessionlocks.Sessions.ReadServerInfo()

	if len(procs) == 0 {
		return
	}

	editInfo := sessionlocks.Sessions.ReadEditInfo()

	// Build all instance lines then emit as a single multi-line warning.
	lines := []string{"Other instances running:"}
	for _, p := range procs {
		// Build tags: [SSH Server: N, Web Server: N] and/or [Edit lock]
		var tags []string
		if p.PID == serverInfo.PID {
			if serverInfo.Port > 0 && serverInfo.WebPort > 0 {
				tags = append(tags, fmt.Sprintf("SSH Server: {{|Version|}}%d{{[-]}}, Web Server: {{|Version|}}%d{{[-]}}",
					serverInfo.Port, serverInfo.WebPort))
			} else if serverInfo.Port > 0 {
				tags = append(tags, fmt.Sprintf("SSH Server: {{|Version|}}%d{{[-]}}", serverInfo.Port))
			}
		}
		if editInfo.PID == p.PID {
			tags = append(tags, "{{|Warn|}}Edit lock{{[-]}}")
		}

		tagStr := ""
		if len(tags) > 0 {
			tagStr = " [" + strings.Join(tags, ", ") + "]"
		}

		// Build the command line: full exe path + args
		cmdLine := p.ExePath
		if p.Args != "" {
			cmdLine += " " + p.Args
		}

		lines = append(lines,
			fmt.Sprintf("\tPID {{|Version|}}%-6d{{[-]}} [{{|Version|}}%s{{[-]}}]%s", p.PID, p.Version, tagStr),
			fmt.Sprintf("\t\t{{|RunningCommand|}}%s{{[-]}}", cmdLine),
		)
	}
	logger.Warn(ctx, lines)
}

// PrintServerStatus prints the current server and session state to stdout.
func PrintServerStatus(_ context.Context, cfg config.ServerConfig) {
	// ── Service ──────────────────────────────────────────────────────────────
	installed, _ := ServiceInstalled()
	if installed {
		enabled, _ := ServiceEnabled()
		if enabled {
			fmt.Println("Service:     installed, enabled (starts at boot)")
		} else {
			fmt.Println("Service:     installed, disabled (won't start at boot)")
		}
	} else {
		fmt.Println("Service:     not installed")
	}

	// ── Server ───────────────────────────────────────────────────────────────
	serverInfo := sessionlocks.Sessions.ReadServerInfo()
	serverRunning := serverInfo.PID != 0 && sessionlocks.ProcessExists(serverInfo.PID)

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
		fmt.Println("Editing:     no active editor")
		return
	}
	if sessionlocks.Sessions.IsEditLocked() {
		editInfo := sessionlocks.Sessions.ReadEditInfo()
		ip := formatIP(editInfo.ClientIP)
		connType := editInfo.ConnType
		if connType == "" {
			connType = "ssh"
		}
		fmt.Printf("Editing:     %s from %s\n", connType, ip)
	} else {
		fmt.Println("Editing:     no active editor")
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
