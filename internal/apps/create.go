package apps

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/env"
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
//  1. Ensures the main .env file exists via EnvCreate
//  2. Identifies all "added" applications (those with __ENABLED variables)
//  3. For each added app, creates its environment variables via Create
//
// The function will log progress and continue processing remaining apps even if
// individual app creation fails.
//
// Returns an error only if critical initialization (EnvCreate or ListAdded) fails.
func CreateAll(ctx context.Context, conf config.AppConfig) error {
	if err := EnvCreate(ctx, conf); err != nil {
		return err
	}

	envFile := filepath.Join(conf.ComposeFolder, ".env")

	// Ensure main env exists
	// This is partly env_create logic but appvars_create_all calls it

	added, err := ListAdded(envFile)
	if err != nil {
		return err
	}

	if len(added) == 0 {
		logger.Notice(ctx, "'{{_File_}}%s{{|-|}}' does not contain any added apps.", envFile)
		return nil
	}

	logger.Notice(ctx, "Creating environment variables for added apps. Please be patient, this can take a while.")

	for _, appNameUpper := range added {
		if err := Create(ctx, appNameUpper, conf); err != nil {
			logger.Error(ctx, "Failed to create variables for %s: %v", appNameUpper, err)
		}
	}

	return nil
}

// Create generates environment variables for a single application.
// Mirrors appvars_create.sh
func Create(ctx context.Context, appNameRaw string, conf config.AppConfig) error {
	appNameUpper := strings.TrimSpace(strings.ToUpper(appNameRaw))
	// Strip colons
	if strings.HasSuffix(appNameUpper, ":") {
		appNameUpper = appNameUpper[:len(appNameUpper)-1]
	} else if strings.HasPrefix(appNameUpper, ":") {
		appNameUpper = appNameUpper[1:]
	}

	niceName := NiceName(appNameUpper)
	if !IsAppNameValid(appNameUpper) {
		return fmt.Errorf("'{{_App_}}%s{{|-|}}' is not a valid application name", niceName)
	}

	envFile := filepath.Join(conf.ComposeFolder, ".env")
	appName := strings.ToLower(appNameUpper)

	if IsBuiltin(appNameUpper) {
		logger.Info(ctx, "Creating environment variables for '{{_App_}}%s{{|-|}}'.", niceName)

		// 1. Process Global .env template
		processedGlobalEnv, err := ProcessInstanceFile(ctx, appName, ".env")
		if err != nil {
			logger.Debug(ctx, "No global .env template for %s: %v", appNameUpper, err)
		} else if processedGlobalEnv != "" {
			if _, err := env.MergeNewOnly(ctx, envFile, processedGlobalEnv); err != nil {
				return fmt.Errorf("failed to merge global env for %s: %w", appNameUpper, err)
			}
		}

		// 2. Process App specific .env template (.env.app.*)
		targetAppEnv := filepath.Join(conf.ComposeFolder, fmt.Sprintf(".env.app.%s", appName))
		processedAppEnv, err := ProcessInstanceFile(ctx, appName, ".env.app.*")
		if err != nil {
			logger.Debug(ctx, "No app-specific .env template for %s: %v", appNameUpper, err)
		} else if processedAppEnv != "" {
			if _, err := env.MergeNewOnly(ctx, targetAppEnv, processedAppEnv); err != nil {
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

// ProcessInstanceFile replicates the logic of Bash app_instance_file.sh
// It returns the path to the processed file, or empty string if no template exists.
func ProcessInstanceFile(ctx context.Context, appName, fileSuffix string) (string, error) {
	templatesDir := paths.GetTemplatesDir()
	instancesDir := paths.GetInstancesDir()

	baseApp := appname_to_baseappname(appName)
	instance := appname_to_instancename(appName)

	appTemplateDir := filepath.Join(templatesDir, ".apps", baseApp)
	if _, err := os.Stat(appTemplateDir); os.IsNotExist(err) {
		// Template folder doesn't exist - this is not an error, just means no template available
		// Matches Bash behavior which silently continues
		return "", nil
	}

	// Resolve template filename (replace * with baseApp)
	templateFilename := strings.ReplaceAll(fileSuffix, "*", baseApp)
	templateFile := filepath.Join(appTemplateDir, templateFilename)

	if _, err := os.Stat(templateFile); os.IsNotExist(err) {
		return "", nil // No template, nothing to do
	}

	// Instance file path (replace * with appName)
	instanceFilename := strings.ReplaceAll(fileSuffix, "*", appName)
	instanceFile := filepath.Join(instancesDir, appName, instanceFilename)

	// Create instance folder
	if err := os.MkdirAll(filepath.Dir(instanceFile), 0755); err != nil {
		return "", err
	}

	// Placeholder replacement logic
	var __INSTANCE, __Instance, __instance string
	if instance != "" {
		__INSTANCE = "__" + strings.ToUpper(instance)
		// Capitalize first letter
		__Instance = "__" + strings.Title(instance)
		__instance = "__" + strings.ToLower(instance)
	}

	content, err := os.ReadFile(templateFile)
	if err != nil {
		return "", err
	}

	strContent := string(content)
	strContent = strings.ReplaceAll(strContent, "<__INSTANCE>", __INSTANCE)
	strContent = strings.ReplaceAll(strContent, "<__Instance>", __Instance)
	strContent = strings.ReplaceAll(strContent, "<__instance>", __instance)

	// Write processed file
	if err := os.WriteFile(instanceFile, []byte(strContent), 0644); err != nil {
		return "", err
	}

	return instanceFile, nil
}
