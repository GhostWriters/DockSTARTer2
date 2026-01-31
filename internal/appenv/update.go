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
	logger.Info(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)

	// Get app config for paths
	conf := config.LoadAppConfig()
	composeEnvFile := filepath.Join(conf.ComposeFolder, ".env")

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

	// Get default .example path
	templatesDir := paths.GetTemplatesDir()
	defaultEnvFile := filepath.Join(templatesDir, ".example")

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
	// Create temp output file
	tempOutputFile, err := os.CreateTemp("", "dockstarter2.env_updated.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp output file: %w", err)
	}
	tempOutputPath := tempOutputFile.Name()
	defer os.Remove(tempOutputPath)

	// Write all formatted lines
	for _, line := range updatedEnvLines {
		fmt.Fprintln(tempOutputFile, line)
	}
	tempOutputFile.Close()

	// Copy temp file to actual .env
	if err := CopyFile(tempOutputPath, file); err != nil {
		return fmt.Errorf("failed to update .env file: %w", err)
	}

	// Set permissions (line 78)
	if err := os.Chmod(file, 0644); err != nil {
		logger.Warn(ctx, "Failed to set permissions on .env: %v", err)
	}

	return nil
}

// CopyFile copies a file from src to dst
