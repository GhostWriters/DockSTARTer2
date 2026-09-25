package config

import (
	"strings"

	"DockSTARTer2/internal/console"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
)

// ConnTypes lists every connection type a session can have, in display order:
// "local" is DS2 run directly in any terminal on this machine (including one
// reached through a real SSH login), "ssh" and "web" are sessions through
// DS2's own SSH and web servers.
var ConnTypes = []string{"local", "ssh", "web"}

// ConnTypeLabel returns connType's user-facing name.
func ConnTypeLabel(connType string) string {
	switch connType {
	case "ssh":
		return "SSH Server"
	case "web":
		return "Web Server"
	default:
		return "Local"
	}
}

// ConnTypeLabels returns the user-facing names of connTypes, comma-separated.
func ConnTypeLabels(connTypes []string) string {
	labels := make([]string, len(connTypes))
	for i, ct := range connTypes {
		labels[i] = ConnTypeLabel(ct)
	}
	return strings.Join(labels, ", ")
}

// AppearanceConfig is the [appearance] table: settings shared by every
// connection type, plus each connection type's own Appearance as a
// sub-table ([appearance.local], [appearance.ssh], [appearance.web]).
type AppearanceConfig struct {
	SpinnerSpeed       int    `toml:"spinner_speed"`       // milliseconds per frame, default 120
	RefreshRate        int    `toml:"refresh_rate"`        // screen repaint interval in milliseconds, default 60
	PanelLocal         string `toml:"panel_local"`         // "log", "console", or "none" (for local sessions)
	PanelRemote        string `toml:"panel_remote"`        // "log", "console", or "none" (for ssh/web sessions)
	ShowPreview        bool   `toml:"show_preview"`        // default visibility of the Appearance Settings preview panel
	MarkdownHyperlinks string `toml:"markdown_hyperlinks"` // "off", "inline", or "auto" -- OSC8 hyperlink rendering for markdown (help dialog doc page, --man)
	Hyperlinks         string `toml:"hyperlinks"`          // "off", "inline", or "auto" -- OSC8 hyperlink rendering for DS2's own console/path/link tags (semstyle.HyperlinkModeFunc)

	Local Appearance `toml:"local"`
	SSH   Appearance `toml:"ssh"`
	Web   Appearance `toml:"web"`
}

// Appearance holds one connection type's theme, the settings a theme's
// [defaults] table can set (see theme.ThemeDefaults), and its ANSI tint.
type Appearance struct {
	Theme              string `toml:"theme"`
	Borders            bool   `toml:"borders"`
	LargeButtons       bool   `toml:"large_buttons"`
	LargeTitleBars     bool   `toml:"large_title_bars"`
	LineCharacters     bool   `toml:"line_characters"`
	Shadow             bool   `toml:"shadow"`
	ShadowLevel        int    `toml:"shadow_level"` // 0=off, 1=light(░), 2=medium(▒), 3=dark(▓), 4=solid(█)
	Scrollbar          bool   `toml:"scrollbar"`
	Spinner            bool   `toml:"spinner"`
	BorderColor        int    `toml:"border_color"`         // 1=Border, 2=Border2, 3=Both
	DialogTitleAlign   string `toml:"dialog_title_align"`   // "center" or "left"
	SubmenuTitleAlign  string `toml:"submenu_title_align"`  // "center" or "left"
	PanelTitleAlign    string `toml:"panel_title_align"`    // "center" or "left"
	CheckboxBrackets   string `toml:"checkbox_brackets"`    // "never", "selected", or "always" -- the focused row's brackets always show regardless
	RadioBrackets      string `toml:"radio_brackets"`       // "never", "selected", or "always" -- the focused row's brackets always show regardless
	MenuBrackets       bool   `toml:"menu_brackets"`        // wrap the focused menu item's tag in [brackets]
	LineNumberBrackets bool   `toml:"line_number_brackets"` // wrap the focused line's number in [brackets] in the env editor
	TabLayout          string `toml:"tab_layout"`           // "maximized", "sidebyside", or "stacked" -- default view when the tabbed vars editor has 2 tabs open

	AnsiColors AnsiColors `toml:"ansi_palette"`
}

// ForConnType returns connType's Appearance; an unrecognized connType gets
// local's.
func (c AppearanceConfig) ForConnType(connType string) Appearance {
	return *c.Ptr(connType)
}

// Ptr returns a pointer to connType's Appearance, for writing; an
// unrecognized connType gets local's.
func (c *AppearanceConfig) Ptr(connType string) *Appearance {
	switch connType {
	case "ssh":
		return &c.SSH
	case "web":
		return &c.Web
	default:
		return &c.Local
	}
}

// CopySharedFrom copies src's shared settings (everything outside the
// per-connection-type blocks) onto c, leaving c's Local/SSH/Web untouched.
func (c *AppearanceConfig) CopySharedFrom(src AppearanceConfig) {
	local, ssh, web := c.Local, c.SSH, c.Web
	*c = src
	c.Local, c.SSH, c.Web = local, ssh, web
}

// CopySettingsFrom copies src's theme and settings onto a, leaving a's
// ANSI palette untouched.
func (a *Appearance) CopySettingsFrom(src Appearance) {
	palette := a.AnsiColors
	*a = src
	a.AnsiColors = palette
}

// ApplyToConsole records every connection type's line_characters and
// spinner settings with the console package (see
// console.SetConnTypeDisplay).
func (c AppearanceConfig) ApplyToConsole() {
	for _, ct := range ConnTypes {
		a := c.ForConnType(ct)
		console.SetConnTypeDisplay(ct, a.LineCharacters, a.Spinner)
	}
}

// decodeWeak decodes src onto dst using the same loose type conversion as
// UnmarshalRobust, leaving any field src doesn't mention untouched.
func decodeWeak(src map[string]any, dst any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           dst,
		TagName:          "toml",
	})
	if err != nil {
		return err
	}
	return decoder.Decode(src)
}

// migrateToAppearance moves settings from a config file that predates the
// [appearance] table into it: [ui]'s shared settings into [appearance],
// [ui]'s theme and theme-default settings into every connection type's
// block, and each connection type's [ansi_palette.<type>] table into
// [appearance.<type>.ansi_palette]. A connection type whose block already
// carries a given setting is left alone.
func migrateToAppearance(data []byte, conf *AppConfig) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return
	}
	appearance, _ := raw["appearance"].(map[string]any)

	if ui, ok := raw["ui"].(map[string]any); ok && appearance == nil {
		_ = decodeWeak(ui, &conf.Appearance)
		for _, ct := range ConnTypes {
			_ = decodeWeak(ui, conf.Appearance.Ptr(ct))
		}
	}

	palette, _ := raw["ansi_palette"].(map[string]any)
	for _, ct := range ConnTypes {
		src, ok := palette[ct].(map[string]any)
		if !ok {
			continue
		}
		if block, ok := appearance[ct].(map[string]any); ok {
			if _, has := block["ansi_palette"]; has {
				continue
			}
		}
		_ = decodeWeak(src, &conf.Appearance.Ptr(ct).AnsiColors)
	}
}
