package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/sessionlocks"
)

// parsePortArgs reads up to two port values from args.
// Returns -1 for any argument that is missing or empty (meaning "not specified / keep original").
// Returns 0 for an explicit "0" or "" sentinel meaning "disable" (web port only).
// The distinction: missing arg → -1 (keep), explicit "" or "0" → 0 (disable/auto-detect).
// parsePortArgs reads up to two port values from args.
// Missing or empty ("") arg → -1 (not specified / keep original).
// Explicit "0" → 0 (disable web, or auto-detect target).
// Any positive number → that port.
func parsePortArgs(args []string) (sshPort, webPort int) {
	sshPort, webPort = -1, -1
	if len(args) > 0 {
		if args[0] != "" {
			p, _ := strconv.Atoi(args[0])
			sshPort = p // "0" → 0, number → number
		}
		// "" → stays -1
	}
	if len(args) > 1 {
		if args[1] != "" {
			p, _ := strconv.Atoi(args[1])
			webPort = p
		}
		// "" → stays -1
	}
	return
}

// handleServer routes --server [subcommand] to the appropriate handler.
// No subcommand defaults to "status". All subcommands print a notice and
// return, allowing further command-line options to continue executing.
func handleServer(ctx context.Context, group *CommandGroup, state *CmdState, conf *config.AppConfig) error {
	sub := "status"
	if len(group.Args) > 0 {
		sub = group.Args[0]
	}
	// Arg layout differs by subcommand:
	//   start   [sshPort [webPort]]
	//   stop    [port]                         — port=0 or "" stops all
	//   restart [port [newSshPort [newWebPort]]] — port=0 or "" targets single instance
	//   install [sshPort [webPort]]
	//   enable  [sshPort [webPort]]
	tail := group.Args[min(1, len(group.Args)):]

	var sshPort, webPort, targetPort int
	switch sub {
	case "stop":
		// targetPort=0 means stop all; "" and "0" both map to 0.
		if len(tail) > 0 && tail[0] != "" {
			targetPort, _ = strconv.Atoi(tail[0])
		}
	case "restart":
		// targetPort=0 means auto-detect single instance; "" and "0" both map to 0.
		if len(tail) > 0 && tail[0] != "" {
			targetPort, _ = strconv.Atoi(tail[0])
		}
		sshPort, webPort = parsePortArgs(tail[min(1, len(tail)):])
	default:
		sshPort, webPort = parsePortArgs(tail)
	}

	switch sub {
	case "status", "":
		return handleServerStatus(ctx, conf)
	case "start":
		return handleServerStart(ctx, conf, sshPort, webPort)
	case "stop":
		return handleServerStop(ctx, state, targetPort)
	case "restart":
		return handleServerRestart(ctx, state, conf, targetPort, sshPort, webPort)
	case "disconnect":
		target := ""
		if len(group.Args) > 1 {
			target = group.Args[1]
		}
		return handleServerDisconnect(ctx, state, target)
	case "install":
		return handleServerInstall(ctx, sshPort, webPort)
	case "uninstall":
		return handleServerUninstall(ctx)
	case "enable":
		return handleServerEnable(ctx, sshPort, webPort)
	case "disable":
		return handleServerDisable(ctx)
	default:
		return fmt.Errorf("unknown server subcommand %q", sub)
	}
}

// handleServerStatus prints the current server and session state.
func handleServerStatus(ctx context.Context, conf *config.AppConfig) error {
	serve.PrintServerStatus(ctx, conf.Server)
	return nil
}

