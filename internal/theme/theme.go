package theme

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	semstyle "github.com/GhostWriters/semstyle/lg"
	semtheme "github.com/GhostWriters/semstyle/theme"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	Borders           *bool   `mapstructure:"borders"`
	LargeButtons      *bool   `mapstructure:"large_buttons"`
	LargeTitleBars    *bool   `mapstructure:"large_title_bars"`
	LineCharacters    *bool   `mapstructure:"line_characters"`
	Shadow            *bool   `mapstructure:"shadow"`
	ShadowLevel       *int    `mapstructure:"shadow_level"`
	Scrollbar         *bool   `mapstructure:"scrollbar"`
	Spinner           *bool   `mapstructure:"spinner"`
	MenuBrackets      *bool   `mapstructure:"menu_brackets"`
	BorderColor       *int    `mapstructure:"border_color"`
	DialogTitleAlign  *string `mapstructure:"dialog_title_align"`
	SubmenuTitleAlign *string `mapstructure:"submenu_title_align"`
	PanelTitleAlign   *string `mapstructure:"panel_title_align"`
	PanelLocal        *string `mapstructure:"panel_local"`
	PanelRemote       *string `mapstructure:"panel_remote"`
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

func GetThemeFile(themeName string) (ThemeFile, error) {
	data, err := resolveThemeData(themeName)
	if err != nil {
		return ThemeFile{}, err
	}
	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return ThemeFile{}, err
	}
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
	if legacyPresent["PanelLocal"] {
		filtered.PanelLocal = nil
	}
	if legacyPresent["PanelRemote"] {
		filtered.PanelRemote = nil
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
	rawDefaults, err := semtheme.RegisterInto(data, prefix)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		registerTagFallbacksOnce.Do(registerTagFallbacks)
	}
	return decodeThemeDefaults(rawDefaults)
}

// registerTagFallbacksOnce guards registerTagFallbacks so it runs exactly
// once per process, not on every theme load -- see registerTagFallbacks'
// doc comment for why.
var registerTagFallbacksOnce sync.Once

