package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/docker"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	_ "DockSTARTer2/internal/tui/screens" // Register screen creators
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color", "--theme-table":
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
				"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color":
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
func handleHelp(group *CommandGroup) error {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	PrintHelp(target)
	return nil
}

func handleVersion(ctx context.Context) error {
	logger.Display(ctx, fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", version.ApplicationName, version.Version))
	logger.Display(ctx, fmt.Sprintf("{{|ApplicationName|}}DockSTARTer-Templates{{[-]}} [{{|Version|}}%s{{[-]}}]", paths.GetTemplatesVersion()))
	return nil
}

func handleInstall(ctx context.Context, group *CommandGroup, state *CmdState) error {
	logger.Warn(ctx, fmt.Sprintf("The '{{|UserCommand|}}%s{{[-]}}' command is deprecated. The only dependency is '{{|UserCommand|}}docker{{[-]}}'.", group.Command))
	if state.Force {
		logger.Notice(ctx, "Force flag ignored.")
	}
	return nil
}

func handleConfigPm(ctx context.Context, group *CommandGroup) error {
	logger.Warn(ctx, fmt.Sprintf("The '{{|UserCommand|}}%s{{[-]}}' command is deprecated. Package manager configuration is no longer needed.", group.Command))
	return nil
}

func handleUpdate(ctx context.Context, group *CommandGroup, state *CmdState, restArgs []string) error {
	errOccurred := false
	switch group.Command {
	case "-u", "--update":
		appVer := ""
		templBranch := ""
		if len(group.Args) > 0 {
			appVer = group.Args[0]
		}
		if len(group.Args) > 1 {
			templBranch = group.Args[1]
		}
		if err := update.UpdateTemplates(ctx, state.Force, state.Yes, templBranch); err != nil {
			if !errors.Is(err, console.ErrUserAborted) {
				logger.Error(ctx, "Templates update failed: %v", err)
			}
			errOccurred = true
		}
		if err := update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs); err != nil {
			if !errors.Is(err, console.ErrUserAborted) {
				logger.Error(ctx, "App update failed: %v", err)
			}
			errOccurred = true
		}
	case "--update-app":
		appVer := ""
		if len(group.Args) > 0 {
			appVer = group.Args[0]
		}
		if err := update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs); err != nil {
			if !errors.Is(err, console.ErrUserAborted) {
				logger.Error(ctx, "App update failed: %v", err)
			}
			errOccurred = true
		}
	case "--update-templates":
		templBranch := ""
		if len(group.Args) > 0 {
			templBranch = group.Args[0]
		}
		if err := update.UpdateTemplates(ctx, state.Force, state.Yes, templBranch); err != nil {
			if !errors.Is(err, console.ErrUserAborted) {
				logger.Error(ctx, "Templates update failed: %v", err)
			}
			errOccurred = true
		}
	}
	if errOccurred {
		return fmt.Errorf("update failed")
	}
	return nil
}

func handleMenu(ctx context.Context, group *CommandGroup) error {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	// Normalize targets that mean "app select"
	switch target {
	case "config-app-select", "app-select", "select":
		target = "app-select"
	}
	if err := tui.Start(ctx, target); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}

func handleAppSelect(ctx context.Context, group *CommandGroup) error {
	// -S / --select always opens the app selection menu
	if err := tui.Start(ctx, "app-select"); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}

// Helper to resolve VAR and FILE from argument
// Arg can be "VAR" (uses default file) or "APP:VAR" (uses app file)
func resolveEnvVar(arg string, conf config.AppConfig) (string, string) {
	if strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		appName := strings.ToLower(parts[0])
		varName := parts[1]
		filename := fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, appName)
		return varName, filepath.Join(conf.ComposeDir, filename)
	}
	// Default to main env file in ComposeDir
	return arg, filepath.Join(conf.ComposeDir, constants.EnvFileName)
}

