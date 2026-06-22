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

// parsePortArgs reads up to two numeric port values from args.
// Returns 0 for any argument that is missing or non-numeric.
func parsePortArgs(args []string) (sshPort, webPort int) {
	if len(args) > 0 {
		if p, err := strconv.Atoi(args[0]); err == nil {
			sshPort = p
		}
	}
	if len(args) > 1 {
		if p, err := strconv.Atoi(args[1]); err == nil {
			webPort = p
		}
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
	//   stop    [pid]
	//   restart [pid [sshPort [webPort]]]
	//   install [sshPort [webPort]]
	//   enable  [sshPort [webPort]]
	tail := group.Args[min(1, len(group.Args)):]

	var sshPort, webPort, targetPID int
	switch sub {
	case "stop":
		if len(tail) > 0 {
			targetPID, _ = strconv.Atoi(tail[0])
		}
	case "restart":
		if len(tail) > 0 {
			targetPID, _ = strconv.Atoi(tail[0])
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
		return handleServerStop(ctx, state, targetPID)
	case "restart":
		if err := handleServerStop(ctx, state, targetPID); err != nil {
			return err
		}
		return handleServerStart(ctx, conf, sshPort, webPort)
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
	if sshPort == 0 {
		sshPort = conf.Server.SSH.Port
	}
	if sshPort == 0 {
		logger.Warn(ctx, "server.ssh.port is not set in dockstarter2.toml — cannot start server.")
		return nil
	}

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
	portArgs = append(portArgs, strconv.Itoa(sshPort))
	if webPort > 0 {
		portArgs = append(portArgs, strconv.Itoa(webPort))
	} else if conf.Server.Web.Port > 0 {
		portArgs = append(portArgs, strconv.Itoa(conf.Server.Web.Port))
	}

	logger.Notice(ctx, "Starting server in the background.")
	proc, err := serve.SpawnDaemon(execPath, portArgs)
	if err != nil {
		return fmt.Errorf("spawning server daemon: %w", err)
	}
	logger.Notice(ctx, "Server started (PID %d).", proc.Pid)
	return nil
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
func handleServerStop(ctx context.Context, state *CmdState, targetPID int) error {
	return serve.StopServer(ctx, state.Force, targetPID)
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
	logger.Notice(ctx, "Service installed. Run 'ds2 --server enable' to start it at boot.")
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
	logger.Notice(ctx, "Service enabled — the server will start automatically at boot.")
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
