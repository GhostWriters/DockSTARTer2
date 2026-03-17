package assets

import (
	"embed"
	"strings"
)

//go:embed all:defaults themes
var embeddedFS embed.FS

// GetDefaultEnv returns the content of the default .env example file.
func GetDefaultEnv() ([]byte, error) {
	return embeddedFS.ReadFile("defaults/.env.example")
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
