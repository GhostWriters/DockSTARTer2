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
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	_ "DockSTARTer2/internal/tui/screens" // Register screen creators
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
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
}

// Execute runs the logic for a sequence of command groups.
// It handles flag application, command switching, and state resetting.
func Execute(ctx context.Context, groups []CommandGroup) int {
	conf := config.LoadAppConfig()
	_ = theme.Load(conf.Theme)

	ranCommand := false

	for i, group := range groups {
		state := CmdState{}

		// Prepare execution arguments
		flags := group.Flags
		fullCmd := group.CommandSlice()
		restArgs := Flatten(groups[i+1:])

		// Apply Flags
		// This logic handles setting state before the command executes.
		for _, flag := range flags {
			switch flag {
			case "-v", "--verbose":
				logger.SetLevel(logger.LevelInfo)
			case "-x", "--debug":
				logger.SetLevel(logger.LevelDebug)
			case "-f", "--force":
				state.Force = true
			case "-g", "--gui":
				state.GUI = true
			case "-y", "--yes":
				state.Yes = true
			}
		}

		// Logging
		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + part
		}
		subtitle := " {{_ThemeCommandLine_}}" + cmdStr + "{{|-|}}"
		logger.Notice(ctx, fmt.Sprintf("%s command: '{{_UserCommand_}}%s{{|-|}}'", version.ApplicationName, cmdStr))

		// Log execution arguments for verification
		logger.Debug(ctx, fmt.Sprintf("Execution Args -> State: %+v, Command: %v, Rest: %v", state, fullCmd, restArgs))

		// Command Execution
		task := func(subCtx context.Context) error {
			switch group.Command {
			case "-h", "--help":
				handleHelp(&group)
				ranCommand = true
			case "-V", "--version":
				handleVersion(subCtx)
				ranCommand = true
			case "-i", "--install":
				handleInstall(subCtx, &group, &state)
				ranCommand = true
			case "-u", "--update", "--update-app", "--update-templates":
				handleUpdate(subCtx, &group, &state, restArgs)
				ranCommand = true
			case "-M", "--menu":
				handleMenu(subCtx, &group)
				ranCommand = true
			case "-T", "--theme", "--theme-list":
				handleTheme(subCtx, &group)
				ranCommand = true

			case "-a", "--add":
				// appvars_create (single)
				handleAppVarsCreate(subCtx, &group, &state)
				ranCommand = true
			case "-c", "--compose":
				handleCompose(subCtx, &group, &state)
				ranCommand = true
			case "-e", "--env":
				handleAppVarsCreateAll(subCtx, &group, &state)
				ranCommand = true
			case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
				handleList(subCtx, &group)
				ranCommand = true
			case "-s", "--status":
				handleStatus(subCtx, &group)
				ranCommand = true
			case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
				handleConfigPm(subCtx, &group)
				ranCommand = true
			case "--status-enable", "--status-disable":
				handleStatusChange(subCtx, &group)
				ranCommand = true
			case "-r", "--remove":
				handleRemove(subCtx, &group, &state)
				ranCommand = true
			case "-S", "--select", "--menu-config-app-select", "--menu-app-select":
				logger.FatalNoTrace(subCtx, "The '{{_UserCommand_}}%s{{|-|}}' command is not implemented yet.", group.Command)
			case "-t", "--test":
				handleTest(subCtx, &group)
				ranCommand = true
			case "--env-appvars":
				handleEnvAppVars(subCtx, &group)
				ranCommand = true
			case "--env-appvars-lines":
				handleEnvAppVarsLines(subCtx, &group)
				ranCommand = true
			case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
				handleEnvGet(subCtx, &group)
				ranCommand = true
			case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
				handleEnvSet(subCtx, &group)
				ranCommand = true
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
				"--theme-scrollbar", "--theme-no-scrollbar":
				handleThemeSettings(subCtx, &group)
				ranCommand = true
			case "--config-show", "--show-config":
				handleConfigShow(subCtx, &conf)
				ranCommand = true
			case "-p", "--prune":
				handlePrune(subCtx, &state)
				ranCommand = true
			case "-R", "--reset":
				handleReset(subCtx)
				ranCommand = true
			case "--theme-table":
				handleThemeTable(subCtx)
				ranCommand = true
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
			title = "{{_ThemeTitleSuccess_}}" + title + "{{|-|}}"
			err := tui.RunCommand(ctx, title, subtitle, task)
			if err != nil {
				logger.Error(ctx, "TUI Run Error: %v", err)
			}
		} else {
			_ = task(ctx)
		}

		// Reset Flags
		// This logic resets state after each command group executes.
		logger.SetLevel(logger.LevelNotice)
	}

	// If no commands matched (or groups empty), launch TUI?
	// Parse typically returns at least one group if we want default behavior?
	// If groups is empty, loop didn't run.
	if !ranCommand {
		if err := tui.Start(ctx, ""); err != nil {
			logger.Error(ctx, "TUI Error: %v", err)
		}
	}

	return 0
}
func handleHelp(group *CommandGroup) {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	PrintHelp(target)
}

