package appenv

import (
	"regexp"
	"strings"
)

// varHelpEntry holds the full description and a short one-liner for a known variable.
type varHelpEntry struct {
	helpText string // Multi-line description for the help dialog
	helpLine string // Short one-liner (e.g. for status bars or inline hints)
}

// globalVarHelp maps exact GLOBAL variable names to their help entries.
// Sourced from DockSTARTer scripts/menu_value_prompt.sh (GLOBAL case).
var globalVarHelp = map[string]varHelpEntry{
	"DOCKER_GID": {
		helpText: "This should be the {{|Highlight|}}Docker group ID{{[-]}}. If you are unsure, check your system's group configuration.",
		helpLine: "The {{|Highlight|}}Docker group ID{{[-]}} on your system.",
	},
	"DOCKER_HOSTNAME": {
		helpText: "This should be your {{|Highlight|}}system hostname{{[-]}}. If you are unsure, check your system configuration.",
		helpLine: "Your {{|Highlight|}}system hostname{{[-]}}.",
	},
	"DOCKER_VOLUME_CONFIG": {
		helpText: "The path where application {{|Highlight|}}config data{{[-]}} is stored.",
		helpLine: "Path to the application {{|Highlight|}}config data{{[-]}} directory.",
	},
	"DOCKER_VOLUME_STORAGE": {
		helpText: "The path where application {{|Highlight|}}storage data{{[-]}} is stored.",
		helpLine: "Path to the application {{|Highlight|}}storage data{{[-]}} directory.",
	},
	"GLOBAL_LAN_NETWORK": {
		helpText: "This is used to define your home LAN network. Do NOT confuse this with the IP address of your router or your server — the value for this key defines your {{|Highlight|}}network{{[-]}}, NOT a single host. See CIDR Notation for more information (e.g. {{|Highlight|}}192.168.1.0/24{{[-]}}).",
		helpLine: "Your home LAN network in CIDR notation (e.g. {{|Highlight|}}192.168.1.0/24{{[-]}}).",
	},
	"PGID": {
		helpText: "This should be your {{|Highlight|}}user group ID{{[-]}}. If you are unsure, check your system's user configuration.",
		helpLine: "Your {{|Highlight|}}user group ID{{[-]}}.",
	},
	"PUID": {
		helpText: "This should be your {{|Highlight|}}user account ID{{[-]}}. If you are unsure, check your system's user configuration.",
		helpLine: "Your {{|Highlight|}}user account ID{{[-]}}.",
	},
	"TZ": {
		helpText: "If this is not the correct timezone, please exit and set your {{|Highlight|}}system timezone{{[-]}} first.",
		helpLine: "Your {{|Highlight|}}system timezone{{[-]}} (e.g. {{|Highlight|}}America/New_York{{[-]}}).",
	},
}

// appVarSuffixHelp maps known APP variable suffixes to their help entries.
// The suffix is the part after APPNAME__ (uppercased).
// Sourced from DockSTARTer scripts/menu_value_prompt.sh (APP case).
var appVarSuffixHelp = map[string]varHelpEntry{
	"ENABLED": {
		helpText: "This is used to set the application as enabled or disabled. If this variable is removed, the application will not be controlled by DockSTARTer. Must be {{|Highlight|}}true{{[-]}} or {{|Highlight|}}false{{[-]}}.",
		helpLine: "Enable or disable this application ({{|Highlight|}}true{{[-]}}/{{|Highlight|}}false{{[-]}}).",
	},
	"NETWORK_MODE": {
		helpText: "Network Mode is usually left blank but can also be {{|Highlight|}}bridge{{[-]}}, {{|Highlight|}}host{{[-]}}, {{|Highlight|}}none{{[-]}}, {{|Highlight|}}service:<appname>{{[-]}}, or {{|Highlight|}}container:<appname>{{[-]}}.",
		helpLine: "Docker network mode (blank, {{|Highlight|}}bridge{{[-]}}, {{|Highlight|}}host{{[-]}}, {{|Highlight|}}none{{[-]}}, {{|Highlight|}}service:X{{[-]}}, {{|Highlight|}}container:X{{[-]}}).",
	},
	"RESTART": {
		helpText: "Restart is usually {{|Highlight|}}unless-stopped{{[-]}} but can also be {{|Highlight|}}no{{[-]}}, {{|Highlight|}}always{{[-]}}, or {{|Highlight|}}on-failure{{[-]}}.",
		helpLine: "Container restart policy ({{|Highlight|}}unless-stopped{{[-]}}, {{|Highlight|}}no{{[-]}}, {{|Highlight|}}always{{[-]}}, {{|Highlight|}}on-failure{{[-]}}).",
	},
	"TAG": {
		helpText: "Tag is usually {{|Highlight|}}latest{{[-]}} but can also be other values based on the image.",
		helpLine: "Docker image tag (usually {{|Highlight|}}latest{{[-]}}).",
	},
}

// portSuffixRe matches a PORT_N suffix (where N is one or more digits).
var portSuffixRe = regexp.MustCompile(`^PORT_[0-9]+$`)

// GetVarHelpText returns a multi-line description for the given variable name.
// Returns empty string if no description is available.
func GetVarHelpText(varName string) string {
	entry, ok := lookupVarHelp(varName)
	if !ok {
		return ""
	}
	return entry.helpText
}

// GetVarHelpLine returns a short one-line description for the given variable name.
// Returns empty string if no description is available.
func GetVarHelpLine(varName string) string {
	entry, ok := lookupVarHelp(varName)
	if !ok {
		return ""
	}
	return entry.helpLine
}

// lookupVarHelp finds help information for a variable name.
func lookupVarHelp(varName string) (varHelpEntry, bool) {
	upper := strings.ToUpper(varName)
	appName := VarNameToAppName(upper)

	if appName == "" {
		// GLOBAL variable — exact name match
		entry, ok := globalVarHelp[upper]
		return entry, ok
	}

	// APP variable — extract the suffix after APPNAME__
	prefix := appName + "__"
	if !strings.HasPrefix(upper, prefix) {
		return varHelpEntry{}, false
	}
	suffix := upper[len(prefix):]

	// Exact suffix match (ENABLED, NETWORK_MODE, RESTART, TAG)
	if entry, ok := appVarSuffixHelp[suffix]; ok {
		return entry, ok
	}

	// VOLUME_ prefix (e.g. VOLUME_CONFIG, VOLUME_DATA)
	if strings.HasPrefix(suffix, "VOLUME_") {
		return varHelpEntry{
			helpText: "If the directory selected does not exist, DockSTARTer will attempt to create it.",
			helpLine: "Path to a volume directory for this application.",
		}, true
	}

	// PORT_N (e.g. PORT_0, PORT_8080)
	if portSuffixRe.MatchString(suffix) {
		return varHelpEntry{
			helpText: "Must be an unused port between {{|Highlight|}}0{{[-]}} and {{|Highlight|}}65535{{[-]}}.",
			helpLine: "A port number between {{|Highlight|}}0{{[-]}} and {{|Highlight|}}65535{{[-]}}.",
		}, true
	}

	return varHelpEntry{}, false
}