func handleEnvGet(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()

	// 1. Determine variables to process
	var args []string
	baseCmd := group.Command
	if idx := strings.Index(baseCmd, "="); idx != -1 {
		// Single parameter version: --env-get=VAR
		args = []string{baseCmd[idx+1:]}
		baseCmd = baseCmd[:idx]
	} else {
		// Multiple parameter version: --env-get VAR1 VAR2 ...
		args = group.Args
	}

	upperCase := !strings.Contains(baseCmd, "-lower")

	for _, arg := range args {
		key, file := resolveEnvVar(arg, conf)
		if upperCase && !strings.Contains(arg, ":") {
			key = strings.ToUpper(key)
		}

		var val string
		var err error

		// Determine operation based on command
		switch {
		case strings.HasPrefix(baseCmd, "--env-get-literal"):
			val, err = appenv.GetLiteral(key, file)
		case strings.HasPrefix(baseCmd, "--env-get-line"):
			val, err = appenv.GetLine(key, file)
		case strings.HasPrefix(baseCmd, "--env-get-line-regex"):
			var lines []string
			lines, err = appenv.GetLineRegex(key, file)
			if err == nil {
				val = strings.Join(lines, "\n")
			}
		case strings.HasPrefix(baseCmd, "--env-get"):
			val, err = appenv.Get(key, file)
		}

		if err != nil {
			logger.Error(ctx, "Error getting %s: %v", arg, err)
			continue
		}

		if val != "" {
			console.Println(val)
		}
	}
	return nil
}

func handleEnvSet(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()

	type kv struct {
		key string
		val string
	}
	var pairs []kv
	var retErr error

	baseCmd := group.Command
	if idx := strings.Index(baseCmd, "="); idx != -1 {
		// Single parameter version: --env-set=VAR,VAL
		param := baseCmd[idx+1:]
		baseCmd = baseCmd[:idx]
		parts := strings.Split(param, ",")
		if len(parts) >= 2 {
			pairs = append(pairs, kv{parts[0], strings.Join(parts[1:], ",")})
		} else {
			logger.Error(ctx, "Command %s requires a variable name and a value (separated by comma).", group.Command)
			return fmt.Errorf("invalid command format")
		}
	} else {
		// Argument version: --env-set VAR=VAL
		for _, arg := range group.Args {
			if strings.Contains(arg, "=") {
				parts := strings.SplitN(arg, "=", 2)
				pairs = append(pairs, kv{parts[0], parts[1]})
			} else {
				// We don't support separate VAR VAL here to match Bash's check for '=' in the arg
				logger.Error(ctx, "Argument %s missing '='", arg)
			}
		}
	}

	upperCase := !strings.Contains(baseCmd, "-lower")
	isLiteral := strings.Contains(baseCmd, "-literal")

	for _, p := range pairs {
		varName, file := resolveEnvVar(p.key, conf)
		if upperCase && !strings.Contains(p.key, ":") {
			varName = strings.ToUpper(varName)
		}

		// Ensure env file exists (create if needed)
		if err := appenv.Create(ctx, file, filepath.Join(conf.Paths.ConfigFolder, constants.EnvExampleFileName)); err != nil {
			logger.Debug(ctx, "Ensure env file error: %v", err)
		}

		var err error
		if isLiteral {
			err = appenv.SetLiteral(ctx, varName, p.val, file)
		} else {
			err = appenv.Set(ctx, varName, p.val, file)
		}

		if err != nil {
			logger.Error(ctx, "Error setting %s: %v", p.key, err)
			retErr = err
		} else {
			logger.Debug(ctx, "Set %s=%s in %s", varName, p.val, file)
		}
	}
	return retErr
}

func handleAppVarsCreateAll(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, state.Force, conf); err != nil {
		logger.Error(ctx, "Failed to create app variables: %v", err)
		return err
	}
	return nil
}

