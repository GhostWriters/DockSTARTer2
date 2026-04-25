package config

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"

	"github.com/adrg/xdg"
	"github.com/go-viper/mapstructure/v2"
	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/ini.v1"
)

//go:embed defaults/dockstarter2.toml
var defaultsToml []byte

// AppConfig holds the application configuration settings.
type AppConfig struct {
	UI     UIConfig     `toml:"ui"`
	Paths  PathConfig   `toml:"paths"`
	Server ServerConfig `toml:"server"`

	// These are helper fields for runtime use, not saved to TOML
	Arch       string     `toml:"-"`
	ConfigDir  string     `toml:"-"`
	ComposeDir string     `toml:"-"`
	RawPaths   PathConfig `toml:"-"` // Unexpanded values as read from TOML
}

// ServerConfig holds SSH and web server settings.
// The server is active when ssh.port > 0. Set ssh.port = 0 (or omit it) to
// disable. There is no separate enabled flag — the port is the intent signal.
type ServerConfig struct {
	SSH     SSHConfig  `toml:"ssh"`
	Web     WebConfig  `toml:"web"`
	Auth    AuthConfig `toml:"auth"`
	HostKey string     `toml:"host_key"` // Path to persistent host key file
}

// SSHConfig holds settings for the SSH server.
type SSHConfig struct {
	Port int `toml:"port"` // TCP port for the SSH server (0 = disabled)
}

// WebConfig holds settings for the optional xterm.js web frontend.
type WebConfig struct {
	Port int `toml:"port"` // TCP port for the HTTP/WebSocket server (0 = disabled)
}

// AuthConfig holds authentication settings for the SSH server.
type AuthConfig struct {
	// Mode: "password", "pubkey", or "none" (none prints a warning on startup)
	Mode         string `toml:"mode"`
	Password     string `toml:"password"`       // bcrypt hash of the password
	AuthKeysFile string `toml:"auth_keys_file"` // Path to authorized_keys file
}

// UIConfig holds user interface related settings.
type UIConfig struct {
	Theme             string `toml:"theme"`
	Borders           bool   `toml:"borders"`
	ButtonBorders     bool   `toml:"button_borders"`
	LineCharacters    bool   `toml:"line_characters"`
	Shadow            bool   `toml:"shadow"`
	ShadowLevel       int    `toml:"shadow_level"` // 0=off, 1=light(░), 2=medium(▒), 3=dark(▓), 4=solid(█)
	Scrollbar         bool   `toml:"scrollbar"`
	BorderColor       int    `toml:"border_color"`        // 1=Border, 2=Border2, 3=Both
	DialogTitleAlign  string `toml:"dialog_title_align"`  // "center" or "left"
	SubmenuTitleAlign string `toml:"submenu_title_align"` // "center" or "left"
	PanelTitleAlign   string `toml:"panel_title_align"`   // "center" or "left"
	PanelLocal        string `toml:"panel_local"`         // "log", "console", or "none" (for local sessions)
	PanelRemote       string `toml:"panel_remote"`        // "log", "console", or "none" (for ssh/web sessions)
}

// PathConfig holds directory path settings.
type PathConfig struct {
	ConfigFolder  string `toml:"config_folder"`
	ComposeFolder string `toml:"compose_folder"`
}

// getArch returns the CPU architecture (x86_64 or aarch64).
func getArch() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

// ExpandVariables expands environment variables in the config values.
// It supports:
// - ${XDG_CONFIG_HOME} -> xdg.ConfigHome
// - ${XDG_DATA_HOME}   -> xdg.DataHome
// - ${XDG_STATE_HOME}  -> xdg.StateHome
// - ${XDG_CACHE_HOME}  -> xdg.CacheHome
// - ${HOME}            -> os.UserHomeDir()
// - ${USER}            -> Current username
// - ~/...              -> home-relative path (tilde expansion)
// - ${ANY}             -> os.Getenv("ANY") fallback for unrecognised variables
func ExpandVariables(val string) string {
	// Support legacy ${VAR?} syntax by normalizing it to ${VAR}
	// Use regex to be precise: find ${...?} and replace with ${...}
	re := regexp.MustCompile(`\$\{([^}]+)\?\}`)
	val = re.ReplaceAllString(val, `${$1}`)

	// Tilde expansion: ~/... or bare ~
	if val == "~" || strings.HasPrefix(val, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			val = home + val[1:]
		}
	}

	mapper := func(varName string) string {
		switch varName {
		case "XDG_CONFIG_HOME":
			return xdg.ConfigHome
		case "XDG_DATA_HOME":
			return xdg.DataHome
		case "XDG_STATE_HOME":
			return xdg.StateHome
		case "XDG_CACHE_HOME":
			return xdg.CacheHome
		case "HOME":
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			return home
		case "USER":
			u, err := user.Current()
			if err != nil {
				return os.Getenv("USERNAME") // Fallback for Windows
			}
			return u.Username
		case "ScriptFolder":
			return paths.GetBashScriptFolder()
		}
		// Fall back to the real environment for any unrecognised variable
		return os.Getenv(varName)
	}
	return os.Expand(val, mapper)
}