func handleVersion(ctx context.Context) {
	logger.Display(ctx, fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} [{{_Version_}}%s{{|-|}}]", version.ApplicationName, version.Version))
	logger.Display(ctx, fmt.Sprintf("{{_ApplicationName_}}DockSTARTer-Templates{{|-|}} [{{_Version_}}%s{{|-|}}]", paths.GetTemplatesVersion()))
}

func handleInstall(ctx context.Context, group *CommandGroup, state *CmdState) {
	logger.Warn(ctx, fmt.Sprintf("The '{{_UserCommand_}}%s{{|-|}}' command is deprecated. The only dependency is '{{_UserCommand_}}docker{{|-|}}'.", group.Command))
	if state.Force {
		logger.Notice(ctx, "Force flag ignored.")
	}
}

func handleConfigPm(ctx context.Context, group *CommandGroup) {
	logger.Warn(ctx, fmt.Sprintf("The '{{_UserCommand_}}%s{{|-|}}' command is deprecated. Package manager configuration is no longer needed.", group.Command))
}

func handleUpdate(ctx context.Context, group *CommandGroup, state *CmdState, restArgs []string) {
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
			logger.Error(ctx, "Templates update failed: %v", err)
		}
		if err := update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs); err != nil {
			logger.Error(ctx, "App update failed: %v", err)
		}
	case "--update-app":
		appVer := ""
		if len(group.Args) > 0 {
			appVer = group.Args[0]
		}
		if err := update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs); err != nil {
			logger.Error(ctx, "App update failed: %v", err)
		}
	case "--update-templates":
		templBranch := ""
		if len(group.Args) > 0 {
			templBranch = group.Args[0]
		}
		if err := update.UpdateTemplates(ctx, state.Force, state.Yes, templBranch); err != nil {
			logger.Error(ctx, "Templates update failed: %v", err)
		}
	}
}

func handleMenu(ctx context.Context, group *CommandGroup) {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	if err := tui.Start(ctx, target); err != nil {
		logger.Error(ctx, "TUI Error: %v", err)
	}
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

func handleEnvGet(ctx context.Context, group *CommandGroup) {
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
}

func handleEnvSet(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()

	type kv struct {
		key string
		val string
	}
	var pairs []kv

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
			return
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
		if err := appenv.Create(ctx, file, filepath.Join(conf.ConfigDir, constants.EnvExampleFileName)); err != nil {
			logger.Debug(ctx, "Ensure env file error: %v", err)
		}

		var err error
		if isLiteral {
			err = appenv.SetLiteral(varName, p.val, file)
		} else {
			err = appenv.Set(varName, p.val, file)
		}

		if err != nil {
			logger.Error(ctx, "Error setting %s: %v", p.key, err)
		} else {
			logger.Debug(ctx, "Set %s=%s in %s", varName, p.val, file)
		}
	}
}

func handleAppVarsCreateAll(ctx context.Context, group *CommandGroup, state *CmdState) {
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, state.Force, conf); err != nil {
		logger.Error(ctx, "Failed to create app variables: %v", err)
	}
}

