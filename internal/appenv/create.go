package appenv

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"path/filepath"
)

// Create ensures the environment file exists.
// If not, it creates it from the embedded default template.
func Create(ctx context.Context, file string) error {
	dir := filepath.Dir(file)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		logger.Info(ctx, "Removing existing file '{{|File|}}%s{{[-]}}' before folder can be created.", dir)
		if err := os.Remove(dir); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to remove existing file.",
				"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
			}, dir)
		}
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Notice(ctx, "Creating folder '{{|Folder|}}%s{{[-]}}'.", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to create folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, dir)
		}
	}

	if _, err := os.Stat(file); err == nil {
		return nil // File exists
	}

	input, err := assets.GetDefaultEnv()
	if err != nil {
		if err := os.WriteFile(file, []byte{}, 0644); err != nil {
			logger.FatalWithStack(ctx, "Failed to create empty env file '{{|File|}}%s{{[-]}}'.", file)
		}
		return nil
	}

	if err := os.WriteFile(file, input, 0644); err != nil {
		logger.FatalWithStack(ctx, "Failed to create env file '{{|File|}}%s{{[-]}}'.", file)
	}

	// Sanitize: Set specific top-level variables
	// We do NOT want to expand everything, as variables defined later should reference these.

	// 1. HOME
	home, err := os.UserHomeDir()
	if err == nil {
		if err := Set(ctx, "HOME", home, file); err != nil {
			logger.FatalWithStack(ctx, "Failed to set HOME in env file: %v", err)
		}
	}

	// 2. CONFIG/COMPOSE FOLDERS
	conf := config.LoadAppConfig()
	if err := Set(ctx, "DOCKER_CONFIG_FOLDER", conf.ConfigDir, file); err != nil {
		logger.FatalWithStack(ctx, "Failed to set DOCKER_CONFIG_FOLDER: %v", err)
	}
	if err := Set(ctx, "DOCKER_COMPOSE_FOLDER", conf.ComposeDir, file); err != nil {
		logger.FatalWithStack(ctx, "Failed to set DOCKER_COMPOSE_FOLDER: %v", err)
	}

	return nil
}
