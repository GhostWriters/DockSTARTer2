package env

import (
	"fmt"
	"regexp"
	"strings"
)

// VarNameToAppName returns the DS application name based on the variable name passed.
// Mirrors varname_to_appname.sh functionality.
//
// The appname will be at the beginning of the variable, and will be in upper case.
// The appname will either be a single alphanumeric word beginning with a letter,
// or two words split by a double underscore.
// The end of the appname will be followed by a double underscore and a word.
//
// Variable names that do not match these conditions will return an empty string.
//
// Examples:
//
//	SONARR__CONTAINER_NAME            -> SONARR
//	SONARR__4K__CONTAINER_NAME        -> SONARR__4K
//	SONARR__4K__CONTAINER_NAME__TEST  -> SONARR__4K
//	DOCKER_VOLUME_STORAGE             -> "" (no double underscore)
//	4RADARR__ANIME__VAR               -> "" (starts with number)
//
// This is a pure extraction function - it does NOT validate if the app name is valid.
// Use AppNameIsValid for validation.
func VarNameToAppName(varName string) string {
	if !strings.Contains(varName, "__") {
		return ""
	}

	// Bash regex: ^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?(?=__[A-Za-z0-9])
	// Since Go doesn't support lookahead, we include the trailing __ in the match
	re := regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:__[A-Z0-9]+)?)__[A-Za-z0-9]`)
	matches := re.FindStringSubmatch(varName)

	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// AppNameToInstanceName extracts the instance suffix from an app name.
// Mirrors appname_to_instancename.sh functionality.
//
// Examples:
//
//	RADARR         -> ""
//	RADARR__4K     -> "4K"
//	RADARR__4K__EXTRA -> "4K__EXTRA" (everything after first __)
func AppNameToInstanceName(appName string) string {
	if !strings.Contains(appName, "__") {
		return ""
	}

	// Bash: ${AppName#*__} - removes shortest match from beginning
	idx := strings.Index(appName, "__")
	if idx >= 0 && idx+2 < len(appName) {
		return appName[idx+2:]
	}

	return ""
}

// AppNameIsValid checks if an app name is valid.
// Mirrors appname_is_valid.sh functionality.
//
// An app name is valid if:
// 1. It matches the pattern ^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$
// 2. If it has an instance name, that instance is not in the invalid list
//
// Invalid instance names are reserved variable suffixes like CONTAINER, ENABLED, TAG, etc.
func AppNameIsValid(appName string) bool {
	// Strip trailing/leading colons (Bash compatibility)
	appName = strings.TrimPrefix(appName, ":")
	appName = strings.TrimSuffix(appName, ":")

	// Check pattern BEFORE converting to uppercase
	// The pattern requires uppercase, so lowercase input should fail
	re := regexp.MustCompile(`^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$`)
	if !re.MatchString(appName) {
		return false
	}

	// Now convert to uppercase for instance validation
	appNameUpper := strings.ToUpper(appName)

	// Check instance name if present
	instanceName := AppNameToInstanceName(appNameUpper)
	if instanceName != "" {
		return InstanceNameIsValid(instanceName)
	}

	return true
}

// InstanceNameIsValid checks if an instance name is allowed.
// Based on appname_is_valid.sh blacklist.
//
// These names are reserved for variable suffixes and cannot be used as instance names.
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

	return !invalidNames[strings.ToUpper(name)]
}

// IsGlobalVar checks if a variable name is a global variable (not app-specific).
// Based on appvars_lines.sh logic for filtering global variables.
//
// A global variable does NOT match the pattern: [A-Z][A-Z0-9]*(__[A-Z0-9]+)+\w+
// Examples:
//
//	PUID         -> true (global)
//	TZ           -> true (global)
//	RADARR__ENABLED -> false (app var)
//	RADARR__4K__ENABLED -> false (app var)
func IsGlobalVar(varName string) bool {
	// Pattern from Bash: [A-Z][A-Z0-9]*(__[A-Z0-9]+)+\w+
	// This matches app variables (must have at least one __ followed by more content)
	appVarPattern := regexp.MustCompile(`^[A-Z][A-Z0-9]*(__[A-Z0-9]+)+\w+`)
	return !appVarPattern.MatchString(varName)
}

// AppVarsLines filters environment variable lines to only those belonging to the specified app.
// Based on appvars_lines.sh regex logic.
//
// The Bash function uses negative lookahead: ${APPNAME}__(?![A-Za-z0-9]+__)\\w+
// Since Go doesn't support lookahead, we match and then manually filter.
//
// Examples for appName="RADARR":
//
//	RADARR__ENABLED='true'        -> included
//	RADARR__PORT_7878='7878'      -> included
//	RADARR__4K__ENABLED='true'    -> excluded (belongs to RADARR__4K instance)
//
// Examples for appName="RADARR__4K":
//
//	RADARR__4K__ENABLED='true'    -> included
//	RADARR__4K__PORT_7878='7878'  -> included
//	RADARR__ENABLED='true'        -> excluded (belongs to RADARR base)
func AppVarsLines(appName string, lines []string) []string {
	if appName == "" {
		// Return all global variables
		var globals []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Extract variable name
			matches := regexp.MustCompile(`^([A-Za-z0-9_]+)=`).FindStringSubmatch(line)
			if len(matches) > 1 {
				varName := matches[1]
				if IsGlobalVar(varName) {
					globals = append(globals, line)
				}
			}
		}
		return globals
	}

	// Match variables that start with APPNAME__
	// Then manually filter out those that have another __ segment (nested instances)
	pattern := fmt.Sprintf(`^\s*(%s__\w+)\s*=`, regexp.QuoteMeta(appName))
	re := regexp.MustCompile(pattern)

	var appVars []string
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			varName := matches[1]
			// Check if this variable belongs to a nested instance
			// by seeing if there's another __ after removing APPNAME__
			suffix := strings.TrimPrefix(varName, appName+"__")
			// If suffix contains __ followed by alphanumeric, it's a nested instance
			if regexp.MustCompile(`^[A-Za-z0-9]+__`).MatchString(suffix) {
				// This is a nested instance like RADARR__4K__ENABLED when appName is RADARR
				continue
			}
			appVars = append(appVars, strings.TrimSpace(line))
		}
	}

	return appVars
}
