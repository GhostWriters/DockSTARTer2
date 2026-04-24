package commands

import (
	"context"
	"errors"
	"fmt"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

// CmdState holds per-group flag state.
type CmdState struct {
	Force   bool
	GUI     bool
	Yes     bool
	Verbose bool
	Debug   bool
}

// UIProvider defines an interface for commands to request UI interactions.
type UIProvider interface {
	Prompt(ctx context.Context, title, message string, defaultVal bool) (bool, error)
	AppSelect(ctx context.Context) error
	ValueEdit(ctx context.Context, appName, varName, file, mode string) error
	RunCommand(ctx context.Context, title, subtitle string, task func(context.Context) error) error
	Navigate(ctx context.Context, target string) error
}

var GlobalUIProvider UIProvider

func SetUIProvider(p UIProvider) {
	GlobalUIProvider = p
}

// Execute runs a sequence of command groups in console mode (no TUI, no GUI wrapping).
// Non-consoleSafe commands are rejected with a warning line.
// Returns the exit code (0 = success).
func Execute(ctx context.Context, groups []CommandGroup) int {
	// Ensure globals are reset on exit so console flags don't leak into the TUI.
	defer func() {
		console.GlobalYes = false
		console.GlobalForce = false
		console.GlobalGUI = false
		console.GlobalVerbose = false
		console.GlobalDebug = false
		logger.SetLevel(logger.LevelNotice)
	}()

	conf := config.LoadAppConfig()
	_, _ = theme.Load(conf.UI.Theme, "")
	exitCode := 0

	shouldValidate := false
	for _, g := range groups {
		switch g.Command {
		case "-h", "--help", "-V", "--version", "--config-show", "--show-config",
			"--config-folder", "--config-compose-folder", "-T", "--theme", "--theme-list",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-button-borders", "--theme-no-button-borders",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color", "--theme-table",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-panel-title",
			"--theme-extract", "--theme-extract-all", "--man":
		default:
			shouldValidate = true
		}
	}
	if shouldValidate {
		appenv.ValidateComposeOverride(ctx, conf)
	}

	for i, group := range groups {
		if ctx.Err() != nil {
			return 1
		}

		// Block non-consoleSafe commands.
		def := Registry[group.Command]
		if group.Command != "" && !def.ConsoleSafe {
			logger.Warn(ctx, fmt.Sprintf("'{{|UserCommand|}}%s{{[-]}}' cannot be run from the console panel.", group.Command))
			continue
		}

		// Reset global state.
		console.GlobalYes = false
		console.GlobalForce = false
		console.GlobalGUI = false
		console.GlobalVerbose = false
		console.GlobalDebug = false
		logger.SetLevel(logger.LevelNotice)

		state := CmdState{}

		flags := group.Flags
		restArgs := Flatten(groups[i+1:])
		console.CurrentFlags = flags
		console.RestArgs = restArgs

		for _, flag := range flags {
			switch flag {
			case "-v", "--verbose":
				state.Verbose = true
				logger.SetLevel(logger.LevelInfo)
				console.GlobalVerbose = true
			case "-x", "--debug":
				state.Debug = true
				logger.SetLevel(logger.LevelDebug)
				console.GlobalDebug = true
			case "-f", "--force":
				state.Force = true
				console.GlobalForce = true
			case "-y", "--yes":
				state.Yes = true
				console.GlobalYes = true
			case "-g", "--gui":
				state.GUI = true
				console.GlobalGUI = true
			}
		}

		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + part
		}
		logger.Notice(ctx, fmt.Sprintf("DockSTARTer2 Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr))

		// Apply final state to globals
		console.GlobalYes = state.Yes
		console.GlobalForce = state.Force
		console.GlobalGUI = state.GUI
		console.GlobalVerbose = state.Verbose
		console.GlobalDebug = state.Debug

		// Sync logger level
		if console.GlobalDebug {
			logger.SetLevel(logger.LevelDebug)
		} else if console.GlobalVerbose {
			logger.SetLevel(logger.LevelInfo)
		} else {
			logger.SetLevel(logger.LevelNotice)
		}

		// Block action commands when someone else is currently editing the configuration.
		if def.SessionLocked {
			if !sessionlocks.Sessions.AcquireEditLock("local", "Console") {
				info := sessionlocks.Sessions.ReadEditInfo()
				ip := info.ClientIP
				if ip == "" || ip == "local" {
					ip = "the local console"
				}
				conn := info.ConnType
				if conn == "" {
					conn = "SSH"
				}
				logger.Error(ctx, "Cannot run '%s' while the configuration is being edited by a %s session from {{|Highlight|}}%s{{[-]}}.", group.Command, conn, ip)
				logger.Notice(ctx, "Use '{{|UserCommand|}}ds2 --server disconnect{{[-]}}' to force-release the lock.")
				return 1
			}
		}

		// Execute logic
		runWithUI := func(innerCtx context.Context) error {
			switch group.Command {
			case "-h", "--help":
				return HandleHelp(innerCtx, &group)
			case "-V", "--version":
				return HandleVersion(innerCtx)
			case "--man":
				return HandleMan(innerCtx, &group)
			case "-i", "--install":
				return HandleInstall(innerCtx, &group, &state)
			case "-u", "--update", "--update-app", "--update-templates":
				return HandleUpdate(innerCtx, &group, &state, restArgs)
			case "-a", "--add":
				return HandleAppVarsCreate(innerCtx, &group, &state)
			case "-S", "--select":
				if GlobalUIProvider != nil {
					return GlobalUIProvider.AppSelect(innerCtx)
				}
				return nil
			case "-M", "--menu":
				if GlobalUIProvider != nil {
					target := ""
					if len(group.Args) > 0 {
						target = group.Args[0]
					}
					return GlobalUIProvider.Navigate(innerCtx, target)
				}
				return nil
			case "--env-edit", "--env-edit-lower":
				if GlobalUIProvider != nil {
					arg := ""
					if len(group.Args) > 0 {
						arg = group.Args[0]
					}
					return GlobalUIProvider.ValueEdit(innerCtx, "", arg, "", "")
				}
				return nil
			case "--edit-global", "--start-edit-global":
				if GlobalUIProvider != nil {
					return GlobalUIProvider.ValueEdit(innerCtx, "", "", "", "global")
				}
				return nil
			case "--edit-app", "--start-edit-app":
				if GlobalUIProvider != nil {
					arg := ""
					if len(group.Args) > 0 {
						arg = group.Args[0]
					}
					return GlobalUIProvider.ValueEdit(innerCtx, arg, "", "", "app")
				}
				return nil
			case "-e", "--env":
				return HandleAppVarsCreateAll(innerCtx, &group, &state)
			case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
				return HandleList(innerCtx, &group)
			case "-s", "--status":
				return HandleStatus(innerCtx, &group)
			case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
				return HandleConfigPm(innerCtx, &group)
			case "--status-enable", "--status-disable":
				return HandleStatusChange(innerCtx, &group)
			case "-r", "--remove":
				return HandleRemove(innerCtx, &group, &state)
			case "-t", "--test":
				return HandleTest(innerCtx, &group)
			case "--env-appvars":
				return HandleEnvAppVars(innerCtx, &group)
			case "--env-appvars-lines":
				return HandleEnvAppVarsLines(innerCtx, &group)
			case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
				return HandleEnvGet(innerCtx, &group)
			case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
				return HandleEnvSet(innerCtx, &group)
			case "--config-show", "--show-config":
				return HandleConfigShow(innerCtx, &conf)
			case "--config-folder", "--config-compose-folder":
				return HandleConfigSettings(innerCtx, &group)
			case "-T", "--theme", "--theme-list":
				return HandleTheme(innerCtx, &group)
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-button-borders", "--theme-no-button-borders",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
				"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color",
				"--theme-dialog-title", "--theme-submenu-title", "--theme-panel-title":
				return HandleThemeSettings(innerCtx, &group)
			case "-c", "--compose":
				return HandleCompose(innerCtx, &group, &state)
			case "-p", "--prune":
				return HandlePrune(innerCtx, &state)
			case "-R", "--reset":
				return HandleReset(innerCtx)
			case "--theme-table":
				return HandleThemeTable(innerCtx)
			case "--theme-extract", "--theme-extract-all":
				return HandleThemeExtract(innerCtx, &group)
			}
			return nil
		}

		var err error
		if state.GUI && GlobalUIProvider != nil && group.Command != "" && group.Command != "-h" && group.Command != "--help" {
			// Wrap in Program Box
			err = GlobalUIProvider.RunCommand(ctx, "Console Command", cmdStr, runWithUI)
		} else {
			err = runWithUI(ctx)
		}

		if err != nil {
			exitCode = 1
			if errors.Is(err, console.ErrUserAborted) {
				return exitCode
			}
		}

		if def.SessionLocked {
			sessionlocks.Sessions.ReleaseEditLock()
		}

		if update.PendingReExec != nil {
			break
		}
	}

	return exitCode
}
