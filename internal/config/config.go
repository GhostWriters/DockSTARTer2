package config

import (
	_ "embed"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"DockSTARTer2/internal/paths"

	"github.com/adrg/xdg"
	toml "github.com/pelletier/go-toml/v2"
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
	Password     string `toml:"password"`      // bcrypt hash of the password
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
	LogTitleAlign     string `toml:"log_title_align"`     // "center" or "left"
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
		}
		// Fall back to the real environment for any unrecognised variable
		return os.Getenv(varName)
	}
	return os.Expand(val, mapper)
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
	if err == nil {
		// Overlay user config on top of defaults.
		if err := toml.Unmarshal(data, &conf); err == nil {
			// Write back the merged config so any keys missing from the user's
			// file (e.g. added in a newer version) are filled in automatically.
			_ = SaveAppConfig(conf)
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
// Paths are stored in their unexpanded form (e.g. ${XDG_CONFIG_HOME}) so that
// the file remains portable and variables are resolved fresh on each read.
func SaveAppConfig(conf AppConfig) error {
	path := paths.GetConfigFilePath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Restore unexpanded path values for on-disk storage.
	// After LoadAppConfig, conf.Paths holds runtime-expanded values while
	// conf.RawPaths holds the original template strings (e.g. ${XDG_CONFIG_HOME}).
	// Use RawPaths when available so we never write expanded paths to disk.
	if conf.RawPaths.ConfigFolder != "" || conf.RawPaths.ComposeFolder != "" {
		conf.Paths = conf.RawPaths
	}

	data, err := toml.Marshal(conf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
