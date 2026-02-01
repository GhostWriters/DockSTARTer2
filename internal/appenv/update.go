package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
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
	composeEnvFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

	// Read current .env file content
	input, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	allLines := strings.Split(string(input), "\n")

	// Get list of referenced apps (using robust logic that includes enabled apps)
	appList, err := ListReferencedApps(ctx, conf)
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
	defaultEnvFile := filepath.Join(configDir, constants.EnvExampleFileName)

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

	// Bash parity: If we reached this point, needs_env_update returned true (or force=true).
	// Bash env_update.sh does NOT check if content changed before writing.
	// It proceeds to write blindly if needs_env_update was true.
	logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)
	if err := os.WriteFile(file, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to update .env file: %w", err)
	}

	// Process all referenced .env.app files
	if len(appList) > 0 {
		for _, appName := range appList {
			appUpper := strings.ToUpper(appName)
			appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))

			// Get default template file for the application
			appDefaultEnvFile, err := AppInstanceFile(ctx, appUpper, fmt.Sprintf("%s*", constants.AppEnvFileNamePrefix))
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

			// Bash parity: For app files, it also checks needs_env_update inside the loop.
			// But we are already inside Update() which is driven by CreateAll or explicit call.
			// Wait, env_update.sh iterates applist and calls needs_env_update for EACH app file.

			// We must check NeedsUpdate for the app file before writing.
			// The original code passed 'force' down implicitly by logic flow, but here we iterate.
			if !force && !NeedsUpdate(ctx, false, appEnvFile) {
				logger.Info(ctx, "'{{_File_}}%s{{|-|}}' already updated.", appEnvFile)
				continue
			}

			if _, err := os.Stat(appEnvFile); os.IsNotExist(err) {
				logger.Notice(ctx, "Creating '{{_File_}}%s{{|-|}}'.", appEnvFile)
			} else {
				logger.Notice(ctx, "Updating '{{_File_}}%s{{|-|}}'.", appEnvFile)
			}

			if err := os.WriteFile(appEnvFile, []byte(appContentStr), 0644); err != nil {
				logger.Warn(ctx, "Failed to update %s: %v", appEnvFile, err)
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
	filename := filepath.Base(file)

	// Check main file timestamp and content markers
	if filename == constants.EnvFileName {
		if updateFileChanged(conf, file, "") {
			return true
		}
		// Check ReferencedApps list
		referencedAppsFile := filepath.Join(paths.GetTimestampsDir(), constants.EnvUpdateMarkerPrefix+filename+"_ReferencedApps")
		currentReferenced, _ := ListReferencedApps(ctx, conf)
		storedReferencedBytes, err := os.ReadFile(referencedAppsFile)
		if err != nil {
			return true
		}
		if strings.TrimSpace(string(storedReferencedBytes)) != strings.TrimSpace(strings.Join(currentReferenced, " ")) {
			return true
		}
		return false
	}

	// Check app-specific files
	if updateFileChanged(conf, file, "") {
		return true
	}

	// Check if .env changed relative to app file (dependency check)
	composeEnv := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if updateFileChanged(conf, composeEnv, filename+"_"+filepath.Base(composeEnv)) {
		// Bash: if file_changed "${COMPOSE_ENV}" "${filename}_$(basename "${COMPOSE_ENV}")"
		// This means we check .env against a specific marker for this app file.

		// If main env changed, we check if ENABLED status changed for this app
		// Extract appname from filename (e.g. .env.app.plex -> PLEX)
		// Assuming VarNameToAppName works on filenames? No, we need a helper.
		appName := ""
		if strings.HasPrefix(filename, constants.AppEnvFileNamePrefix) {
			appName = strings.ToUpper(strings.TrimPrefix(filename, constants.AppEnvFileNamePrefix))
		}

		if appName != "" {
			enabledVar := appName + "__ENABLED"
			enabledValPtr := ""
			vars, err := ListVars(composeEnv)
			if err == nil {
				if v, ok := vars[enabledVar]; ok {
					enabledValPtr = v
				}
			}

			enabledMarkerFile := filepath.Join(paths.GetTimestampsDir(), constants.EnvUpdateMarkerPrefix+filename+"_"+enabledVar)
			storedEnabledBytes, err := os.ReadFile(enabledMarkerFile)
			if err != nil {
				return true
			}

			// Compare
			if strings.TrimSpace(string(storedEnabledBytes)) != strings.TrimSpace(enabledValPtr) {
				return true
			}
		}
	}

	return false
}

