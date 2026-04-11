package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
)

// NeedsYMLMerge checks if YML merge is needed using timestamp comparison
func NeedsYMLMerge(ctx context.Context, force bool) bool {
	if force {
		return true
	}

	conf := config.LoadAppConfig()

	// Check main files
	dockerCompose := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	if fileChanged(dockerCompose) {
		return true
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if fileChanged(envFile) {
		return true
	}

	// Check enabled apps .env files and their source templates
	enabledApps, _ := appenv.ListEnabledApps(conf)
	templatesDir := paths.GetTemplatesDir()
	
	// Use docker-compose.yml marker as reference time for template changes
	var referenceTime time.Time
	dcMarker := filepath.Join(paths.GetTimestampsDir(), "yml_merge", constants.ComposeFileName)
	if info, err := os.Stat(dcMarker); err == nil {
		referenceTime = info.ModTime()
	}

	for _, appName := range enabledApps {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		if fileChanged(appEnvFile) {
			return true
		}

		// Check app-specific template directory
		if !referenceTime.IsZero() {
			baseAppName := strings.ToLower(appenv.AppNameToBaseAppName(appName))
			appTemplateDir := filepath.Join(templatesDir, constants.TemplatesDirName, baseAppName)
			if appenv.IsAnyFileNewer(appTemplateDir, referenceTime) {
				return true
			}
		}
	}

	return false
}

// UnsetNeedsYMLMerge marks YML merge as complete by clearing all yml_merge_* files
// and updating timestamps for current state.
func UnsetNeedsYMLMerge(ctx context.Context) {
	conf := config.LoadAppConfig()
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "yml_merge")

	// Clear existing yml_merge markers
	_ = os.RemoveAll(timestampsFolder)
	_ = os.MkdirAll(timestampsFolder, 0755)

	dockerCompose := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	updateTimestamp(dockerCompose)

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	updateTimestamp(envFile)

	enabledApps, _ := appenv.ListEnabledApps(conf)
	for _, appName := range enabledApps {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		updateTimestamp(appEnvFile)
	}

}

// Helper functions

func fileChanged(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "yml_merge", filename)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	if os.IsNotExist(tsErr) {
		return true
	}

	if err != nil {
		return false
	}

	if !info.ModTime().Equal(tsInfo.ModTime()) {
		if appenv.CompareFiles(path, timestampFile) {
			_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
			return false
		}
		return true
	}

	return false
}

func updateTimestamp(path string) {
	if !fileExists(path) {
		return
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "yml_merge", filename)

	_ = os.MkdirAll(filepath.Dir(timestampFile), 0755)

	_ = appenv.CopyFile(path, timestampFile)

	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
