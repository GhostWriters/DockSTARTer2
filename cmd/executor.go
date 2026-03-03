package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	_ "DockSTARTer2/internal/tui/screens" // Register screen creators
	"DockSTARTer2/internal/version"
	"context"
	"errors"
	"fmt"
)

// CmdState holds the state of flags for a single command group.
type CmdState struct {
	Force bool
	GUI   bool
	Yes   bool
}

// commandTitles maps command flags to display titles for the TUI dialog
var commandTitles = map[string]string{
	"-a":                         "Add Application",
	"--add":                      "Add Application",
	"-c":                         "Docker Compose",
	"--compose":                  "Docker Compose",
	"--config-pm":                "Select Package Manager",
	"--config-pm-auto":           "Select Package Manager",
	"--config-pm-list":           "List Known Package Managers",
	"--config-pm-table":          "List Known Package Managers",
	"--config-pm-existing-list":  "List Existing Package Managers",
	"--config-pm-existing-table": "List Existing Package Managers",
	"--config-show":              "Show Configuration",
	"--show-config":              "Show Configuration",
	"-e":                         "Creating Environment Variables",
	"--env":                      "Creating Environment Variables",
	"--env-appvars":              "Variables for Application",
	"--env-appvars-lines":        "Variable Lines for Application",
	"--env-get":                  "Get Value of Variable",
	"--env-get-lower":            "Get Value of Variable",
	"--env-get-line":             "Get Line of Variable",
	"--env-get-lower-line":       "Get Line of Variable",
	"--env-get-literal":          "Get Literal Value of Variable",
	"--env-get-lower-literal":    "Get Literal Value of Variable",
	"--env-set":                  "Set Value of Variable",
	"--env-set-lower":            "Set Value of Variable",
	"-l":                         "List All Applications",
	"--list":                     "List All Applications",
	"--list-builtin":             "List Builtin Applications",
	"--list-deprecated":          "List Deprecated Applications",
	"--list-nondeprecated":       "List Non-Deprecated Applications",
	"--list-added":               "List Added Applications",
	"--list-enabled":             "List Enabled Applications",
	"--list-disabled":            "List Disabled Applications",
	"--list-referenced":          "List Referenced Applications",
	"-p":                         "Docker Prune",
	"--prune":                    "Docker Prune",
	"-r":                         "Remove Application",
	"--remove":                   "Remove Application",
	"-R":                         "Reset Actions",
	"--reset":                    "Reset Actions",
	"-s":                         "Application Status",
	"--status":                   "Application Status",
	"--status-enable":            "Enable Application",
	"--status-disable":           "Disable Application",
	"--theme-list":               "List Themes",
	"--theme-table":              "List Themes",
	"--theme-shadows":            "Turned On Shadows",
	"--theme-no-shadows":         "Turned Off Shadows",
	"--theme-shadow-level":       "Set Shadow Level",
	"--theme-scrollbar":          "Turned On Scrollbars",
	"--theme-no-scrollbar":       "Turned Off Scrollbars",
	"--theme-lines":              "Turned On Line Drawing",
	"--theme-no-lines":           "Turned Off Line Drawing",
	"--theme-borders":            "Turned On Borders",
	"--theme-no-borders":         "Turned Off Borders",
	"-S":                         "Select Applications",
	"--select":                   "Select Applications",
}

func handleConfigSettings(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	switch group.Command {
	case "--config-folder":
		if len(group.Args) > 0 {
			conf.Paths.ConfigFolder = group.Args[0]
		} else {
			logger.Display(ctx, "Current config folder: {{|Folder|}}%s{{[-]}}", conf.Paths.ConfigFolder)
			return nil
		}
	case "--config-compose-folder":
		if len(group.Args) > 0 {
			conf.Paths.ComposeFolder = group.Args[0]
		} else {
			logger.Display(ctx, "Current compose folder: {{|Folder|}}%s{{[-]}}", conf.Paths.ComposeFolder)
			return nil
		}
	}
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save configuration: %v", err)
		return err
	}
	logger.Notice(ctx, "Configuration updated successfully.")
	return nil
}

