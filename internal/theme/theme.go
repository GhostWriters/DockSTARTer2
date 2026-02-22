package theme

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/pelletier/go-toml/v2"
)

// StyleFlags holds ANSI style modifiers
type StyleFlags struct {
	Bold          bool
	Underline     bool
	Italic        bool
	Blink         bool
	Dim           bool
	Reverse       bool
	Strikethrough bool
	HighIntensity bool
}

// Apply applies all style flags to a lipgloss style
func (f StyleFlags) Apply(s lipgloss.Style) lipgloss.Style {
	return s.
		Bold(f.Bold).
		Underline(f.Underline).
		Italic(f.Italic).
		Blink(f.Blink).
		Faint(f.Dim).
		Reverse(f.Reverse).
		Strikethrough(f.Strikethrough)
}

// ResetFlags clears all text attributes from a style
func ResetFlags(s lipgloss.Style) lipgloss.Style {
	return StyleFlags{}.Apply(s)
}

// ThemeConfig holds the metadata for the active theme
type ThemeConfig struct {
	Name string
}

// Current holds the active theme configuration
var Current ThemeConfig

// Load theme by name. Returns theme-defined defaults if found.
// If prefix is provided, semantic tags are registered with that prefix (e.g. "Preview_Theme_Screen")
// without affecting the global active theme (Current).
func Load(themeName string, prefix string) (*ThemeDefaults, error) {
	// 1. Initialize with defaults first (Classic colors)
	Default(prefix)

	// If main load, set Current name
	if prefix == "" {
		Current.Name = themeName
	}

	themesDir := paths.GetThemesDir()
	themePath := filepath.Join(themesDir, themeName+".ds2theme")

	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		// If theme doesn't exist, try falling back to "DockSTARTer"
		if themeName != "DockSTARTer" {
			return Load("DockSTARTer", prefix)
		}
		// If even DockSTARTer doesn't exist, we stay with minimal defaults
		return nil, nil
	}

	// 2. Parse .ds2theme (Overrides defaults)
	defaults, err := parseThemeTOML(themePath, prefix)
	return defaults, err
}

// Apply updates the global console.Colors with theme-specific tags
func Apply() {
	// 0. Ensure base tags and color map are built from defaults FIRST
	// This prevents theme-specific registration from being wiped out later.
	console.RegisterBaseTags()
	console.BuildColorMap()
}

// prefixTag is a helper to consistently prefix theme-related semantic tags
func prefixTag(prefix, name string) string {
	if prefix == "" {
		return name
	}
	p := strings.TrimSuffix(prefix, "_")
	return p + "_" + name
}

// Unload unregisters all theme-prefixed tags from the console registry.
func Unload(prefix string) {
	if prefix == "" {
		return // Cannot unload main theme
	}
	console.UnregisterPrefix(prefixTag(prefix, "Theme_"))
}

// Default initializes the Current configuration with standard DockSTARTer colors (Classic)
// If prefix is provided, semantic tags are registered with that prefix.
func Default(prefix string) {
	// Only update global Current if prefix is empty
	if prefix == "" {
		Current.Name = "DockSTARTer"
	}
}

