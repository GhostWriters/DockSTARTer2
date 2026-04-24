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
		if migrated, ok := MigrateFromLegacy(); ok {
			// Save the migrated config so we don't migrate again next time
			_ = SaveAppConfig(migrated)
			// Re-load to ensure all paths and late-stage initializations are performed correctly
			conf = LoadAppConfig()
			// Show the config after migration, matching bash version behavior
			ShowAppConfig(context.Background(), &conf)
			return conf
		}
	} else {
		// Overlay user config on top of defaults.
		// Use Robust unmarshaling to handle any loose types from manual edits
		if err := UnmarshalRobust(data, &conf); err == nil {
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
		_ = os.WriteFile(cfgPath, defaultsToml, 0644)
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
// This is primarily intended for migrating configuration from legacy versions
// that may have stored boolean values as quoted strings.
func UnmarshalRobust(data []byte, v any) error {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return err
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           v,
		TagName:          "toml",
	})
	if err != nil {
		return err
	}

	return decoder.Decode(raw)
}

// UnmarshalLegacyIni parses a legacy .ini configuration file and maps it to AppConfig.
func UnmarshalLegacyIni(data []byte, v *AppConfig) error {
	cfg, err := ini.Load(data)
	if err != nil {
		return err
	}

	// Default section or [DockSTARTer]
	ds := cfg.Section("DockSTARTer")
	if ds == nil {
		ds = cfg.Section(ini.DefaultSection)
	}

	// Mapping Paths
	if val := ds.Key("ConfigFolder").String(); val != "" {
		v.Paths.ConfigFolder = ExpandVariables(val)
	}
	if val := ds.Key("ComposeFolder").String(); val != "" {
		v.Paths.ComposeFolder = ExpandVariables(val)
	}

	// Mapping UI (often in [Theme] section or default)
	theme := cfg.Section("Theme")
	if theme == nil {
		theme = ds
	}
	if val := theme.Key("Theme").String(); val != "" {
		v.UI.Theme = val
	}

	// Permissive boolean parsing (matching legacy is_true)
	isTrue := func(s string) bool {
		s = strings.ToUpper(strings.TrimSpace(s))
		return s == "TRUE" || s == "1" || s == "ON" || s == "YES"
	}

	if theme.HasKey("Scrollbars") {
		v.UI.Scrollbar = isTrue(theme.Key("Scrollbars").String())
	} else if theme.HasKey("Scrollbar") {
		v.UI.Scrollbar = isTrue(theme.Key("Scrollbar").String())
	}

	if theme.HasKey("Shadows") {
		v.UI.Shadow = isTrue(theme.Key("Shadows").String())
	} else if theme.HasKey("Shadow") {
		v.UI.Shadow = isTrue(theme.Key("Shadow").String())
	}

	if theme.HasKey("Borders") {
		v.UI.Borders = isTrue(theme.Key("Borders").String())
	} else if theme.HasKey("LineCharacters") {
		v.UI.Borders = isTrue(theme.Key("LineCharacters").String())
	}

	if theme.HasKey("LineCharacters") {
		v.UI.LineCharacters = isTrue(theme.Key("LineCharacters").String())
	}

	return nil
}

