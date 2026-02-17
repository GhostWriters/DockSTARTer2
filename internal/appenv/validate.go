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
	"gopkg.in/yaml.v3"
)

// ValidateComposeOverride checks if the docker-compose.override.yml file is valid YAML.
// It logs a warning if the file contains syntax errors.
func ValidateComposeOverride(ctx context.Context, conf config.AppConfig) {
	overrideFile := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName)

	// We can check existence first
	if _, err := os.Stat(overrideFile); os.IsNotExist(err) {
		return
	}

	content, err := os.ReadFile(overrideFile)
	if err != nil {
		logger.Warn(ctx, "Failed to read '{{|File|}}%s{{[-]}}': %v", constants.ComposeOverrideFileName, err)
		return
	}

	// Unmarshal into a generic map to check for YAML syntax errors
	var dst map[string]any
	if err := yaml.Unmarshal(content, &dst); err != nil {
		// For errors, we warn.
		logger.Warn(ctx, "Failed to validate '{{|File|}}%s{{[-]}}': %v", constants.ComposeOverrideFileName, err)
		logger.Warn(ctx, "Please fix the syntax in your override file.")
	}
}

// ValidateComposeOverrideStrict checks if the docker-compose.override.yml file is valid using the Docker Compose SDK.
// This is a stricter check that validates against the Compose schema.
// It is currently unused but kept for potential future use (e.g. before 'compose up').
func ValidateComposeOverrideStrict(ctx context.Context, conf config.AppConfig) {
	overrideFile := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName)

	// We can check existence first
	if _, err := os.Stat(overrideFile); os.IsNotExist(err) {
		return
	}

	configDetails := types.ConfigDetails{
		WorkingDir: conf.ComposeDir,
		ConfigFiles: []types.ConfigFile{
			{
				Filename: overrideFile,
			},
		},
	}

	_, err := loader.LoadWithContext(ctx, configDetails, func(options *loader.Options) {
		options.SetProjectName("dockstarter", true)
		options.SkipInterpolation = true
		options.SkipValidation = false // Strict validation
		options.SkipConsistencyCheck = true
	})

	if err != nil {
		logger.Warn(ctx, "Strict validation failed for '{{|File|}}%s{{[-]}}': %v", constants.ComposeOverrideFileName, err)
	}
}
