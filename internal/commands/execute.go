package commands

import (
	"context"
	"errors"
	"fmt"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

// CmdState holds per-group flag state.
type CmdState struct {
	Force bool
	GUI   bool
	Yes   bool
}

// Execute runs a sequence of command groups in console mode (no TUI, no GUI wrapping).
// Non-consoleSafe commands are rejected with a warning line.
// Returns the exit code (0 = success).
func Execute(ctx context.Context, groups []CommandGroup) int {
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
			"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title",
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
		console.GlobalYes = true // console path always auto-confirms
		console.GlobalForce = false
		console.GlobalGUI = false
		console.GlobalVerbose = false
		console.GlobalDebug = false
		logger.SetLevel(logger.LevelNotice)

		state := CmdState{Yes: true}

		flags := group.Flags
		restArgs := Flatten(groups[i+1:])
		console.CurrentFlags = flags
		console.RestArgs = restArgs

		for _, flag := range flags {
			switch flag {
			case "-v", "--verbose":
				logger.SetLevel(logger.LevelInfo)
				console.GlobalVerbose = true
			case "-x", "--debug":
				logger.SetLevel(logger.LevelDebug)
				console.GlobalDebug = true
			case "-f", "--force":
				state.Force = true
				console.GlobalForce = true
			case "-y", "--yes":
				state.Yes = true
				console.GlobalYes = true
			}
		}

		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + part
		}
		logger.Notice(context.Background(), fmt.Sprintf("DockSTARTer2 Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr))

		var err error
		switch group.Command {
		case "-h", "--help":
			err = handleHelp(ctx, &group)
		case "-V", "--version":
			err = handleVersion(ctx)
		case "--man":
			err = handleMan(ctx, &group)
		case "-i", "--install":
			err = handleInstall(ctx, &group, &state)
		case "-u", "--update", "--update-app", "--update-templates":
			err = handleUpdate(ctx, &group, &state, restArgs)
		case "-a", "--add":
			err = handleAppVarsCreate(ctx, &group, &state)
		case "-e", "--env":
			err = handleAppVarsCreateAll(ctx, &group, &state)
		case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
			err = handleList(ctx, &group)
		case "-s", "--status":
			err = handleStatus(ctx, &group)
		case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
			err = handleConfigPm(ctx, &group)
		case "--status-enable", "--status-disable":
			err = handleStatusChange(ctx, &group)
		case "-r", "--remove":
			err = handleRemove(ctx, &group, &state)
		case "-t", "--test":
			err = handleTest(ctx, &group)
		case "--env-appvars":
			err = handleEnvAppVars(ctx, &group)
		case "--env-appvars-lines":
			err = handleEnvAppVarsLines(ctx, &group)
		case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
			err = handleEnvGet(ctx, &group)
		case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
			err = handleEnvSet(ctx, &group)
		case "--config-show", "--show-config":
			err = handleConfigShow(ctx, &conf)
		case "--config-folder", "--config-compose-folder":
			err = handleConfigSettings(ctx, &group)
		case "-T", "--theme", "--theme-list":
			err = handleTheme(ctx, &group)
		case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-button-borders", "--theme-no-button-borders",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title":
			err = handleThemeSettings(ctx, &group)
		case "-c", "--compose":
			err = handleCompose(ctx, &group, &state)
		case "-p", "--prune":
			err = handlePrune(ctx, &state)
		case "-R", "--reset":
			err = handleReset(ctx)
		case "--theme-table":
			err = handleThemeTable(ctx)
		case "--theme-extract", "--theme-extract-all":
			err = handleThemeExtract(ctx, &group)
		}

		if err != nil {
			exitCode = 1
			if errors.Is(err, console.ErrUserAborted) {
				return exitCode
			}
		}

		if update.PendingReExec != nil {
			break
		}
	}

	return exitCode
}
