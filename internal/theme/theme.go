package theme

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// ThemeDisplayName returns the human-readable theme name from a config value.
// "user:MyTheme" or "user:MyTheme.ds2theme" → "MyTheme"
// "file:/path/to/GreenScreen.ds2theme"       → "GreenScreen"
// "DockSTARTer"                              → "DockSTARTer"
func ThemeDisplayName(themeNameOrURI string) string {
	if strings.HasPrefix(themeNameOrURI, "user:") {
		name := strings.TrimPrefix(themeNameOrURI, "user:")
		return strings.TrimSuffix(name, ".ds2theme")
	}
	if strings.HasPrefix(themeNameOrURI, "file:") {
		base := filepath.Base(strings.TrimPrefix(themeNameOrURI, "file:"))
		return strings.TrimSuffix(base, ".ds2theme")
	}
	return themeNameOrURI
}

// resolveThemeData reads theme bytes directly from its source without touching the state file.
// Used for preview loads (prefix != "") to avoid disk writes on every cursor move.
func resolveThemeData(themeNameOrURI string) ([]byte, error) {
	if strings.HasPrefix(themeNameOrURI, "user:") {
		themeName := strings.TrimSuffix(strings.TrimPrefix(themeNameOrURI, "user:"), ".ds2theme")
		return os.ReadFile(filepath.Join(paths.GetThemesDir(), themeName+".ds2theme"))
	}
	if strings.HasPrefix(themeNameOrURI, "file:") {
		return os.ReadFile(strings.TrimPrefix(themeNameOrURI, "file:"))
	}
	if EmbeddedThemeReader != nil {
		return EmbeddedThemeReader(themeNameOrURI)
	}
	return nil, fmt.Errorf("embedded theme reader not initialised")
}

// ResolveThemeData reads raw bytes for a theme by name or URI.
// Exported for use by CLI extract commands.
func ResolveThemeData(themeNameOrURI string) ([]byte, error) {
	return resolveThemeData(themeNameOrURI)
}

// FileStemFromURI returns the file stem (without .ds2theme) for a theme URI.
// "user:GreenScreen" or "user:GreenScreen.ds2theme" → "GreenScreen"
// "file:/path/to/GreenScreen.ds2theme"              → "GreenScreen"
// "DockSTARTer"                                     → "DockSTARTer"
func FileStemFromURI(themeNameOrURI string) string {
	if strings.HasPrefix(themeNameOrURI, "user:") {
		name := strings.TrimPrefix(themeNameOrURI, "user:")
		return strings.TrimSuffix(name, ".ds2theme")
	}
	if strings.HasPrefix(themeNameOrURI, "file:") {
		base := filepath.Base(strings.TrimPrefix(themeNameOrURI, "file:"))
		return strings.TrimSuffix(base, ".ds2theme")
	}
	return strings.TrimSuffix(themeNameOrURI, ".ds2theme")
}

// activeThemeMatchesData returns true if the active state theme file has identical content to data.
func activeThemeMatchesData(data []byte) bool {
	existing, err := os.ReadFile(paths.GetActiveThemeFile())
	if err != nil {
		return false
	}
	return bytes.Equal(existing, data)
}

