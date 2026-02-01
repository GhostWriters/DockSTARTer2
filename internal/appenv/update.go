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
// Mirrors env_update.sh functionality.
func Update(ctx context.Context, force bool, file string) error {
	if !force && !NeedsUpdate(ctx, false, file) {
		logger.Info(ctx, "Environment variable file '{{_File_}}%s{{|-|}}' already updated.", file)
		return nil
	}

	logger.Notice(ctx, "Updating environment variable files.")

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

	// Temporary file for current env lines
	currentLinesFile, err := os.CreateTemp("", "dockstarter2.env_update.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(currentLinesFile.Name())
	defer currentLinesFile.Close()

	// Variable to accumulate all formatted lines
	var updatedEnvLines []string

	// Format global .env section
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

	// Format app sections
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

			// Call FormatLines for this app
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

	// Write to .env file
	contentStr := strings.Join(updatedEnvLines, "\n")
	if len(updatedEnvLines) > 0 {
		contentStr += "\n"
	}

	// Read existing content for comparison
	existingContent, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		// If we can't read it, we assume it needs updating
		existingContent = []byte{}
	}

	if force || string(existingContent) != contentStr {
		logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)
		if err := os.WriteFile(file, []byte(contentStr), 0644); err != nil {
			return fmt.Errorf("failed to update .env file: %w", err)
		}
	} else {
		logger.Info(ctx, "'{{_File_}}%s{{|-|}}' already updated.", file)
	}

	// Process all referenced .env.app files
	if len(appList) > 0 {
		for _, appName := range appList {
			appUpper := strings.ToUpper(appName)
			appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", strings.ToLower(appName)))

			// Get default template file for the application
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

			appContentStr := strings.Join(formattedAppFile, "\n")
			if len(formattedAppFile) > 0 {
				appContentStr += "\n"
			}

			// Read existing app env content for comparison
			existingAppContent, err := os.ReadFile(appEnvFile)
			// checking err here is good but we also handle NotExist in strict write

			if force || os.IsNotExist(err) || string(existingAppContent) != appContentStr {
				if os.IsNotExist(err) {
					logger.Notice(ctx, "Creating '{{_File_}}%s{{|-|}}'.", appEnvFile)
				} else {
					logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", appEnvFile)
				}

				if err := os.WriteFile(appEnvFile, []byte(appContentStr), 0644); err != nil {
					logger.Warn(ctx, "Failed to update %s: %v", appEnvFile, err)
				}
			} else {
				logger.Info(ctx, "'{{_File_}}%s{{|-|}}' already updated.", appEnvFile)
			}
		}
	}

	UnsetNeedsUpdate(ctx, file)

	return nil
}

// NeedsUpdate checks if an environment file update is required.
// Mirrors needs_env_update.sh
func NeedsUpdate(ctx context.Context, force bool, file string) bool {
	if force {
		return true
	}
	conf := config.LoadAppConfig()

	// Check main file timestamp
	if updateFileChanged(conf, file) {
		return true
	}

	// Check app-specific files
	appList, _ := GetReferencedApps(file)
	for _, appName := range appList {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", strings.ToLower(appName)))
		if updateFileChanged(conf, appEnvFile) {
			return true
		}
	}

	return false
}

// UnsetNeedsUpdate marks the environment files as updated by creating markers.
// Mirrors unset_needs_env_update.sh
func UnsetNeedsUpdate(ctx context.Context, file string) {
	conf := config.LoadAppConfig()
	updateTimestampForUpdate(conf, file)

	appList, _ := GetReferencedApps(file)
	for _, appName := range appList {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", strings.ToLower(appName)))
		updateTimestampForUpdate(conf, appEnvFile)
	}
}

func updateFileChanged(conf config.AppConfig, path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "env_update_"+filename)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	if os.IsNotExist(tsErr) {
		return true
	}

	if err != nil {
		return false
	}

	return !info.ModTime().Equal(tsInfo.ModTime())
}

func updateTimestampForUpdate(conf config.AppConfig, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "env_update_"+filename)

	_ = os.MkdirAll(filepath.Dir(timestampFile), 0755)

	f, err := os.Create(timestampFile)
	if err != nil {
		return
	}
	f.Close()

	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
	}
}

// CopyFile copies a file from src to dst
