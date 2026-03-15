package theme

import (
	"DockSTARTer2/internal/paths"
	"os"
	"strings"
)

// EmbeddedThemeLister is set at startup to provide the list of built-in theme names.
// EmbeddedThemeReader is set at startup to read a built-in theme by name.
// Both use callbacks to avoid an import cycle (theme → assets → logger → theme).
var (
	EmbeddedThemeLister func() ([]string, error)
	EmbeddedThemeReader func(name string) ([]byte, error)
)

// ThemeMetadata holds information about a theme.
type ThemeMetadata struct {
	Name        string // display name, e.g. "MyTheme"
	Description string
	Author      string
	IsUserTheme bool   // true if sourced from user:// (not an embedded built-in)
	ConfigValue string // raw value for config/Load(), e.g. "user://MyTheme" or "DockSTARTer"
}

// List returns a list of available themes with their metadata.
func List() ([]ThemeMetadata, error) {
	themesDir := paths.GetThemesDir()

	var metas []ThemeMetadata
	seenNames := make(map[string]bool)

	// 1. User themes from filesystem: files with "user_" prefix
	entries, err := os.ReadDir(themesDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ds2theme") {
				continue
			}
			filename := entry.Name()
			if !strings.HasPrefix(filename, userThemePrefix) {
				continue
			}
			// Strip prefix and extension to get display name
			name := strings.TrimSuffix(strings.TrimPrefix(filename, userThemePrefix), ".ds2theme")
			if seenNames[name] {
				continue
			}
			seenNames[name] = true
			configValue := "user://" + name
			tf, _ := GetThemeFile(configValue)
			metas = append(metas, ThemeMetadata{
				Name:        name,
				Description: tf.Metadata.Description,
				Author:      tf.Metadata.Author,
				IsUserTheme: true,
				ConfigValue: configValue,
			})
		}
	}

	// 2. Embedded built-in themes
	if EmbeddedThemeLister == nil {
		return metas, nil
	}
	if embeddedThemes, err := EmbeddedThemeLister(); err == nil {
		for _, name := range embeddedThemes {
			if seenNames[name] {
				continue
			}
			seenNames[name] = true
			tf, _ := GetThemeFile(name)
			metas = append(metas, ThemeMetadata{
				Name:        name,
				Description: tf.Metadata.Description,
				Author:      tf.Metadata.Author,
				IsUserTheme: false,
				ConfigValue: name,
			})
		}
	}

	return metas, nil
}