// EnsureThemeExtracted ensures the active theme state file is up to date from its source.
// For embedded themes: compares embedded bytes to state file, updates if different.
// For user: themes: compares config themes dir copy to state file, updates if different.
// Returns the path to the active theme state file.
func EnsureThemeExtracted(themeNameOrURI string) (string, error) {
	stateFile := paths.GetActiveThemeFile()

	var sourceData []byte
	var err error

	if strings.HasPrefix(themeNameOrURI, "user:") {
		// User theme — source is in the themes dir
		themeName := strings.TrimSuffix(strings.TrimPrefix(themeNameOrURI, "user:"), ".ds2theme")
		sourcePath := filepath.Join(paths.GetThemesDir(), themeName+".ds2theme")
		sourceData, err = os.ReadFile(sourcePath)
		if err != nil {
			// Source gone — use existing state file if available
			if _, statErr := os.Stat(stateFile); statErr == nil {
				return stateFile, nil
			}
			return "", fmt.Errorf("user theme not found: %s", themeName)
		}
	} else if strings.HasPrefix(themeNameOrURI, "file:") {
		// File theme — absolute path stored in config
		sourcePath := strings.TrimPrefix(themeNameOrURI, "file:")
		sourceData, err = os.ReadFile(sourcePath)
		if err != nil {
			// Source gone — use existing state file if available
			if _, statErr := os.Stat(stateFile); statErr == nil {
				return stateFile, nil
			}
			return "", fmt.Errorf("theme file not found: %s", sourcePath)
		}
	} else {
		// Named embedded theme — source is in the binary
		if EmbeddedThemeReader == nil {
			return "", fmt.Errorf("embedded theme reader not initialised")
		}
		sourceData, err = EmbeddedThemeReader(themeNameOrURI)
		if err != nil {
			return "", fmt.Errorf("theme not found: %s", themeNameOrURI)
		}
	}
	// Write to state file if missing or content differs
	if !activeThemeMatchesData(sourceData) {
		if info, err := os.Stat(paths.GetStateDir()); err == nil && !info.IsDir() {
			logger.Info(context.Background(), "Removing existing file '{{|File|}}%s{{[-]}}' before folder can be created.", paths.GetStateDir())
			if err := os.Remove(paths.GetStateDir()); err != nil {
				logger.FatalWithStack(context.Background(), []string{
					"Failed to remove existing file.",
					"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
				}, paths.GetStateDir())
			}
		}
		if _, err := os.Stat(paths.GetStateDir()); os.IsNotExist(err) {
			logger.Info(context.Background(), "Creating folder '{{|Folder|}}%s{{[-]}}'.", paths.GetStateDir())
			if err := os.MkdirAll(paths.GetStateDir(), 0755); err != nil {
				logger.FatalWithStack(context.Background(), []string{
					"Failed to create folder.",
					"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
				}, paths.GetStateDir())
			}
		}
		if err := os.WriteFile(stateFile, sourceData, 0644); err != nil {
			return "", fmt.Errorf("failed to write active theme: %w", err)
		}
	}
	return stateFile, nil
}

