package commands

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

func HandleList(ctx context.Context, group *CommandGroup) error {
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
		console.PrintTableCtx(ctx, headers, data, conf.UI.LineCharacters)
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

func HandleConfigShow(ctx context.Context, conf *config.AppConfig) error {
	headers := []string{
		"{{|UsageCommand|}}Option{{[-]}}",
		"{{|UsageCommand|}}Value{{[-]}}",
		"{{|UsageCommand|}}Expanded Value{{[-]}}",
	}

	keys := []string{
		"ConfigFolder", "ComposeFolder",
		"Theme", "Borders", "ButtonBorders", "LineCharacters", "Scrollbar", "Shadow", "ShadowLevel", "BorderColor",
		"SSHPort", "WebPort", "AuthMode",
	}
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
		"SSHPort":        "SSH Port",
		"WebPort":        "Web Port",
		"AuthMode":       "Auth Mode",
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
		case "SSHPort":
			if conf.Server.SSH.Port > 0 {
				value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.Server.SSH.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "WebPort":
			if conf.Server.Web.Port > 0 {
				value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.Server.Web.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "AuthMode":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.Server.Auth.Mode)
		}

		colorTag := "{{|Var|}}"
		if useFolderColor {
			colorTag = "{{|Folder|}}"
		}

		data = append(data, displayNames[key])

		if useFolderColor || key == "Theme" {
			data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, value))
		} else {
			data = append(data, value)
		}

		if expandedValue != "" {
			data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, expandedValue))
		} else {
			data = append(data, "")
		}
	}

	logger.Info(ctx, "Configuration options stored in '{{|File|}}%s{{[-]}}':", paths.GetConfigFilePath())
	console.PrintTableCtx(ctx, headers, data, conf.UI.LineCharacters)
	return nil
}

func HandlePrune(ctx context.Context, state *CmdState) error {
	if err := docker.Prune(ctx, state.Yes); err != nil {
		logger.Error(ctx, "Prune failed: %v", err)
		return err
	}
	return nil
}

func HandleReset(ctx context.Context) error {
	logger.Notice(ctx, "Resetting {{|ApplicationName|}}%s{{[-]}} to process all actions.", version.ApplicationName)
	system.SetPermissions(ctx, paths.GetTimestampsDir())
	if err := paths.ResetNeeds(); err != nil {
		logger.Error(ctx, "Failed to reset: %v", err)
		return err
	}
	return nil
}

func HandleTest(ctx context.Context, group *CommandGroup) error {
	args := []string{"test", "-v", "./..."}
	if len(group.Args) > 0 {
		args = append(args, "-run", strings.Join(group.Args, "|"))
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "."

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func HandleCompose(ctx context.Context, group *CommandGroup, state *CmdState) error {
	operation := ""
	var appsList []string

	if len(group.Args) > 0 {
		operation = group.Args[0]
		if len(group.Args) > 1 {
			appsList = group.Args[1:]
		}
	}

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
func HandleConfigPanel(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		return errors.New("missing panel mode (log, console, none)")
	}
	mode := strings.ToLower(group.Args[0])
	if mode != "log" && mode != "console" && mode != "none" {
		return fmt.Errorf("invalid panel mode: %s (use log, console, or none)", mode)
	}

	conf := config.LoadAppConfig()
	conf.UI.Panel = mode
	if err := config.SaveAppConfig(conf); err != nil {
		return err
	}

	logger.Notice(ctx, "Panel mode set to: {{|Var|}}%s{{[-]}}", mode)
	return nil
}
