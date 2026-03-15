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
		helpText: "This should be the {{|Theme_Highlight|}}Docker group ID{{[-]}}.\nIf you are unsure, check your system's group configuration.",
		helpLine: "The Docker group ID on your system.",
	},
	"DOCKER_HOSTNAME": {
		helpText: "This should be your {{|Theme_Highlight|}}system hostname{{[-]}}.\nIf you are unsure, check your system configuration.",
		helpLine: "Your system hostname.",
	},
	"DOCKER_VOLUME_CONFIG": {
		helpText: "The path where application {{|Theme_Highlight|}}config data{{[-]}} is stored.",
		helpLine: "Path to the application config data directory.",
	},
	"DOCKER_VOLUME_STORAGE": {
		helpText: "The path where application {{|Theme_Highlight|}}storage data{{[-]}} is stored.",
		helpLine: "Path to the application storage data directory.",
	},
	"GLOBAL_LAN_NETWORK": {
		helpText: "This is used to define your home LAN network.\nDo NOT confuse this with the IP address of your router or your server —\nthe value for this key defines your {{|Theme_Highlight|}}network{{[-]}}, NOT a single host.\nSee CIDR Notation for more information (e.g. {{|Theme_Highlight|}}192.168.1.0/24{{[-]}}).",
		helpLine: "Your home LAN network in CIDR notation (e.g. 192.168.1.0/24).",
	},
	"PGID": {
		helpText: "This should be your {{|Theme_Highlight|}}user group ID{{[-]}}.\nIf you are unsure, check your system's user configuration.",
		helpLine: "Your user group ID.",
	},
	"PUID": {
		helpText: "This should be your {{|Theme_Highlight|}}user account ID{{[-]}}.\nIf you are unsure, check your system's user configuration.",
		helpLine: "Your user account ID.",
	},
	"TZ": {
		helpText: "If this is not the correct timezone, please exit and set your {{|Theme_Highlight|}}system timezone{{[-]}} first.",
		helpLine: "Your system timezone (e.g. America/New_York).",
	},
}

// appVarSuffixHelp maps known APP variable suffixes to their help entries.
// The suffix is the part after APPNAME__ (uppercased).
// Sourced from DockSTARTer scripts/menu_value_prompt.sh (APP case).
var appVarSuffixHelp = map[string]varHelpEntry{
	"ENABLED": {
		helpText: "Used to set the application as enabled or disabled.\nIf this variable is removed, the application will not be controlled by DockSTARTer.\nMust be {{|Theme_Highlight|}}true{{[-]}} or {{|Theme_Highlight|}}false{{[-]}}.",
		helpLine: "Enable or disable this application (true/false).",
	},
	"NETWORK_MODE": {
		helpText: "Network Mode is usually left blank but can also be\n{{|Theme_Highlight|}}bridge{{[-]}}, {{|Theme_Highlight|}}host{{[-]}}, {{|Theme_Highlight|}}none{{[-]}}, {{|Theme_Highlight|}}service:<appname>{{[-]}}, or {{|Theme_Highlight|}}container:<appname>{{[-]}}.",
		helpLine: "Docker network mode (blank, bridge, host, none, service:X, container:X).",
	},
	"RESTART": {
		helpText: "Restart is usually {{|Theme_Highlight|}}unless-stopped{{[-]}} but can also be\n{{|Theme_Highlight|}}no{{[-]}}, {{|Theme_Highlight|}}always{{[-]}}, or {{|Theme_Highlight|}}on-failure{{[-]}}.",
		helpLine: "Container restart policy (unless-stopped, no, always, on-failure).",
	},
	"TAG": {
		helpText: "Tag is usually {{|Theme_Highlight|}}latest{{[-]}} but can also be other values based on the image.",
		helpLine: "Docker image tag (usually latest).",
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
			helpText: "Must be an unused port between {{|Theme_Highlight|}}0{{[-]}} and {{|Theme_Highlight|}}65535{{[-]}}.",
			helpLine: "A port number between 0 and 65535.",
		}, true
	}

	return varHelpEntry{}, false
}
