package cmd

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// CmdState holds the state of flags for a single command group.
type CmdState struct {
	Force bool
	GUI   bool
	Yes   bool
}

// Execute runs the logic for a sequence of command groups.
// It handles flag application, command switching, and state resetting.
func Execute(ctx context.Context, groups []CommandGroup) int {
	conf := config.LoadGUIConfig()
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
		logger.Notice(ctx, fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} command: '{{_UserCommand_}}%s{{|-|}}'", version.ApplicationName, cmdStr))

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
			case "--theme-table", "--config-pm-table", "--config-pm-existing-table",
				"-a", "--add",
				"-c", "--compose",
				"-e", "--env",
				"-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced",
				"-p", "--prune",
				"-r", "--remove",
				"-R", "--reset",
				"-s", "--status", "--status-enable", "--status-disable",
				"-S", "--select", "--menu-config-app-select", "--menu-app-select",
				"-t", "--test",
				"--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-existing-list",
				"--config-show", "--show-config",
				"--env-appvars", "--env-appvars-lines",
				"--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal",
				"--env-set", "--env-set-lower":
				logger.FatalNoTrace(subCtx, "The '{{_UserCommand_}}%s{{|-|}}' command is not implemented yet.", group.Command)
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow",
				"--theme-scrollbar", "--theme-no-scrollbar":
				handleThemeSettings(subCtx, &group)
				ranCommand = true
			default:
				// Custom command logic would be hooked in here.
				// If we just had flags (group.Command == ""), ranCommand remains false
			}
			return nil
		}

		if state.GUI && group.Command != "" && group.Command != "-M" && group.Command != "--menu" {
			err := tui.RunCommand(ctx, "Command Execution", task)
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

func handleTheme(ctx context.Context, group *CommandGroup) {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadGUIConfig()
		if len(group.Args) > 0 {
			newTheme := group.Args[0]
			// Validate theme existence
			themesDir := paths.GetThemesDir()
			themePath := filepath.Join(themesDir, newTheme)
			if _, err := os.Stat(themePath); os.IsNotExist(err) {
				logger.Error(ctx, "Theme '{{_Theme_}}%s{{|-|}}' not found in '{{_Folder_}}%s{{|-|}}'.", newTheme, themesDir)
				return
			}

			conf.Theme = newTheme
			if err := config.SaveGUIConfig(conf); err != nil {
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
	conf := config.LoadGUIConfig()
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
	case "--theme-scrollbar":
		conf.Scrollbar = true
	case "--theme-no-scrollbar":
		conf.Scrollbar = false
	}
	if err := config.SaveGUIConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save theme setting: %v", err)
	} else {
		logger.Notice(ctx, "Theme setting updated: %s", group.Command)
	}
}
