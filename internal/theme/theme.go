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

	semstyle "github.com/GhostWriters/semstyle/lg"
	semtheme "github.com/GhostWriters/semstyle/theme"

	"charm.land/lipgloss/v2"
	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
)

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
			logger.Info(context.Background(), "Removing existing file '"+console.FormatFilePath(paths.GetStateDir())+"' before folder can be created.")
			if err := os.Remove(paths.GetStateDir()); err != nil {
				logger.FatalWithStack(context.Background(), []string{
					"Failed to remove existing file.",
					"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
				}, paths.GetStateDir())
			}
		}
		if _, err := os.Stat(paths.GetStateDir()); os.IsNotExist(err) {
			logger.Info(context.Background(), "Creating folder '"+console.FormatFolderPath(paths.GetStateDir())+"'.")
			if err := os.MkdirAll(paths.GetStateDir(), 0700); err != nil {
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
				deflts, defaultErr := Load("DockSTARTer", "")
				if defaultErr != nil {
					return nil, fmt.Errorf("theme '%s' failed: %w; default theme also failed: %v", themeNameOrURI, err, defaultErr)
				}
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
	semstyle.BuildColorMap()
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
		semstyle.ClearThemeMap()
		return
	}
	semstyle.UnregisterPrefix(prefixTag(prefix, ""))
}

// Default initializes the Current configuration with standard DockSTARTer colors (Classic)
// If prefix is provided, semantic tags are registered with that prefix.
func Default(prefix string) {
	// Only update global Current if prefix is empty
	if prefix == "" {
		Current.Name = "DockSTARTer"
	}
}

// resolveThemeValue delegates to the semtheme parse layer; retained so existing callers
// and tests in this package keep their signature.
func resolveThemeValue(raw string, rawValues map[string]string, visiting map[string]bool,
	semPre, semSuf, dirPre, dirSuf string) (string, error) {
	return semtheme.ResolveValue(raw, rawValues, visiting, semPre, semSuf, dirPre, dirSuf)
}

// ThemeFile lives in the semtheme parse layer; aliased here so theme.* consumers are unchanged.
type ThemeFile = semtheme.ThemeFile

// ThemeDefaults holds the DockSTARTer2-specific UI defaults a theme may suggest under its
// [defaults] table. semtheme keeps that table opaque (app-defined); this struct is how DS2
// interprets it. Pointers distinguish "unset" from a zero value.
type ThemeDefaults struct {
	Borders            *bool   `mapstructure:"borders"`
	LargeButtons       *bool   `mapstructure:"large_buttons"`
	LargeTitleBars     *bool   `mapstructure:"large_title_bars"`
	LineCharacters     *bool   `mapstructure:"line_characters"`
	Shadow             *bool   `mapstructure:"shadow"`
	ShadowLevel        *int    `mapstructure:"shadow_level"`
	Scrollbar          *bool   `mapstructure:"scrollbar"`
	Spinner            *bool   `mapstructure:"spinner"`
	MenuBrackets       *bool   `mapstructure:"menu_brackets"`
	LineNumberBrackets *bool   `mapstructure:"line_number_brackets"`
	CheckboxBrackets   *string `mapstructure:"checkbox_brackets"`
	RadioBrackets      *string `mapstructure:"radio_brackets"`
	BorderColor        *int    `mapstructure:"border_color"`
	DialogTitleAlign   *string `mapstructure:"dialog_title_align"`
	SubmenuTitleAlign  *string `mapstructure:"submenu_title_align"`
	PanelTitleAlign    *string `mapstructure:"panel_title_align"`
	TabLayout          *string `mapstructure:"tab_layout"`
}

