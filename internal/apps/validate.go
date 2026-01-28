package apps

import (
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// IsBuiltin checks if the application has a corresponding template folder.
func IsBuiltin(appName string) bool {
	// 1. Resolve base app name (strip instance suffix)
	base := appname_to_baseappname(appName)
	base = strings.ToLower(base)

	// 2. Check templates folder
	templatesDir := paths.GetTemplatesDir()
	appDir := filepath.Join(templatesDir, ".apps", base)
	info, err := os.Stat(appDir)
	return err == nil && info.IsDir()
}

// IsAppNameValid checks if the application name is valid according to DS rules.
// Mirrors appname_is_valid.sh
func IsAppNameValid(appName string) bool {
	// 1. Upper case and Trim
	name := strings.TrimSpace(strings.ToUpper(appName))

	// 2. Strip leading/trailing colons
	if strings.HasSuffix(name, ":") {
		name = name[:len(name)-1]
	} else if strings.HasPrefix(name, ":") {
		name = name[1:]
	}

	// 3. Regex check: ^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$
	re := regexp.MustCompile(`^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$`)
	if !re.MatchString(name) {
		return false
	}

	// 4. Instance name validation
	instance := appname_to_instancename(name)
	if instance != "" {
		invalid := []string{
			"CONTAINER", "DEVICE", "DEVICES", "ENABLED", "ENVIRONMENT",
			"HOSTNAME", "PORT", "NETWORK", "RESTART", "STORAGE",
			"STORAGE2", "STORAGE3", "STORAGE4", "TAG",
		}
		instanceUpper := strings.ToUpper(instance)
		for _, inv := range invalid {
			if instanceUpper == inv {
				return false
			}
		}
	}

	return true
}

// IsDeprecated checks if an app is marked deprecated in its labels.yml.
func IsDeprecated(app string) bool {
	base := appname_to_baseappname(app)
	templatesDir := paths.GetTemplatesDir()
	labelsFile := filepath.Join(templatesDir, ".apps", strings.ToLower(base), base+".labels.yml")
	if _, err := os.Stat(labelsFile); err != nil {
		return false
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return false
	}

	re := regexp.MustCompile(`(?m)^\s*com\.dockstarter\.appinfo\.deprecated:\s*(.*)`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) > 1 {
		val := strings.Trim(matches[1], `"' `)
		return val == "true"
	}
	return false
}