// CollapseVariables replaces absolute paths with their environment variable equivalents.
// It specifically EXCLUDES ${ScriptFolder} from reverse resolution.
func CollapseVariables(path string) string {
	if path == "" {
		return ""
	}

	// Order matters: more specific paths first
	path = filepath.Clean(path)
	home, _ := os.UserHomeDir()
	home = filepath.Clean(home)

	// Use a slice of pairs to ensure deterministic order
	type pair struct {
		abs    string
		envVar string
	}
	vars := []pair{
		{filepath.Clean(xdg.ConfigHome), "${XDG_CONFIG_HOME}"},
		{filepath.Clean(xdg.DataHome), "${XDG_DATA_HOME}"},
		{filepath.Clean(xdg.StateHome), "${XDG_STATE_HOME}"},
		{filepath.Clean(xdg.CacheHome), "${XDG_CACHE_HOME}"},
		{home, "${HOME}"},
	}

	for _, v := range vars {
		if v.abs != "" {
			// Check if the path is exactly the variable's value or if it's a sub-path
			if path == v.abs {
				return v.envVar
			}
			if strings.HasPrefix(path, v.abs+string(os.PathSeparator)) {
				return v.envVar + strings.TrimPrefix(path, v.abs)
			}
		}
	}

	return path
}

// LoadAppConfig reads the configuration file and returns the configuration.
// defaults.toml (embedded) is always unmarshalled first; the user's file is
// overlaid on top, so keys present in the user's file override the defaults
// and keys absent from the user's file fall back to the embedded defaults.
func LoadAppConfig() AppConfig {
	var conf AppConfig
	// Start from embedded defaults so every key has a known baseline value.
	_ = toml.Unmarshal(defaultsToml, &conf)

	// Set architecture (runtime only)
	conf.Arch = getArch()

	path := paths.GetConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		// No config file found. Attempt migration from legacy.
		migrationCtx := context.WithValue(context.Background(), "migration_mode", true)
		if migrated, ok := MigrateFromLegacy(migrationCtx); ok {
			// Save the migrated config so we don't migrate again next time
			_ = SaveAppConfig(migrated)
			// Re-load to ensure all paths and late-stage initializations are performed correctly
			conf = LoadAppConfig()
			// Show the config after migration, matching bash version behavior
			logNotice(migrationCtx, "")
			ShowAppConfig(migrationCtx, &conf)
			logNotice(migrationCtx, "")
			return conf
		}
	} else {
		// Overlay user config on top of defaults.
		// Use Robust unmarshaling to handle any loose types from manual edits
		if _, err := UnmarshalRobust(data, &conf); err == nil {
			// Write back only if the merged config differs from what was on disk
			// (e.g. new keys added in a newer version). Avoids a pointless write
			// on every load which would also trigger any file watchers.
			if merged, err := toml.Marshal(conf); err == nil {
				if string(merged) != string(data) {
					_ = SaveAppConfig(conf)
				}
			}
			conf.RawPaths = conf.Paths
			conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
			conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
			conf.ConfigDir = conf.Paths.ConfigFolder
			conf.ComposeDir = conf.Paths.ComposeFolder
			return conf
		}
	}

	// No user config found; write the embedded defaults to disk (preserving comments).
	cfgPath := paths.GetConfigFilePath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err == nil {
		logNotice(context.Background(), "Creating '{{|Folder|}}%s{{[-]}}'.", filepath.Dir(cfgPath))
		_ = os.WriteFile(cfgPath, defaultsToml, 0644)
		logNotice(context.Background(), "Copying '{{|File|}}%s{{[-]}}' to '{{|File|}}%s{{[-]}}'.", "embedded defaults", cfgPath)
	}
	conf.RawPaths = conf.Paths
	conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
	conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
	conf.ConfigDir = conf.Paths.ConfigFolder
	conf.ComposeDir = conf.Paths.ComposeFolder
	return conf
}