// decodeThemeDefaults converts the opaque [defaults] table from a parsed theme into DS2's
// typed ThemeDefaults. Returns nil when there is no defaults table.
func decodeThemeDefaults(raw map[string]any) (*ThemeDefaults, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var d ThemeDefaults
	if err := mapstructure.Decode(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// FileDefaults decodes a parsed theme's opaque [defaults] table into DS2's typed
// ThemeDefaults. Returns nil if the theme has no defaults.
func FileDefaults(tf ThemeFile) (*ThemeDefaults, error) {
	return decodeThemeDefaults(tf.Defaults)
}

// GetThemeFile reads a theme file and returns its structured content without applying it.
// resolveThemeColors resolves all color values in tf, returning the resolved map or an error
// (e.g. on circular reference). The returned map is keyed the same as tf.Styles.
func resolveThemeColors(tf ThemeFile) (map[string]string, error) {
	return semtheme.ResolveColors(tf)
}

// defaultPalette holds fallback values for $palette variables a theme
// references but doesn't define itself. Unlike style-tag fallbacks (see
// registerTagFallbacks), a palette entry can't fall back to "whatever
// another theme picked" -- there's no other theme to inherit a color
// choice from -- so this is a literal baseline rather than a reference
// chain. white/black covers the common case of a theme using $primary/
// $secondary purely as light/dark text-on-background roles without
// needing to state the obvious.
var defaultPalette = map[string]string{
	"primary":   "white",
	"secondary": "black",
}

// applyPaletteDefaults fills in any defaultPalette entries tf.Palette
// doesn't already define, without overwriting the theme's own choices.
func applyPaletteDefaults(tf *ThemeFile) {
	if tf.Palette == nil {
		tf.Palette = make(map[string]string, len(defaultPalette))
	}
	for name, value := range defaultPalette {
		if _, ok := tf.Palette[name]; !ok {
			tf.Palette[name] = value
		}
	}
}

func GetThemeFile(themeName string) (ThemeFile, error) {
	data, err := resolveThemeData(themeName)
	if err != nil {
		return ThemeFile{}, err
	}
	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return ThemeFile{}, err
	}
	applyPaletteDefaults(&tf)
	if _, err := resolveThemeColors(tf); err != nil {
		return tf, err
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
	if defaults.LargeButtons != nil {
		conf.UI.LargeButtons = *defaults.LargeButtons
		applied["Large Buttons"] = fmt.Sprintf("%v", conf.UI.LargeButtons)
	}
	if defaults.LargeTitleBars != nil {
		conf.UI.LargeTitleBars = *defaults.LargeTitleBars
		applied["Large Title Bars"] = fmt.Sprintf("%v", conf.UI.LargeTitleBars)
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
	if defaults.Spinner != nil {
		conf.UI.Spinner = *defaults.Spinner
		applied["Spinner"] = fmt.Sprintf("%v", conf.UI.Spinner)
	}
	if defaults.MenuBrackets != nil {
		conf.UI.MenuBrackets = *defaults.MenuBrackets
		applied["Menu Brackets"] = fmt.Sprintf("%v", conf.UI.MenuBrackets)
	}
	if defaults.LineNumberBrackets != nil {
		conf.UI.LineNumberBrackets = *defaults.LineNumberBrackets
		applied["Line Number Brackets"] = fmt.Sprintf("%v", conf.UI.LineNumberBrackets)
	}
	if defaults.CheckboxBrackets != nil {
		conf.UI.CheckboxBrackets = *defaults.CheckboxBrackets
		applied["Checkbox Brackets"] = conf.UI.CheckboxBrackets
	}
	if defaults.RadioBrackets != nil {
		conf.UI.RadioBrackets = *defaults.RadioBrackets
		applied["Radio Brackets"] = conf.UI.RadioBrackets
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
	if defaults.TabLayout != nil {
		conf.UI.TabLayout = *defaults.TabLayout
		applied["Tab Layout"] = conf.UI.TabLayout
	}
	return applied
}

func init() {
	config.ThemeDefaultsOverlayHook = applyMigrationThemeDefaults
}

// applyMigrationThemeDefaults backfills the theme's suggested defaults for
// fields not in legacyPresent, and resets an unrecognized DS1 theme name to
// the default theme.
func applyMigrationThemeDefaults(conf *config.AppConfig, legacyPresent map[string]bool) {
	if legacyPresent["Theme"] && !isBuiltInTheme(conf.UI.Theme) {
		conf.UI.Theme = "DockSTARTer"
	}
	tf, err := GetThemeFile(conf.UI.Theme)
	if err != nil {
		return
	}
	defaults, err := FileDefaults(tf)
	if err != nil || defaults == nil {
		return
	}
	filtered := *defaults
	if legacyPresent["Borders"] {
		filtered.Borders = nil
	}
	if legacyPresent["LargeButtons"] {
		filtered.LargeButtons = nil
	}
	if legacyPresent["LargeTitleBars"] {
		filtered.LargeTitleBars = nil
	}
	if legacyPresent["LineCharacters"] {
		filtered.LineCharacters = nil
	}
	if legacyPresent["Shadow"] {
		filtered.Shadow = nil
	}
	if legacyPresent["ShadowLevel"] {
		filtered.ShadowLevel = nil
	}
	if legacyPresent["Scrollbar"] {
		filtered.Scrollbar = nil
	}
	if legacyPresent["Spinner"] {
		filtered.Spinner = nil
	}
	if legacyPresent["MenuBrackets"] {
		filtered.MenuBrackets = nil
	}
	if legacyPresent["LineNumberBrackets"] {
		filtered.LineNumberBrackets = nil
	}
	if legacyPresent["CheckboxBrackets"] {
		filtered.CheckboxBrackets = nil
	}
	if legacyPresent["RadioBrackets"] {
		filtered.RadioBrackets = nil
	}
	if legacyPresent["BorderColor"] {
		filtered.BorderColor = nil
	}
	if legacyPresent["DialogTitleAlign"] {
		filtered.DialogTitleAlign = nil
	}
	if legacyPresent["SubmenuTitleAlign"] {
		filtered.SubmenuTitleAlign = nil
	}
	if legacyPresent["PanelTitleAlign"] {
		filtered.PanelTitleAlign = nil
	}
	if legacyPresent["TabLayout"] {
		filtered.TabLayout = nil
	}
	ApplyThemeDefaults(conf, filtered)
}

// isBuiltInTheme reports whether name matches one of DS2's embedded themes.
func isBuiltInTheme(name string) bool {
	if EmbeddedThemeLister == nil {
		return true // can't verify; don't reset a theme we can't check
	}
	names, err := EmbeddedThemeLister()
	if err != nil {
		return true
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func parseThemeTOMLData(data []byte, prefix string) (*ThemeDefaults, error) {
	// Fallback rules must exist before RegisterInto resolves the theme's raw
	// values, since a tag referenced inline (e.g. "{{|PanelBorder|}}") that
	// the theme itself omits is resolved against these rules at parse time
	// -- registering them after RegisterInto would leave every reference in
	// the very first theme load unresolved.
	if prefix == "" {
		registerTagFallbacksOnce.Do(registerTagFallbacks)
	}
	rawDefaults, err := registerThemeData(data, prefix)
	if err != nil {
		return nil, err
	}
	return decodeThemeDefaults(rawDefaults)
}

// registerThemeData mirrors semtheme.RegisterInto (parse -> resolve ->
// register into the semstyle tag registry) but is reimplemented locally so
// applyPaletteDefaults can run between parsing and resolving -- RegisterInto
// does its own independent parse internally and has no hook point for this.
// This is the actual live-rendering path (parseThemeTOMLData's only caller,
// for both the main theme and the Appearance Settings preview's prefixed
// namespace); GetThemeFile's own applyPaletteDefaults call is a separate
// code path (theme inspection/migration) that doesn't cover it.
func registerThemeData(data []byte, prefix string) (map[string]any, error) {
	tf, err := semtheme.Parse(data)
	if err != nil {
		return nil, err
	}
	applyPaletteDefaults(&tf)
	resolved, err := semtheme.ResolveColors(tf)
	if err != nil {
		return nil, err
	}
	for key, styleValue := range resolved {
		semstyle.RegisterThemeTagRaw(semtheme.PrefixTag(prefix, key), styleValue)
	}
	if prefix == "" {
		semstyle.BuildColorMap()
	}
	return tf.Defaults, nil
}

// registerTagFallbacksOnce guards registerTagFallbacks so it runs exactly
// once per process, not on every theme load -- see registerTagFallbacks'
// doc comment for why.
var registerTagFallbacksOnce sync.Once

// registerTagFallbacks declares every optional-tag fallback rule DS2 defines
// via semstyle's RegisterFallback, sourced from the embedded
// .FALLBACKS.ds2theme file's [styles] table -- a real theme file (dot-
// prefixed and excluded from ListThemes the same way .TEMPLATE.ds2theme
// is), so changing a default is a one-file edit, no code changes. These
// are structural relationships in DS2's own tag-naming scheme (e.g. "a
// Radio tag falls back to its Checkbox equivalent") that hold regardless
// of which theme is loaded, not per-theme state, so they're registered
// once rather than re-registered on every theme load/switch --
// semstyle.ClearThemeMap (called by Unload before every load, including
// the first) deliberately leaves fallback rules untouched for exactly
// this reason. Resolution itself is handled entirely by semstyle --
// lazily, at the point something actually resolves the tag
// (GetRawTagCode, GetColorDefinition, or inline "{{|name|}}" text
// expansion) -- so registration order (both among these calls, and
// within .FALLBACKS.ds2theme itself) doesn't matter, and every existing
// (and future) call site referencing e.g. "TitleWarn" or
// "IconMaximizeFocused" by name gets the fallback for free with no
// per-call-site resolution code needed anywhere.
//
// Also disables semstyle's automatic theme-to-console fallback tier: with
// it left on, a theme tag the theme author simply forgot to define would
// silently render using whatever unrelated console/CLI-log color happens
// to share that name, rather than looking wrong in an obvious way. Every
// tag DS2's TUI actually references either has a real per-theme value or
// an explicit entry in .FALLBACKS.ds2theme; none of them rely on the
// automatic console tier (verified: no console-only AppColors entry is
// referenced anywhere under internal/tui or internal/displayengine).
func registerTagFallbacks() {
	semstyle.SetAutoConsoleFallback(false)
	if EmbeddedThemeReader == nil {
		logger.FatalWithStack(context.Background(), []string{"Embedded theme reader not initialised."})
	}
	data, err := EmbeddedThemeReader(".FALLBACKS")
	if err != nil {
		logger.FatalWithStack(context.Background(), []string{"Failed to load .FALLBACKS.ds2theme.", "%v"}, err)
	}
	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		logger.FatalWithStack(context.Background(), []string{"Failed to parse .FALLBACKS.ds2theme.", "%v"}, err)
	}
	// Deliberately just toml.Unmarshal, not GetThemeFile/resolveThemeColors:
	// resolving tf.Styles' own {{|Name|}} references now would need the
	// fallback map that doesn't exist until this function returns --
	// RegisterFallback resolves candidates lazily at usage time instead.
	for name, fallback := range tf.Styles {
		semstyle.RegisterFallback(name, true, fallback)
	}
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
	if strings.HasPrefix(tag, semstyle.SemanticPrefix) && strings.HasSuffix(tag, semstyle.SemanticSuffix) {
		name := tag[len(semstyle.SemanticPrefix) : len(tag)-len(semstyle.SemanticSuffix)]
		style = SemanticRawStyleWithRegistry(name, prefix, useConsole)
	} else {
		style = semstyle.ToStyle(semstyle.Default, tag, lipgloss.NewStyle(), lipgloss.NewStyle())
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

	var s lipgloss.Style
	if prefix != "" && !useConsole {
		// Prefix-scoped lookup (e.g. "Preview_"): resolves name's fallback
		// rule (if it has no explicit value under this prefix) within the
		// prefix's own namespace instead of the bare/global one.
		raw := semstyle.GetRawTagCodeWithPrefix(name, prefix)
		s = semstyle.CodeToStyle(raw, lipgloss.NewStyle(), lipgloss.NewStyle())
	} else {
		s = semstyle.ToStyle(semstyle.Default, semstyle.WrapSemantic(name), lipgloss.NewStyle(), lipgloss.NewStyle())
	}

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

// ToANSI converts semantic and direct tags to ANSI. Without a prefix uses the console
// map; with "" uses the theme map (theme-first, console fallback); with a named prefix
// qualifies theme map lookups. Delegates to semstyle.ToANSI.
func ToANSI(text string, prefix ...string) string {
	return semstyle.ToANSI(text, prefix...)
}

// ToTags expands semantic tags to direct tags without converting to ANSI.
func ToTags(text string, prefix ...string) string {
	return semstyle.ToTags(text, prefix...)
}

// ToPlain removes all tags and ANSI sequences, returning plain text.
func ToPlain(text string) string {
	return semstyle.ToPlain(text)
}

// StripTags removes semantic and direct tags, leaving ANSI sequences intact.
func StripTags(text string) string {
	return semstyle.StripTags(text)
}
