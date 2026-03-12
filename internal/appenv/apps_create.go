package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
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

	// 1. Check if update is needed BEFORE doing setup tasks like EnvCreate (which triggers backups)
	// Get currently added apps from .env if it exists
	var added []string
	if _, err := os.Stat(envFile); err == nil {
		added, _ = ListAddedApps(ctx, envFile)
		if !NeedsCreateAll(ctx, force, added, conf) {
			logger.Notice(ctx, "Environment variables already created for all added apps.")
			return nil
		}
	}

	// 2. Ensure main env exists (and is sanitized/backed up) only now that we know we need it
	if err := EnvCreate(ctx, conf); err != nil {
		return err
	}

	// Re-verify added apps after EnvCreate (which might have sanitized it)
	added, err := ListAddedApps(ctx, envFile)
	if err != nil {
		return err
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
		// Check if update is needed for this app.
		if !NeedsCreateAll(ctx, false, []string{appNameUpper}, conf) {
			logger.Info(ctx, "Environment variables already created for '{{|App|}}%s{{[-]}}'.", niceName)
			return nil
		}

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

		UnsetNeedsCreateApp(ctx, appNameUpper, conf)

		return nil
	} else {
		logger.Warn(ctx, "Application '{{|App|}}%s{{[-]}}' does not exist.", niceName)
		return nil
	}
}