func handleAppVarsCreate(ctx context.Context, group *CommandGroup, state *CmdState) {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{_UserCommand_}}%s{{|-|}}' command requires at least one application name.", group.Command)
		return
	}

	// Ensure env file exists (create if needed)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if err := appenv.Create(ctx, envFile, filepath.Join(conf.ConfigDir, constants.EnvExampleFileName)); err != nil {
		logger.Debug(ctx, "Ensure env file error: %v", err)
	}

	// Enable the apps first
	if err := appenv.Enable(ctx, group.Args, conf); err != nil {
		logger.Error(ctx, "Failed to enable apps: %v", err)
	}

	for _, arg := range group.Args {
		if err := appenv.CreateApp(ctx, arg, conf); err != nil {
			logger.Error(ctx, "%v", err)
		}
	}

	if err := appenv.Update(ctx, state.Force, envFile); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}
}

func handleTheme(ctx context.Context, group *CommandGroup) {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		if len(group.Args) > 0 {
			newTheme := group.Args[0]
			// Validate theme existence
			themesDir := paths.GetThemesDir()
			themePath := filepath.Join(themesDir, newTheme+".ds2theme")
			if _, err := os.Stat(themePath); os.IsNotExist(err) {
				logger.Error(ctx, "Theme '{{_Theme_}}%s{{|-|}}' not found in '{{_Folder_}}%s{{|-|}}'.", newTheme, themesDir)
				return
			}

			conf.Theme = newTheme
			if err := config.SaveAppConfig(conf); err != nil {
				logger.Error(ctx, "Failed to save theme setting: %v", err)
			} else {
				logger.Notice(ctx, "Theme updated to: {{_Theme_}}%s{{|-|}}", newTheme)
				// Reload theme for subsequent commands in the same execution
				_ = theme.Load(newTheme)
			}
		} else {
			// No args? Show current theme
			logger.Notice(ctx, "Current theme is: {{_Theme_}}%s{{|-|}}", conf.Theme)
			logger.Notice(ctx, "Run '{{_UserCommand_}}%s --theme-list{{|-|}}' to see available themes.", version.CommandName)
		}
	case "--theme-list":
		themesDir := paths.GetThemesDir()
		entries, err := os.ReadDir(themesDir)
		if err != nil {
			logger.Error(ctx, "Failed to read themes directory: %v", err)
			return
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
			logger.Warn(ctx, "No themes found in '{{_Folder_}}%s{{|-|}}'.", themesDir)
		} else {
			logger.Notice(ctx, "Available themes in '{{_Folder_}}%s{{|-|}}':", themesDir)
			for _, t := range themes {
				logger.Notice(ctx, "  - %s", t)
			}
		}
	}
}

