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
	Name        string // display name from metadata.name, or file stem if not set
	FileStem    string // file stem, e.g. "GreenScreen"; used in ConfigValue and for disambiguation
	Description string
	Author      string
	IsUserTheme bool // true if sourced from user: (not an embedded built-in)
	IsInvalid   bool // true if theme file is corrupted/unparseable
	ConfigValue string // raw value for config/Load(), e.g. "user:GreenScreen" or "DockSTARTer"
}

// List returns a list of available themes with their metadata.
func List() ([]ThemeMetadata, error) {
	themesDir := paths.GetThemesDir()

	var metas []ThemeMetadata
	seenUserStems := make(map[string]bool)
	seenEmbeddedStems := make(map[string]bool)

	// 1. Embedded built-in themes
	if EmbeddedThemeLister != nil {
		if embeddedThemes, err := EmbeddedThemeLister(); err == nil {
			for _, fileStem := range embeddedThemes {
				if seenEmbeddedStems[fileStem] {
					continue
				}
				seenEmbeddedStems[fileStem] = true
				tf, err := GetThemeFile(fileStem)
				displayName := fileStem
				if tf.Metadata.Name != "" {
					displayName = tf.Metadata.Name
				}
				metas = append(metas, ThemeMetadata{
					Name:        displayName,
					FileStem:    fileStem,
					Description: tf.Metadata.Description,
					Author:      tf.Metadata.Author,
					IsUserTheme: false,
					IsInvalid:   err != nil,
					ConfigValue: fileStem,
				})
			}
		}
	}

	// 2. User themes from filesystem: any .ds2theme file in the themes dir
	entries, err := os.ReadDir(themesDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ds2theme") {
				continue
			}
			fileStem := strings.TrimSuffix(entry.Name(), ".ds2theme")
			if seenUserStems[fileStem] {
				continue
			}
			seenUserStems[fileStem] = true
			configValue := "user:" + fileStem
			tf, err := GetThemeFile(configValue)
			displayName := fileStem
			if tf.Metadata.Name != "" {
				displayName = tf.Metadata.Name
			}
			metas = append(metas, ThemeMetadata{
				Name:        displayName,
				FileStem:    fileStem,
				Description: tf.Metadata.Description,
				Author:      tf.Metadata.Author,
				IsUserTheme: true,
				IsInvalid:   err != nil,
				ConfigValue: configValue,
			})
		}
	}

	return disambiguateNames(metas), nil
}

// disambiguateNames annotates entries that share a display name.
// User themes are shown as "user:Name", embedded themes keep their plain name.
func disambiguateNames(metas []ThemeMetadata) []ThemeMetadata {
	counts := make(map[string]int, len(metas))
	for _, m := range metas {
		counts[m.Name]++
	}
	for i := range metas {
		if counts[metas[i].Name] > 1 {
			if metas[i].IsUserTheme {
				metas[i].Name = "user:" + metas[i].Name
			}
		}
	}
	return metas
}