// TryLoadAppConfig loads and validates the config file strictly, returning an
// error if the file cannot be read or parsed. Unlike LoadAppConfig it does not
// fall back to defaults or write anything to disk, making it safe to call from
// a file watcher to gate whether a change should be applied.
func TryLoadAppConfig() (AppConfig, error) {
	var conf AppConfig
	_ = toml.Unmarshal(defaultsToml, &conf)
	conf.Arch = getArch()

	data, err := os.ReadFile(paths.GetConfigFilePath())
	if err != nil {
		return AppConfig{}, err
	}
	if err := toml.Unmarshal(data, &conf); err != nil {
		return AppConfig{}, err
	}
	conf.RawPaths = conf.Paths
	conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
	conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
	conf.ConfigDir = conf.Paths.ConfigFolder
	conf.ComposeDir = conf.Paths.ComposeFolder
	return conf, nil
}

// SaveAppConfig writes the configuration to dockstarter2.toml.
// SaveAppConfig writes the configuration to the application configuration file.
// Paths are stored in their unexpanded form (e.g. ${XDG_CONFIG_HOME}) so that
// the file remains portable and variables are resolved fresh on each read.
func SaveAppConfig(conf AppConfig) error {
	path := paths.GetConfigFilePath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 1. If RawPaths was set (e.g. from a recent Load), prioritize those original strings
	if conf.RawPaths.ConfigFolder != "" {
		conf.Paths.ConfigFolder = conf.RawPaths.ConfigFolder
	} else {
		// 2. Otherwise, auto-collapse absolute paths to variables (XDG, HOME, etc.)
		conf.Paths.ConfigFolder = CollapseVariables(conf.Paths.ConfigFolder)
	}

	if conf.RawPaths.ComposeFolder != "" {
		conf.Paths.ComposeFolder = conf.RawPaths.ComposeFolder
	} else {
		conf.Paths.ComposeFolder = CollapseVariables(conf.Paths.ComposeFolder)
	}

	data, err := toml.Marshal(conf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// UnmarshalRobust unmarshals TOML data into a struct using mapstructure
// to allow for "weak" type conversion (e.g., string "true" to boolean true).
func UnmarshalRobust(data []byte, v any) (map[string]bool, error) {
	present := make(map[string]bool)
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Helper to track present keys in the flat namespace of AppConfig
	var trackKeys func(m map[string]any, prefix string)
	trackKeys = func(m map[string]any, prefix string) {
		for k, val := range m {
			fullKey := k
			if prefix != "" {
				fullKey = prefix + "." + k
			}

			// Special case: map the TOML structure to the flat keys used in ShowAppConfig
			switch fullKey {
			case "paths.config_folder":
				present["ConfigFolder"] = true
			case "paths.compose_folder":
				present["ComposeFolder"] = true
			case "ui.theme":
				present["Theme"] = true
			case "ui.borders":
				present["Borders"] = true
			case "ui.button_borders":
				present["ButtonBorders"] = true
			case "ui.line_characters":
				present["LineCharacters"] = true
			case "ui.scrollbar":
				present["Scrollbar"] = true
			case "ui.shadow":
				present["Shadow"] = true
			case "ui.shadow_level":
				present["ShadowLevel"] = true
			case "ui.border_color":
				present["BorderColor"] = true
			case "ui.dialog_title_align":
				present["DialogTitleAlign"] = true
			case "ui.submenu_title_align":
				present["SubmenuTitleAlign"] = true
			case "ui.panel_title_align":
				present["PanelTitleAlign"] = true
			case "ui.panel_local":
				present["PanelLocal"] = true
			case "ui.panel_remote":
				present["PanelRemote"] = true
			case "server.ssh.port":
				present["SSHPort"] = true
			case "server.web.port":
				present["WebPort"] = true
			case "server.auth.mode":
				present["AuthMode"] = true
			}

			if subMap, ok := val.(map[string]any); ok {
				trackKeys(subMap, fullKey)
			}
		}
	}
	trackKeys(raw, "")

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           v,
		TagName:          "toml",
	})
	if err != nil {
		return nil, err
	}

	return present, decoder.Decode(raw)
}

