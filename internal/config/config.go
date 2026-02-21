package config

import (
	"DockSTARTer2/internal/paths"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
	toml "github.com/pelletier/go-toml/v2"
)

// AppConfig holds the application configuration settings.
type AppConfig struct {
	UI    UIConfig   `toml:"ui"`
	Paths PathConfig `toml:"paths"`

	// These are helper fields for runtime use, not saved to TOML
	Arch       string     `toml:"-"`
	ConfigDir  string     `toml:"-"`
	ComposeDir string     `toml:"-"`
	RawPaths   PathConfig `toml:"-"` // Unexpanded values as read from TOML
}

// UIConfig holds user interface related settings.
type UIConfig struct {
	Theme          string `toml:"theme"`
	Borders        bool   `toml:"borders"`
	LineCharacters bool   `toml:"line_characters"`
	Shadow         bool   `toml:"shadow"`
	ShadowLevel    int    `toml:"shadow_level"` // 0=off, 1=light(░), 2=medium(▒), 3=dark(▓), 4=solid(█)
	Scrollbar      bool   `toml:"scrollbar"`
	BorderColor    int    `toml:"border_color"` // 1=Border, 2=Border2, 3=Both
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
func LoadAppConfig() AppConfig {
	conf := AppConfig{
		UI: UIConfig{
			Theme:          "DockSTARTer",
			Borders:        true,
			LineCharacters: true,
			Shadow:         true,
			ShadowLevel:    2, // Default: medium (▒)
			Scrollbar:      true,
			BorderColor:    3,
		},
		Paths: PathConfig{
			ConfigFolder:  "${XDG_CONFIG_HOME}/dockstarter2",
			ComposeFolder: "${XDG_CONFIG_HOME}/dockstarter2/compose",
		},
	}

	// Set architecture (runtime only)
	conf.Arch = getArch()

	path := paths.GetConfigFilePath()
	data, err := os.ReadFile(path)
	if err == nil {
		// Found TOML config
		if err := toml.Unmarshal(data, &conf); err == nil {
			// Save raw (unexpanded) values for display purposes
			conf.RawPaths = conf.Paths
			// Expand variables in paths for runtime use
			conf.Paths.ConfigFolder = ExpandVariables(conf.Paths.ConfigFolder)
			conf.Paths.ComposeFolder = ExpandVariables(conf.Paths.ComposeFolder)
			conf.ConfigDir = conf.Paths.ConfigFolder
			conf.ComposeDir = conf.Paths.ComposeFolder
			return conf
		}
	}

	// No config found; save defaults to TOML (with template variables, not expanded)
	SaveAppConfig(conf)
	// Save raw (unexpanded) values for display purposes
	conf.RawPaths = conf.Paths
	// Expand after saving so the on-disk file retains ${XDG_CONFIG_HOME} references
	conf.Paths.ConfigFolder = ExpandVariables(conf.Paths.ConfigFolder)
	conf.Paths.ComposeFolder = ExpandVariables(conf.Paths.ComposeFolder)
	conf.ConfigDir = conf.Paths.ConfigFolder
	conf.ComposeDir = conf.Paths.ComposeFolder
	return conf
}

// SaveAppConfig writes the configuration to dockstarter2.toml.
func SaveAppConfig(conf AppConfig) error {
	path := paths.GetConfigFilePath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(conf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