// resolveThemeValue recursively resolves a theme value string, handling semantic references and overrides.
// It supports chaining (A->B->C) and partial overlays.
// Uses file-specific delimiters (semPre/semSuf for semantic, dirPre/dirSuf for direct).
// Returns a RAW style string (fg:bg:flags) without any delimiters.
func resolveThemeValue(raw string, rawValues map[string]string, visiting map[string]bool,
	semPre, semSuf, dirPre, dirSuf string) (string, error) {

	var finalFG, finalBG string
	var finalFlags string

	// Helper to merge a raw style string (fg:bg:flags)
	mergeStyle := func(styleStr string) {
		// Strip any delimiters to get raw content
		inner := console.StripDelimiters(styleStr)
		parts := strings.Split(inner, ":")

		if len(parts) > 0 && parts[0] != "" {
			finalFG = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			finalBG = parts[1]
		}
		if len(parts) > 2 {
			// Merge flags (concatenate - renderer handles ordering)
			for _, f := range parts[2] {
				finalFlags += string(f)
			}
		}
	}

	// Iterate through the string seeking {{ }}
	cur := raw
	for {
		start := strings.Index(cur, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(cur[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		tag := cur[start:end]

		if strings.HasPrefix(tag, dirPre) {
			// Direct style tag - extract and merge
			mergeStyle(tag)
		} else if strings.HasPrefix(tag, semPre) {
			// Semantic reference - extract tag name
			refKey := strings.TrimSuffix(strings.TrimPrefix(tag, semPre), semSuf)

			// 1. Try resolving as internal reference first (with or without 'Theme_' prefix)
			targetKey := strings.TrimPrefix(refKey, "Theme_")
			if _, exists := rawValues[targetKey]; exists {
				resolvedRef, err := resolveThemeValue(rawValues[targetKey], rawValues, visiting,
					semPre, semSuf, dirPre, dirSuf)
				if err == nil {
					mergeStyle(resolvedRef)
					cur = cur[end:]
					continue
				}
			}

			// 2. Fallback to global semantic tags (e.g. Notice, Success)
			expanded := console.ExpandTags(tag)
			if expanded != tag && expanded != "" {
				mergeStyle(expanded)
			}
		}

		cur = cur[end:]
	}

	// Return RAW style string (no delimiters)
	return fmt.Sprintf("%s:%s:%s", finalFG, finalBG, finalFlags), nil
}

type ThemeDefaults struct {
	Borders        *bool `toml:"borders"`
	LineCharacters *bool `toml:"line_characters"`
	Shadow         *bool `toml:"shadow"`
	ShadowLevel    *int  `toml:"shadow_level"`
	Scrollbar      *bool `toml:"scrollbar"`
	BorderColor    *int  `toml:"border_color"`
}

type ThemeFile struct {
	Metadata struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
		Author      string `toml:"author"`
	} `toml:"metadata"`
	Syntax *struct {
		SemanticPrefix string `toml:"semantic_prefix"`
		SemanticSuffix string `toml:"semantic_suffix"`
		DirectPrefix   string `toml:"direct_prefix"`
		DirectSuffix   string `toml:"direct_suffix"`
	} `toml:"syntax"`
	Defaults *ThemeDefaults    `toml:"defaults"`
	Colors   map[string]string `toml:"colors"`
}

// GetThemeFile reads a theme file and returns its structured content without applying it.
func GetThemeFile(themeName string) (ThemeFile, error) {
	themePath := filepath.Join(paths.GetThemesDir(), themeName+".ds2theme")
	data, err := os.ReadFile(themePath)
	if err != nil {
		return ThemeFile{}, err
	}

	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return ThemeFile{}, err
	}
	return tf, nil
}

// ApplyThemeDefaults updates the app config with any defaults provided by the theme.
// It returns a map of all settings provided by the theme and their values.
func ApplyThemeDefaults(conf *config.AppConfig, defaults ThemeDefaults) map[string]string {
	applied := make(map[string]string)
	if defaults.Borders != nil {
		conf.UI.Borders = *defaults.Borders
		applied["Borders"] = fmt.Sprintf("%v", conf.UI.Borders)
	}
	if defaults.LineCharacters != nil {
		conf.UI.LineCharacters = *defaults.LineCharacters
		applied["Line Characters"] = fmt.Sprintf("%v", conf.UI.LineCharacters)
	}
	if defaults.Shadow != nil {
		conf.UI.Shadow = *defaults.Shadow
		applied["Shadow"] = fmt.Sprintf("%v", conf.UI.Shadow)
	}
	if defaults.ShadowLevel != nil {
		conf.UI.ShadowLevel = *defaults.ShadowLevel
		applied["Shadow Level"] = fmt.Sprintf("%d", conf.UI.ShadowLevel)
	}
	if defaults.Scrollbar != nil {
		conf.UI.Scrollbar = *defaults.Scrollbar
		applied["Scrollbar"] = fmt.Sprintf("%v", conf.UI.Scrollbar)
	}
	if defaults.BorderColor != nil {
		conf.UI.BorderColor = *defaults.BorderColor
		applied["Border Color"] = fmt.Sprintf("%d", conf.UI.BorderColor)
	}
	return applied
}

func parseThemeTOML(path string, prefix string) (*ThemeDefaults, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}

	// Get file-specific delimiters (or use code defaults)
	semPre, semSuf := console.SemanticPrefix, console.SemanticSuffix
	dirPre, dirSuf := console.DirectPrefix, console.DirectSuffix
	if tf.Syntax != nil {
		if tf.Syntax.SemanticPrefix != "" {
			semPre = tf.Syntax.SemanticPrefix
		}
		if tf.Syntax.SemanticSuffix != "" {
			semSuf = tf.Syntax.SemanticSuffix
		}
		if tf.Syntax.DirectPrefix != "" {
			dirPre = tf.Syntax.DirectPrefix
		}
		if tf.Syntax.DirectSuffix != "" {
			dirSuf = tf.Syntax.DirectSuffix
		}
	}

	// 1. Resolve values and register/apply them
	// We need to resolve references (e.g., TitleSuccess -> Title) before parsing colors
	rawValues := tf.Colors
	visiting := make(map[string]bool)

	// Maintains consistent registration/mapping logic from INI version
	for key, raw := range rawValues {
		styleValue, err := resolveThemeValue(raw, rawValues, visiting, semPre, semSuf, dirPre, dirSuf)
		if err != nil {
			// Fallback to raw expansion for robustness
			styleValue = console.StripDelimiters(console.ExpandTags(raw))
		}

		// Register using raw value (no delimiters)
		console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_"+key), styleValue)

	}

	// 2. Re-apply tags if loading main theme
	if prefix == "" {
		console.RegisterBaseTags()
		console.BuildColorMap()
	}

	return tf.Defaults, nil
}