// handleServerStart spawns the server as a background daemon process.
// It validates the config before attempting to start.
func handleServerStart(ctx context.Context, conf *config.AppConfig, sshPort, webPort int) error {
	// -1 means "not specified" — fall back to config.
	if sshPort < 0 {
		sshPort = conf.Server.SSH.Port
	}
	if sshPort == 0 {
		logger.Warn(ctx, "server.ssh.port is not set in dockstarter2.toml — cannot start server.")
		return nil
	}
	if webPort < 0 {
		webPort = conf.Server.Web.Port // -1 = not specified, use config
	}
	// webPort == 0 means explicitly disabled — no web server

	// Check if already running on the target port.
	for _, info := range sessionlocks.Sessions.ListServerInfos() {
		if info.Port == sshPort {
			logger.Notice(ctx, "Server is already running on port %d (PID %d).", info.Port, info.PID)
			return nil
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	var portArgs []string
	if sshPort != conf.Server.SSH.Port || webPort != conf.Server.Web.Port {
		// Only embed ports in the daemon args when explicitly overriding config.
		portArgs = append(portArgs, strconv.Itoa(sshPort))
		if webPort > 0 {
			portArgs = append(portArgs, strconv.Itoa(webPort))
		}
	}

	logger.Notice(ctx, "Starting server in the background on SSH port %d%s.", sshPort, fmtWebPort(webPort))
	proc, err := serve.SpawnDaemon(execPath, portArgs)
	if err != nil {
		return fmt.Errorf("spawning server daemon: %w", err)
	}
	logger.Notice(ctx, "Server started (PID %d).", proc.Pid)
	return nil
}

// fmtWebPort returns a display string for the web port, or empty if none.
func fmtWebPort(webPort int) string {
	if webPort > 0 {
		return fmt.Sprintf(", web port %d", webPort)
	}
	return ""
}

// handleServeDaemon is the internal handler for --server-daemon.
// It runs the blocking SSH (and web) server loop. Never called directly by
// the user — only invoked when --server start re-execs the binary.
//
// group.Args may contain nav args such as ["--menu", "start-options"] or
// ["--start-edit-app", "plex"] that were appended by SpawnDaemon so the
// daemon can restore navigation state for reconnecting web/SSH sessions.
func handleServeDaemon(ctx context.Context, group *CommandGroup, conf *config.AppConfig) error {
	console.IsDaemon = true
	startMenu := extractNavArg(group.Args)
	sshPort, webPort := parsePortArgs(group.Args)
	if sshPort > 0 {
		conf.Server.SSH.Port = sshPort
	}
	if webPort > 0 {
		conf.Server.Web.Port = webPort
	}
	return serve.StartSSHServer(ctx, conf.Server, startMenu)
}

// extractNavArg parses nav args appended after --server-daemon and returns
// the startMenu string to pass to the TUI (e.g. "start-options", "plex").
// Returns "" if no nav arg is found.
func extractNavArg(args []string) string {
	for i, arg := range args {
		switch arg {
		case "--menu", "-M":
			if i+1 < len(args) {
				return args[i+1]
			}
		case "--start-edit-global":
			return "edit-global"
		case "--start-edit-app":
			if i+1 < len(args) {
				return "edit-app:" + args[i+1]
			}
		}
	}
	return ""
}

// handleServerStop signals the server daemon to shut down.
func handleServerStop(ctx context.Context, state *CmdState, targetPort int) error {
	return serve.StopServer(ctx, state.Force, targetPort)
}

// handleServerRestart stops matching instance(s) and starts a new one.
// targetPort=0 with new ports: restarts the single running instance with new ports, or warns and restarts all without port changes if multiple.
// targetPort=0 with no new ports: restarts all instances keeping their ports.
// targetPort>0: stops that instance, starts new one with new ports (or config ports).
func handleServerRestart(ctx context.Context, state *CmdState, conf *config.AppConfig, targetPort, newSSHPort, newWebPort int) error {
	all := serve.FindServersByPort(0)
	targets := serve.FindServersByPort(targetPort)

	// -1 means "not specified / keep original".
	// 0 for SSH means "keep original" (0 is never a valid SSH port).
	// 0 for web means "disable web server".
	// > 0: explicit new port.
	newPortsSpecified := newSSHPort != -1 || newWebPort != -1

	if targetPort == 0 && newPortsSpecified && len(all) > 1 {
		logger.Warn(ctx, "Multiple server instances running — cannot determine which to restart with new ports. Restarting all without changing ports.")
		newSSHPort, newWebPort = -1, -1
		newPortsSpecified = false
	}

	if err := serve.StopServer(ctx, state.Force, targetPort); err != nil {
		return err
	}

	// No new ports specified — restart each stopped instance keeping its original ports.
	if !newPortsSpecified {
		for _, s := range targets {
			if err := handleServerStart(ctx, conf, s.Port, s.WebPort); err != nil {
				return err
			}
		}
		return nil
	}

	// New ports specified — resolve unspecified/keep values against the stopped instance's ports.
	origSSH, origWeb := -1, -1
	if len(targets) == 1 {
		origSSH, origWeb = targets[0].Port, targets[0].WebPort
	}
	// SSH: -1 or 0 both mean "keep original" (0 is never a valid SSH port).
	if newSSHPort <= 0 {
		if origSSH > 0 {
			newSSHPort = origSSH
		} else {
			newSSHPort = -1 // fall back to config in handleServerStart
		}
	}
	// Web: -1 means "keep original"; 0 means "disable".
	if newWebPort == -1 {
		if origWeb >= 0 {
			newWebPort = origWeb
		} else {
			newWebPort = -1 // fall back to config
		}
	}

	return handleServerStart(ctx, conf, newSSHPort, newWebPort)
}

// handleServerDisconnect requests a graceful disconnect of the active session or a targeted set.
// target may be "", "all", "web", "ssh", or "ip:port".
func handleServerDisconnect(ctx context.Context, state *CmdState, target string) error {
	if target == "" {
		return serve.Disconnect(ctx, state.Force)
	}
	return serve.DisconnectSessions(ctx, target, state.Force)
}

// handleServerInstall writes the OS service unit for the server daemon.
func handleServerInstall(ctx context.Context, sshPort, webPort int) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}
	if err := serve.InstallService(execPath, sshPort, webPort); err != nil {
		return err
	}
	if sshPort > 0 {
		logger.Notice(ctx, "Service installed with SSH port %d%s. Run 'ds2 --server enable' to start it at boot.", sshPort, fmtWebPort(webPort))
	} else {
		logger.Notice(ctx, "Service installed (ports from config). Run 'ds2 --server enable' to start it at boot.")
	}
	return nil
}

// handleServerUninstall removes the OS service unit for the server daemon.
func handleServerUninstall(ctx context.Context) error {
	if err := serve.UninstallService(); err != nil {
		return err
	}
	logger.Notice(ctx, "Service uninstalled.")
	return nil
}

// handleServerEnable enables and starts the OS service.
// Always reinstalls the unit file so ExecStart reflects the current binary path.
func handleServerEnable(ctx context.Context, sshPort, webPort int) error {
	if err := handleServerInstall(ctx, sshPort, webPort); err != nil {
		return err
	}
	if err := serve.EnableService(); err != nil {
		return err
	}
	if sshPort > 0 {
		logger.Notice(ctx, "Service enabled with SSH port %d%s — the server will start automatically at boot.", sshPort, fmtWebPort(webPort))
	} else {
		logger.Notice(ctx, "Service enabled (ports from config) — the server will start automatically at boot.")
	}
	return nil
}

// handleServerDisable disables (but does not uninstall) the OS service.
func handleServerDisable(ctx context.Context) error {
	if err := serve.DisableService(); err != nil {
		return err
	}
	logger.Notice(ctx, "Service disabled — the server will no longer start automatically at boot.")
	return nil
}
