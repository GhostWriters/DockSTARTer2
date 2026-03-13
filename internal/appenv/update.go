package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Update regenerates the .env file to ensure correct sorting and headers.
// Mirrors env_update.sh functionality.
func Update(ctx context.Context, force bool, file string) error {
	conf := config.LoadAppConfig()
	composeEnvFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	configDir := paths.GetConfigDir()

	// 1. Delete orphaned .env.app.* files (Parity with env_update.sh lines 12-28)
	// Bash does this at the start of env_update
	if err := CleanupOrphanedEnvFiles(ctx, conf); err != nil {
		logger.Warn(ctx, "Failed to cleanup orphaned env files: %v", err)
	}

	// 2. Get list of referenced apps (Parity with env_update.sh lines 30-32)
	appList, err := ListReferencedApps(ctx, conf)
	if err != nil {
		logger.Warn(ctx, "Failed to get referenced apps: %v", err)
		appList = []string{}
	}

	// 3. Update main .env file (Parity with env_update.sh lines 33-80)
	if NeedsUpdate(ctx, force, composeEnvFile) {
		logger.Notice(ctx, "Updating '{{|File|}}%s{{[-]}}'.", composeEnvFile)

		// Read current .env file content
		input, _ := os.ReadFile(composeEnvFile)
		allLines := strings.Split(string(input), "\n")

		var updatedEnvLines []string

		// a) Format global section (Parity lines 38-45)
		globalVars := AppVarsLines("", allLines)
		tmpGlobalFile, _ := os.CreateTemp("", "ds2.global.*.tmp")
		os.WriteFile(tmpGlobalFile.Name(), []byte(strings.Join(globalVars, "\n")), 0644)
		defer os.Remove(tmpGlobalFile.Name())

		defaultEnvFile := filepath.Join(configDir, constants.EnvExampleFileName)
		formattedGlobals, err := FormatLines(ctx, tmpGlobalFile.Name(), defaultEnvFile, "", composeEnvFile)
		if err == nil {
			updatedEnvLines = append(updatedEnvLines, formattedGlobals...)
		}

		// b) Format app sections in main .env (Parity lines 47-58)
		for _, appName := range appList {
			appVars := AppVarsLines(appName, allLines)
			tmpAppFile, _ := os.CreateTemp("", "ds2.app_main.*.tmp")
			os.WriteFile(tmpAppFile.Name(), []byte(strings.Join(appVars, "\n")), 0644)
			defer os.Remove(tmpAppFile.Name())

			// App default file for main .env section
			appDefaultGlobalFile, _ := AppInstanceFile(ctx, strings.ToUpper(appName), ".env")

			formattedApp, err := FormatLines(ctx, tmpAppFile.Name(), appDefaultGlobalFile, appName, composeEnvFile)
			if err == nil {
				updatedEnvLines = append(updatedEnvLines, formattedApp...)
			}
		}

		// c) Write main .env (Parity lines 64-78)
		finalContent := strings.Join(updatedEnvLines, "\n")
		// Ensure trailing newline
		if len(updatedEnvLines) > 0 && !strings.HasSuffix(finalContent, "\n") {
			finalContent += "\n"
		}
		if err := os.WriteFile(composeEnvFile, []byte(finalContent), 0644); err != nil {
			return fmt.Errorf("failed to update main .env: %w", err)
		}
		UnsetNeedsUpdate(ctx, composeEnvFile)
	} else {
		logger.Info(ctx, "Environment variable file '{{|File|}}%s{{[-]}}' already updated.", composeEnvFile)
	}

	// 4. Update individual .env.app.* files (Parity with env_update.sh lines 82-121)
	for _, appName := range appList {
		appEnvFile := GetAppEnvFile(appName, conf)
		if NeedsUpdate(ctx, force, appEnvFile) {
			if _, err := os.Stat(appEnvFile); os.IsNotExist(err) {
				logger.Notice(ctx, "Creating '{{|File|}}%s{{[-]}}'.", appEnvFile)
			} else {
				logger.Notice(ctx, "Updating '{{|File|}}%s{{[-]}}'.", appEnvFile)
			}

			appDefaultEnvFile, _ := AppInstanceFile(ctx, strings.ToUpper(appName), ".env.app.*")
			formattedAppFile, err := FormatLines(ctx, appEnvFile, appDefaultEnvFile, appName, composeEnvFile)
			if err == nil {
				finalAppContent := strings.Join(formattedAppFile, "\n")
				// Ensure trailing newline
				if len(formattedAppFile) > 0 && !strings.HasSuffix(finalAppContent, "\n") {
					finalAppContent += "\n"
				}

				// Parity lines 103-116: uses temp file and copies
				tmpFile, _ := os.CreateTemp("", "ds2.app_env.*.tmp")
				os.WriteFile(tmpFile.Name(), []byte(finalAppContent), 0644)
				tmpFile.Close()
				defer os.Remove(tmpFile.Name())

				if err := CopyFile(tmpFile.Name(), appEnvFile); err != nil {
					logger.Warn(ctx, "Failed to update %s: %v", appEnvFile, err)
				} else {
					system.SetPermissions(ctx, appEnvFile)
					UnsetNeedsUpdate(ctx, appEnvFile)
				}
			}
		} else {
			logger.Info(ctx, "Environment variable file '{{|File|}}%s{{[-]}}' already updated.", appEnvFile)
		}
	}

	return nil
}

