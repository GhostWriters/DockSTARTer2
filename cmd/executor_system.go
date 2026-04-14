package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/docker"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"DockSTARTer2/internal/version"
)

func handleList(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	var result []string
	var err error

	switch group.Command {
	case "-l", "--list":
		apps, err := appenv.ListBuiltinApps()
		if err != nil {
			logger.Error(ctx, "List failed: %v", err)
			return err
		}
		headers := []string{
			"{{|UsageCommand|}}Application{{[-]}}",
			"{{|UsageCommand|}}Deprecated{{[-]}}",
			"{{|UsageCommand|}}Added{{[-]}}",
			"{{|UsageCommand|}}Disabled{{[-]}}",
		}
		var data []string
		for _, app := range apps {
			nice := appenv.GetNiceName(ctx, app)
			deprecated := ""
			if appenv.IsAppDeprecated(ctx, app) {
				deprecated = "[*DEPRECATED*]"
			}
			added := ""
			disabled := ""
			if appenv.IsAppAdded(ctx, app, envFile) {
				added = "*ADDED*"
				if !appenv.IsAppEnabled(app, envFile) {
					disabled = "(Disabled)"
				}
			}
			data = append(data, nice, deprecated, added, disabled)
		}
		console.PrintTable(headers, data, conf.UI.LineCharacters)
		return nil
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

func handleConfigShow(ctx context.Context, conf *config.AppConfig) error {
	headers := []string{
		"{{|UsageCommand|}}Option{{[-]}}",
		"{{|UsageCommand|}}Value{{[-]}}",
		"{{|UsageCommand|}}Expanded Value{{[-]}}",
	}

	keys := []string{"ConfigFolder", "ComposeFolder", "Theme", "Borders", "ButtonBorders", "LineCharacters", "Scrollbar", "Shadow", "ShadowLevel", "BorderColor"}
	displayNames := map[string]string{
		"ConfigFolder":   "Config Folder",
		"ComposeFolder":  "Compose Folder",
		"Theme":          "Theme",
		"Borders":        "Borders",
		"ButtonBorders":  "Button Borders",
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
		case "ButtonBorders":
			value = boolToYesNo(conf.UI.ButtonBorders)
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

func handleTest(_ctx context.Context, group *CommandGroup) error {
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
		if !errors.Is(err, console.ErrUserAborted) {
			logger.Error(ctx, "Compose failed: %v", err)
		}
		return err
	}
	return nil
}
