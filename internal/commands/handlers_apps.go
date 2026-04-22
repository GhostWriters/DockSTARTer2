package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

func HandleHelp(ctx context.Context, group *CommandGroup) error {
	target := ""
	if len(group.Args) > 0 {
		target = group.Args[0]
	}
	PrintHelp(ctx, target)
	return nil
}

func HandleVersion(ctx context.Context) error {
	logger.Display(ctx, fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", version.ApplicationName, version.Version))
	logger.Display(ctx, fmt.Sprintf("{{|ApplicationName|}}DockSTARTer-Templates{{[-]}} [{{|Version|}}%s{{[-]}}]", paths.GetTemplatesVersion()))
	logger.Display(ctx, fmt.Sprintf("{{|ApplicationName|}}Docker Compose SDK{{[-]}} [{{|Version|}}%s{{[-]}}]", version.GetComposeSdkVersion()))
	return nil
}

func HandleInstall(ctx context.Context, group *CommandGroup, state *CmdState) error {
	logger.Warn(ctx, fmt.Sprintf("The '{{|UserCommand|}}%s{{[-]}}' command is deprecated. The only dependency is '{{|UserCommand|}}docker{{[-]}}'.", group.Command))
	if state.Force {
		logger.Notice(ctx, "Force flag ignored.")
	}
	return nil
}

func HandleConfigPm(ctx context.Context, group *CommandGroup) error {
	logger.Warn(ctx, fmt.Sprintf("The '{{|UserCommand|}}%s{{[-]}}' command is deprecated. Package manager configuration is no longer needed.", group.Command))
	return nil
}

func HandleUpdate(ctx context.Context, group *CommandGroup, state *CmdState, restArgs []string) error {
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
		_ = update.UpdateTemplates(ctx, state.Force, state.Yes, templBranch)
		_ = update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs)
	case "--update-app":
		appVer := ""
		if len(group.Args) > 0 {
			appVer = group.Args[0]
		}
		_ = update.SelfUpdate(ctx, state.Force, state.Yes, appVer, restArgs)
	case "--update-templates":
		templBranch := ""
		if len(group.Args) > 0 {
			templBranch = group.Args[0]
		}
		_ = update.UpdateTemplates(ctx, state.Force, state.Yes, templBranch)
	}
	// Server restart after update is handled by the cmd-layer caller (which has access to serve).
	return nil
}

func HandleStatus(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	for _, arg := range group.Args {
		status := appenv.Status(ctx, arg, conf)
		logger.Display(ctx, status)
	}
	return nil
}

func HandleStatusChange(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	var err error
	switch group.Command {
	case "--status-enable":
		err = appenv.Enable(ctx, group.Args, conf)
	case "--status-disable":
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

func HandleRemove(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()

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

func HandleAppVarsCreate(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if err := appenv.Create(ctx, envFile); err != nil {
		logger.Debug(ctx, "Ensure env file error: %v", err)
	}

	var validArgs []string
	for _, arg := range group.Args {
		upper := strings.ToUpper(strings.TrimSpace(arg))
		if !appenv.IsAppBuiltIn(upper) {
			logger.Warn(ctx, "Application '{{|App|}}%s{{[-]}}' does not exist.", appenv.GetNiceName(ctx, upper))
			continue
		}
		validArgs = append(validArgs, arg)
	}
	if len(validArgs) == 0 {
		return nil
	}

	_ = appenv.MigrateEnabledLines(ctx, conf)

	if err := appenv.Enable(ctx, validArgs, conf); err != nil {
		logger.Error(ctx, "Failed to enable apps: %v", err)
		return err
	}

	for _, arg := range validArgs {
		if err := appenv.CreateApp(ctx, arg, state.Force, conf); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
	}

	if err := appenv.Update(ctx, state.Force, envFile); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	} else {
		for _, arg := range group.Args {
			appenv.UnsetNeedsCreateApp(ctx, strings.ToUpper(arg), conf)
		}
	}
	return nil
}

func HandleAppVarsCreateAll(ctx context.Context, _group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, state.Force, conf); err != nil {
		logger.Error(ctx, "Failed to create app variables: %v", err)
		return err
	}
	return nil
}
