package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// CreateAll generates environment variables for all added applications.
//
// This function mirrors appvars_create_all.sh and performs the following steps:
//  1. Ensures the main .env file exists via EnvCreate
//  2. Identifies all "added" applications (those with __ENABLED variables)
//  3. For each added app, creates its environment variables via CreateApp
//
// The function will log progress and continue processing remaining apps even if
// individual app creation fails.
//
// Returns an error only if critical initialization (EnvCreate or ListAdded) fails.
func CreateAll(ctx context.Context, conf config.AppConfig) error {
	if err := EnvCreate(ctx, conf); err != nil {
		return err
	}

	envFile := filepath.Join(conf.ComposeDir, ".env")

	// Ensure main env exists
	// This is partly env_create logic but appvars_create_all calls it
	// Check installed apps
	added, err := ListAddedApps(ctx, envFile)
	if err != nil {
		return err
	}

	if len(added) == 0 {
		logger.Notice(ctx, "'{{_File_}}%s{{|-|}}' does not contain any added ", envFile)
		return nil
	}

	logger.Notice(ctx, "Creating environment variables for added  Please be patient, this can take a while.")

	for _, appNameUpper := range added {
		if err := CreateApp(ctx, appNameUpper, conf); err != nil {
			logger.Error(ctx, "Failed to create variables for %s: %v", appNameUpper, err)
		}
	}

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
		return fmt.Errorf("'{{_App_}}%s{{|-|}}' is not a valid application name", niceName)
	}

	envFile := filepath.Join(conf.ComposeDir, ".env")
	appName := strings.ToLower(appNameUpper)

	if IsAppBuiltIn(appName) {
		logger.Info(ctx, "Creating environment variables for '{{_App_}}%s{{|-|}}'.", niceName)

		// 1. Get path to Global .env instance file
		processedGlobalEnv, err := AppInstanceFile(ctx, appName, ".env")
		if err != nil {
			logger.Debug(ctx, "No global .env template for %s: %v", appNameUpper, err)
		} else if processedGlobalEnv != "" {
			if _, err := MergeNewOnly(ctx, envFile, processedGlobalEnv); err != nil {
				return fmt.Errorf("failed to merge global env for %s: %w", appNameUpper, err)
			}
		}

		// 2. Get path to App specific .env instance file (.env.app.*)
		targetAppEnv := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", appName))
		processedAppEnv, err := AppInstanceFile(ctx, appName, ".env.app.*")
		if err != nil {
			logger.Debug(ctx, "No app-specific .env template for %s: %v", appNameUpper, err)
		} else if processedAppEnv != "" {
			if _, err := MergeNewOnly(ctx, targetAppEnv, processedAppEnv); err != nil {
				return fmt.Errorf("failed to merge app env for %s: %w", appNameUpper, err)
			}
		}

		logger.Info(ctx, "Environment variables created for '{{_App_}}%s{{|-|}}'.", niceName)
		return nil
	} else {
		logger.Warn(ctx, "Application '{{_App_}}%s{{|-|}}' does not exist.", niceName)
		return nil
	}
}
