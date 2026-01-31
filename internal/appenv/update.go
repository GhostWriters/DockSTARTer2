package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Update regenerates the .env file to ensure correct sorting and headers.
// Mirrors env_update.sh functionality (lines 33-80).
func Update(ctx context.Context, file string) error {
	logger.Notice(ctx, "Updating environment variable files.")
	logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)

	// Get app config for paths
	conf := config.LoadAppConfig()
	composeEnvFile := filepath.Join(conf.ComposeDir, ".env")

	// Read current .env file content
	input, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	allLines := strings.Split(string(input), "\n")

	// Get list of referenced apps
	appList, err := GetReferencedApps(composeEnvFile)
	if err != nil {
		logger.Warn(ctx, "Failed to get referenced apps: %v", err)
		appList = []string{}
	}

	// Temporary file for current env lines (bash line 39)
	currentLinesFile, err := os.CreateTemp("", "dockstarter2.env_update.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(currentLinesFile.Name())
	defer currentLinesFile.Close()

	// Variable to accumulate all formatted lines
	var updatedEnvLines []string

	// === Format global .env section (lines 40-45) ===
	// Get current global vars
	globalVars := AppVarsLines("", allLines)

	// Write to temp file
	currentLinesFile.Truncate(0)
	currentLinesFile.Seek(0, 0)
	for _, line := range globalVars {
		fmt.Fprintln(currentLinesFile, line)
	}
	currentLinesFile.Sync()

	// Get default .env.example path
	configDir := paths.GetConfigDir()
	defaultEnvFile := filepath.Join(configDir, ".env.example")

	// Call FormatLines for globals
	formattedGlobals, err := FormatLines(
		ctx,
		currentLinesFile.Name(),
		defaultEnvFile,
		"", // empty appName for globals
		composeEnvFile,
	)
	if err != nil {
		return fmt.Errorf("failed to format global vars: %w", err)
	}
	updatedEnvLines = append(updatedEnvLines, formattedGlobals...)

	// === Format app sections (lines 47-59) ===
	if len(appList) > 0 {
		for _, appName := range appList {
			// Get app-specific vars
			appVars := AppVarsLines(appName, allLines)

			// Write to temp file
			currentLinesFile.Truncate(0)
			currentLinesFile.Seek(0, 0)
			for _, line := range appVars {
				fmt.Fprintln(currentLinesFile, line)
			}
			currentLinesFile.Sync()

			// Get default app .env file path (will be built by format package)
			templatesDir := paths.GetTemplatesDir()

			// Call FormatLines for this app (line 55-57)
			// It will determine the default env file internally
			formattedApp, err := FormatLinesForApp(
				ctx,
				currentLinesFile.Name(),
				appName,
				templatesDir,
				composeEnvFile,
			)
			if err != nil {
				return fmt.Errorf("failed to format %s vars: %w", appName, err)
			}
			updatedEnvLines = append(updatedEnvLines, formattedApp...)
		}
	}

	// === Write to .env file (lines 64-78) ===
	tempOutputFile, err := os.CreateTemp("", "dockstarter2.env_updated.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp output file: %w", err)
	}
	tempOutputPath := tempOutputFile.Name()

	for _, line := range updatedEnvLines {
		fmt.Fprintln(tempOutputFile, line)
	}
	tempOutputFile.Close()

	if err := CopyFile(tempOutputPath, file); err != nil {
		os.Remove(tempOutputPath)
		return fmt.Errorf("failed to update .env file: %w", err)
	}
	os.Remove(tempOutputPath)
	os.Chmod(file, 0644)

	// === Process all referenced .env.app.appname files (lines 82-121) ===
	if len(appList) > 0 {
		for _, appName := range appList {
			appUpper := strings.ToUpper(appName)
			appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", strings.ToLower(appName)))

			if _, err := os.Stat(appEnvFile); os.IsNotExist(err) {
				logger.Notice(ctx, "Creating '{{_File_}}%s{{|-|}}'.", appEnvFile)
			} else {
				logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", appEnvFile)
			}

			// In Bash: APP_DEFAULT_ENV_FILE="$(run_script 'app_instance_file' "${appname}" ".env.app.*")"
			appDefaultEnvFile, err := AppInstanceFile(ctx, appUpper, ".env.app.*")
			if err != nil {
				logger.Warn(ctx, "Failed to get default env file for %s: %v", appName, err)
				appDefaultEnvFile = ""
			}

			// env_format_lines for these uses the actual app file as currentEnvFile
			formattedAppFile, err := FormatLines(
				ctx,
				appEnvFile,
				appDefaultEnvFile,
				appUpper,
				composeEnvFile,
			)
			if err != nil {
				logger.Warn(ctx, "Failed to format app file for %s: %v", appName, err)
				continue
			}

			// Write to temp and copy to actual
			tempAppFile, err := os.CreateTemp("", fmt.Sprintf("dockstarter2.env_app_%s.*.tmp", appName))
			if err != nil {
				logger.Warn(ctx, "Failed to create temp file for %s: %v", appName, err)
				continue
			}
			tempAppPath := tempAppFile.Name()

			for _, line := range formattedAppFile {
				fmt.Fprintln(tempAppFile, line)
			}
			tempAppFile.Close()

			if err := CopyFile(tempAppPath, appEnvFile); err != nil {
				logger.Warn(ctx, "Failed to update %s: %v", appEnvFile, err)
			}
			os.Remove(tempAppPath)

			// Set permissions
			os.Chmod(appEnvFile, 0644)
		}
	}

	return nil
}

// CopyFile copies a file from src to dst
