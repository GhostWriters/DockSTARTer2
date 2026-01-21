package config

import (
	"DockSTARTer2/internal/paths"
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// GUIConfig holds the TUI configuration settings.
type GUIConfig struct {
	Borders        bool
	LineCharacters bool
	Shadow         bool
	Scrollbar      bool
	Theme          string
	ConfigFolder   string
	ComposeFolder  string
}

// LoadGUIConfig reads the dockstarter2.ini file and returns the configuration.
func LoadGUIConfig() GUIConfig {
	conf := GUIConfig{
		Borders:        true,
		LineCharacters: true,
		Shadow:         true,
		Scrollbar:      true,
		Theme:          "DockSTARTer",
		ConfigFolder:   "${XDG_CONFIG_HOME}",
		ComposeFolder:  "${XDG_CONFIG_HOME}/compose",
	}

	path := paths.GetConfigFilePath()
	file, err := os.Open(path)
	if err != nil {
		// If file doesn't exist, create it with defaults
		if os.IsNotExist(err) {
			SaveGUIConfig(conf)
		}
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
			conf.ConfigFolder = value
		case "ComposeFolder":
			conf.ComposeFolder = value
		}
	}

	return conf
}

// SaveGUIConfig writes the configuration to dockstarter2.ini.
func SaveGUIConfig(conf GUIConfig) error {
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
