package config

import (
	"DockSTARTer2/internal/paths"
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
)

// AppConfig holds the application configuration settings.
type AppConfig struct {
	Borders                 bool
	LineCharacters          bool
	Shadow                  bool
	Scrollbar               bool
	Theme                   string
	Arch                    string
	ConfigFolder            string
	ConfigFolderUnexpanded  string
	ComposeFolder           string
	ComposeFolderUnexpanded string
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
		Borders:                 true,
		LineCharacters:          true,
		Shadow:                  true,
		Scrollbar:               true,
		Theme:                   "DockSTARTer",
		ConfigFolder:            "${XDG_CONFIG_HOME}",
		ConfigFolderUnexpanded:  "${XDG_CONFIG_HOME}",
		ComposeFolder:           "${XDG_CONFIG_HOME}/compose",
		ComposeFolderUnexpanded: "${XDG_CONFIG_HOME}/compose",
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
		conf.ConfigFolderUnexpanded = conf.ConfigFolder
		conf.ComposeFolderUnexpanded = conf.ComposeFolder
		conf.ConfigFolder = ExpandVariables(conf.ConfigFolder)
		conf.ComposeFolder = ExpandVariables(conf.ComposeFolder)
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
		case "Borders":
			conf.Borders = isTrue(value)
		case "LineCharacters":
			conf.LineCharacters = isTrue(value)
		case "Shadow":
			conf.Shadow = isTrue(value)
		case "Scrollbar":
			conf.Scrollbar = isTrue(value)
		case "Theme":
			conf.Theme = value
		case "ConfigFolder":
			conf.ConfigFolderUnexpanded = value
			conf.ConfigFolder = expandedValue
		case "ComposeFolder":
			conf.ComposeFolderUnexpanded = value
			conf.ComposeFolder = expandedValue
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

	writeOption("ConfigFolder", conf.ConfigFolder)
	writeOption("ComposeFolder", conf.ComposeFolder)
	writeOption("Borders", boolToYesNo(conf.Borders))
	writeOption("LineCharacters", boolToYesNo(conf.LineCharacters))
	writeOption("Scrollbar", boolToYesNo(conf.Scrollbar))
	writeOption("Shadow", boolToYesNo(conf.Shadow))
	writeOption("Theme", conf.Theme)

	return writer.Flush()
}
