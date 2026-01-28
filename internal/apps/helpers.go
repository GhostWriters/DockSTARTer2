package apps

import (
	"strings"
)

// appname_to_baseappname extracts the base application name from a potentially instanced app name.
// Example: "RADARR__4K" returns "radarr", "RADARR" returns "radarr"
// Always returns lowercase to match Bash's 'local -l' behavior
func appname_to_baseappname(appname string) string {
	appname = strings.ToLower(appname)
	if strings.Contains(appname, "__") {
		parts := strings.Split(appname, "__")
		return parts[0]
	}
	return appname
}

// appname_to_instancename extracts the instance suffix from an instanced app name.
// Example: "RADARR__4K" returns "4k", "RADARR" returns ""
// Always returns lowercase to match Bash's 'local -l' behavior
func appname_to_instancename(appname string) string {
	appname = strings.ToLower(appname)
	if strings.Contains(appname, "__") {
		parts := strings.Split(appname, "__")
		return parts[1]
	}
	return ""
}
