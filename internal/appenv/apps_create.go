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

// CreateAll generates environment variables for all added applications.
//
// This function mirrors appvars_create_all.sh and performs the following steps:
//  1. Checks if an update is required via timestamp tracking
//  2. Ensures the main .env file exists via EnvCreate
//  3. Identifies all "added" applications (those with __ENABLED variables)
//  4. For each added app, creates its environment variables via CreateApp
//
// The function will log progress and continue processing remaining apps even if
// individual app creation fails.
//
// Returns an error only if critical initialization (EnvCreate or ListAdded) fails.
func CreateAll(ctx context.Context, force bool, conf config.AppConfig) error {
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

	// Ensure main env exists first to check for added apps
	if err := EnvCreate(ctx, conf); err != nil {
		return err
	}

	// Check installed apps
	added, err := ListAddedApps(ctx, envFile)
	if err != nil {
		return err
	}

	// Check if update is needed
	if !NeedsCreateAll(ctx, force, added, conf) {
		logger.Notice(ctx, "Environment variables already created for all added apps.")
		return nil
	}

	if len(added) == 0 {
		logger.Notice(ctx, "'{{|File|}}%s{{[-]}}' does not contain any added ", envFile)
		return nil
	}

	logger.Notice(ctx, "Creating environment variables for added apps. Please be patient, this can take a while.")

	for _, appNameUpper := range added {
		if err := CreateApp(ctx, appNameUpper, conf); err != nil {
			logger.Error(ctx, "Failed to create variables for %s: %v", appNameUpper, err)
		}
	}

	// Format and sort all environment files
	_ = Update(ctx, force, envFile)

	// Mark as complete by updating timestamps
	UnsetNeedsCreateAll(ctx, added, conf)

	return nil
}

// CreateApp generates environment variables for a single application.
// Mirrors appvars_create.sh
func CreateApp(ctx context.Context, appNameRaw string, conf config.AppConfig) error {
	appNameUpper := strings.TrimSpace(strings.ToUpper(appNameRaw))
	// Strip colons
	if strings.HasSuffix(appNameUpper, ":") {
		appNameUpper = appNameUpper[:len(appNameUpper)-1]
	} else if strings.HasPrefix(appNameUpper, ":") {
		appNameUpper = appNameUpper[1:]
	}

	niceName := GetNiceName(ctx, appNameUpper)
	if !IsAppNameValid(appNameUpper) {
		return fmt.Errorf("'{{|App|}}%s{{[-]}}' is not a valid application name", niceName)
	}

	envFile := filepath.Join(conf.ComposeDir, ".env")
	appName := strings.ToLower(appNameUpper)

	if IsAppBuiltIn(appName) {
		logger.Info(ctx, "Creating environment variables for '{{|App|}}%s{{[-]}}'.", niceName)

		// 1. Get path to Global .env instance file
		processedGlobalEnv, err := AppInstanceFile(ctx, appName, constants.EnvFileName)
		if err != nil {
			logger.Debug(ctx, "No global .env template for %s: %v", appNameUpper, err)
		} else if processedGlobalEnv != "" {
			if _, err := MergeNewOnly(ctx, envFile, processedGlobalEnv); err != nil {
				return fmt.Errorf("failed to merge global env for %s: %w", appNameUpper, err)
			}
		}

		// 2. Get path to App specific .env instance file (.env.app.*)
		targetAppEnv := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, appName))
		processedAppEnv, err := AppInstanceFile(ctx, appName, fmt.Sprintf("%s*", constants.AppEnvFileNamePrefix))
		if err != nil {
			logger.Debug(ctx, "No app-specific .env template for %s: %v", appNameUpper, err)
		} else if processedAppEnv != "" {
			if _, err := MergeNewOnly(ctx, targetAppEnv, processedAppEnv); err != nil {
				return fmt.Errorf("failed to merge app env for %s: %w", appNameUpper, err)
			}
		}

		logger.Info(ctx, "Environment variables created for '{{|App|}}%s{{[-]}}'.", niceName)
		return nil
	} else {
		logger.Warn(ctx, "Application '{{|App|}}%s{{[-]}}' does not exist.", niceName)
		return nil
	}
}

// --- Parity Timestamp Helpers ---

// NeedsCreateAll checks if environment variables need to be created/updated.
// Mirrors needs_appvars_create.sh
func NeedsCreateAll(ctx context.Context, force bool, added []string, conf config.AppConfig) bool {
	if force {
		return true
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	// 1. Check if bulk LastSynced exists
	lastSynced := filepath.Join(paths.GetTimestampsDir(), "appvars_create", "LastSynced")
	if _, err := os.Stat(lastSynced); os.IsNotExist(err) {
		return true
	}

	// 2. Check main .env
	if fileChanged(conf, envFile) {
		return true
	}

	// 3. Track changes to the set of enabled applications via AddedApps file parity
	addedAppsFile := filepath.Join(paths.GetTimestampsDir(), "appvars_create", "AddedApps")
	storedApps, err := os.ReadFile(addedAppsFile)
	if err != nil || string(storedApps) != strings.Join(added, "\n") {
		return true
	}

	// 4. Track individuals
	for _, appName := range added {
		appUpper := strings.ToUpper(appName)
		// Precise LastSynced check
		if _, err := os.Stat(filepath.Join(paths.GetTimestampsDir(), "appvars_create", "LastSynced_"+appUpper)); os.IsNotExist(err) {
			return true
		}

		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		if fileChanged(conf, appEnvFile) {
			return true
		}
	}

	return false
}

// UnsetNeedsCreateAll marks environment variable creation as complete by updating timestamps.
// Mirrors unset_needs_appvars_create.sh
func UnsetNeedsCreateAll(ctx context.Context, added []string, conf config.AppConfig) {
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "appvars_create")
	_ = os.MkdirAll(timestampsFolder, 0755)

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	recordFileState(conf, envFile)

	// Update AddedApps
	_ = os.WriteFile(filepath.Join(timestampsFolder, "AddedApps"), []byte(strings.Join(added, "\n")), 0644)

	// Create bulk LastSynced
	f, _ := os.Create(filepath.Join(timestampsFolder, "LastSynced"))
	if f != nil {
		f.Close()
	}

	for _, appName := range added {
		appUpper := strings.ToUpper(appName)
		f, _ := os.Create(filepath.Join(timestampsFolder, "LastSynced_"+appUpper))
		if f != nil {
			f.Close()
		}

		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		recordFileState(conf, appEnvFile)
	}
}

func fileChanged(conf config.AppConfig, path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true // File missing -> needs creation (or re-check)
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "appvars_create", filename)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	if os.IsNotExist(tsErr) {
		return true
	}

	if err != nil {
		return false
	}

	if !info.ModTime().Equal(tsInfo.ModTime()) {
		// Contents comparison parity with cmp -s
		if CompareFiles(path, timestampFile) {
			// Contents are same, sync timestamp to avoid re-check
			_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
			return false
		}
		return true
	}

	return false
}

func recordFileState(conf config.AppConfig, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "appvars_create", filename)

	_ = os.MkdirAll(filepath.Dir(timestampFile), 0755)

	_ = CopyFile(path, timestampFile)

	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
	}
}