// Load theme by name or URI. Returns theme-defined defaults if found.
// If prefix is provided, semantic tags are registered with that prefix (e.g. "Preview_Screen")
// without affecting the global active theme (Current).
func Load(themeNameOrURI string, prefix string) (*ThemeDefaults, error) {
	// 0. Clear previous registration for this namespace to avoid tag leakage
	Unload(prefix)

	// 1. Initialize with defaults
	Default(prefix)

	var data []byte

	if prefix == "" {
		// Active theme load — update state file, then read from it
		Current.Name = ThemeDisplayName(themeNameOrURI)
		statePath, err := EnsureThemeExtracted(themeNameOrURI)
		if err != nil {
			if themeNameOrURI != "DockSTARTer" {
				return Load("DockSTARTer", prefix)
			}
			return nil, err
		}
		data, err = os.ReadFile(statePath)
		if err != nil {
			return nil, err
		}
	} else {
		// Preview load — read directly from source, no state file writes
		var err error
		data, err = resolveThemeData(themeNameOrURI)
		if err != nil {
			return nil, err
		}
	}

	// Invalidate style cache for this prefix (covers both main and preview)
	if prefix == "" {
		ClearSemanticCache()
	} else {
		ClearSemanticCachePrefix(prefix)
	}

	defaults, err := parseThemeTOMLData(data, prefix)
	if err != nil {
		if themeNameOrURI != "DockSTARTer" {
			// For active theme loads, persist the fallback to config
			if prefix == "" {
				conf := config.LoadAppConfig()
				conf.UI.Theme = "DockSTARTer"
				_ = config.SaveAppConfig(conf)
				// Load default theme but still return an error so the caller knows the switch occurred
				deflts, _ := Load("DockSTARTer", "")
				return deflts, fmt.Errorf("theme parsing error: falling back to default")
			}
			return Load("DockSTARTer", prefix)
		}
		return nil, err
	}

	return defaults, nil
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
		console.ClearThemeMap()
		return
	}
	console.UnregisterPrefix(prefixTag(prefix, ""))
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
		// Strip file-specific (or global fallback) delimiters to get raw content
		inner := styleStr
		switch {
		case strings.HasPrefix(inner, dirPre) && strings.HasSuffix(inner, dirSuf):
			inner = inner[len(dirPre) : len(inner)-len(dirSuf)]
		case strings.HasPrefix(inner, semPre) && strings.HasSuffix(inner, semSuf):
			inner = inner[len(semPre) : len(inner)-len(semSuf)]
		default:
			// Already raw or uses global delimiters (e.g. result from ExpandTags)
			inner = console.StripDelimiters(inner)
		}
		// Bare "-" is the full-reset shorthand: treat it as resetting fg, bg, and all flags.
		// This makes {{[-]}}{{::B}} equivalent to {{[-:-:-B]}} in theme values.
		if inner == "-" {
			inner = "-:-:-"
		}

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

	// Iterate through the string seeking the next tag (semantic or direct)
	cur := raw
	for {
		// Find the nearest occurrence of either file-specific prefix
		nextSem := strings.Index(cur, semPre)
		nextDir := strings.Index(cur, dirPre)
		if nextSem == -1 && nextDir == -1 {
			break
		}

		// Determine which prefix comes first and select its matching suffix
		var start int
		var closeSuf string
		switch {
		case nextSem == -1:
			start, closeSuf = nextDir, dirSuf
		case nextDir == -1:
			start, closeSuf = nextSem, semSuf
		case nextDir < nextSem:
			start, closeSuf = nextDir, dirSuf
		default:
			start, closeSuf = nextSem, semSuf
		}

		end := strings.Index(cur[start:], closeSuf)
		if end == -1 {
			break
		}
		end += start + len(closeSuf)

		tag := cur[start:end]

		if strings.HasPrefix(tag, dirPre) {
			// Direct style tag - extract and merge
			mergeStyle(tag)
		} else if strings.HasPrefix(tag, semPre) {
			// Semantic reference - extract tag name (may include inline modifiers, e.g. "Title:::R")
			refKey := strings.TrimSuffix(strings.TrimPrefix(tag, semPre), semSuf)

			// Split off any inline modifiers after the semantic name
			semanticRef := refKey
			modifiers := ""
			if idx := strings.IndexByte(refKey, ':'); idx >= 0 {
				semanticRef = refKey[:idx]
				modifiers = refKey[idx+1:]
			}

			// 1. Try resolving as internal reference first (with or without '' prefix)
			targetKey := strings.TrimPrefix(semanticRef, "")
			if _, exists := rawValues[targetKey]; exists {
				resolvedRef, err := resolveThemeValue(rawValues[targetKey], rawValues, visiting,
					semPre, semSuf, dirPre, dirSuf)
				if err == nil {
					mergeStyle(resolvedRef)
					if modifiers != "" {
						mergeStyle(modifiers)
					}
					cur = cur[end:]
					continue
				}
			}

			// 2. Fallback to global semantic tags (e.g. Notice, Success).
			// Re-wrap in global standard delimiters so ExpandTags can resolve it
			// regardless of the file-specific delimiters in use.
			standardTag := console.SemanticPrefix + semanticRef + console.SemanticSuffix
			expanded := console.ExpandConsoleTags(standardTag)
			if expanded != standardTag && expanded != "" {
				mergeStyle(expanded)
			}
			if modifiers != "" {
				mergeStyle(modifiers)
			}
		}

		cur = cur[end:]
	}

	// Return RAW style string (no delimiters)
	return fmt.Sprintf("%s:%s:%s", finalFG, finalBG, finalFlags), nil
}

