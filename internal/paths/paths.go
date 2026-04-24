package paths

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/version"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

var (
	// StateHomeOverride allows overriding the state home for tests.
	StateHomeOverride string
	// TemplatesDirOverride allows overriding the templates directory for tests.
	TemplatesDirOverride string
	// ConfigHomeOverride allows overriding the config home for tests.
	ConfigHomeOverride string

	// Version caching
	versionCacheMu sync.RWMutex
	lastTmplVer    string
	lastTmplCheck  time.Time
)

// GetConfigFilePath returns the absolute path to the dockstarter2.toml file.
// It places it in a subdirectory named after the application (e.g., ~/.config/dockstarter2/dockstarter2.toml).
func GetConfigFilePath() string {
	appName := strings.ToLower(version.ApplicationName)
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", appName, constants.AppConfigFileName)
	}
	if ConfigHomeOverride != "" {
		return filepath.Join(ConfigHomeOverride, appName, constants.AppConfigFileName)
	}
	return filepath.Join(xdg.ConfigHome, appName, constants.AppConfigFileName)
}

// GetTemplatesDir returns the absolute path to the DockSTARTer-Templates repository.
// It uses xdg.StateHome (e.g., %LOCALAPPDATA% on Windows) with a dockstarter subfolder (SHARED WITH BASH).
func GetTemplatesDir() string {
	if TemplatesDirOverride != "" {
		return TemplatesDirOverride
	}
	appName := constants.LegacyApplicationName
	return filepath.Join(xdg.StateHome, appName, "templates", "DockSTARTer-Templates")
}

// GetTemplatesVersion retrieves the current version of the DockSTARTer-Templates repository.
func GetTemplatesVersion() string {
	versionCacheMu.RLock()
	if time.Since(lastTmplCheck) < 60*time.Second {
		v := lastTmplVer
		versionCacheMu.RUnlock()
		return v
	}
	versionCacheMu.RUnlock()

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

	var result string
	if foundTag != "" {
		result = foundTag
	} else {
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
		result = fmt.Sprintf("%s commit %s", branchName, hash)
	}

	versionCacheMu.Lock()
	lastTmplVer = result
	lastTmplCheck = time.Now()
	versionCacheMu.Unlock()

	return result
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

// GetThemesDir returns the absolute path to the themes directory in the config folder.
func GetThemesDir() string {
	return filepath.Join(GetConfigDir(), constants.ThemesDirName)
}

// GetStateDir returns the absolute path to the dockstarter2 state directory.
func GetStateDir() string {
	if StateHomeOverride != "" {
		return StateHomeOverride
	}
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.StateHome, appName)
}

// GetLocksDir returns the absolute path to the dockstarter2 locks directory.
func GetLocksDir() string {
	return filepath.Join(GetStateDir(), "locks")
}

// GetInstancesDir returns the absolute path to the dockstarter2 instances directory.
func GetInstancesDir() string {
	return filepath.Join(GetStateDir(), constants.InstancesDirName)
}

// GetTimestampsDir returns the absolute path to the dockstarter2 timestamps directory.
func GetTimestampsDir() string {
	return filepath.Join(GetStateDir(), constants.TimestampsDirName)
}

// GetLocalLockPath returns the path to the local-side operation lock file.
// Written by the local TUI/CLI while a destructive operation is in progress.
func GetLocalLockPath() string {
	return filepath.Join(GetLocksDir(), "local.lock")
}

// GetRemoteLockPath returns the path to the remote-side operation lock file.
// Written by the SSH/web server session while a destructive operation is in progress.
func GetRemoteLockPath() string {
	return filepath.Join(GetLocksDir(), "remote.lock")
}

// GetActiveThemeFile returns the path to the currently-active theme file in the state directory.
// This is the single file Load() reads at runtime; it is written by EnsureThemeExtracted.
func GetActiveThemeFile() string {
	return filepath.Join(GetStateDir(), "theme.ds2theme")
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

// GetBashScriptFolder attempts to find the installation directory of the legacy Bash version of DockSTARTer.
// It mimics the logic in main.sh by finding the 'ds' command and following symlinks to its source.
func GetBashScriptFolder() string {
	// 1. Try to find the 'ds' command in the system path
	dsPath, err := exec.LookPath("ds")
	if err == nil {
		// Resolve all symlinks to find the canonical path (equivalent to the while loop in main.sh)
		if realPath, err := filepath.EvalSymlinks(dsPath); err == nil {
			return filepath.Dir(realPath)
		}
	}

	// 2. Fallback to default locations (~/.docker then ~/.dockstarter)
	home, err := os.UserHomeDir()
	if err == nil {
		for _, name := range []string{".dockstarter", ".docker"} {
			legacyPath := filepath.Join(home, name)
			if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
				return legacyPath
			}
		}
	}

	return ""
}

// GetLegacyIniPaths returns a slice of potential paths for legacy .ini configuration files.
func GetLegacyIniPaths() []string {
	var paths []string

	// 1. Check XDG Config Home (Modern nested first, then older root)
	paths = append(paths, filepath.Join(xdg.ConfigHome, "dockstarter", "dockstarter.ini"))
	paths = append(paths, filepath.Join(xdg.ConfigHome, "dockstarter.ini"))

	// 2. Check Legacy Script Folder locations
	if bashFolder := GetBashScriptFolder(); bashFolder != "" {
		paths = append(paths, filepath.Join(bashFolder, "dockstarter.ini"))
		paths = append(paths, filepath.Join(bashFolder, "menu.ini"))
	}

	// Return only paths that actually exist
	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}

	return existing
}

// ResolvePath resolves legacy variables like ${ScriptFolder}, ${HOME}, and ${XDG_CONFIG_HOME}.
func ResolvePath(path string) string {
	bashFolder := GetBashScriptFolder()
	home, _ := os.UserHomeDir()

	r := strings.NewReplacer(
		"${ScriptFolder}", bashFolder,
		"${HOME}", home,
		"${XDG_CONFIG_HOME}", xdg.ConfigHome,
	)
	return r.Replace(path)
}

// DetectComposeFolderResult holds the results of the compose folder detection.
type DetectComposeFolderResult struct {
	LegacyPath    string
	CurrentPath   string
	LegacyExists  bool
	CurrentExists bool
}

// DetectComposeFolder replicates the detection logic from config_create.sh.
func DetectComposeFolder(currentConfiguredPath string) DetectComposeFolderResult {
	res := DetectComposeFolderResult{}

	// 1. Resolve Legacy Path (${ScriptFolder}/compose)
	res.LegacyPath = ResolvePath("${ScriptFolder}/compose")
	if res.LegacyPath != "" {
		if info, err := os.Stat(res.LegacyPath); err == nil && info.IsDir() {
			if entries, err := os.ReadDir(res.LegacyPath); err == nil && len(entries) > 0 {
				res.LegacyExists = true
			}
		}
	}

	// 2. Resolve Current Configured Path
	res.CurrentPath = ResolvePath(currentConfiguredPath)
	if res.CurrentPath != "" {
		if info, err := os.Stat(res.CurrentPath); err == nil && info.IsDir() {
			if entries, err := os.ReadDir(res.CurrentPath); err == nil && len(entries) > 0 {
				res.CurrentExists = true
			}
		}
	}

	return res
}
