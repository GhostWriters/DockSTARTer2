package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupEnv creates a timestamped backup of the environment files.
// Matches env_backup.sh logic exactly.
func BackupEnv(ctx context.Context, envFile string, conf config.AppConfig) error {
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		logger.Warn(ctx, "No .env file to back up.")
		return nil
	}

	// 1. Get DOCKER_CONFIG_FOLDER
	dockerConfigFolder, _ := Get("DOCKER_CONFIG_FOLDER", envFile)
	if dockerConfigFolder == "" {
		dockerConfigFolder = VarDefaultValue(ctx, "DOCKER_CONFIG_FOLDER", conf)
	}
	dockerConfigFolder = sanitizePath(dockerConfigFolder)
	literalConfigFolder := dockerConfigFolder

	// Sanitize/Expand DOCKER_CONFIG_FOLDER
	// Bash: eval echo "${LITERAL_CONFIG_FOLDER}" with HOME and XDG_CONFIG_HOME
	expandedConfigFolder := shellExpand(literalConfigFolder, map[string]string{
		"HOME":            os.Getenv("HOME"),
		"XDG_CONFIG_HOME": os.Getenv("XDG_CONFIG_HOME"),
	})
	expandedConfigFolder = filepath.Clean(expandedConfigFolder)

	// 2. Get DOCKER_VOLUME_CONFIG
	dockerVolumeConfig, _ := Get("DOCKER_VOLUME_CONFIG", envFile)
	if dockerVolumeConfig == "" {
		dockerVolumeConfig, _ = Get("DOCKERCONFDIR", envFile)
	}
	if dockerVolumeConfig == "" {
		dockerVolumeConfig = VarDefaultValue(ctx, "DOCKER_VOLUME_CONFIG", conf)
		_ = SetLiteral(ctx, "DOCKER_VOLUME_CONFIG", dockerVolumeConfig, envFile)
		dockerVolumeConfig, _ = Get("DOCKER_VOLUME_CONFIG", envFile)
	}

	if dockerVolumeConfig == "" {
		return fmt.Errorf("Variable '{{|Var|}}DOCKER_VOLUME_CONFIG{{[-]}}' is not set in the '{{|File|}}.env{{[-]}}' file")
	}

	// Sanitize/Expand DOCKER_VOLUME_CONFIG
	// Bash: eval echo with HOME, XDG_CONFIG_HOME, and DOCKER_CONFIG_FOLDER
	expandedVolumeConfig := shellExpand(dockerVolumeConfig, map[string]string{
		"HOME":                 os.Getenv("HOME"),
		"XDG_CONFIG_HOME":      os.Getenv("XDG_CONFIG_HOME"),
		"DOCKER_CONFIG_FOLDER": expandedConfigFolder,
	})
	expandedVolumeConfig = sanitizePath(expandedVolumeConfig)
	expandedVolumeConfig = filepath.Clean(expandedVolumeConfig)

	if expandedVolumeConfig == "" {
		return fmt.Errorf("DOCKER_VOLUME_CONFIG is not set and could not be determined")
	}

	// info "Taking ownership of '${C["Folder"]}${DOCKER_VOLUME_CONFIG}${NC}' (non-recursive)."
	// (Non-functional on Windows, but preserved for parity intent)
	system.TakeOwnership(ctx, expandedVolumeConfig)

	// 3. Setup backup paths
	composeBackupsFolder := filepath.Join(expandedVolumeConfig, ".compose.backups")
	backupTime := time.Now().Format("20060102.15.04.05")

	composeFolder := filepath.Dir(envFile)
	composeFolderName := filepath.Base(composeFolder)
	backupFolder := filepath.Join(composeBackupsFolder, fmt.Sprintf("%s.%s", composeFolderName, backupTime))

	// info "Copying '${C["File"]}.env${NC}' file to '${C["Folder"]}${BACKUP_FOLDER}/.env${NC}'"
	// Note: the second .env in the path is inferred from the bash copy command
	logger.Info(ctx, "Copying '{{|File|}}.env{{[-]}}' file to '{{|Folder|}}%s/.env{{[-]}}'", backupFolder)

	// 4. Create backup folder
	if err := os.MkdirAll(backupFolder, 0755); err != nil {
		return fmt.Errorf("Failed to make directory.")
	}

	// 5. Copy files

	// cp "${COMPOSE_ENV}" "${BACKUP_FOLDER}/"
	if err := CopyFile(envFile, filepath.Join(backupFolder, constants.EnvFileName)); err != nil {
		return fmt.Errorf("Failed to copy backup.")
	}

	// if [[ -n $(${FIND} "${COMPOSE_FOLDER}" -type f -maxdepth 1 -name ".env.app.*" 2> /dev/null) ]]; then
	files, _ := os.ReadDir(composeFolder)
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), constants.AppEnvFileNamePrefix) {
			src := filepath.Join(composeFolder, f.Name())
			dst := filepath.Join(backupFolder, f.Name())
			if err := CopyFile(src, dst); err != nil {
				return fmt.Errorf("Failed to copy backup.")
			}
		}
	}

	// if [[ -d ${APP_ENV_FOLDER} ]]; then
	// app_env_folder is env_files
	appEnvFolder := filepath.Join(composeFolder, constants.EnvFilesDirName)
	if info, err := os.Stat(appEnvFolder); err == nil && info.IsDir() {
		logger.Info(ctx, "Copying appplication env folder to '{{|Folder|}}%s/%s{{[-]}}'", backupFolder, constants.EnvFilesDirName)
		if err := copyDir(appEnvFolder, filepath.Join(backupFolder, constants.EnvFilesDirName)); err != nil {
			return fmt.Errorf("Failed to copy backup.")
		}
	}

	// if [[ -f ${COMPOSE_OVERRIDE} ]]; then
	composeOverride := filepath.Join(composeFolder, constants.ComposeOverrideFileName)
	if _, err := os.Stat(composeOverride); err == nil {
		logger.Info(ctx, "Copying override file to '{{|Folder|}}%s/%s{{[-]}}'", backupFolder, constants.ComposeOverrideFileName)
		if err := CopyFile(composeOverride, filepath.Join(backupFolder, constants.ComposeOverrideFileName)); err != nil {
			return fmt.Errorf("Failed to copy backup.")
		}
	}

	// run_script 'set_permissions' "${COMPOSE_BACKUPS_FOLDER}"
	system.SetPermissions(ctx, composeBackupsFolder)

	// info "Removing old compose backups."
	logger.Info(ctx, "Removing old compose backups.")
	pruneOldBackupsParity(ctx, composeBackupsFolder, composeFolderName)

	// if [[ -d "${DOCKER_VOLUME_CONFIG}/.env.backups" ]]; then
	legacyBackupDir := filepath.Join(expandedVolumeConfig, ".env.backups")
	if _, err := os.Stat(legacyBackupDir); err == nil {
		logger.Info(ctx, "Removing old backup location.")
		if err := os.RemoveAll(legacyBackupDir); err != nil {
			return fmt.Errorf("Failed to remove directory.")
		}
	}

	return nil
}