func handleThemeSettings(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()
	switch group.Command {
	case "--theme-lines", "--theme-line":
		conf.LineCharacters = true
	case "--theme-no-lines", "--theme-no-line":
		conf.LineCharacters = false
	case "--theme-borders", "--theme-border":
		conf.Borders = true
	case "--theme-no-borders", "--theme-no-border":
		conf.Borders = false
	case "--theme-shadows", "--theme-shadow":
		conf.Shadow = true
	case "--theme-no-shadows", "--theme-no-shadow":
		conf.Shadow = false
	case "--theme-shadow-level":
		// Set shadow level (0-4 or aliases)
		if len(group.Args) > 0 {
			arg := strings.ToLower(group.Args[0])
			switch arg {
			case "0", "off", "none", "false", "no":
				conf.ShadowLevel = 0
				conf.Shadow = false
			case "1", "light":
				conf.ShadowLevel = 1
				conf.Shadow = true
			case "2", "medium":
				conf.ShadowLevel = 2
				conf.Shadow = true
			case "3", "dark":
				conf.ShadowLevel = 3
				conf.Shadow = true
			case "4", "solid", "full":
				conf.ShadowLevel = 4
				conf.Shadow = true
			default:
				// specialized handling for percentage strings
				if strings.HasSuffix(arg, "%") {
					var percent int
					if _, err := fmt.Sscanf(arg, "%d%%", &percent); err == nil {
						if percent <= 12 {
							conf.ShadowLevel = 0
							conf.Shadow = false
						} else if percent <= 37 {
							conf.ShadowLevel = 1
							conf.Shadow = true
						} else if percent <= 62 {
							conf.ShadowLevel = 2
							conf.Shadow = true
						} else if percent <= 87 {
							conf.ShadowLevel = 3
							conf.Shadow = true
						} else {
							conf.ShadowLevel = 4
							conf.Shadow = true
						}
						break
					}
				}
				logger.Error(ctx, "Invalid shadow level: %s (use 0-4, or: off, light, medium, dark, solid, or percentage e.g. 50%%)", arg)
				return
			}
		} else {
			logger.Display(ctx, "Current shadow level: %d", conf.ShadowLevel)
			return
		}
	case "--theme-scrollbar":
		conf.Scrollbar = true
	case "--theme-no-scrollbar":
		conf.Scrollbar = false
		// theme handlers above...
	}
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save theme setting: %v", err)
	} else {
		// Specialized output for shadow level
		if group.Command == "--theme-shadow-level" {
			var percent int
			switch conf.ShadowLevel {
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
		} else {
			logger.Notice(ctx, "Theme setting updated: %s", group.Command)
		}
	}
}

func handleList(ctx context.Context, group *CommandGroup) {
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
		return
	}

	for _, item := range result {
		logger.Display(ctx, appenv.GetNiceName(ctx, item))
	}
}

func handleStatus(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{_UserCommand_}}%s{{|-|}}' command requires at least one application name.", group.Command)
		return
	}

	for _, arg := range group.Args {
		// Bash splits by space, our parser already did that if they are separate args.
		// If they passed "app1 app2" as one arg, we might need more splitting but pflag usually treats spaces as separate unless quoted.
		status := appenv.Status(ctx, arg, conf)
		logger.Display(ctx, status)
	}
}

func handleStatusChange(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{_UserCommand_}}%s{{|-|}}' command requires at least one application name.", group.Command)
		return
	}

	var err error
	if group.Command == "--status-enable" {
		err = appenv.Enable(ctx, group.Args, conf)
	} else if group.Command == "--status-disable" {
		err = appenv.Disable(ctx, group.Args, conf)
	}

	if err != nil {
		logger.Error(ctx, "Failed to change app status: %v", err)
	}
	if err := appenv.Update(ctx, false, filepath.Join(conf.ComposeDir, constants.EnvFileName)); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}
}

func handleRemove(ctx context.Context, group *CommandGroup, state *CmdState) {
	conf := config.LoadAppConfig()

	// Remove accepts optional app names (empty = all disabled apps)
	err := appenv.Remove(ctx, group.Args, conf, state.Yes)

	if err != nil {
		logger.Error(ctx, "Failed to remove app variables: %v", err)
	}
	if err := appenv.Update(ctx, false, filepath.Join(conf.ComposeDir, constants.EnvFileName)); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}
}

