package assets

import (
	"embed"
	"strings"
)

// "all:" applies to both patterns here -- without it, go:embed silently
// excludes dot-prefixed files (e.g. themes/.TEMPLATE.ds2theme) from the
// build.
//
//go:embed all:defaults all:themes all:tint_themes
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

// GetTintTheme reads a bundled base16 scheme YAML file from the embedded
// tint_themes folder by name (no ".yaml" suffix), or an error if name isn't
// one of ListTintThemes. Checked by --tint-repo before falling back to the
// cloned tinted-theming/schemes repo, so a name shipped here always
// resolves even before that repo is cloned -- and if tinted-theming ever
// publishes an equivalent scheme upstream, removing the file here just
// falls through to that instead.
func GetTintTheme(name string) ([]byte, error) {
	return embeddedFS.ReadFile("tint_themes/" + name + ".yaml")
}

// ListTintThemes returns all scheme names bundled in the embedded
// tint_themes folder.
func ListTintThemes() ([]string, error) {
	entries, err := embeddedFS.ReadDir("tint_themes")
	if err != nil {
		return nil, err
	}
	var schemes []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			schemes = append(schemes, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return schemes, nil
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