// registerTagFallbacks declares every optional-tag fallback rule DS2 defines
// via semstyle's RegisterFallback. These are structural relationships in
// DS2's own tag-naming scheme (e.g. "a Radio tag falls back to its Checkbox
// equivalent") that hold regardless of which theme is loaded, not per-theme
// state, so they're registered once rather than re-registered on every
// theme load/switch -- semstyle.ClearThemeMap (called by Unload before every
// load, including the first) deliberately leaves fallback rules untouched
// for exactly this reason. Resolution itself is handled entirely by
// semstyle -- lazily, at the point something actually resolves the tag
// (GetRawTagCode, GetColorDefinition, or inline "{{|name|}}" text expansion)
// -- so registration order among these calls doesn't matter, and every
// existing (and future) call site referencing e.g. "TitleWarn" or
// "IconMaximizeFocused" by name gets the fallback for free with no
// per-call-site resolution code needed anywhere.
//
// Also disables semstyle's automatic theme-to-console fallback tier: with
// it left on, a theme tag the theme author simply forgot to define would
// silently render using whatever unrelated console/CLI-log color happens
// to share that name, rather than looking wrong in an obvious way. Every
// tag DS2's TUI actually references either has a real per-theme value or
// an explicit RegisterFallback rule above; none of them rely on the
// automatic console tier (verified: no console-only AppColors entry is
// referenced anywhere under internal/tui or internal/displayengine).
func registerTagFallbacks() {
	semstyle.SetAutoConsoleFallback(false)
	for _, tf := range titleFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range itemListFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range iconFallbackTags() {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range checkboxFallbackTags() {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range highlightFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range statusFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range helpFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
	for _, tf := range markerFallbackTags {
		semstyle.RegisterFallback(tf.name, true, tf.fallback)
	}
}

// markerFallbackTags lists the button lock-marker tags that fall back to
// another tag when the active theme doesn't define them. ButtonLockedMarker
// falls back to the generic MarkerLocked (also used for the session-active
// input prompt and menu lock markers); LargeButtonLockedMarker falls back
// to ButtonLockedMarker, so a theme need only override MarkerLocked to
// affect every locked marker at once, or override ButtonLockedMarker/
// LargeButtonLockedMarker individually for button-specific styling.
var markerFallbackTags = []struct{ name, fallback string }{
	{"ButtonLockedMarker", "MarkerLocked"},
	{"LargeButtonLockedMarker", "ButtonLockedMarker"},
}

// helpFallbackTags lists the help-bar tags that fall back to another tag
// when the active theme doesn't define them. HelpTag falls back to Tag,
// matching 12 of 13 bundled themes (DockSTARTer itself uses a distinct
// black-on-red highlight, so it keeps its own explicit definition).
// HelpItem falls back to Dialog, DockSTARTer's own pattern -- its prior
// "{{[black]}}" override only set the foreground, which Dialog already
// provides, so it was removed in favor of the fallback.
var helpFallbackTags = []struct{ name, fallback string }{
	{"HelpTag", "Tag"},
	{"HelpItem", "Dialog"},
}

// statusFallbackTags lists optional header-bar (Status*) tags that fall
// back to another tag when the active theme doesn't define them, anchored
// to DockSTARTer.ds2theme's own pattern as the canonical default -- other
// bundled themes that deviate (an added :::B/:::R modifier, a different
// target tag, or a fully custom direct color) keep their own explicit
// definition as an override, whether or not they happen to coincide with
// each other. StatusFlagsBrackets is deliberately excluded: DockSTARTer
// itself uses a fully custom direct color there, not a tag reference, so
// there's no canonical pattern to anchor a fallback rule to.
//
// StatusVersionSpace falls back to a literal reset rather than a named
// tag: header_render.go's only use of StatusVersionSpace immediately
// re-establishes "{{|StatusBar|}}" right after it, and it's never rendered
// anywhere else, so a plain reset there is functionally equivalent to
// StatusBar in the one context it's ever used -- but using the literal
// "{{[-]}}" (DockSTARTer's own exact authored value) instead of naming
// StatusBar keeps that equivalence exact even if StatusBar's own
// definition ever changes.
var statusFallbackTags = []struct{ name, fallback string }{
	{"StatusBarBorder", "StatusBar"},
	{"StatusVersion", "StatusFields"},
	{"StatusVersionBrackets", "StatusFields"},
	{"StatusHostname", "StatusFields"},
	{"StatusUpdateMarker", "StatusUpdate"},
	{"StatusBarFocused", "TagFocused"},
	{"StatusName", "StatusFields"},
	{"StatusFlags", "Dialog"},
	{"StatusVersionSpace", "{{[-]}}"},
	{"StatusFlagsSpace", "StatusVersionSpace"},
	{"StatusUpdateBrackets", "StatusVersionBrackets"},
	// Helpline/PanelBorder are conceptually part of the same backdrop
	// grouping as StatusBar/StatusBarBorder, hence living in this list
	// despite the name. PanelBorder = Helpline is identical in all 13
	// bundled themes; Helpline itself varies (most reference Dialog, some
	// a fully custom color), but StatusBar and Dialog resolve to the same
	// value in most of those themes anyway, so StatusBar is the more
	// direct backdrop-family anchor for a theme that wants to skip
	// defining Helpline separately.
	{"Helpline", "StatusBar"},
	{"PanelBorder", "Helpline"},
}

// highlightFallbackTags lists the inline-text-highlight tags that fall back
// to the generic Highlight tag when the active theme doesn't define them --
// every bundled theme but RetroDockSTARTer (which points Version at its own
// StatusVersion instead) already just aliases each of these directly to
// Highlight. FailingCommand/Yes/No are deliberately excluded: every bundled
// theme points them at TitleError/TitleSuccess instead, never at Highlight,
// so they're a different (unrelated) fallback relationship, not this one.
// Safe now that automatic console fallback is disabled (registerTagFallbacks'
// doc comment) -- previously, removing a theme's redundant "= Highlight"
// line for these exact names surfaced an unrelated console-map default
// instead of this fallback, since console-map checks took priority over
// this rule.
var highlightFallbackTags = []struct{ name, fallback string }{
	{"ApplicationName", "Highlight"},
	{"RunningCommand", "Highlight"},
	{"UserCommand", "Highlight"},
	{"Branch", "Highlight"},
	{"Version", "Highlight"},
	{"MenuPage", "Highlight"},
	{"File", "Highlight"},
	{"Folder", "Highlight"},
	{"App", "Highlight"},
	{"IPAddress", "Highlight"},
	{"Var", "Highlight"},
	{"User", "Highlight"},
	{"Theme", "Highlight"},
}

// titleFallbackTags lists optional Title-variant tags that fall back to
// another tag when the active theme doesn't define them.
//
// Large* variants fall back to their own small tag (LargeTitleWarn ->
// TitleWarn), not to the generic LargeTitle -- matching what every bundled
// theme already does explicitly for these (LargeTitleWarn =
// "{{|TitleWarn|}}", not "{{|LargeTitle|}}"). LargeTitleHelp is the one
// exception left out here: most bundled themes point it at the generic
// LargeTitle instead of TitleHelp, and the two don't necessarily resolve to
// the same color, so it keeps requiring an explicit per-theme definition
// rather than getting an assumed fallback.
var titleFallbackTags = []struct{ name, fallback string }{
	{"TitleHelp", "Title"},
	{"TitleNotice", "Title"},
	{"TitleWarn", "Title"},
	{"TitleWarning", "Title"},
	{"TitleError", "Title"},
	{"TitleSuccess", "Title"},
	{"TitleQuestion", "Title"},
	{"TitleSubMenu", "Title"},
	{"TitleFocused", "Title"},
	{"TitleSubMenuFocused", "TitleFocused"},
	// *Focused variants of each title state -- unused by any bundled theme
	// or call site today (only SubMenu currently has a distinct focused
	// look), but a theme designer may want a dialog type's title to look
	// different while focused, so these are wired up ready for that even
	// though nothing queries them yet.
	{"TitleHelpFocused", "TitleFocused"},
	{"TitleNoticeFocused", "TitleFocused"},
	{"TitleWarnFocused", "TitleFocused"},
	{"TitleWarningFocused", "TitleFocused"},
	{"TitleErrorFocused", "TitleFocused"},
	{"TitleSuccessFocused", "TitleFocused"},
	{"TitleQuestionFocused", "TitleFocused"},
	{"LargeTitleNotice", "TitleNotice"},
	{"LargeTitleWarn", "TitleWarn"},
	{"LargeTitleWarning", "TitleWarning"},
	{"LargeTitleError", "TitleError"},
	{"LargeTitleSuccess", "TitleSuccess"},
	{"LargeTitleQuestion", "TitleQuestion"},
	{"LargeTitleSubMenu", "TitleSubMenu"},
	{"LargeTitleSubMenuFocused", "TitleSubMenuFocused"},
	{"LargeTitleHelpFocused", "TitleHelpFocused"},
	{"LargeTitleNoticeFocused", "TitleNoticeFocused"},
	{"LargeTitleWarnFocused", "TitleWarnFocused"},
	{"LargeTitleWarningFocused", "TitleWarningFocused"},
	{"LargeTitleErrorFocused", "TitleErrorFocused"},
	{"LargeTitleSuccessFocused", "TitleSuccessFocused"},
	{"LargeTitleQuestionFocused", "TitleQuestionFocused"},
	// LargeTitleArea variants -- distinct from the LargeTitle family above
	// (background-area coloring for the large-titlebar row, not the title
	// text itself); 12 of 13 bundled themes just alias each of these to the
	// base LargeTitleArea, DockSTARTer being the one exception with a real
	// per-state color for each.
	{"LargeTitleAreaHelp", "LargeTitleArea"},
	{"LargeTitleAreaNotice", "LargeTitleArea"},
	{"LargeTitleAreaWarn", "LargeTitleArea"},
	{"LargeTitleAreaWarning", "LargeTitleArea"},
	{"LargeTitleAreaError", "LargeTitleArea"},
	{"LargeTitleAreaSuccess", "LargeTitleArea"},
	{"LargeTitleAreaQuestion", "LargeTitleArea"},
	{"LargeTitleFocusIndicator", "TitleFocusIndicator"},
	{"LargeTitleUnfocusedIndicator", "TitleUnfocusedIndicator"},
}

// itemListFallbackTags lists optional ItemList-variant tags that fall back
// to another tag when the active theme doesn't define them, same idea as
// titleFallbackTags. The bare ItemList tag itself is deliberately not
// included: 11 of 13 bundled themes set it to "" (a deliberate "render
// unstyled" choice, not an oversight), and GetRawTagCode can't distinguish
// an explicit "" from "not defined" -- adding a fallback for it would
// silently reintroduce an Item-derived color into every one of those
// themes. ItemListUserDefinedFocused/ItemListDeprecatedFocused already
// vary legitimately per theme (some derive from their own base tag with
// Bold instead of the generic ItemFocused), so this only supplies a
// fallback for when a theme skips them entirely, never overriding an
// existing per-theme definition.
var itemListFallbackTags = []struct{ name, fallback string }{
	{"ItemListFocused", "ItemFocused"},
	{"ItemListUserDefinedFocused", "ItemFocused"},
	{"ItemListDeprecated", "ItemListUserDefined"},
	{"ItemListDeprecatedFocused", "ItemListUserDefinedFocused"},
}

// iconWidgets is every title-bar/pane-layout widget with its own optional
// Icon<Name><State>/LargeIcon<Name><State> tag family -- Help/Refresh/Exit
// (dialog_titlebar.go's WidgetDef), ResizeUp/ResizeDn (panel resize
// widgets), and Maximize/SideBySide/Stacked (the tabbed vars editor's pane
// layout widgets).
var iconWidgets = []string{"Help", "Refresh", "Exit", "ResizeUp", "ResizeDn", "Maximize", "SideBySide", "Stacked"}

// iconFallbackTags returns, for every widget in iconWidgets, its Inactive/
// Focused/Pressed and LargeIcon* fallback rules: IconWidgetFocused/Pressed
// fall back to the generic IconFocused/IconPressed; IconWidgetInactive falls
// back to the generic IconInactive; LargeIconWidgetFocused/Pressed fall back
// to the generic LargeIconFocused/LargeIconPressed; LargeIconWidgetInactive
// falls back to the small IconWidgetInactive tag (itself already
// fallback-resolved) rather than a generic LargeIconInactive, since none
// exists -- matching what every bundled theme already does explicitly for
// the pre-existing widgets (LargeIconExitInactive would otherwise need its
// own generic large counterpart that was never defined).
func iconFallbackTags() []struct{ name, fallback string } {
	var tags []struct{ name, fallback string }
	for _, w := range iconWidgets {
		tags = append(tags,
			struct{ name, fallback string }{"Icon" + w + "Inactive", "IconInactive"},
			struct{ name, fallback string }{"Icon" + w + "Focused", "IconFocused"},
			struct{ name, fallback string }{"Icon" + w + "Pressed", "IconPressed"},
			struct{ name, fallback string }{"LargeIcon" + w + "Inactive", "Icon" + w + "Inactive"},
			struct{ name, fallback string }{"LargeIcon" + w + "Focused", "LargeIconFocused"},
			struct{ name, fallback string }{"LargeIcon" + w + "Pressed", "LargeIconPressed"},
		)
	}
	// panel_render.go renders these via RenderThemeText(iconStr, baseStyle)
	// where baseStyle derives from PanelBorder, so a bare reset resolves
	// equivalently to every bundled theme's explicit PanelBorder override.
	// A literal reset is used rather than the named tag so this stays
	// correct if these icon tags are ever reused somewhere with a different
	// base style (e.g. a dialog), where PanelBorder specifically wouldn't
	// apply but a reset to the surrounding context still would.
	tags = append(tags,
		struct{ name, fallback string }{"IconResizeUpInactive", "{{[-]}}"},
		struct{ name, fallback string }{"IconResizeDnInactive", "{{[-]}}"},
	)
	return tags
}

// checkboxBaseFallbackTags lists the base (unfocused) Checkbox tags'
// fallback targets, anchored to DockSTARTer's own pattern: CheckboxOff/
// CheckboxBracketsOff/CheckboxBracketsOn are identical in all 13 bundled
// themes; CheckboxOn matches in 12 of 13 (ZenFocus customizes it).
var checkboxBaseFallbackTags = []struct{ name, fallback string }{
	{"CheckboxOff", "Dialog"},
	{"CheckboxOn", "TagKey"},
	{"CheckboxBracketsOff", "Dialog"},
	{"CheckboxBracketsOn", "Dialog"},
}

// checkboxFallbackTags returns checkboxBaseFallbackTags plus the generated
// Radio*->Checkbox* and Checkbox*Focused/Radio*Focused->TagFocused fallback
// rules matching checkboxStylePair (internal/displayengine/classic/
// menu_render.go)'s tag-name construction: Checkbox|Radio + Brackets? +
// On|Off + Focused?. Every bundled theme already either duplicates or
// explicitly aliases Radio's tags to Checkbox's, and Checkbox*Focused to
// the generic TagFocused.
func checkboxFallbackTags() []struct{ name, fallback string } {
	tags := append([]struct{ name, fallback string }{}, checkboxBaseFallbackTags...)
	for _, brackets := range []string{"", "Brackets"} {
		for _, state := range []string{"On", "Off"} {
			for _, suffix := range []string{"", "Focused"} {
				checkboxTag := "Checkbox" + brackets + state + suffix
				radioTag := "Radio" + brackets + state + suffix
				tags = append(tags, struct{ name, fallback string }{radioTag, checkboxTag})
				if suffix == "Focused" {
					tags = append(tags, struct{ name, fallback string }{checkboxTag, "TagFocused"})
				}
			}
		}
	}
	return tags
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

	s := semstyle.ToStyle(semstyle.Default, semstyle.WrapSemantic(name), lipgloss.NewStyle(), lipgloss.NewStyle())

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
