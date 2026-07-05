package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupEnv creates a timestamped backup of the environment files.
func BackupEnv(ctx context.Context, envFile string, conf config.AppConfig) error {
	composeFolder := filepath.Dir(envFile)

	// Check if we should backup at all
	hasEnv := false
	if _, err := os.Stat(envFile); err == nil {
		hasEnv = true
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
		// Strip quotes from default value (Read-only, no SetLiteral)
		dockerVolumeConfig = strings.Trim(dockerVolumeConfig, "'\"")
	}

	if dockerVolumeConfig == "" {
		return fmt.Errorf("Variable '{{|Var|}}DOCKER_VOLUME_CONFIG{{[-]}}' is not set.")
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
		return fmt.Errorf("DOCKER_VOLUME_CONFIG is not set and could not be determined.")
	}

	// info "Taking ownership of '${C["Folder"]}${DOCKER_VOLUME_CONFIG}${NC}' (non-recursive)."
	// (Non-functional on Windows, but preserved for intent)
	system.TakeOwnership(ctx, expandedVolumeConfig)

	// 3. Setup backup paths
	composeBackupsFolder := filepath.Join(expandedVolumeConfig, ".compose.backups")
	composeFolderName := filepath.Base(composeFolder)

	// Retry until we find a folder name that doesn't already exist.
	var backupFolder string
	for {
		backupFolder = filepath.Join(composeBackupsFolder, fmt.Sprintf("%s.%s", composeFolderName, time.Now().Format("20060102.15.04.05")))
		if _, err := os.Stat(backupFolder); os.IsNotExist(err) {
			break
		}
		time.Sleep(time.Second)
	}

	// 4. Gather and Copy files
	var backupList []string

	// Main .env
	if hasEnv {
		backupList = append(backupList, envFile)
	}

	// .env.app.*
	files, _ := os.ReadDir(composeFolder)
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), constants.AppEnvFileNamePrefix) {
			backupList = append(backupList, filepath.Join(composeFolder, f.Name()))
		}
	}

	// legacy application env folder (env_files)
	appEnvFolder := filepath.Join(composeFolder, constants.EnvFilesDirName)
	if info, err := os.Stat(appEnvFolder); err == nil && info.IsDir() {
		backupList = append(backupList, appEnvFolder)
	}

	// docker-compose.override.yml
	composeOverride := filepath.Join(composeFolder, constants.ComposeOverrideFileName)
	if _, err := os.Stat(composeOverride); err == nil {
		backupList = append(backupList, composeOverride)
	}

	sort.Strings(backupList)

	if len(backupList) > 0 {
		logger.Notice(ctx, "Backing up user files to folder:")
		logger.Notice(ctx, "\t'"+console.FormatFolderPath(backupFolder)+"'")
		logger.Info(ctx, "Creating folder '"+console.FormatFolderPath(backupFolder)+"'.")
		if err := os.MkdirAll(backupFolder, 0755); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to create folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, backupFolder)
		}
		logger.Info(ctx, "Backing up files:")
		for _, item := range backupList {
			logger.Info(ctx, "\t'"+console.FormatFilePath(item)+"'")
			dst := filepath.Join(backupFolder, filepath.Base(item))
			if info, err := os.Stat(item); err == nil && info.IsDir() {
				if err := copyDir(item, dst); err != nil {
					return fmt.Errorf("Failed to copy directory: %w", err)
				}
			} else {
				if err := CopyFile(item, dst); err != nil {
					return fmt.Errorf("Failed to copy file: %w", err)
				}
			}
		}
	} else {
		logger.Info(ctx, "No files to backup.")
	}

	// run_script 'set_permissions' "${COMPOSE_BACKUPS_FOLDER}"
	system.SetPermissions(ctx, composeBackupsFolder)

	// info "Removing old compose backups."
	logger.Info(ctx, "Removing old compose backups.")
	pruneOldBackups(ctx, composeBackupsFolder, composeFolderName)

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
		// Strip ? suffix used in `${VAR?}`
		// In bash, ${VAR?} means "error if not set", but we treat it as a normal expansion for intent
		cleanName := strings.TrimSuffix(varName, "?")

		// Handle fallback like `${VAR:-DEFAULT}`
		if strings.Contains(cleanName, ":-") {
			parts := strings.SplitN(cleanName, ":-", 2)
			name := parts[0]
			fallback := parts[1]
			if v, ok := ctx[name]; ok && v != "" {
				return v
			}
			if v := os.Getenv(name); v != "" {
				return v
			}
			return fallback
		}

		if v, ok := ctx[cleanName]; ok {
			return v
		}
		return os.Getenv(cleanName)
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

func pruneOldBackups(ctx context.Context, backupsFolder, prefix string) {
	files, err := os.ReadDir(backupsFolder)
	if err != nil {
		return
	}

	threshold := time.Now().AddDate(0, 0, -3)
	envPruneOk := true
	composePruneOk := true

	for _, f := range files {
		fullPath := filepath.Join(backupsFolder, f.Name())
		info, err := f.Info()
		if err != nil {
			continue
		}

		// find "${COMPOSE_BACKUPS_FOLDER}" -type f -name ".env.*" -mtime +3 -delete
		if !f.IsDir() && strings.HasPrefix(f.Name(), ".env.") {
			if info.ModTime().Before(threshold) {
				if err := os.Remove(fullPath); err != nil {
					envPruneOk = false
				}
			}
		}

		// find "${COMPOSE_BACKUPS_FOLDER}" -type d -name "${COMPOSE_FOLDER_NAME}.*" -mtime +3 -prune -exec rm -rf {} +
		if f.IsDir() && strings.HasPrefix(f.Name(), prefix+".") {
			if info.ModTime().Before(threshold) {
				if err := os.RemoveAll(fullPath); err != nil {
					composePruneOk = false
				}
			}
		}
	}

	if !envPruneOk {
		logger.Warn(ctx, "Old .env backups not removed.")
	}
	if !composePruneOk {
		logger.Warn(ctx, "Old compose backups not removed.")
	}
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info, err := os.Stat(dst); err == nil && !info.IsDir() {
		logger.Info(context.Background(), "Removing existing file '"+console.FormatFilePath(dst)+"' before folder can be created.")
		if err := os.Remove(dst); err != nil {
			logger.FatalWithStack(context.Background(), []string{
				"Failed to remove existing file.",
				"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
			}, dst)
		}
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		logger.FatalWithStack(context.Background(), []string{
			"Failed to create folder.",
			"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
		}, dst)
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
