package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// GetConfigFilePath returns the absolute path to the dockstarter2.ini file.
// On macOS, it explicitly uses ~/.config/dockstarter2.ini to match bash version behavior.
// On other platforms, it uses the standard xdg.ConfigHome.
func GetConfigFilePath() string {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "dockstarter2.ini")
	}
	return filepath.Join(xdg.ConfigHome, "dockstarter2.ini")
}

// GetTemplatesDir returns the absolute path to the DockSTARTer-Templates repository.
// It uses xdg.StateHome (e.g., %LOCALAPPDATA% on Windows) with a dockstarter subfolder.
func GetTemplatesDir() string {
	return filepath.Join(xdg.StateHome, "dockstarter", "templates", "DockSTARTer-Templates")
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

// GetCacheDir returns the absolute path to the dockstarter cache directory.
func GetCacheDir() string {
	return filepath.Join(xdg.CacheHome, "dockstarter")
}

// GetThemesDir returns the absolute path to the .themes directory.
// It looks for a .themes directory in the same folder as the executable, or the current working directory.
func GetThemesDir() string {
	// 1. Check relative to executable
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	path := filepath.Join(exeDir, ".themes")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// 2. Check current working directory (useful for development)
	cwd, _ := os.Getwd()
	path = filepath.Join(cwd, ".themes")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return path // Fallback to executable-relative path even if missing
}

// GetStateDir returns the absolute path to the dockstarter state directory.
func GetStateDir() string {
	return filepath.Join(xdg.StateHome, "dockstarter")
}