func handleAppVarsCreate(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	// Ensure env file exists (create if needed)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if err := appenv.Create(ctx, envFile, filepath.Join(conf.Paths.ConfigFolder, constants.EnvExampleFileName)); err != nil {
		logger.Debug(ctx, "Ensure env file error: %v", err)
	}

	// Enable the apps first
	if err := appenv.Enable(ctx, group.Args, conf); err != nil {
		logger.Error(ctx, "Failed to enable apps: %v", err)
		return err
	}

	for _, arg := range group.Args {
		if err := appenv.CreateApp(ctx, arg, conf); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
	}

	if err := appenv.Update(ctx, state.Force, envFile); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
		// Not returning error here as it's a warning
	}
	return nil
}

func handleTheme(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		if len(group.Args) > 0 {
			newTheme := group.Args[0]
			// Validate theme existence
			themesDir := paths.GetThemesDir()
			themePath := filepath.Join(themesDir, newTheme+".ds2theme")
			if _, err := os.Stat(themePath); os.IsNotExist(err) {
				logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found in '{{|Folder|}}%s{{[-]}}'.", newTheme, themesDir)
				return err
			}

			conf.UI.Theme = newTheme
			// Apply theme defaults if any
			if tf, err := theme.GetThemeFile(newTheme); err == nil && tf.Defaults != nil {
				changes := theme.ApplyThemeDefaults(&conf, *tf.Defaults)
				if len(changes) > 0 {
					var lines []string
					for k, v := range changes {
						status := v
						if v == "true" {
							status = "{{|Var|}}ON{{[-]}}"
						} else if v == "false" {
							status = "{{|Var|}}OFF{{[-]}}"
						} else {
							status = fmt.Sprintf("{{|Var|}}%s{{[-]}}", v)
						}
						lines = append(lines, fmt.Sprintf("\t- %s: %s", k, status))
					}
					logger.Notice(ctx, "Applying settings from theme file:\n%s", strings.Join(lines, "\n"))
				}
			}

			if err := config.SaveAppConfig(conf); err != nil {
				logger.Error(ctx, "Failed to save theme setting: %v", err)
				return err
			} else {
				logger.Notice(ctx, "Theme updated to: {{|Theme|}}%s{{[-]}}", newTheme)
				// Reload theme for subsequent commands in the same execution
				_, _ = theme.Load(newTheme, "")
			}
		} else {
			// No args? Show current theme
			logger.Notice(ctx, "Current theme is: {{|Theme|}}%s{{[-]}}", conf.UI.Theme)
			logger.Notice(ctx, "Run '{{|UserCommand|}}%s --theme-list{{[-]}}' to see available themes.", version.CommandName)
		}
	case "--theme-list":
		themesDir := paths.GetThemesDir()
		entries, err := os.ReadDir(themesDir)
		if err != nil {
			logger.Error(ctx, "Failed to read themes directory: %v", err)
			return err
		}

		var themes []string
		for _, entry := range entries {
			if entry.IsDir() {
				// Basic check: does it have a theme.ini or .dialogrc?
				themePath := filepath.Join(themesDir, entry.Name())
				if _, err := os.Stat(filepath.Join(themePath, "theme.ini")); err == nil {
					themes = append(themes, entry.Name())
				} else if _, err := os.Stat(filepath.Join(themePath, ".dialogrc")); err == nil {
					themes = append(themes, entry.Name())
				}
			}
		}

		if len(themes) == 0 {
			logger.Warn(ctx, "No themes found in '{{|Folder|}}%s{{[-]}}'.", themesDir)
		} else {
			logger.Notice(ctx, "Available themes in '{{|Folder|}}%s{{[-]}}':", themesDir)
			for _, t := range themes {
				logger.Notice(ctx, "  - %s", t)
			}
		}
	}
	return nil
}