// GetAppEnvFile returns the absolute path to the app-specific env file (.env.app.appname).
func GetAppEnvFile(appName string, conf config.AppConfig) string {
	return filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
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
		referencedAppsFile := filepath.Join(paths.GetTimestampsDir(), "env_update", filename+"_ReferencedApps")
		currentReferenced, _ := ListReferencedApps(ctx, conf)
		storedReferencedBytes, err := os.ReadFile(referencedAppsFile)
		if err != nil {
			return true
		}
		if strings.TrimSpace(string(storedReferencedBytes)) != strings.TrimSpace(strings.Join(currentReferenced, "\n")) {
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
		// If main env changed, we check if ENABLED status changed for this app
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

			enabledMarkerFile := filepath.Join(paths.GetTimestampsDir(), "env_update", filename+"_"+enabledVar)
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
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "env_update")
	_ = os.MkdirAll(timestampsFolder, 0755)

	filename := filepath.Base(file)

	// Update main timestamp for this file
	recordUpdateFileState(conf, file, "")

	if filename == constants.EnvFileName {
		// Update ReferencedApps marker
		referencedAppsFile := filepath.Join(timestampsFolder, filename+"_ReferencedApps")
		apps, _ := ListReferencedApps(ctx, conf)
		_ = os.WriteFile(referencedAppsFile, []byte(strings.Join(apps, "\n")), 0644)

		for _, appName := range apps {
			appEnvFile := GetAppEnvFile(appName, conf)
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
			enabledMarkerFile := filepath.Join(timestampsFolder, filename+"_"+enabledVar)
			_ = os.WriteFile(enabledMarkerFile, []byte(enabledVal), 0644)

			// Also update the dependency timestamp (main env vs this app)
			recordUpdateFileState(conf, composeEnv, filename+"_"+filepath.Base(composeEnv))
		}
	}
}

func updateFileChanged(conf config.AppConfig, path string, markerSuffix string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	filename := filepath.Base(path)
	markerName := filename
	if markerSuffix != "" {
		markerName = markerSuffix
	}
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "env_update", markerName)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	if os.IsNotExist(tsErr) {
		return true
	}

	if err != nil {
		return false
	}

	if !info.ModTime().Equal(tsInfo.ModTime()) {
		if CompareFiles(path, timestampFile) {
			_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
			return false
		}
		return true
	}

	return false
}

func recordUpdateFileState(conf config.AppConfig, path string, markerSuffix string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	filename := filepath.Base(path)
	markerName := filename
	if markerSuffix != "" {
		markerName = markerSuffix
	}
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "env_update", markerName)

	_ = os.MkdirAll(filepath.Dir(timestampFile), 0755)

	_ = CopyFile(path, timestampFile)

	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
	}
}
