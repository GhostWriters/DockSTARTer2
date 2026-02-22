package theme

import (
	"DockSTARTer2/internal/paths"
	"os"
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
			tf, _ := GetThemeFile(name)
			themes = append(themes, ThemeMetadata{
				Name:        name,
				Description: tf.Metadata.Description,
				Author:      tf.Metadata.Author,
			})
		}
	}
	return themes, nil
}