func handleThemeSettings(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	switch group.Command {
	case "--theme-lines", "--theme-line":
		conf.UI.LineCharacters = true
	case "--theme-no-lines", "--theme-no-line":
		conf.UI.LineCharacters = false
	case "--theme-borders", "--theme-border":
		conf.UI.Borders = true
	case "--theme-no-borders", "--theme-no-border":
		conf.UI.Borders = false
	case "--theme-shadows", "--theme-shadow":
		conf.UI.Shadow = true
	case "--theme-no-shadows", "--theme-no-shadow":
		conf.UI.Shadow = false
	case "--theme-shadow-level":
		// Set shadow level (0-4 or aliases)
		if len(group.Args) > 0 {
			arg := strings.ToLower(group.Args[0])
			switch arg {
			case "0", "off", "none", "false", "no":
				conf.UI.ShadowLevel = 0
				conf.UI.Shadow = false
			case "1", "light":
				conf.UI.ShadowLevel = 1
				conf.UI.Shadow = true
			case "2", "medium":
				conf.UI.ShadowLevel = 2
				conf.UI.Shadow = true
			case "3", "dark":
				conf.UI.ShadowLevel = 3
				conf.UI.Shadow = true
			case "4", "solid", "full":
				conf.UI.ShadowLevel = 4
				conf.UI.Shadow = true
			default:
				// specialized handling for percentage strings
				if strings.HasSuffix(arg, "%") {
					var percent int
					if _, err := fmt.Sscanf(arg, "%d%%", &percent); err == nil {
						if percent <= 12 {
							conf.UI.ShadowLevel = 0
							conf.UI.Shadow = false
						} else if percent <= 37 {
							conf.UI.ShadowLevel = 1
							conf.UI.Shadow = true
						} else if percent <= 62 {
							conf.UI.ShadowLevel = 2
							conf.UI.Shadow = true
						} else if percent <= 87 {
							conf.UI.ShadowLevel = 3
							conf.UI.Shadow = true
						} else {
							conf.UI.ShadowLevel = 4
							conf.UI.Shadow = true
						}
						break
					}
				}
				logger.Error(ctx, "Invalid shadow level: %s (use 0-4, or: off, light, medium, dark, solid, or percentage e.g. 50%%)", arg)
				return fmt.Errorf("invalid shadow level")
			}
		} else {
			logger.Display(ctx, "Current shadow level: %d", conf.UI.ShadowLevel)
			return nil
		}
	case "--theme-scrollbar":
		conf.UI.Scrollbar = true
	case "--theme-no-scrollbar":
		conf.UI.Scrollbar = false
	case "--theme-border-color":
		if len(group.Args) > 0 {
			switch group.Args[0] {
			case "1":
				conf.UI.BorderColor = 1
			case "2":
				conf.UI.BorderColor = 2
			case "3":
				conf.UI.BorderColor = 3
			default:
				logger.Error(ctx, "Invalid border color: %s (use 1, 2, or 3)", group.Args[0])
				return fmt.Errorf("invalid border color")
			}
		} else {
			logger.Display(ctx, "Current border color setting: %d", conf.UI.BorderColor)
			return nil
		}
	}

	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save theme setting: %v", err)
		return err
	}

	// Log specific update if appropriate
	if group.Command == "--theme-border-color" && len(group.Args) > 0 {
		logger.Notice(ctx, "Border color set to: {{|Var|}}%s{{[-]}}", group.Args[0])
	}
	// Specialized output for shadow level
	if group.Command == "--theme-shadow-level" && len(group.Args) > 0 {
		var percent int
		switch conf.UI.ShadowLevel {
		case 0:
			percent = 0
		case 1:
			percent = 25
		case 2:
			percent = 50
		case 3:
			percent = 75
		case 4:
			percent = 100
		}
		logger.Notice(ctx, "Shadow level set to %d%%.", percent)
		logger.Notice(ctx, "Theme setting updated: %s", group.Command)
	}

	return nil
}

