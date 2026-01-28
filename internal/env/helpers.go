package env

import (
	"regexp"
	"strings"
)

// VarNameToAppName returns the DS application name based on the variable name passed.
// Mirrors varname_to_appname.sh functionality.
//
// Logic:
// The appname will be at the beginning of the variable, and will be in upper case.
// It matches the first alphanumeric word, optionally followed by "__" and another word.
// The end of the appname will be followed by a double underscore and a word.
// Use Regex equivalent to: ^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?(?=__[A-Za-z0-9])
// Since Go doesn't support lookahead, we match the suffix group but extract only the prefix.
func VarNameToAppName(key string) string {
	if !strings.Contains(key, "__") {
		return ""
	}

	// Pattern: Start + (BaseApp) + Optional(__Instance) + Must be followed by (__VarStart)
	// BaseApp: [A-Z][A-Z0-9]*
	// Instance: __[A-Z0-9]+
	// VarStart: __[A-Za-z0-9]
	// We use capturing group for the App Name part.
	re := regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:__[A-Z0-9]+)?)__[A-Za-z0-9]`)

	matches := re.FindStringSubmatch(key)
	if len(matches) > 1 {
		appName := matches[1]
		// Check for multiple underscores to identify instance
		if strings.Contains(appName, "__") {
			parts := strings.Split(appName, "__")
			instance := parts[1]
			if !InstanceNameIsValid(instance) {
				// Fallback to base app name
				return parts[0]
			}
		}
		return appName
	}

	return ""
}

// InstanceNameIsValid checks if the instance name is allowed.
// Based on appname_is_valid.sh blacklist.
func InstanceNameIsValid(name string) bool {
	invalidNames := map[string]bool{
		"CONTAINER":   true,
		"DEVICE":      true,
		"DEVICES":     true,
		"ENABLED":     true,
		"ENVIRONMENT": true,
		"HOSTNAME":    true,
		"PORT":        true,
		"NETWORK":     true,
		"RESTART":     true,
		"STORAGE":     true,
		"STORAGE2":    true,
		"STORAGE3":    true,
		"STORAGE4":    true,
		"TAG":         true,
	}
	return !invalidNames[name]
}
