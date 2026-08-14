package theme

import (
	"DockSTARTer2/internal/paths"
	"io/fs"
	"path/filepath"
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
	FileStem    string // file stem, e.g. "GreenScreen" or "Chris's Themes/Chris's Awesome Theme" for a nested user theme; used in ConfigValue and for disambiguation
	Description string
	Author      string
	IsUserTheme bool   // true if sourced from user: (not an embedded built-in)
	IsInvalid   bool   // true if theme file is corrupted/unparseable
	ConfigValue string // raw value for config/Load(), e.g. "user:GreenScreen" or "DockSTARTer"
}

// List returns a list of available themes with their metadata. currentValue
// is the active config.AppConfig.UI.Theme value (e.g. "user:MyTheme"),
// used only so a dot-prefixed user theme file that's somehow the active
// selection still appears -- otherwise the Appearance menu would show no
// entry at all matching the theme actually in use. Pass "" if the caller
// doesn't need this (dot-prefixed files are then always excluded).
func List(currentValue string) ([]ThemeMetadata, error) {
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

	// 2. User themes from filesystem: any .ds2theme file under the themes
	// dir, including subfolders -- fileStem is the path relative to
	// themesDir (slash-separated, extension stripped) so themes organized
	// into folders (e.g. "user:Chris's Themes/Chris's Awesome Theme") get a
	// distinct FileStem/ConfigValue instead of colliding with a same-named
	// file elsewhere in the tree. Dot-prefixed files are skipped unless
	// currently selected (see List's doc comment) -- this is how
	// .TEMPLATE.ds2theme (see main.go) stays out of the theme list while
	// still sitting right in the folder as a copyable reference. Dot-prefixed
	// folders are always skipped (not descended into): the "currently
	// selected" exception only needs to reach a top-level dot-file, the
	// concrete case this exists for.
	_ = filepath.WalkDir(themesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != themesDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".ds2theme") {
			return nil
		}
		rel, err := filepath.Rel(themesDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		fileStem := strings.TrimSuffix(rel, ".ds2theme")
		configValue := "user:" + fileStem
		if strings.HasPrefix(d.Name(), ".") && configValue != currentValue {
			return nil
		}
		if seenUserStems[fileStem] {
			return nil
		}
		seenUserStems[fileStem] = true
		tf, err := GetThemeFile(configValue)
		displayName := fileStem
		if tf.Metadata.Name != "" {
			displayName = tf.Metadata.Name
		}
		// Prefix the containing folder onto the display name for a nested
		// theme, so two themes that happen to share a display name (or a
		// filename) in different folders remain distinguishable in the
		// Appearance menu.
		if dir := filepath.Dir(fileStem); dir != "." {
			displayName = dir + "/" + displayName
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
		return nil
	})

	return disambiguateNames(metas), nil
}

// disambiguateNames prefixes every user theme's display name with "user:"
// (matching its ConfigValue), so it's visually distinguishable from an
// embedded theme even when a folder-qualified name (e.g. "test/DockSTARTer")
// already happens to be unique on its own. Embedded themes keep their plain
// name.
func disambiguateNames(metas []ThemeMetadata) []ThemeMetadata {
	for i := range metas {
		if metas[i].IsUserTheme {
			metas[i].Name = "user:" + metas[i].Name
		}
	}
	return metas
}