type ThemeDefaults struct {
	Borders           *bool   `toml:"borders"`
	ButtonBorders     *bool   `toml:"button_borders"`
	LineCharacters    *bool   `toml:"line_characters"`
	Shadow            *bool   `toml:"shadow"`
	ShadowLevel       *int    `toml:"shadow_level"`
	Scrollbar         *bool   `toml:"scrollbar"`
	BorderColor       *int    `toml:"border_color"`
	DialogTitleAlign  *string `toml:"dialog_title_align"`
	SubmenuTitleAlign *string `toml:"submenu_title_align"`
	PanelTitleAlign     *string `toml:"panel_title_align"`
	// Panel modes: themes may suggest "log" or "none" but never "console".
	// Any attempt to set "console" via a theme is silently clamped to "log".
	PanelLocal  *string `toml:"panel_local"`
	PanelRemote *string `toml:"panel_remote"`
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
	data, err := resolveThemeData(themeName)
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
	if defaults.ButtonBorders != nil {
		conf.UI.ButtonBorders = *defaults.ButtonBorders
		applied["Button Borders"] = fmt.Sprintf("%v", conf.UI.ButtonBorders)
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
	if defaults.DialogTitleAlign != nil {
		conf.UI.DialogTitleAlign = *defaults.DialogTitleAlign
		applied["Dialog Title Align"] = conf.UI.DialogTitleAlign
	}
	if defaults.SubmenuTitleAlign != nil {
		conf.UI.SubmenuTitleAlign = *defaults.SubmenuTitleAlign
		applied["Submenu Title Align"] = conf.UI.SubmenuTitleAlign
	}
	if defaults.PanelTitleAlign != nil {
		conf.UI.PanelTitleAlign = *defaults.PanelTitleAlign
		applied["Panel Title Align"] = conf.UI.PanelTitleAlign
	}
	// PanelLocal: no restrictions — local sessions may use any mode.
	if defaults.PanelLocal != nil {
		conf.UI.PanelLocal = *defaults.PanelLocal
		applied["Panel Local"] = conf.UI.PanelLocal
	}
	// PanelRemote: clamp "system" → "log" — themes must never grant full shell access to remote users.
	// "console" (ds2-only, ConsoleSafe-enforced) is permitted remotely.
	if defaults.PanelRemote != nil {
		v := *defaults.PanelRemote
		if strings.ToLower(v) == "system" {
			v = "log"
		}
		conf.UI.PanelRemote = v
		applied["Panel Remote"] = conf.UI.PanelRemote
	}
	return applied
}

func parseThemeTOMLData(data []byte, prefix string) (*ThemeDefaults, error) {
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
			styleValue = console.StripDelimiters(console.ExpandConsoleTags(raw))
		}

		// Register using raw value (no delimiters)
		console.RegisterThemeTagRaw(prefixTag(prefix, key), styleValue)

	}

	// 2. Re-apply tags if loading main theme
	if prefix == "" {
		console.RegisterBaseTags()
		console.BuildColorMap()
	}

	return tf.Defaults, nil
}

var (
	semanticStyleCache = make(map[string]lipgloss.Style)
	cacheMu            = new(sync.RWMutex)
)

// ThemeSemanticStyle translates a semantic tag or direct style code strictly using the theme registry.
func ThemeSemanticStyle(tag string) lipgloss.Style {
	return ThemeSemanticStyleWithPrefix(tag, "")
}

// ThemeSemanticStyleWithPrefix translates a semantic tag strictly using the theme registry with a prefix.
func ThemeSemanticStyleWithPrefix(tag string, prefix string) lipgloss.Style {
	return SemanticStyleWithRegistry(tag, prefix, false)
}

// ConsoleSemanticStyle translates a semantic color tag strictly using the console registry.
func ConsoleSemanticStyle(tag string) lipgloss.Style {
	return SemanticStyleWithRegistry(tag, "", true)
}

