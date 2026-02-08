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
)

// AppConfig holds the application configuration settings.
type AppConfig struct {
	Borders              bool
	LineCharacters       bool
	Shadow               bool
	ShadowLevel          int // 0=off, 1=light(░), 2=medium(▒), 3=dark(▓), 4=solid(█)
	Scrollbar            bool
	Theme                string
	Arch                 string
	ConfigDir            string
	ConfigDirUnexpanded  string
	ComposeDir           string
	ComposeDirUnexpanded string
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

// LoadAppConfig reads the dockstarter2.ini file and returns the configuration.
func LoadAppConfig() AppConfig {
	conf := AppConfig{
		Borders:              true,
		LineCharacters:       true,
		Shadow:               true,
		ShadowLevel:          3, // Default: dark (▓)
		Scrollbar:            true,
		Theme:                "DockSTARTer",
		ConfigDir:            "${XDG_CONFIG_HOME}",
		ConfigDirUnexpanded:  "${XDG_CONFIG_HOME}",
		ComposeDir:           "${XDG_CONFIG_HOME}/compose",
		ComposeDirUnexpanded: "${XDG_CONFIG_HOME}/compose",
	}

	// Set architecture
	conf.Arch = getArch()

	path := paths.GetConfigFilePath()
	file, err := os.Open(path)
	if err != nil {
		// If file doesn't exist, create it with defaults
		if os.IsNotExist(err) {
			SaveAppConfig(conf)
		}
		conf.ConfigDirUnexpanded = conf.ConfigDir
		conf.ComposeDirUnexpanded = conf.ComposeDir
		conf.ConfigDir = ExpandVariables(conf.ConfigDir)
		conf.ComposeDir = ExpandVariables(conf.ComposeDir)
		return conf
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
		expandedValue := ExpandVariables(value)

		isTrue := func(v string) bool {
			v = strings.ToLower(v)
			return v == "1" || v == "true" || v == "yes" || v == "on"
		}

		switch key {
		case constants.BordersKey:
			conf.Borders = isTrue(value)
		case constants.LineCharactersKey:
			conf.LineCharacters = isTrue(value)
		case constants.ShadowKey:
			conf.Shadow = isTrue(value)
		case constants.ShadowLevelKey:
			// Parse shadow level (0-4)
			level := 3 // default to dark
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
			default:
				// specialized handling for percentage strings
				if strings.HasSuffix(val, "%") {
					var percent int
					if _, err := fmt.Sscanf(val, "%d%%", &percent); err == nil {
						if percent <= 12 {
							level = 0
						} else if percent <= 37 {
							level = 1
						} else if percent <= 62 {
							level = 2
						} else if percent <= 87 {
							level = 3
						} else {
							level = 4
						}
					}
				}
			}
			conf.ShadowLevel = level
		case constants.ScrollbarKey:
			conf.Scrollbar = isTrue(value)
		case constants.ThemeKey:
			conf.Theme = value
		case constants.ConfigFolderKey:
			conf.ConfigDirUnexpanded = value
			conf.ConfigDir = expandedValue
		case constants.ComposeFolderKey:
			conf.ComposeDirUnexpanded = value
			conf.ComposeDir = expandedValue
		}
	}

	return conf
}

// SaveAppConfig writes the configuration to dockstarter2.ini.
// Note: It writes the raw values currently in the struct.
// TODO: If we want to preserve variables (like ${XDG_CONFIG_HOME}) on save,
// we would need to track raw vs expanded values. For now, it saves the expanded path.
func SaveAppConfig(conf AppConfig) error {
	path := paths.GetConfigFilePath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	writeOption := func(key string, val string) {
		writer.WriteString(key + "='" + val + "'\n")
	}

	boolToYesNo := func(val bool) string {
		if val {
			return "yes"
		}
		return "no"
	}

	writeOption(constants.ConfigFolderKey, conf.ConfigDir)
	writeOption(constants.ComposeFolderKey, conf.ComposeDir)
	writeOption(constants.BordersKey, boolToYesNo(conf.Borders))
	writeOption(constants.LineCharactersKey, boolToYesNo(conf.LineCharacters))
	writeOption(constants.ScrollbarKey, boolToYesNo(conf.Scrollbar))
	writeOption(constants.ShadowKey, boolToYesNo(conf.Shadow))
	writeOption(constants.ShadowLevelKey, fmt.Sprintf("%d", conf.ShadowLevel))
	writeOption(constants.ThemeKey, conf.Theme)

	return writer.Flush()
}
