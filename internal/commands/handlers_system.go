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
	config.ShowAppConfig(ctx, conf)
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
		if strings.ToLower(group.Args[0]) == "panic" {
			logger.FatalWithStack(ctx, "SIMULATED PANIC: This is a test of the stack trace colorization.")
		}
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
	conf.UI.PanelLocal = mode
	conf.UI.PanelRemote = mode
	if err := config.SaveAppConfig(conf); err != nil {
		return err
	}

	logger.Notice(ctx, "Panel mode set to: {{|Var|}}%s{{[-]}}", mode)
	return nil
}