// UnmarshalLegacyIni parses a legacy .ini configuration file and maps it to AppConfig.
func UnmarshalLegacyIni(data []byte, v *AppConfig) (map[string]bool, error) {
	present := make(map[string]bool)
	cfg, err := ini.Load(data)
	if err != nil {
		return nil, err
	}

	// Default section or [DockSTARTer]
	ds := cfg.Section("DockSTARTer")
	if ds == nil {
		ds = cfg.Section(ini.DefaultSection)
	}

	// Mapping Paths
	if ds.HasKey("ConfigFolder") {
		val := ds.Key("ConfigFolder").String()
		v.Paths.ConfigFolder = ExpandVariables(val)
		present["ConfigFolder"] = true
	}
	if ds.HasKey("ComposeFolder") {
		val := ds.Key("ComposeFolder").String()
		v.Paths.ComposeFolder = ExpandVariables(val)
		present["ComposeFolder"] = true
	}

	// Mapping UI (often in [Theme] section or default)
	theme := cfg.Section("Theme")
	if theme == nil {
		theme = ds
	}
	if theme.HasKey("Theme") {
		v.UI.Theme = theme.Key("Theme").String()
		present["Theme"] = true
	}

	// Permissive boolean parsing (matching legacy is_true)
	isTrue := func(s string) bool {
		s = strings.ToUpper(strings.TrimSpace(s))
		return s == "TRUE" || s == "1" || s == "ON" || s == "YES"
	}

	if theme.HasKey("Scrollbars") || theme.HasKey("Scrollbar") {
		key := "Scrollbar"
		if theme.HasKey("Scrollbars") {
			key = "Scrollbars"
		}
		v.UI.Scrollbar = isTrue(theme.Key(key).String())
		present["Scrollbar"] = true
	}

	if theme.HasKey("Shadows") || theme.HasKey("Shadow") {
		key := "Shadow"
		if theme.HasKey("Shadows") {
			key = "Shadows"
		}
		v.UI.Shadow = isTrue(theme.Key(key).String())
		present["Shadow"] = true
	}

	if theme.HasKey("Borders") || theme.HasKey("LineCharacters") {
		key := "Borders"
		if theme.HasKey("LineCharacters") {
			key = "LineCharacters"
		}
		v.UI.Borders = isTrue(theme.Key(key).String())
		present["Borders"] = true
	}

	if theme.HasKey("LineCharacters") {
		v.UI.LineCharacters = isTrue(theme.Key("LineCharacters").String())
		present["LineCharacters"] = true
	}

	return present, nil
}

