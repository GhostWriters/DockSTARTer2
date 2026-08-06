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
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

// HandleSetcap runs the explicit --setcap command: offer/apply the optional
// CAP_CHOWN/CAP_FOWNER grant on the binary and persist the answer.
// "-y --setcap" makes it fully scriptable (QuestionPrompt auto-accepts).
func HandleSetcap(ctx context.Context) error {
	conf := config.LoadAppConfig()
	asked, enabled, applied, err := system.RunSetcapCommand(ctx)
	if asked != conf.System.SetcapAsked || enabled != conf.System.AutoSetcap {
		conf.System.SetcapAsked = asked
		conf.System.AutoSetcap = enabled
		if saveErr := config.SaveAppConfig(conf); saveErr != nil {
			logger.Warn(ctx, "Failed to save auto_setcap setting: %v", saveErr)
		}
	}
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	// File capabilities only bind at exec time, so when more commands follow
	// on this command line, re-exec with the remainder (the same pattern as
	// "-u -c": update, then re-exec with "-c") so they run with the
	// capabilities active instead of falling back to sudo.
	if applied && len(console.RestArgs) > 0 {
		return reExecRest(ctx)
	}
	return nil
}

// HandleConfigSetcap runs --config-setcap / --config-no-setcap: directly set
// the auto_setcap config option with no question asked (the config-style
// counterpart of --setcap, like --theme-shadows/--theme-no-shadows).
// Enabling applies the grant immediately; disabling also removes the
// capabilities from the binary.
func HandleConfigSetcap(ctx context.Context, enable bool) error {
	conf := config.LoadAppConfig()
	if !conf.System.SetcapAsked || conf.System.AutoSetcap != enable {
		conf.System.SetcapAsked = true
		conf.System.AutoSetcap = enable
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Warn(ctx, "Failed to save auto_setcap setting: %v", err)
		}
	}
	if !enable {
		if err := system.DisableSetcap(ctx); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		return nil
	}
	applied, err := system.EnableSetcap(ctx)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	// Same re-exec-with-the-remainder pattern as --setcap, so commands that
	// follow on this command line run with the capabilities active.
	if applied && len(console.RestArgs) > 0 {
		return reExecRest(ctx)
	}
	return nil
}

// reExecRest re-executes DS2 with the remaining (not yet processed) command
// line, so those commands run in a process that picked up the just-applied
// file capabilities at exec time.
func reExecRest(ctx context.Context) error {
	exePath, err := os.Executable()
	if err != nil {
		logger.Warn(ctx, "Cannot re-exec for the remaining commands: %v", err)
		return nil
	}
	return update.ReExec(ctx, exePath, console.RestArgs)
}

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

// ComposeSubtitle returns the yesNotice subtitle for a compose command group.
func ComposeSubtitle(group *CommandGroup) string {
	operation := "update"
	var appsList []string
	if len(group.Args) > 0 {
		operation = group.Args[0]
		if len(group.Args) > 1 {
			appsList = group.Args[1:]
		}
	}
	var appNamesJoined string
	if len(appsList) > 0 {
		appNamesJoined = strings.Join(appsList, ", ")
	}
	return compose.YesNotice(operation, appNamesJoined)
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
