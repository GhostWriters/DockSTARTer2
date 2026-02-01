package theme

import (
	"DockSTARTer2/internal/paths"
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ThemeMetadata holds information about a theme.
type ThemeMetadata struct {
	Name        string
	Description string
	Author      string
}

// List returns a list of available themes with their metadata.
func List() ([]ThemeMetadata, error) {
	themesDir := paths.GetThemesDir()
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No themes directory means no themes (except maybe default?)
		}
		return nil, err
	}

	var themes []ThemeMetadata
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ds2theme") {
			name := strings.TrimSuffix(entry.Name(), ".ds2theme")
			meta, _ := getThemeMetadata(filepath.Join(themesDir, entry.Name()))
			meta.Name = name // Ensure name is set from filename
			themes = append(themes, meta)
		}
	}
	return themes, nil
}

func getThemeMetadata(path string) (ThemeMetadata, error) {
	var meta ThemeMetadata
	// Simple INI parsing just for metadata
	file, err := os.Open(path)
	if err != nil {
		return meta, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")

		switch key {
		case "ThemeDescription":
			meta.Description = val
		case "ThemeAuthor":
			meta.Author = val
		case "ThemeName":
			meta.Name = val // Optional override
		}
	}
	return meta, scanner.Err()
}
