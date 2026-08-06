package appenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
)

// --- Timestamp Helpers ---

// NeedsCreateAll checks if environment variables need to be created/updated.
// Mirrors needs_appvars_create.sh
func NeedsCreateAll(ctx context.Context, force bool, added []string, conf config.AppConfig) bool {
	if force {
		return true
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "appvars_create")

	// 1. Check Global .env
	if fileChanged(envFile) {
		return true
	}

	if len(added) == 0 {
		// BULK MODE
		// 2. Check Added Apps List
		addedAppsFile := filepath.Join(timestampsFolder, "AddedApps")
		storedApps, err := os.ReadFile(addedAppsFile)
		// We use current added apps for comparison
		allAdded, _ := ListAddedApps(ctx, envFile)
		if err != nil || strings.TrimSpace(string(storedApps)) != strings.TrimSpace(strings.Join(allAdded, "\n")) {
			return true
		}

		// 3. Bulk Scan (Templates and App Env Files)
		sentinelFile := filepath.Join(timestampsFolder, "LastSynced")
		sentinelInfo, err := os.Stat(sentinelFile)
		if err != nil {
			return true
		}
		sentinelTime := sentinelInfo.ModTime()

		// Check if any added app's template or env file has changed
		templatesDir := paths.GetTemplatesDir()
		for _, appName := range allAdded {
			// Check app-specific env file
			appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
			if fileChanged(appEnvFile) {
				return true
			}

			// Check app-specific template directory
			baseAppName := strings.ToLower(AppNameToBaseAppName(appName))
			appTemplateDir := filepath.Join(templatesDir, constants.TemplatesDirName, baseAppName)
			if IsAnyFileNewer(appTemplateDir, sentinelTime) {
				return true
			}
		}

		return false
	}

	// PRECISE MODE (One or more apps)
	for _, appName := range added {
		appUpper := strings.ToUpper(appName)
		// Check if app is added
		if !IsAppAdded(ctx, appUpper, envFile) {
			return true
		}

		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		if fileChanged(appEnvFile) {
			return true
		}

		// Implement NewestSentinel logic
		globalSentinel := filepath.Join(timestampsFolder, "LastSynced")
		appSentinel := filepath.Join(timestampsFolder, "LastSynced_"+appUpper)

		newestSentinelTime := time.Time{}
		if info, err := os.Stat(appSentinel); err == nil {
			newestSentinelTime = info.ModTime()
		}

		if info, err := os.Stat(globalSentinel); err == nil {
			if newestSentinelTime.IsZero() || info.ModTime().After(newestSentinelTime) {
				newestSentinelTime = info.ModTime()
			}
		}

		if newestSentinelTime.IsZero() {
			return true
		}

		// Scan app-specific template folder
		baseAppName := strings.ToLower(AppNameToBaseAppName(appName))
		appTemplateDir := filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, baseAppName)
		if IsAnyFileNewer(appTemplateDir, newestSentinelTime) {
			return true
		}
	}

	return false
}

// UnsetNeedsCreateAll marks environment variable creation as complete by updating timestamps for all added apps.
// Mirrors unset_needs_appvars_create.sh (bulk)
func UnsetNeedsCreateAll(ctx context.Context, added []string, conf config.AppConfig) {
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "appvars_create")
	_ = os.MkdirAll(timestampsFolder, 0755)

	// 1. Record global .env state
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	recordFileState(envFile)

	// 2. Update AddedApps list record (with trailing newline for Bash printf parity)
	_ = os.WriteFile(filepath.Join(timestampsFolder, "AddedApps"), []byte(strings.Join(added, "\n")+"\n"), 0644)

	// 3. Create bulk LastSynced sentinel
	f, _ := os.Create(filepath.Join(timestampsFolder, "LastSynced"))
	if f != nil {
		f.Close()
	}

	// 4. Update individual markers
	for _, appName := range added {
		UnsetNeedsCreateApp(ctx, appName, conf)
	}
}

// UnsetNeedsCreateApp marks environment variable creation as complete for a single app.
// Mirrors unset_needs_appvars_create.sh (precise)
func UnsetNeedsCreateApp(ctx context.Context, appNameRaw string, conf config.AppConfig) {
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "appvars_create")
	_ = os.MkdirAll(timestampsFolder, 0755)

	appUpper := strings.ToUpper(appNameRaw)
	appLower := strings.ToLower(appNameRaw)

	// 1. Create individual LastSynced marker
	f, _ := os.Create(filepath.Join(timestampsFolder, "LastSynced_"+appUpper))
	if f != nil {
		f.Close()
	}

	// 2. Record app-specific .env state
	appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, appLower))
	recordFileState(appEnvFile)
}

func fileChanged(path string) bool {
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
		// Compare file contents
		if CompareFiles(path, timestampFile) {
			// Contents are same, sync timestamp to avoid re-check
			_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
			return false
		}
		return true
	}

	return false
}

func recordFileState(path string) {
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