// Execute runs the logic for a sequence of command groups.
// It handles flag application, command switching, and state resetting.
func Execute(ctx context.Context, groups []CommandGroup) int {
	conf := config.LoadAppConfig()
	_, _ = theme.Load(conf.UI.Theme, "")
	exitCode := 0

	// Validate override file for operational commands
	shouldValidate := false
	for _, g := range groups {
		switch g.Command {
		case "-h", "--help", "-V", "--version", "--config-show", "--show-config",
			"--config-folder", "--config-compose-folder", "-T", "--theme", "--theme-list",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color", "--theme-table",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title":
			// Skip validation for meta/config commands
		default:
			shouldValidate = true
		}
	}

	if shouldValidate {
		appenv.ValidateComposeOverride(ctx, conf)
	}

	ranCommand := false

	for i, group := range groups {
		// Check for context cancellation (e.g. Ctrl-C)
		if ctx.Err() != nil {
			return 1
		}

		// Reset global state for this command set
		console.GlobalYes = false
		console.GlobalForce = false
		console.GlobalGUI = false
		console.GlobalVerbose = false
		console.GlobalDebug = false
		logger.SetLevel(logger.LevelNotice)

		state := CmdState{}

		// Prepare execution arguments
		flags := group.Flags
		restArgs := Flatten(groups[i+1:])
		console.CurrentFlags = flags
		console.RestArgs = restArgs

		// Apply Flags
		// This logic handles setting state before the command executes.
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
			case "-g", "--gui":
				state.GUI = true
				console.GlobalGUI = true
			case "-y", "--yes":
				state.Yes = true
				console.GlobalYes = true
			}
		}

		// Logging
		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + part
		}
		subtitle := " {{|Theme_CommandLine|}}" + cmdStr + "{{[-]}}"
		logger.Notice(ctx, fmt.Sprintf("%s command: '{{|UserCommand|}}%s{{[-]}}'", version.ApplicationName, cmdStr))

		// Command Execution
		task := func(subCtx context.Context) error {
			switch group.Command {
			case "-h", "--help":
				ranCommand = true
				return handleHelp(&group)
			case "-V", "--version":
				ranCommand = true
				return handleVersion(subCtx)
			case "-i", "--install":
				ranCommand = true
				return handleInstall(subCtx, &group, &state)
			case "-u", "--update", "--update-app", "--update-templates":
				ranCommand = true
				return handleUpdate(subCtx, &group, &state, restArgs)
			case "-M", "--menu":
				ranCommand = true
				return handleMenu(subCtx, &group)
			case "-T", "--theme", "--theme-list":
				ranCommand = true
				return handleTheme(subCtx, &group)

			case "-a", "--add":
				// appvars_create (single)
				ranCommand = true
				return handleAppVarsCreate(subCtx, &group, &state)
			case "-c", "--compose":
				ranCommand = true
				return handleCompose(subCtx, &group, &state)
			case "-e", "--env":
				ranCommand = true
				return handleAppVarsCreateAll(subCtx, &group, &state)
			case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
				ranCommand = true
				return handleList(subCtx, &group)
			case "-s", "--status":
				ranCommand = true
				return handleStatus(subCtx, &group)
			case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
				ranCommand = true
				return handleConfigPm(subCtx, &group)
			case "--status-enable", "--status-disable":
				ranCommand = true
				return handleStatusChange(subCtx, &group)
			case "-r", "--remove":
				ranCommand = true
				return handleRemove(subCtx, &group, &state)
			case "-S", "--select", "--menu-config-app-select", "--menu-app-select":
				ranCommand = true
				return handleAppSelect(subCtx, &group)
			case "-t", "--test":
				ranCommand = true
				return handleTest(subCtx, &group)
			case "--env-appvars":
				ranCommand = true
				return handleEnvAppVars(subCtx, &group)
			case "--env-appvars-lines":
				ranCommand = true
				return handleEnvAppVarsLines(subCtx, &group)
			case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
				ranCommand = true
				return handleEnvGet(subCtx, &group)
			case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
				ranCommand = true
				return handleEnvSet(subCtx, &group)
			case "--config-show", "--show-config":
				ranCommand = true
				return handleConfigShow(subCtx, &conf)
			case "--config-folder", "--config-compose-folder":
				ranCommand = true
				return handleConfigSettings(subCtx, &group)
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
				"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color",
				"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title":
				ranCommand = true
				return handleThemeSettings(subCtx, &group)
			case "-p", "--prune":
				ranCommand = true
				return handlePrune(subCtx, &state)
			case "-R", "--reset":
				ranCommand = true
				return handleReset(subCtx)
			case "--theme-table":
				ranCommand = true
				return handleThemeTable(subCtx)
			default:
				// Custom command logic would be hooked in here.
				// If we just had flags (group.Command == ""), ranCommand remains false
			}
			return nil
		}

		if state.GUI && group.Command != "" && group.Command != "-M" && group.Command != "--menu" {
			// Look up display title for this command
			title := commandTitles[group.Command]
			if title == "" {
				title = "Running Command"
			}
			title = "{{|Theme_TitleSuccess|}}" + title + "{{[-]}}"
			err := tui.RunCommand(ctx, title, subtitle, task)
			if err != nil {
				exitCode = 1
				if errors.Is(err, console.ErrUserAborted) {
					return exitCode // Stop execution immediately on user abort
				}
				logger.Error(ctx, "TUI Run Error: %v", err)
			}
		} else {
			if err := task(ctx); err != nil {
				exitCode = 1
				if errors.Is(err, console.ErrUserAborted) {
					return exitCode // Stop execution immediately on user abort
				}
				// Logic for non-abort errors if needed, but usually task handles its own logging
			}
		}

	}

	// If no commands matched (or groups empty), launch TUI
	if !ranCommand {
		if err := tui.Start(ctx, ""); err != nil {
			exitCode = 1
			if errors.Is(err, tui.ErrUserAborted) {
				return exitCode // Stop execution immediately on user abort
			}
			logger.Error(ctx, "TUI Error: %v", err)
		}
	}

	return exitCode
}
