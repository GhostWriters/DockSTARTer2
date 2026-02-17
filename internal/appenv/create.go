package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"path/filepath"
)

// Create ensures the environment file exists.
// If not, it copies from the default template.
func Create(ctx context.Context, file, defaultFile string) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.FatalWithStack(ctx, "Failed to create folder '{{|Folder|}}%s{{[-]}}'.", dir)
	}

	if _, err := os.Stat(file); err == nil {
		return nil // File exists
	}

	// Copy from default
	input, err := os.ReadFile(defaultFile)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(file, []byte{}, 0644); err != nil {
				logger.FatalWithStack(ctx, "Failed to create empty env file '{{|File|}}%s{{[-]}}'.", file)
			}
			return nil
		}
		logger.FatalWithStack(ctx, "Failed to read default env '{{|File|}}%s{{[-]}}'.", defaultFile)
	}

	// Write raw content first
	if err := os.WriteFile(file, input, 0644); err != nil {
		logger.FatalWithStack(ctx, "Failed to create env file '{{|File|}}%s{{[-]}}'.", file)
	}

	// Sanitize: Set specific top-level variables
	// We do NOT want to expand everything, as variables defined later should reference these.

	// 1. HOME
	home, err := os.UserHomeDir()
	if err == nil {
		if err := Set("HOME", home, file); err != nil {
			logger.FatalWithStack(ctx, "Failed to set HOME in env file: %v", err)
		}
	}

	// 2. CONFIG/COMPOSE FOLDERS
	conf := config.LoadAppConfig()
	if err := Set("DOCKER_CONFIG_FOLDER", conf.ConfigDir, file); err != nil {
		logger.FatalWithStack(ctx, "Failed to set DOCKER_CONFIG_FOLDER: %v", err)
	}
	if err := Set("DOCKER_COMPOSE_FOLDER", conf.ComposeDir, file); err != nil {
		logger.FatalWithStack(ctx, "Failed to set DOCKER_COMPOSE_FOLDER: %v", err)
	}

	return nil
}