// UnsetNeedsUpdate marks the environment files as updated by creating markers.
// Mirrors unset_needs_env_update.sh
func UnsetNeedsUpdate(ctx context.Context, file string) {
	conf := config.LoadAppConfig()

	// If file is empty/not provided (conceptually), process all
	// In Go, we are usually called with the main env file or specific one?
	// The function signature takes 'file'.
	// Bash unset_needs_env_update is called with NO args to do everything.
	// But our Update function calls it with 'file'.
	// If 'file' is the main env file, we should recursively call for all referenced apps?
	// The Bash script does recursion if no args.

	filename := filepath.Base(file)

	// Update main timestamp for this file
	updateTimestampForUpdate(conf, file, "")

	if filename == constants.EnvFileName {
		// Update ReferencedApps marker
		referencedAppsFile := filepath.Join(paths.GetTimestampsDir(), constants.EnvUpdateMarkerPrefix+filename+"_ReferencedApps")
		apps, _ := ListReferencedApps(ctx, conf)
		_ = os.WriteFile(referencedAppsFile, []byte(strings.Join(apps, " ")), 0644)

		// Also recurses for all referenced apps in Bash if no args provided.
		// Since we called Update which processes everything, we should probably update markers for all of them too.
		// "Process all referenced .env.app files" loop in Update calls UnsetNeedsUpdate? No, it calls Update?
		// No, the loop in Update does internal logic. It does NOT call UnsetNeedsUpdate for each app.
		// So we must do it here.

		for _, appName := range apps {
			appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
			UnsetNeedsUpdate(ctx, appEnvFile)
		}

	} else {
		// App specific file
		// Store ENABLED status
		appName := ""
		if strings.HasPrefix(filename, constants.AppEnvFileNamePrefix) {
			appName = strings.ToUpper(strings.TrimPrefix(filename, constants.AppEnvFileNamePrefix))
		}

		if appName != "" {
			composeEnv := filepath.Join(conf.ComposeDir, constants.EnvFileName)
			enabledVar := appName + "__ENABLED"
			enabledVal := ""
			vars, err := ListVars(composeEnv)
			if err == nil {
				if v, ok := vars[enabledVar]; ok {
					enabledVal = v
				}
			}

			// Write ENABLED marker
			enabledMarkerFile := filepath.Join(paths.GetTimestampsDir(), constants.EnvUpdateMarkerPrefix+filename+"_"+enabledVar)
			_ = os.WriteFile(enabledMarkerFile, []byte(enabledVal), 0644)

			// Also update the dependency timestamp (main env vs this app)
			// Bash: touch -r "${VarFile}" "$(timestamp_file "${filename}")"
			// But for app file, we also need to handle the COMPOSE_ENV check in NeedsUpdate.
			// Bash needs_env_update checks `file_changed "${COMPOSE_ENV}" "${filename}_$(basename "${COMPOSE_ENV}")"`.
			// So we need to create/touch `${filename}_$(basename "${COMPOSE_ENV}")` reference to COMPOSE_ENV's time?
			// Unlike Bash which can touch -r, we just write/copy the modtime.
			// Actually Go's updateTimestampForUpdate does the touch.

			updateTimestampForUpdate(conf, composeEnv, filename+"_"+filepath.Base(composeEnv))
		}
	}
}

func updateFileChanged(conf config.AppConfig, path string, markerSuffix string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	filename := filepath.Base(path)
	markerName := constants.EnvUpdateMarkerPrefix + filename
	if markerSuffix != "" {
		markerName = constants.EnvUpdateMarkerPrefix + markerSuffix
	}
	timestampFile := filepath.Join(paths.GetTimestampsDir(), markerName)

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

func updateTimestampForUpdate(conf config.AppConfig, path string, markerSuffix string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	filename := filepath.Base(path)
	markerName := constants.EnvUpdateMarkerPrefix + filename
	if markerSuffix != "" {
		markerName = constants.EnvUpdateMarkerPrefix + markerSuffix
	}
	timestampFile := filepath.Join(paths.GetTimestampsDir(), markerName)

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
