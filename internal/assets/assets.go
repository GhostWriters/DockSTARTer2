package assets

import (
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:defaults themes
var embeddedFS embed.FS

// GetDefaultEnv returns the content of the default .env example file.
func GetDefaultEnv() ([]byte, error) {
	return embeddedFS.ReadFile("defaults/.env.example")
}

// EnsureAssets extracts embedded assets to the user's system if they are missing.
func EnsureAssets(ctx context.Context) error {
	// 1. Extract defaults (to config directory)
	if err := extractFolder(ctx, "defaults", paths.GetConfigDir()); err != nil {
		return fmt.Errorf("failed to extract defaults: %w", err)
	}

	// 2. Extract themes (to state directory)
	if err := extractFolder(ctx, "themes", paths.GetThemesDir()); err != nil {
		return fmt.Errorf("failed to extract themes: %w", err)
	}

	return nil
}

// GetTheme reads a theme from the embedded filesystem.
func GetTheme(name string) ([]byte, error) {
	// embed.FS always uses forward slashes regardless of OS.
	return embeddedFS.ReadFile("themes/" + name + ".ds2theme")
}

// ListThemes returns all themes found in the embedded filesystem.
func ListThemes() ([]string, error) {
	entries, err := embeddedFS.ReadDir("themes")
	if err != nil {
		return nil, err
	}
	var themes []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ds2theme") {
			themes = append(themes, strings.TrimSuffix(e.Name(), ".ds2theme"))
		}
	}
	return themes, nil
}

func extractFolder(ctx context.Context, srcDir, destDir string) error {
	return fs.WalkDir(embeddedFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from srcDir
		relPath, _ := filepath.Rel(srcDir, path)
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Extract file if it doesn't exist, OR if it's a theme file (force update for dev)
		// TODO: implementing a deeper check (hash/version) would be better for prod
		if _, err := os.Stat(targetPath); err == nil && !strings.Contains(targetPath, "themes") {
			// File exists, skip (unless it's a theme)
			return nil
		}

		logger.Info(ctx, "Extracting asset: %s", relPath)

		// Create parent dir just in case
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		srcFile, err := embeddedFS.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})
}