// shellExpand mimics a single-pass shell expansion like 'eval echo'
func shellExpand(val string, ctx map[string]string) string {
	return os.Expand(val, func(varName string) string {
		if v, ok := ctx[varName]; ok {
			return v
		}
		return os.Getenv(varName)
	})
}

// sanitizePath mimics the bash sanitize_path.sh logic
func sanitizePath(val string) string {
	if strings.Contains(val, "~") {
		home, _ := os.UserHomeDir()
		return strings.ReplaceAll(val, "~", home)
	}
	return val
}

func pruneOldBackupsParity(ctx context.Context, backupsFolder, prefix string) {
	files, err := os.ReadDir(backupsFolder)
	if err != nil {
		return
	}

	threshold := time.Now().AddDate(0, 0, -3)

	for _, f := range files {
		fullPath := filepath.Join(backupsFolder, f.Name())
		info, err := f.Info()
		if err != nil {
			continue
		}

		// find "${COMPOSE_BACKUPS_FOLDER}" -type f -name ".env.*" -mtime +3 -delete
		if !f.IsDir() && strings.HasPrefix(f.Name(), ".env.") {
			if info.ModTime().Before(threshold) {
				_ = os.Remove(fullPath)
			}
		}

		// find "${COMPOSE_BACKUPS_FOLDER}" -type d -name "${COMPOSE_FOLDER_NAME}.*" -mtime +3 -prune -exec rm -rf {} +
		if f.IsDir() && strings.HasPrefix(f.Name(), prefix+".") {
			if info.ModTime().Before(threshold) {
				_ = os.RemoveAll(fullPath)
			}
		}
	}
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	files, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, f := range files {
		srcPath := filepath.Join(src, f.Name())
		dstPath := filepath.Join(dst, f.Name())

		if f.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
