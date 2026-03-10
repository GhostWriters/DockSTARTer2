package theme

import (
	"DockSTARTer2/internal/assets"
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

	themeNamesMap := make(map[string]bool)
	var themeNames []string

	// 1. Get custom themes from the filesystem
	entries, err := os.ReadDir(themesDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ds2theme") {
				name := strings.TrimSuffix(entry.Name(), ".ds2theme")
				if !themeNamesMap[name] {
					themeNamesMap[name] = true
					themeNames = append(themeNames, name)
				}
			}
		}
	}

	// 2. Get embedded themes
	if embeddedThemes, err := assets.ListThemes(); err == nil {
		for _, name := range embeddedThemes {
			if !themeNamesMap[name] {
				themeNamesMap[name] = true
				themeNames = append(themeNames, name)
			}
		}
	}

	var themes []ThemeMetadata
	for _, name := range themeNames {
		tf, _ := GetThemeFile(name)
		themes = append(themes, ThemeMetadata{
			Name:        name,
			Description: tf.Metadata.Description,
			Author:      tf.Metadata.Author,
		})
	}

	return themes, nil
}