func handleConfigShow(ctx context.Context, conf *config.AppConfig) {
	headers := []string{
		"{{_UsageCommand_}}Option{{|-|}}",
		"{{_UsageCommand_}}Value{{|-|}}",
		"{{_UsageCommand_}}Expanded Value{{|-|}}",
	}

	keys := []string{
		"ConfigFolder",
		"ComposeFolder",
		"Theme",
		"Borders",
		"LineCharacters",
		"Scrollbar",
		"Shadow",
		"ShadowLevel",
	}

	displayNames := map[string]string{
		"ConfigFolder":   "Config Folder",
		"ComposeFolder":  "Compose Folder",
		"Theme":          "Theme",
		"Borders":        "Borders",
		"LineCharacters": "Line Characters",
		"Scrollbar":      "Scrollbar",
		"Shadow":         "Shadow",
		"ShadowLevel":    "Shadow Level",
	}

	var data []string

	boolToYesNo := func(val bool) string {
		if val {
			return "{{_Var_}}yes{{|-|}}"
		}
		return "{{_Var_}}no{{|-|}}"
	}

	for _, key := range keys {
		var value, expandedValue string
		var useFolderColor bool

		switch key {
		case "ConfigFolder":
			value = conf.ConfigDirUnexpanded
			expandedValue = conf.ConfigDir
			useFolderColor = true
		case "ComposeFolder":
			value = conf.ComposeDirUnexpanded
			expandedValue = conf.ComposeDir
			useFolderColor = true
		case "Theme":
			value = conf.Theme
		case "Borders":
			value = boolToYesNo(conf.Borders)
		case "LineCharacters":
			value = boolToYesNo(conf.LineCharacters)
		case "Scrollbar":
			value = boolToYesNo(conf.Scrollbar)
		case "Shadow":
			value = boolToYesNo(conf.Shadow)
		case "ShadowLevel":
			value = fmt.Sprintf("{{_Var_}}%d{{|-|}}", conf.ShadowLevel)
		}

		colorTag := "{{_Var_}}"
		if useFolderColor {
			colorTag = "{{_Folder_}}"
		}

		// Option column
		data = append(data, displayNames[key])

		// Value column
		// For booleans, value already has formatting. For strings, add color.
		if useFolderColor || key == "Theme" {
			data = append(data, fmt.Sprintf("%s%s{{|-|}}", colorTag, value))
		} else {
			data = append(data, value)
		}

		// Expanded Value column
		if expandedValue != "" {
			if useFolderColor {
				data = append(data, fmt.Sprintf("%s%s{{|-|}}", colorTag, expandedValue))
			} else {
				data = append(data, fmt.Sprintf("%s%s{{|-|}}", colorTag, expandedValue))
			}
		} else {
			data = append(data, "")
		}
	}

	logger.Info(ctx, "Configuration options stored in '{{_File_}}%s{{|-|}}':", paths.GetConfigFilePath())
	console.PrintTable(headers, data, conf.LineCharacters)
}

func handlePrune(ctx context.Context, state *CmdState) {
	if err := docker.Prune(ctx, state.Yes); err != nil {
		logger.Error(ctx, "Prune failed: %v", err)
	}
}

func handleReset(ctx context.Context) {
	logger.Notice(ctx, "Resetting {{_ApplicationName_}}%s{{|-|}} to process all actions.", version.ApplicationName)
	if err := paths.ResetNeeds(); err != nil {
		logger.Error(ctx, "Failed to reset: %v", err)
	}
	// Also ensure permissions are set? Bash script calls set_permissions.
	// We might need a set_permissions equivalent eventually.
}

func handleThemeTable(ctx context.Context) {
	headers := []string{"Theme", "Description", "Author"}
	themes, err := theme.List()
	if err != nil {
		logger.Error(ctx, "Failed to list themes: %v", err)
		return
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
}

func handleEnvAppVars(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.FatalNoTrace(ctx, "Command --env-appvars requires one or more application names.")
	}

	for _, appName := range args {
		vars, err := appenv.ListAppVars(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variables for %s: %v", appName, err)
			continue
		}
		for _, v := range vars {
			logger.Display(ctx, v)
		}
	}
}

func handleEnvAppVarsLines(ctx context.Context, group *CommandGroup) {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.FatalNoTrace(ctx, "Command --env-appvars-lines requires one or more application names.")
	}

	for _, appName := range args {
		lines, err := appenv.ListAppVarLines(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variable lines for %s: %v", appName, err)
			continue
		}
		for _, l := range lines {
			logger.Display(ctx, l)
		}
	}
}

func handleTest(ctx context.Context, group *CommandGroup) {
	args := []string{"test", "-v", "./..."}
	if len(group.Args) > 0 {
		args = append(args, "-run", strings.Join(group.Args, "|"))
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "." // Project root

	if err := cmd.Run(); err != nil {
		// Test failures are handled by the 'go test' output itself
	}
}

func handleCompose(ctx context.Context, group *CommandGroup, state *CmdState) {
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
	}
}
