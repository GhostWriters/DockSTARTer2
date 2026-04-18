package commands

import (
	"context"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
)

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
