package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateAppFolders reads *.folders files from app templates and creates corresponding directories.
// Mirrors appfolders_create.sh
func CreateAppFolders(ctx context.Context, appNameRaw string, conf config.AppConfig) error {
	appName := strings.ToLower(appNameRaw)
	niceName := GetNiceName(ctx, appName)

	// 1. Find .folders file
	foldersFile, err := AppInstanceFile(ctx, appName, "*.folders")
	if err != nil {
		return fmt.Errorf("failed to get folders template for %s: %w", appName, err)
	}

	if foldersFile == "" || !fileExists(foldersFile) {
		return nil
	}

	// 2. Read lines and filter to only those that need creating
	var foldersToCreate []string

	// Load environment variables for expansion context
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, appName))

	vars, err := ListVars(envFile)
	if err != nil {
		logger.Debug(ctx, "Failed to list global vars for expansion: %v", err)
	}
	appVars, err := ListVars(appEnvFile)
	if err != nil {
		logger.Debug(ctx, "Failed to list app vars for expansion: %v", err)
	}

	// Combine vars
	for k, v := range appVars {
		vars[k] = v
	}

	f, err := os.Open(foldersFile)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Expand variables in path
		expanded := ExpandVars(line, vars)
		if expanded == "" {
			continue
		}

		// Clean path and check if it exists
		expanded = filepath.Clean(expanded)
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			foldersToCreate = append(foldersToCreate, expanded)
		}
	}

	// 3. Create folders and log
	if len(foldersToCreate) > 0 {
		logger.Notice(ctx, "Creating config folders for '{{|App|}}%s{{[-]}}'.", niceName)
		for _, folder := range foldersToCreate {
			logger.Notice(ctx, "Creating folder '{{|Folder|}}%s{{[-]}}'", folder)
			if err := os.MkdirAll(folder, 0755); err != nil {
				logger.Warn(ctx, "Could not create folder '{{|Folder|}}%s{{[-]}}'", folder)
				logger.Warn(ctx, "Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}", folder)
			}
			// Bash version calls set_permissions here.
			// We might need a set_permissions equivalent eventually.
		}
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