func handleList(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	var result []string
	var err error

	switch group.Command {
	case "-l", "--list":
		result, err = appenv.ListBuiltinApps()
	case "--list-added":
		result, err = appenv.ListAddedApps(ctx, envFile)
	case "--list-builtin":
		result, err = appenv.ListBuiltinApps()
	case "--list-deprecated":
		result, err = appenv.ListDeprecatedApps(ctx)
	case "--list-enabled":
		result, err = appenv.ListEnabledApps(conf)
	case "--list-disabled":
		result, err = appenv.ListDisabledApps(envFile)
	case "--list-nondeprecated":
		result, err = appenv.ListNonDeprecatedApps(ctx)
	case "--list-referenced":
		result, err = appenv.ListReferencedApps(ctx, conf)
	}

	if err != nil {
		logger.Error(ctx, "List failed: %v", err)
		return err
	}

	for _, item := range result {
		logger.Display(ctx, appenv.GetNiceName(ctx, item))
	}
	return nil
}

func handleStatus(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	for _, arg := range group.Args {
		// Bash splits by space, our parser already did that if they are separate args.
		// If they passed "app1 app2" as one arg, we might need more splitting but pflag usually treats spaces as separate unless quoted.
		status := appenv.Status(ctx, arg, conf)
		logger.Display(ctx, status)
	}
	return nil
}

func handleStatusChange(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	var err error
	if group.Command == "--status-enable" {
		err = appenv.Enable(ctx, group.Args, conf)
	} else if group.Command == "--status-disable" {
		err = appenv.Disable(ctx, group.Args, conf)
	}

	if err != nil {
		logger.Error(ctx, "Failed to change app status: %v", err)
		return err
	}
	if err := appenv.Update(ctx, false, filepath.Join(conf.ComposeDir, constants.EnvFileName)); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}
	return nil
}

func handleRemove(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()

	// Remove accepts optional app names (empty = all disabled apps)
	err := appenv.Remove(ctx, group.Args, conf, state.Yes)

	if err != nil {
		logger.Error(ctx, "Failed to remove app variables: %v", err)
		return err
	}
	if err := appenv.Update(ctx, false, filepath.Join(conf.ComposeDir, constants.EnvFileName)); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}
	return nil
}

func handleConfigShow(ctx context.Context, conf *config.AppConfig) error {
	headers := []string{
		"{{|UsageCommand|}}Option{{[-]}}",
		"{{|UsageCommand|}}Value{{[-]}}",
		"{{|UsageCommand|}}Expanded Value{{[-]}}",
	}

	keys := []string{"ConfigFolder", "ComposeFolder", "Theme", "Borders", "LineCharacters", "Scrollbar", "Shadow", "ShadowLevel", "BorderColor"}
	displayNames := map[string]string{
		"ConfigFolder":   "Config Folder",
		"ComposeFolder":  "Compose Folder",
		"Theme":          "Theme",
		"Borders":        "Borders",
		"LineCharacters": "Line Characters",
		"Scrollbar":      "Scrollbar",
		"Shadow":         "Shadow",
		"ShadowLevel":    "Shadow Level",
		"BorderColor":    "Border Color",
	}

	var data []string

	boolToYesNo := func(val bool) string {
		if val {
			return "{{|Var|}}yes{{[-]}}"
		}
		return "{{|Var|}}no{{[-]}}"
	}

	for _, key := range keys {
		var value, expandedValue string
		var useFolderColor bool

		switch key {
		case "ConfigFolder":
			value = conf.RawPaths.ConfigFolder
			expandedValue = conf.ConfigDir
			useFolderColor = true
		case "ComposeFolder":
			value = conf.RawPaths.ComposeFolder
			expandedValue = conf.ComposeDir
			useFolderColor = true
		case "Theme":
			value = conf.UI.Theme
		case "Borders":
			value = boolToYesNo(conf.UI.Borders)
		case "LineCharacters":
			value = boolToYesNo(conf.UI.LineCharacters)
		case "Scrollbar":
			value = boolToYesNo(conf.UI.Scrollbar)
		case "Shadow":
			value = boolToYesNo(conf.UI.Shadow)
		case "ShadowLevel":
			value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.UI.ShadowLevel)
		case "BorderColor":
			value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.UI.BorderColor)
		}

		colorTag := "{{|Var|}}"
		if useFolderColor {
			colorTag = "{{|Folder|}}"
		}

		// Option column
		data = append(data, displayNames[key])

		// Value column
		// For booleans, value already has formatting. For strings, add color.
		if useFolderColor || key == "Theme" {
			data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, value))
		} else {
			data = append(data, value)
		}

		// Expanded Value column
		if expandedValue != "" {
			if useFolderColor {
				data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, expandedValue))
			} else {
				data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, expandedValue))
			}
		} else {
			data = append(data, "")
		}
	}

	logger.Info(ctx, "Configuration options stored in '{{|File|}}%s{{[-]}}':", paths.GetConfigFilePath())
	console.PrintTable(headers, data, conf.UI.LineCharacters)
	return nil
}

