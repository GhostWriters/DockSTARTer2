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

		// Extract file if it doesn't exist
		if _, err := os.Stat(targetPath); err == nil {
			// File exists, skip
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
