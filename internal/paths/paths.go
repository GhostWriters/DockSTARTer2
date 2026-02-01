package paths

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/version"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

var (
	// StateHomeOverride allows overriding the state home for tests.
	StateHomeOverride string
	// TemplatesDirOverride allows overriding the templates directory for tests.
	TemplatesDirOverride string
)

// GetConfigFilePath returns the absolute path to the dockstarter2.ini file.
// It places it in a subdirectory named after the application (e.g., ~/.config/dockstarter2/dockstarter2.ini).
func GetConfigFilePath() string {
	appName := strings.ToLower(version.ApplicationName)
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", appName, constants.AppINIFileName)
	}
	return filepath.Join(xdg.ConfigHome, appName, constants.AppINIFileName)
}

// GetTemplatesDir returns the absolute path to the DockSTARTer-Templates repository.
// It uses xdg.StateHome (e.g., %LOCALAPPDATA% on Windows) with a dockstarter2 subfolder.
func GetTemplatesDir() string {
	if TemplatesDirOverride != "" {
		return TemplatesDirOverride
	}
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.StateHome, appName, "templates", "DockSTARTer-Templates") // Note: Repo name is still specific, but subfolders are constants
}

// GetTemplatesVersion retrieves the current version of the DockSTARTer-Templates repository.
func GetTemplatesVersion() string {
	templatesDir := GetTemplatesDir()

	// Open repository
	r, err := git.PlainOpen(templatesDir)
	if err != nil {
		return "Unknown Version"
	}

	// Get HEAD
	head, err := r.Head()
	if err != nil {
		return "Unknown Version"
	}

	// Get Tag (if any)
	// Iterate valid tags and check if any point to HEAD
	tags, err := r.Tags()
	foundTag := ""
	if err == nil {
		_ = tags.ForEach(func(ref *plumbing.Reference) error {
			if ref.Hash() == head.Hash() {
				// Found a tag for this commit. Use strict short name (e.g. v1.0.0)
				foundTag = ref.Name().Short()
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		})
	}
	if foundTag != "" {
		return foundTag
	}

	// 3. Fallback to format: "BranchName commit shortHash"
	branchName := "HEAD"
	if head.Name().IsBranch() {
		branchName = head.Name().Short()
	}

	// Short hash
	hash := head.Hash().String()
	if len(hash) > 7 {
		hash = hash[:7]
	}

	return fmt.Sprintf("%s commit %s", branchName, hash)
}

// GetCacheDir returns the absolute path to the dockstarter2 cache directory.
func GetCacheDir() string {
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.CacheHome, appName)
}

// GetConfigDir returns the absolute path to the dockstarter2 configuration directory.
func GetConfigDir() string {
	return filepath.Dir(GetConfigFilePath())
}

// GetThemesDir returns the absolute path to the themes directory in the state folder.
func GetThemesDir() string {
	return filepath.Join(GetStateDir(), constants.ThemesDirName)
}

// GetStateDir returns the absolute path to the dockstarter2 state directory.
func GetStateDir() string {
	if StateHomeOverride != "" {
		return StateHomeOverride
	}
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.StateHome, appName)
}

// GetInstancesDir returns the absolute path to the dockstarter2 instances directory.
func GetInstancesDir() string {
	return filepath.Join(GetStateDir(), constants.InstancesDirName)
}

// GetTimestampsDir returns the absolute path to the dockstarter2 timestamps directory.
func GetTimestampsDir() string {
	return filepath.Join(GetStateDir(), constants.TimestampsDirName)
}

// GetInstanceDir returns the absolute path to the folder for a specific app instance.
func GetInstanceDir(appName string) string {
	return filepath.Join(GetInstancesDir(), appName)
}

// GetExecDirectory returns the directory of the currently running executable.
func GetExecDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ResetNeeds deletes the timestamps directory, mirroring reset_needs.sh.
func ResetNeeds() error {
	timestampDir := GetTimestampsDir()
	if _, err := os.Stat(timestampDir); err == nil {
		return os.RemoveAll(timestampDir)
	}
	return nil
}