func handlePrune(ctx context.Context, state *CmdState) error {
	if err := docker.Prune(ctx, state.Yes); err != nil {
		logger.Error(ctx, "Prune failed: %v", err)
		return err
	}
	return nil
}

func handleReset(ctx context.Context) error {
	logger.Notice(ctx, "Resetting {{|ApplicationName|}}%s{{[-]}} to process all actions.", version.ApplicationName)
	// Ensure we can delete the directory (parity with bash reset_needs.sh)
	system.SetPermissions(ctx, paths.GetTimestampsDir())
	if err := paths.ResetNeeds(); err != nil {
		logger.Error(ctx, "Failed to reset: %v", err)
		return err
	}
	return nil
}

func handleThemeTable(ctx context.Context) error {
	headers := []string{"Theme", "Description", "Author"}
	themes, err := theme.List()
	if err != nil {
		logger.Error(ctx, "Failed to list themes: %v", err)
		return err
	}

	var data []string
	for _, t := range themes {
		data = append(data, t.Name, t.Description, t.Author)
	}

	// Default theme info (hardcoded for now as it's built-in)
	// Bash implementation iterates folders, so it only shows what's in the folder.
	// If we want to show 'default' or 'classic' if it's not a file, we should add it?
	// But theme.go defines Default() which is hardcoded.
	// We should probably include it if it's not in the list.
	// Bash: lists folders. If "Default" is a folder, it shows it.

	console.PrintTable(headers, data, true)
	return nil
}

func handleEnvAppVars(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.Fatal(ctx, "Command --env-appvars requires one or more application names.")
		return fmt.Errorf("no application name provided")
	}

	for _, appName := range args {
		vars, err := appenv.ListAppVars(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variables for %s: %v", appName, err)
			return err
		}
		for _, v := range vars {
			logger.Display(ctx, v)
		}
	}
	return nil
}

func handleEnvAppVarsLines(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.Fatal(ctx, "Command --env-appvars-lines requires one or more application names.")
		return fmt.Errorf("no application name provided")
	}

	for _, appName := range args {
		lines, err := appenv.ListAppVarLines(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variable lines for %s: %v", appName, err)
			return err
		}
		for _, l := range lines {
			logger.Display(ctx, l)
		}
	}
	return nil
}

func handleTest(ctx context.Context, group *CommandGroup) error {
	args := []string{"test", "-v", "./..."}
	if len(group.Args) > 0 {
		args = append(args, "-run", strings.Join(group.Args, "|"))
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "." // Project root

	if err := cmd.Run(); err != nil {
		// Test failures are handled by the 'go test' output itself, so we don't need to log specific errors here.
		return err
	}
	return nil
}

func handleCompose(ctx context.Context, group *CommandGroup, state *CmdState) error {
	operation := ""
	var appsList []string

	// Parse compose operation and app names
	if len(group.Args) > 0 {
		operation = group.Args[0]
		if len(group.Args) > 1 {
			appsList = group.Args[1:]
		}
	}

	// If no operation specified, default to "update"
	if operation == "" {
		operation = "update"
	}

	if err := compose.ExecuteCompose(ctx, state.Yes, state.Force, operation, appsList...); err != nil {
		logger.Error(ctx, "Compose failed: %v", err)
		return err
	}
	return nil
}
