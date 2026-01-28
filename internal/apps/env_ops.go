package apps

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/env"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvCreate initializes the DockSTARTer environment file.
//
// This function mirrors env_create.sh and performs the following steps:
//  1. Creates the compose folder if it doesn't exist
//  2. If .env exists: backs it up and sanitizes it
//  3. If .env missing: creates it from the default template and sanitizes it
//  4. If no apps are added: automatically enables WATCHTOWER as the default app
//
// The sanitization process ensures all required variables are present and sets
// platform-specific defaults for PUID, PGID, TZ, HOME, and Docker paths.
//
// Returns an error if critical operations like folder creation or file writing fail.
func EnvCreate(ctx context.Context, conf config.AppConfig) error {
	envFile := filepath.Join(conf.ComposeFolder, ".env")

	// 1. Ensure Folder
	if _, err := os.Stat(conf.ComposeFolder); os.IsNotExist(err) {
		logger.Notice(ctx, "Creating folder '{{_Folder_}}%s{{|-|}}'.", conf.ComposeFolder)
		if err := os.MkdirAll(conf.ComposeFolder, 0755); err != nil {
			return fmt.Errorf("failed to create compose folder: %w", err)
		}
	} else if err != nil {
		return err
	}

	// 2. Backup
	if _, err := os.Stat(envFile); err == nil {
		logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' found.", envFile)
		if err := BackupEnv(ctx, envFile); err != nil {
			logger.Warn(ctx, "Failed to backup .env: %v", err)
		}
		// Sanitize existing
		if err := SanitizeEnv(ctx, envFile, conf); err != nil {
			logger.Warn(ctx, "Failed to sanitize .env: %v", err)
		}
	} else {
		// 3. Create from default if missing
		logger.Warn(ctx, "File '{{_File_}}%s{{|-|}}' not found. Copying example template.", envFile)

		defaultContent, err := assets.GetDefaultEnv()
		if err != nil {
			return fmt.Errorf("failed to load default env template: %w", err)
		}

		if err := os.WriteFile(envFile, defaultContent, 0644); err != nil {
			return fmt.Errorf("failed to create default env file: %w", err)
		}
		// Sanitize new
		if err := SanitizeEnv(ctx, envFile, conf); err != nil {
			logger.Warn(ctx, "Failed to sanitize .env: %v", err)
		}
	}

	// 4. Default Apps (Watchtower) if none referenced
	// Check regardless of creation or existing
	added, _ := ListAdded(envFile)
	if len(added) == 0 {
		logger.Info(ctx, "Installing default applications.")
		// Add Watchtower
		if err := env.Set("WATCHTOWER__ENABLED", "true", envFile); err != nil {
			return err
		}
		// Sanitize again? Bash says env_sanitize.
	}

	return nil
}

// BackupEnv creates a backup of the environment file
func BackupEnv(ctx context.Context, file string) error {
	// Simple backup: .env.bak
	// Bash does full timestamp folder.
	// For now, let's do .env.bak to basic safety.
	bak := file + ".bak"
	logger.Info(ctx, "Copying '{{_File_}}.env{{|-|}}' file to '{{_File_}}%s{{|-|}}'", bak)
	input, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return os.WriteFile(bak, input, 0644)
}

// SanitizeEnv sanitizes the environment file by setting default values
func SanitizeEnv(ctx context.Context, file string, conf config.AppConfig) error {
	// 1. Merge default values (if keys missing)
	// Create temp default file
	tmpDefault := filepath.Join(os.TempDir(), "ds2_default.env")
	defaultContent, err := assets.GetDefaultEnv()
	if err != nil {
		return fmt.Errorf("failed to load default env for sanitization: %w", err)
	}
	if err := os.WriteFile(tmpDefault, defaultContent, 0644); err != nil {
		return err
	}
	defer os.Remove(tmpDefault)

	if _, err := env.MergeNewOnly(ctx, file, tmpDefault); err != nil {
		logger.Error(ctx, "Failed to merge defaults: %v", err)
	}

	// 2. Collect Updates
	var updatedVars []string
	updates := make(map[string]string)

	home, _ := os.UserHomeDir()

	addUpdate := func(key, value string, literal bool) {
		// Replace absolute home with ${HOME} reference if it matches
		if home != "" && strings.HasPrefix(value, home) {
			value = strings.Replace(value, home, "${HOME}", 1)
		}

		// Normalize separators for the platform
		// But don't use filepath.Clean on variable strings if they look like / paths for Docker
		if runtime.GOOS == "windows" {
			// If it starts with a drive letter or looks like an absolute path, normalize it
			if len(value) > 2 && value[1] == ':' {
				value = filepath.Clean(value)
			} else {
				// For variable-based paths like ${VAR}/path, we prefer keeping them as-is or using /
				// However, if we want consistency with the platform:
				value = filepath.FromSlash(value)
			}
		}

		var finalVal string
		if literal {
			finalVal = value
		} else {
			finalVal = "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
		}

		updatedVars = append(updatedVars, key)
		updates[key] = finalVal
	}

	// PUID/PGID
	currentUser, err := user.Current()
	if err == nil {
		puid := currentUser.Uid
		pgid := currentUser.Gid
		if runtime.GOOS == "windows" {
			puid = "1000"
			pgid = "1000"
		}

		val, _ := env.Get("PUID", file)
		if strings.Contains(val, "x") || val == "" {
			addUpdate("PUID", puid, false)
		}

		val, _ = env.Get("PGID", file)
		if strings.Contains(val, "x") || val == "" {
			addUpdate("PGID", pgid, false)
		}
	}

	// TZ
	val, _ := env.Get("TZ", file)
	if val == "" {
		addUpdate("TZ", "UTC", false)
	}

	// HOME
	home, _ = os.UserHomeDir()
	val, _ = env.Get("HOME", file)
	if val == "" || strings.Contains(val, "x") {
		addUpdate("HOME", home, true)
	}

	// DOCKER_CONFIG_FOLDER
	val, _ = env.Get("DOCKER_CONFIG_FOLDER", file)
	if val == "" || strings.Contains(val, "x") {
		// Use unexpanded to preserve ${XDG_CONFIG_HOME}
		addUpdate("DOCKER_CONFIG_FOLDER", conf.ConfigFolderUnexpanded, true)
	}

	// DOCKER_COMPOSE_FOLDER
	val, _ = env.Get("DOCKER_COMPOSE_FOLDER", file)
	if val == "" || strings.Contains(val, "x") {
		// Use unexpanded to preserve ${XDG_CONFIG_HOME}
		addUpdate("DOCKER_COMPOSE_FOLDER", conf.ComposeFolderUnexpanded, true)
	}

	// DOCKER_HOSTNAME
	val, _ = env.Get("DOCKER_HOSTNAME", file)
	if val == "" || strings.Contains(val, "x") {
		hostname, err := os.Hostname()
		if err == nil {
			addUpdate("DOCKER_HOSTNAME", hostname, false)
		}
	}

	// 3. Log and Apply Updates
	if len(updatedVars) > 0 {
		logger.Notice(ctx, "Setting variables in '{{_File_}}%s{{|-|}}':", file)
		for _, key := range updatedVars {
			val := updates[key]
			logger.Notice(ctx, "   {{_Var_}}%s=%s{{|-|}}", key, val)
			env.SetLiteral(key, val, file)
		}
	}

	return nil
}
