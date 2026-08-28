package assets

import (
	"embed"
	"strings"
)

// "all:" applies to both patterns here -- without it, go:embed silently
// excludes dot-prefixed files (e.g. themes/.TEMPLATE.ds2theme) from the
// build.
//
//go:embed all:defaults all:themes
var embeddedFS embed.FS

// GetDefaultConfig returns the content of the default dockstarter2.toml file.
func GetDefaultConfig() ([]byte, error) {
	return embeddedFS.ReadFile("defaults/dockstarter2.toml")
}

// GetTheme reads a theme from the embedded filesystem.
func GetTheme(name string) ([]byte, error) {
	// embed.FS always uses forward slashes regardless of OS.
	return embeddedFS.ReadFile("themes/" + name + ".ds2theme")
}

// ListThemes returns all themes found in the embedded filesystem. Dot-prefixed
// files (e.g. .TEMPLATE.ds2theme, a starter reference copied into the user
// themes folder at startup -- see main.go) are excluded: they're not meant
// to be selectable themes.
func ListThemes() ([]string, error) {
	entries, err := embeddedFS.ReadDir("themes")
	if err != nil {
		return nil, err
	}
	var themes []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ds2theme") && !strings.HasPrefix(e.Name(), ".") {
			themes = append(themes, strings.TrimSuffix(e.Name(), ".ds2theme"))
		}
	}
	return themes, nil
}