// MigrateFromLegacy coordinates the discovery and ingestion of legacy configuration.
// It returns the migrated configuration and true if any legacy data was found.
func MigrateFromLegacy() (AppConfig, bool) {
	conf := AppConfig{}
	// Load defaults first
	if err := toml.Unmarshal(defaultsToml, &conf); err != nil {
		// This should never happen as defaults are embedded
		return conf, false
	}

	foundLegacy := false
	ctx := context.Background()

	// 1. Check for legacy .ini files (Priority: Modern XDG -> Old XDG -> Local)
	iniPaths := paths.GetLegacyIniPaths()
	for i := len(iniPaths) - 1; i >= 0; i-- { // Process in reverse so prioritized paths overwrite
		path := iniPaths[i]
		if data, err := os.ReadFile(path); err == nil {
			slog.Log(ctx, slog.LevelInfo, "Migrating legacy configuration from '{{|File|}}"+path+"{{[-]}}'.")
			if err := UnmarshalLegacyIni(data, &conf); err == nil {
				foundLegacy = true
			}
		}
	}

	// 2. Check for late-stage DS1 .toml file
	var tomlPaths []string
	tomlPaths = append(tomlPaths, filepath.Join(xdg.ConfigHome, "dockstarter", "dockstarter.toml"))

	for _, legacyToml := range tomlPaths {
		if data, err := os.ReadFile(legacyToml); err == nil {
			slog.Log(ctx, slog.LevelInfo, "Migrating legacy configuration from '{{|File|}}"+legacyToml+"{{[-]}}'.")
			// Use Robust unmarshaling to handle Bash's loose typing
			if err := UnmarshalRobust(data, &conf); err == nil {
				foundLegacy = true
				// Ensure any legacy variables are expanded so they can be re-collapsed to standard ones
				conf.Paths.ConfigFolder = ExpandVariables(conf.Paths.ConfigFolder)
				conf.Paths.ComposeFolder = ExpandVariables(conf.Paths.ComposeFolder)
			}
		}
	}

	configNotice := func(ctx context.Context, msg any, args ...any) {
		msgStr := fmt.Sprint(msg)
		if len(args) > 0 && strings.Contains(msgStr, "%") {
			msgStr = fmt.Sprintf(msgStr, args...)
		}
		slog.Log(ctx, slog.LevelInfo, msgStr)
	}

	// 3. Detect Compose Folder
	detection := paths.DetectComposeFolder(conf.Paths.ComposeFolder)
	foundComposeMigration := false
	if detection.LegacyExists && detection.CurrentExists && detection.LegacyPath != detection.CurrentPath {
		promptMsg := fmt.Sprintf("Existing docker compose folders found in multiple locations.\n   Legacy:  '{{|Folder|}}%s{{[-]}}'\n   Default: '{{|Folder|}}%s{{[-]}}'\n\nWould you like to use the Legacy location?", detection.LegacyPath, detection.CurrentPath)
		useLegacy, err := console.QuestionPrompt(ctx, configNotice, "Multiple Compose Folders Detected", promptMsg, "Y", false)
		if err == nil && useLegacy {
			configNotice(ctx, "Chose the Legacy compose folder location: '{{|Folder|}}%s{{[-]}}'", detection.LegacyPath)
			conf.Paths.ComposeFolder = detection.LegacyPath
			foundComposeMigration = true
		} else if err == nil {
			configNotice(ctx, "Chose the Default compose folder location: '{{|Folder|}}%s{{[-]}}'", detection.CurrentPath)
		}
	} else if detection.LegacyExists && !detection.CurrentExists {
		configNotice(ctx, "Legacy compose folder found at '{{|Folder|}}%s{{[-]}}'. Auto-migrating.", detection.LegacyPath)
		conf.Paths.ComposeFolder = detection.LegacyPath
		foundComposeMigration = true
	}

	if !foundLegacy && !foundComposeMigration {
		return conf, false
	}

	slog.Info("Legacy migration complete.")
	return conf, true
}

// ShowAppConfig prints a summary table of the current configuration.
func ShowAppConfig(ctx context.Context, conf *AppConfig) {
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
		"ConfigFolder":   "Config Folder",
		"ComposeFolder":  "Compose Folder",
		"Theme":          "Theme",
		"Borders":        "Borders",
		"ButtonBorders":  "Button Borders",
		"LineCharacters": "Line Characters",
		"Scrollbar":      "Scrollbar",
		"Shadow":         "Shadow",
		"ShadowLevel":    "Shadow Level",
		"BorderColor":    "Border Color",
		"DialogTitleAlign":  "Dialog Title Align",
		"SubmenuTitleAlign": "Submenu Title Align",
		"PanelTitleAlign":   "Panel Title Align",
		"PanelLocal":        "Panel Local",
		"PanelRemote":       "Panel Remote",
		"SSHPort":        "SSH Port",
		"WebPort":        "Web Port",
		"AuthMode":       "Auth Mode",
	}

	var data []string

	boolToYesNo := func(val bool) string {
		if val {
			return "{{|Var|}}yes{{[-]}}"
		}
		return "{{|Var|}}no{{[-]}}"
	}

	for _, key := range keys {
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

	fmt.Println(console.ToConsoleANSI("Configuration options stored in '{{|File|}}" + paths.GetConfigFilePath() + "{{[-]}}':"))
	console.PrintTableCtx(ctx, headers, data, conf.UI.LineCharacters)
}
