package config

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"bufio"
	"fmt"
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
	Arch       string `toml:"-"`
	ConfigDir  string `toml:"-"`
	ComposeDir string `toml:"-"`
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
func ExpandVariables(val string) string {
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
		return ""
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
			ConfigFolder:  "${XDG_CONFIG_HOME}",
			ComposeFolder: "${XDG_CONFIG_HOME}/compose",
		},
	}

	// Set architecture (runtime only)
	conf.Arch = getArch()

	path := paths.GetConfigFilePath()
	data, err := os.ReadFile(path)
	if err == nil {
		// Found TOML config
		if err := toml.Unmarshal(data, &conf); err == nil {
			// Expand variables for runtime use
			conf.ConfigDir = ExpandVariables(conf.Paths.ConfigFolder)
			conf.ComposeDir = ExpandVariables(conf.Paths.ComposeFolder)
			return conf
		}
	}

	// If TOML not found or invalid, check for migration from old INI
	oldPath := filepath.Join(filepath.Dir(path), "dockstarter2.ini")
	if _, err := os.Stat(oldPath); err == nil {
		fmt.Printf("Migrating configuration from %s to %s\n", oldPath, path)
		if oldConf, err := loadLegacyConfig(oldPath); err == nil {
			conf = oldConf
			// Save to new format
			SaveAppConfig(conf)
			// Cleanup old file
			os.Rename(oldPath, oldPath+".bak")
			return conf
		}
	}

	// If neither exists, save defaults to TOML
	conf.ConfigDir = ExpandVariables(conf.Paths.ConfigFolder)
	conf.ComposeDir = ExpandVariables(conf.Paths.ComposeFolder)
	SaveAppConfig(conf)
	return conf
}

// loadLegacyConfig reads the old .ini format for migration purposes.
func loadLegacyConfig(path string) (AppConfig, error) {
	conf := AppConfig{
		UI: UIConfig{
			Theme:          "DockSTARTer",
			Borders:        true,
			LineCharacters: true,
			Shadow:         true,
			ShadowLevel:    2,
			Scrollbar:      true,
		},
		Paths: PathConfig{
			ConfigFolder:  "${XDG_CONFIG_HOME}",
			ComposeFolder: "${XDG_CONFIG_HOME}/compose",
		},
	}
	conf.Arch = getArch()

	file, err := os.Open(path)
	if err != nil {
		return conf, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "'\"")

		isTrue := func(v string) bool {
			v = strings.ToLower(v)
			return v == "1" || v == "true" || v == "yes" || v == "on"
		}

		switch key {
		case constants.BordersKey:
			conf.UI.Borders = isTrue(value)
		case constants.LineCharactersKey:
			conf.UI.LineCharacters = isTrue(value)
		case constants.ShadowKey:
			conf.UI.Shadow = isTrue(value)
		case constants.ShadowLevelKey:
			level := 3
			val := strings.ToLower(value)
			switch val {
			case "0", "off", "none", "false", "no":
				level = 0
			case "1", "light":
				level = 1
			case "2", "medium":
				level = 2
			case "3", "dark":
				level = 3
			case "4", "solid", "full":
				level = 4
			}
			conf.UI.ShadowLevel = level
		case constants.ScrollbarKey:
			conf.UI.Scrollbar = isTrue(value)
		case constants.ThemeKey:
			conf.UI.Theme = value
		case constants.ConfigFolderKey:
			conf.Paths.ConfigFolder = value
		case constants.ComposeFolderKey:
			conf.Paths.ComposeFolder = value
		}
	}
	conf.ConfigDir = ExpandVariables(conf.Paths.ConfigFolder)
	conf.ComposeDir = ExpandVariables(conf.Paths.ComposeFolder)
	return conf, nil
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