// SemanticStyleWithRegistry is the internal helper for translating tags.
func SemanticStyleWithRegistry(tag string, prefix string, useConsole bool) lipgloss.Style {
	registryKey := "theme"
	if useConsole {
		registryKey = "console"
	}
	cacheKey := "tag:" + registryKey + ":" + prefix + ":" + tag
	cacheMu.RLock()
	s, ok := semanticStyleCache[cacheKey]
	cacheMu.RUnlock()
	if ok {
		return s
	}

	var style lipgloss.Style
	if strings.HasPrefix(tag, console.SemanticPrefix) && strings.HasSuffix(tag, console.SemanticSuffix) {
		name := tag[len(console.SemanticPrefix) : len(tag)-len(console.SemanticSuffix)]
		style = SemanticRawStyleWithRegistry(name, prefix, useConsole)
	} else {
		var expanded string
		if useConsole {
			expanded = console.ExpandConsoleTags(tag)
		} else {
			expanded = console.ExpandThemeTags(tag, prefix)
		}
		style = ApplyTagsToStyle(expanded, lipgloss.NewStyle(), lipgloss.NewStyle())
	}

	cacheMu.Lock()
	semanticStyleCache[cacheKey] = style
	cacheMu.Unlock()
	return style
}

// ThemeSemanticRawStyle translates a raw semantic name strictly using the theme registry.
func ThemeSemanticRawStyle(name string) lipgloss.Style {
	return ThemeSemanticRawStyleWithPrefix(name, "")
}

// ConsoleSemanticRawStyle translates a raw semantic name strictly using the console registry.
func ConsoleSemanticRawStyle(name string) lipgloss.Style {
	return SemanticRawStyleWithRegistry(name, "", true)
}

// ThemeSemanticRawStyleWithPrefix translates a raw semantic name strictly using the theme registry with a prefix.
func ThemeSemanticRawStyleWithPrefix(name string, prefix string) lipgloss.Style {
	return SemanticRawStyleWithRegistry(name, prefix, false)
}

// SemanticRawStyleWithRegistry is the internal helper for translating raw names.
func SemanticRawStyleWithRegistry(name string, prefix string, useConsole bool) lipgloss.Style {
	registryKey := "theme"
	if useConsole {
		registryKey = "console"
	}
	cacheKey := "raw:" + registryKey + ":" + prefix + ":" + name
	cacheMu.RLock()
	if s, ok := semanticStyleCache[cacheKey]; ok {
		cacheMu.RUnlock()
		return s
	}
	cacheMu.RUnlock()

	var expanded string
	if useConsole {
		expanded = console.ExpandConsoleTags(console.WrapSemantic(name))
	} else {
		expanded = console.ExpandThemeTags(console.WrapSemantic(name), prefix)
	}
	s := ApplyTagsToStyle(expanded, lipgloss.NewStyle(), lipgloss.NewStyle())

	cacheMu.Lock()
	semanticStyleCache[cacheKey] = s
	cacheMu.Unlock()
	return s
}

// ClearSemanticCache clears the cached lipgloss styles.
// Call this after loading a new theme.
func ClearSemanticCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	semanticStyleCache = make(map[string]lipgloss.Style)
}

// ClearSemanticCachePrefix removes cached styles whose key contains the given prefix.
// Used to invalidate preview theme styles without discarding the active theme cache.
func ClearSemanticCachePrefix(prefix string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for k := range semanticStyleCache {
		if strings.Contains(k, prefix) {
			delete(semanticStyleCache, k)
		}
	}
}
// ToThemeANSI translates text with theme tags into ANSI escape sequences strictly using the theme registry.
func ToThemeANSI(text string) string {
	return console.ToThemeANSI(text)
}

// ToThemeANSIWithPrefix translates text with theme tags and a prefix into ANSI escape sequences.
func ToThemeANSIWithPrefix(text string, prefix string) string {
	return console.ToThemeANSIWithPrefix(text, prefix)
}

// ToConsoleANSI translates text with console tags into ANSI escape sequences strictly using the console registry.
func ToConsoleANSI(text string) string {
	return console.ToConsoleANSI(text)
}