// MigrateFromLegacy coordinates the discovery and ingestion of legacy configuration.
// It returns the migrated configuration and true if any legacy data was found.
func MigrateFromLegacy(ctx context.Context) (AppConfig, bool) {
	conf := AppConfig{}
	// Load defaults first
	if err := toml.Unmarshal(defaultsToml, &conf); err != nil {
		// This should never happen as defaults are embedded
		return conf, false
	}

	foundLegacy := false

	// 1. Build priority list of legacy files (matches Bash pattern)
	var legacyFiles []string
	// Priority 1: Late-stage DS1 .toml file
	legacyFiles = append(legacyFiles, filepath.Join(xdg.ConfigHome, "dockstarter", "dockstarter.toml"))
	// Priority 2-N: Legacy .ini files (XDG then script folder)
	legacyFiles = append(legacyFiles, paths.GetLegacyIniPaths()...)

	for _, path := range legacyFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Try next file
		}

		logNotice(ctx, "Migrating '{{|File|}}%s{{[-]}}' to '{{|File|}}%s{{[-]}}'.", path, paths.GetConfigFilePath())
		var oldConf AppConfig // Clean config for display (no defaults merged)
		var present map[string]bool
		var unmarshalErr error

		if strings.HasSuffix(path, ".toml") {
			present, unmarshalErr = UnmarshalRobust(data, &oldConf)
		} else {
			present, unmarshalErr = UnmarshalLegacyIni(data, &oldConf)
		}

		if unmarshalErr == nil {
			heading := fmt.Sprintf("Configuration options in old config file '{{|File|}}%s{{[-]}}':", path)
			logNotice(ctx, "")
			ShowAppConfigWithTitleAndPresent(ctx, &oldConf, heading, present)

			// Apply to the actual merged config
			if strings.HasSuffix(path, ".toml") {
				_, _ = UnmarshalRobust(data, &conf)
				// Ensure any legacy variables are expanded so they can be re-collapsed to standard ones
				conf.Paths.ConfigFolder = ExpandVariables(conf.Paths.ConfigFolder)
				conf.Paths.ComposeFolder = ExpandVariables(conf.Paths.ComposeFolder)
			} else {
				_, _ = UnmarshalLegacyIni(data, &conf)
			}
			foundLegacy = true
			break // STOP after the first successful migration source is processed
		}
	}

	// 3. Detect Compose Folder
	detection := paths.DetectComposeFolder(conf.Paths.ComposeFolder)
	foundComposeMigration := false
	if detection.LegacyExists && detection.CurrentExists && detection.LegacyPath != detection.CurrentPath {
		promptMsg := fmt.Sprintf("Existing docker compose folders found in multiple locations.\n   Legacy:  '{{|Folder|}}%s{{[-]}}'\n   Default: '{{|Folder|}}%s{{[-]}}'\n\nWould you like to use the Legacy location?", detection.LegacyPath, detection.CurrentPath)
		useLegacy, err := console.QuestionPrompt(ctx, logNotice, "Multiple Compose Folders Detected", promptMsg, "Y", false)
		if err == nil && useLegacy {
			logNotice(ctx, "Chose the Legacy compose folder location:\n   '{{|Folder|}}%s{{[-]}}'", detection.LegacyPath)
			conf.Paths.ComposeFolder = detection.LegacyPath
			foundComposeMigration = true
		} else if err == nil {
			logNotice(ctx, "Chose the Default compose folder location:\n   '{{|Folder|}}%s{{[-]}}'", detection.CurrentPath)
		}
	} else if detection.LegacyExists && !detection.CurrentExists {
		logNotice(ctx, "Legacy compose folder found at '{{|Folder|}}%s{{[-]}}'. Auto-migrating.", detection.LegacyPath)
		conf.Paths.ComposeFolder = detection.LegacyPath
		foundComposeMigration = true
	}

	if !foundLegacy && !foundComposeMigration {
		return conf, false
	}

	return conf, true
}

// ShowAppConfig prints a summary table of the current configuration.
func ShowAppConfig(ctx context.Context, conf *AppConfig) {
	ShowAppConfigWithTitle(ctx, conf, "")
}

// ShowAppConfigWithTitle prints a summary table with a custom title.
func ShowAppConfigWithTitle(ctx context.Context, conf *AppConfig, title string) {
	ShowAppConfigWithTitleAndPresent(ctx, conf, title, nil)
}

