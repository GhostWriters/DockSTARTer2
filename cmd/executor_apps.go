package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

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
