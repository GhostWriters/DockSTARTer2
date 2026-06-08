package serve

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/version"
)

// CheckStartupStatus logs information about other running instances of the
// application. Called during startup so the user is aware of concurrent
// sessions. Suppressed when no other instances are running.
func CheckStartupStatus(ctx context.Context) {
	procs := sessionlocks.Sessions.ListProcInfos()

	if len(procs) == 0 {
		return
	}

	editInfo := sessionlocks.Sessions.ReadEditInfo()
	connSessions := sessionlocks.Sessions.ListConnectedSessions()
	serverInfo := sessionlocks.Sessions.ReadServerInfo()

	// Sort so the server instance appears first.
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].PID == serverInfo.PID
	})

	// Build all instance lines then emit as a single multi-line warning.
	lines := []string{fmt.Sprintf("Other %s instances running:", version.ApplicationName)}
	for _, p := range procs {
		// Build tag blocks — each becomes its own [block]
		var tagBlocks []string

		// Server port info: [SSH Port: N, Web Port: N]
		// Only trust ConnInfo if this PID is actually the server.
		isServer := p.PID == serverInfo.PID
		if p.ConnInfo != "" && isServer {
			parts := strings.Fields(p.ConnInfo)
			var portTags []string
			for _, part := range parts {
				kv := strings.SplitN(part, ":", 2)
				if len(kv) == 2 {
					portTags = append(portTags, fmt.Sprintf("%s Port: {{|Version|}}%s{{[-]}}", kv[0], kv[1]))
				}
			}
			if len(portTags) > 0 {
				tagBlocks = append(tagBlocks, strings.Join(portTags, ", "))
			}
		}

		// Connection info for non-server instances.
		// Suppressed for the server instance — its connections show under Connected: instead.
		if !isServer {
			termStr := ""
			if p.Terminal != "" {
				termStr = fmt.Sprintf(" ({{|RunningCommand|}}%s{{[-]}}", p.Terminal) + ")"
			}
			if p.SSHClient != "" {
				tagBlocks = append(tagBlocks, fmt.Sprintf("SSH: {{|IPAddress|}}%s{{[-]}}%s", p.SSHClient, termStr))
			} else {
				tagBlocks = append(tagBlocks, fmt.Sprintf("Local%s", termStr))
			}
		}


		var tagBuf strings.Builder
		for _, t := range tagBlocks {
			tagBuf.WriteString(" [")
			tagBuf.WriteString(t)
			tagBuf.WriteString("]")
		}
		tagStr := tagBuf.String()

		// Build the command line: full exe path + args
		cmdLine := p.ExePath
		if p.Args != "" {
			cmdLine += " " + p.Args
		}

		lines = append(lines,
			fmt.Sprintf("\tPID {{|Version|}}%-7d{{[-]}} [{{|Version|}}%s{{[-]}}]%s", p.PID, p.Version, tagStr),
			fmt.Sprintf("\t\t{{|RunningCommand|}}%s{{[-]}}", cmdLine),
		)
		if !isServer && p.PID == editInfo.PID && editInfo.ConnType != "" {
			conn := editInfo.ConnType
			switch editInfo.LockSource {
			case "cli":
				lines = append(lines, fmt.Sprintf("\t\t{{|Warn|}}Edit lock:{{[-]}} Running CLI command '{{|RunningCommand|}}%s{{[-]}}'.", conn))
			case "console":
				lines = append(lines, fmt.Sprintf("\t\t{{|Warn|}}Edit lock:{{[-]}} Running console command '{{|RunningCommand|}}%s{{[-]}}'.", conn))
			default:
				lines = append(lines, fmt.Sprintf("\t\t{{|Warn|}}Edit lock:{{[-]}} In the '{{|RunningCommand|}}%s{{[-]}}' menu.", conn))
			}
		}

		// Show active connected sessions under the server instance only.
		if isServer && len(connSessions) > 0 {
			lines = append(lines, "\t\t{{|Warn|}}Connected:{{[-]}}")
			for _, cs := range connSessions {
				termStr := ""
				if cs.Terminal != "" {
					termStr = fmt.Sprintf(" ({{|RunningCommand|}}%s{{[-]}}", cs.Terminal) + ")"
				}
				isEditSession := cs.ClientIP == editInfo.ClientIP
				lines = append(lines,
					fmt.Sprintf("\t\t\t%s: {{|IPAddress|}}%s{{[-]}}%s", cs.ConnType, cs.ClientIP, termStr),
				)
				if isEditSession && editInfo.ConnType != "" {
					conn := editInfo.ConnType
					switch editInfo.LockSource {
					case "cli":
						lines = append(lines, fmt.Sprintf("\t\t\t\t{{|Warn|}}Edit lock:{{[-]}} Running CLI command '{{|RunningCommand|}}%s{{[-]}}'.", conn))
					case "console":
						lines = append(lines, fmt.Sprintf("\t\t\t\t{{|Warn|}}Edit lock:{{[-]}} Running console command '{{|RunningCommand|}}%s{{[-]}}'.", conn))
					default:
						lines = append(lines, fmt.Sprintf("\t\t\t\t{{|Warn|}}Edit lock:{{[-]}} In the '{{|RunningCommand|}}%s{{[-]}}' menu.", conn))
					}
				}
			}
		}
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
		connType := editInfo.ConnType
		if connType == "" {
			connType = "unknown"
		}
		var connTypeTag string
		switch editInfo.LockSource {
		case "cli":
			connTypeTag = fmt.Sprintf("{{|UserCommand|}}%s{{[-]}}", connType)
		case "console":
			connTypeTag = fmt.Sprintf("{{|RunningCommand|}}%s{{[-]}}", connType)
		default: // "menu"
			connTypeTag = fmt.Sprintf("{{|Version|}}%s{{[-]}}", connType)
		}
		fmt.Println(console.Sprintf("Editing:     %s from %s", connTypeTag, editInfo.FormatSession()))
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