// ShowAppConfigWithTitleAndPresent prints a summary table with a custom title, optionally filtering by present keys.
func ShowAppConfigWithTitleAndPresent(ctx context.Context, conf *AppConfig, title string, presentKeys map[string]bool) {
	headers := []string{
		"{{|UsageCommand|}}Option{{[-]}}",
		"{{|UsageCommand|}}Value{{[-]}}",
		"{{|UsageCommand|}}Expanded Value{{[-]}}",
	}

	keys := []string{
		"ConfigFolder", "ComposeFolder",
		"Theme", "Borders", "ButtonBorders", "LineCharacters", "Scrollbar", "Shadow", "ShadowLevel", "BorderColor",
		"DialogTitleAlign", "SubmenuTitleAlign", "PanelTitleAlign", "PanelLocal", "PanelRemote",
		"SSHPort", "WebPort", "AuthMode",
	}
	displayNames := map[string]string{
		"ConfigFolder":      "Config Folder",
		"ComposeFolder":     "Compose Folder",
		"Theme":             "Theme",
		"Borders":           "Borders",
		"ButtonBorders":     "Button Borders",
		"LineCharacters":    "Line Characters",
		"Scrollbar":         "Scrollbar",
		"Shadow":            "Shadow",
		"ShadowLevel":       "Shadow Level",
		"BorderColor":       "Border Color",
		"DialogTitleAlign":  "Dialog Title Align",
		"SubmenuTitleAlign": "Submenu Title Align",
		"PanelTitleAlign":   "Panel Title Align",
		"PanelLocal":        "Panel Local",
		"PanelRemote":       "Panel Remote",
		"SSHPort":           "SSH Port",
		"WebPort":           "Web Port",
		"AuthMode":          "Auth Mode",
	}

	var data []string

	boolToYesNo := func(val bool) string {
		if val {
			return "{{|Var|}}yes{{[-]}}"
		}
		return "{{|Var|}}no{{[-]}}"
	}

	for _, key := range keys {
		// Filter out keys not present in the legacy file if presentKeys is provided
		if presentKeys != nil && !presentKeys[key] {
			continue
		}

		var value, expandedValue string
		var useFolderColor bool

		switch key {
		case "ConfigFolder":
			value = conf.RawPaths.ConfigFolder
			expandedValue = conf.ConfigDir
			useFolderColor = true
		case "ComposeFolder":
			value = conf.RawPaths.ComposeFolder
			expandedValue = conf.ComposeDir
			useFolderColor = true
		case "Theme":
			value = conf.UI.Theme
		case "Borders":
			value = boolToYesNo(conf.UI.Borders)
		case "ButtonBorders":
			value = boolToYesNo(conf.UI.ButtonBorders)
		case "LineCharacters":
			value = boolToYesNo(conf.UI.LineCharacters)
		case "Scrollbar":
			value = boolToYesNo(conf.UI.Scrollbar)
		case "Shadow":
			value = boolToYesNo(conf.UI.Shadow)
		case "ShadowLevel":
			value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.UI.ShadowLevel)
		case "BorderColor":
			value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.UI.BorderColor)
		case "DialogTitleAlign":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.UI.DialogTitleAlign)
		case "SubmenuTitleAlign":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.UI.SubmenuTitleAlign)
		case "PanelTitleAlign":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.UI.PanelTitleAlign)
		case "PanelLocal":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.UI.PanelLocal)
		case "PanelRemote":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.UI.PanelRemote)
		case "SSHPort":
			if conf.Server.SSH.Port > 0 {
				value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.Server.SSH.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "WebPort":
			if conf.Server.Web.Port > 0 {
				value = fmt.Sprintf("{{|Var|}}%d{{[-]}}", conf.Server.Web.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "AuthMode":
			value = fmt.Sprintf("{{|Var|}}%s{{[-]}}", conf.Server.Auth.Mode)
		}

		colorTag := "{{|Var|}}"
		if useFolderColor {
			colorTag = "{{|Folder|}}"
		}

		data = append(data, displayNames[key])

		if useFolderColor || key == "Theme" {
			data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, value))
		} else {
			data = append(data, value)
		}

		if expandedValue != "" {
			data = append(data, fmt.Sprintf("%s%s{{[-]}}", colorTag, expandedValue))
		} else {
			data = append(data, "")
		}
	}

	if title == "" {
		title = "Configuration options stored in '{{|File|}}" + paths.GetConfigFilePath() + "{{[-]}}':"
	}

	if val, ok := ctx.Value("migration_mode").(bool); ok && val {
		logNotice(ctx, title)
		var sb strings.Builder
		console.PrintTableCtx(console.WithTUIWriter(ctx, &sb), headers, data, conf.UI.LineCharacters)
		logNotice(ctx, strings.TrimSuffix(sb.String(), "\n"))
	} else {
		fmt.Println(console.ToConsoleANSI(title))
		var sb strings.Builder
		console.PrintTableCtx(console.WithTUIWriter(ctx, &sb), headers, data, conf.UI.LineCharacters)
		fmt.Println(console.ToConsoleANSI(sb.String()))
	}
}

// logNotice logs a notice message, splitting multi-line messages and logging each separately.
// It uses slog.LevelInfo which is mapped to LevelNotice in the application logger.
func logNotice(ctx context.Context, msg any, args ...any) {
	msgStr := fmt.Sprint(msg)
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
	} else if len(args) > 0 {
		msgStr = fmt.Sprint(append([]any{msgStr}, args...)...)
	}
	lines := strings.Split(msgStr, "\n")
	for _, line := range lines {
		slog.Log(ctx, slog.LevelInfo, line)
	}
}
