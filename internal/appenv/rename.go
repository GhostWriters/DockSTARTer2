package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RenameAppVars renames application-specific variables and folders.
// It mirrors the intent of appvars_rename.sh.
func RenameAppVars(ctx context.Context, fromApp, toApp string, conf config.AppConfig) error {
	fromUpper := strings.ToUpper(fromApp)
	toUpper := strings.ToUpper(toApp)
	fromLower := strings.ToLower(fromApp)
	toLower := strings.ToLower(toApp)

	logger.Notice(ctx, "Migrating from '{{|App|}}%s{{[-]}}' to '{{|App|}}%s{{[-]}}'.", fromUpper, toUpper)

	// 1. Stop container (if running)
	cmd := exec.CommandContext(ctx, "docker", "stop", fromLower)
	if err := cmd.Run(); err != nil {
		logger.Debug(ctx, "Failed to stop container %s: %v", fromLower, err)
	}

	// 2. Move config folder
	fromFolder := filepath.Join(conf.ConfigDir, fromLower)
	toFolder := filepath.Join(conf.ConfigDir, toLower)

	if _, err := os.Stat(fromFolder); err == nil {
		logger.Notice(ctx, "Moving configuration folder from '{{|Folder|}}%s{{[-]}}' to '{{|Folder|}}%s{{[-]}}'.", fromFolder, toFolder)
		if err := os.Rename(fromFolder, toFolder); err != nil {
			logger.Warn(ctx, "Failed to move folder: %v", err)
		}
		system.SetPermissions(ctx, toFolder)
	}

	// 3. Migrate variables in .env
	globalEnv := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if err := renameInFile(ctx, fromUpper, toUpper, globalEnv); err != nil {
		return err
	}

	// 4. Migrate and rename app-specific env file
	fromAppEnv := filepath.Join(conf.ComposeDir, constants.AppEnvFileNamePrefix+fromLower)
	toAppEnv := filepath.Join(conf.ComposeDir, constants.AppEnvFileNamePrefix+toLower)

	if _, err := os.Stat(fromAppEnv); err == nil {
		logger.Notice(ctx, "Renaming environment file '{{|File|}}%s{{[-]}}' to '{{|File|}}%s{{[-]}}'.", fromAppEnv, toAppEnv)
		if err := renameInFile(ctx, fromUpper, toUpper, fromAppEnv); err != nil {
			return err
		}

		if err := os.Rename(fromAppEnv, toAppEnv); err != nil {
			logger.Warn(ctx, "Failed to rename env file: %v", err)
		}
		system.SetPermissions(ctx, toAppEnv)
	}

	// 5. Create variables for the new app (ensures templates are added)
	if err := CreateApp(ctx, toUpper, conf); err != nil {
		logger.Warn(ctx, "Failed to create app variables for %s: %v", toUpper, err)
	}

	// 6. Format and sort all environment files
	if err := Update(ctx, false, globalEnv); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	}

	return nil
}

func renameInFile(ctx context.Context, fromUpper, toUpper, file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	changed := false
	pattern := fmt.Sprintf(`^\s*%s__`, regexp.QuoteMeta(fromUpper))
	re := regexp.MustCompile(pattern)
	replacement := toUpper + "__"

	for i, line := range lines {
		if re.MatchString(line) {
			lines[i] = re.ReplaceAllString(line, replacement)
			changed = true
		}
	}

	if changed {
		logger.Notice(ctx, "Renamed variables in '{{|File|}}%s{{[-]}}'.", filepath.Base(file))
		output := strings.Join(lines, "\n")
		if err := os.WriteFile(file, []byte(output), 0644); err != nil {
			return err
		}
		system.SetPermissions(ctx, file)
	}

	return nil
}
