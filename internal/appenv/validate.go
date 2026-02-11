package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"path/filepath"

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
		logger.Warn(ctx, "Failed to read '{{_File_}}%s{{|-|}}': %v", constants.ComposeOverrideFileName, err)
		return
	}

	// Unmarshal into a generic map to check for YAML syntax errors
	var dst map[string]any
	if err := yaml.Unmarshal(content, &dst); err != nil {
		// For errors, we warn.
		logger.Warn(ctx, "Failed to validate '{{_File_}}%s{{|-|}}': %v", constants.ComposeOverrideFileName, err)
		logger.Warn(ctx, "Please fix the syntax in your override file.")
	}
}
