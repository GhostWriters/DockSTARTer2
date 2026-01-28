package env

import "strings"

// VarNameToAppName returns the DS application name based on the variable name passed.
// Mirrors varname_to_appname.sh functionality.
//
// Logic:
// The appname will be at the beginning of the variable, and will be in upper case.
// It matches the first alphanumeric word, optionally followed by "__" and another word.
//
// Examples:
// SONARR__CONTAINER_NAME            -> SONARR
// SONARR__4K__CONTAINER_NAME        -> SONARR__4K
// DOCKER_VOLUME_STORAGE             -> ""
func VarNameToAppName(key string) string {
	if !strings.Contains(key, "__") {
		return ""
	}

	parts := strings.Split(key, "__")
	if len(parts) >= 3 {
		// e.g. RADARR__4K__ENABLED -> RADARR__4K
		return parts[0] + "__" + parts[1]
	} else if len(parts) == 2 {
		// e.g. RADARR__ENABLED -> RADARR
		return parts[0]
	}

	return ""
}
