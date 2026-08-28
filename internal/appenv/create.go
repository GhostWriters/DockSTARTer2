package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"os"
	"path/filepath"
)

// Create ensures the environment file exists.
// If not, it creates it from the embedded default template.
func Create(ctx context.Context, file string) error {
	dir := filepath.Dir(file)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		logger.Info(ctx, "Removing existing file '"+console.FormatFilePath(dir)+"' before folder can be created.")
		if err := os.Remove(dir); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to remove existing file.",
				"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
			}, dir)
		}
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Notice(ctx, "Creating folder '"+console.FormatFolderPath(dir)+"'.")
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

	input, err := os.ReadFile(paths.GetTemplatesEnvFile())
	if err != nil {
		logger.Fatal(ctx, "Global .env template '"+console.FormatFilePath(paths.GetTemplatesEnvFile())+"' not found. Run '{{|UserCommand|}}"+version.CommandName+" -u{{[-]}}' to update {{|ApplicationName|}}DockSTARTer-Templates{{[-]}}.")
	}

	if err := os.WriteFile(file, input, 0644); err != nil {
		logger.FatalWithStack(ctx, "Failed to create env file '"+console.FormatFilePath(file)+"'.")
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
