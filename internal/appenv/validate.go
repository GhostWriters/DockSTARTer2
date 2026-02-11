package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// ValidateComposeOverride checks if the docker-compose.override.yml file is valid using the Docker Compose SDK.
// It logs a warning if the file is invalid.
func ValidateComposeOverride(ctx context.Context, conf config.AppConfig) {
	overrideFile := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName)

	// We can check existence first to avoid "no such file" errors from the loader
	if _, err := os.Stat(overrideFile); os.IsNotExist(err) {
		return
	}

	// The loader expects a list of config files.
	// We load just the override file to check its syntax/structure validity in isolation.
	configDetails := types.ConfigDetails{
		WorkingDir: conf.ComposeDir,
		ConfigFiles: []types.ConfigFile{
			{
				Filename: overrideFile,
			},
		},
	}

	// We use loader.LoadWithContext to validate the config.
	// We set SkipValidation to false to ensure structural validation occurs.
	// We skip interpolation to avoid errors about missing variables.
	_, err := loader.LoadWithContext(ctx, configDetails, func(options *loader.Options) {
		options.SetProjectName("dockstarter", true) // Set a dummy project name to satisfy loader requirements
		options.SkipInterpolation = true
		options.SkipValidation = false
		options.SkipConsistencyCheck = true
	})

	if err != nil {
		// For errors, we warn.
		logger.Warn(ctx, "Failed to validate '{{_File_}}%s{{|-|}}': %v", constants.ComposeOverrideFileName, err)
		logger.Warn(ctx, "Please fix the syntax in your override file.")
	}
}
