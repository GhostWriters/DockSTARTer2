package appenv

import (
	"regexp"
	"strings"
)

// VarOption represents a single predefined selectable value for a variable,
// ported from DockSTARTer's menu_value_prompt.sh option tables.
// TOML tags allow loading options from appname.meta.toml files.
type VarOption struct {
	Display string `toml:"label"`  // Human-readable label (e.g. "Enabled")
	Value   string `toml:"value"`  // Actual value to set, quoted (e.g. "'true'")
	Help    string `toml:"help"`   // Optional short help text shown in the helpline when focused
}

// portRegexp matches variables like APPNAME__PORT_0, APPNAME__PORT_1, etc.
var portRegexp = regexp.MustCompile(`^[A-Z0-9_]+__PORT_[0-9]+$`)

// systemDetectedVars are variables whose default is determined by probing the
// running system (hostname, group IDs, LAN network, timezone). The context
// menu labels these "System Value" rather than "Default Value".
var systemDetectedVars = map[string]bool{
	"DOCKER_GID":         true,
	"DOCKER_HOSTNAME":    true,
	"GLOBAL_LAN_NETWORK": true,
	"PGID":               true,
	"PUID":               true,
	"TZ":                 true,
}

// GetVarOptions returns the predefined selectable options for a variable.
//
// varName         is the full variable name (e.g. "WATCHTOWER__ENABLED").
// appName         is the uppercase app-name prefix (e.g. "WATCHTOWER"), or "" for global variables.
// computedDefault is the pre-computed quoted default value from builtinDefaults
//
//	(e.g. "'false'", "'192.168.1.0/24'"). Pass "" when unknown.
//
// The first returned option (when computedDefault is meaningful) is labelled
// "Default Value" or "System Value" to match the bash menu_value_prompt.sh wording.
// Subsequent options are curated alternatives.
// Returns nil when no options exist (caller should allow free-form editing only).
func GetVarOptions(varName, appName, computedDefault string) []VarOption {
	var opts []VarOption

	// --- Prepend Default / System Value option ---
	if computedDefault != "" {
		label := "Default Value"
		help := "This is the recommended default value."
		if systemDetectedVars[varName] {
			label = "System Value"
			help = "This is the recommended system detected value."
		}
		opts = append(opts, VarOption{Display: label, Value: computedDefault, Help: help})
	}

	if appName != "" {
		// --- App-specific variables ---
		switch {
		case varName == appName+"__ENABLED":
			if len(opts) > 0 {
				opts[0].Help = "Must be true or false. Controls whether DockSTARTer manages this application."
			}
			opts = append(opts,
				VarOption{Display: "Enabled", Value: "'true'",
					Help: "Enable this application."},
				VarOption{Display: "Disabled", Value: "'false'",
					Help: "Disable this application."},
			)

		case varName == appName+"__NETWORK_MODE":
			if len(opts) > 0 {
				opts[0].Help = "Usually left blank (bridge). Can also be host, none, service:<appname>, or container:<appname>."
			}
			opts = append(opts,
				VarOption{Display: "Bridge Network", Value: "'bridge'",
					Help: "Connects to the internal Docker bridge network (same as leaving the value empty)."},
				VarOption{Display: "Host Network", Value: "'host'",
					Help: "Connects to the host OS's network stack."},
				VarOption{Display: "No Network", Value: "'none'",
					Help: "Leaves the container without a network connection."},
				VarOption{Display: "Use Gluetun", Value: "'service:gluetun'",
					Help: "Routes traffic through the Gluetun VPN container if running."},
				VarOption{Display: "Use PrivoxyVPN", Value: "'service:privoxyvpn'",
					Help: "Routes traffic through the PrivoxyVPN container if running."},
			)

		case varName == appName+"__RESTART":
			if len(opts) > 0 {
				opts[0].Help = "Usually unless-stopped. Can also be no, always, or on-failure."
			}
			opts = append(opts,
				VarOption{Display: "Unless Stopped", Value: "'unless-stopped'",
					Help: "Restart unless the container was manually stopped (recommended)."},
				VarOption{Display: "Never Restart", Value: "'no'",
					Help: "Never automatically restart."},
				VarOption{Display: "Always Restart", Value: "'always'",
					Help: "Always restart, even after a manual stop."},
				VarOption{Display: "On Failure", Value: "'on-failure'",
					Help: "Restart only when the container exits with a non-zero code."},
			)

		case varName == appName+"__TAG":
			if len(opts) > 0 {
				opts[0].Help = "Usually latest, but can also be another tag based on the image."
			}

		case strings.HasPrefix(varName, appName+"__VOLUME_"):
			if len(opts) > 0 {
				opts[0].Help = "If the directory does not exist, an attempt will be made to create it."
			}

		case portRegexp.MatchString(varName):
			if len(opts) > 0 {
				opts[0].Help = "Must be an unused port between 0 and 65535."
			}

		default:
			// Other app variables: only Default Value (already prepended above)
		}

	} else {
		// --- Global variables ---
		switch varName {
		case "DOCKER_GID":
			if len(opts) > 0 {
				opts[0].Help = "This should be the Docker group ID."
			}

		case "DOCKER_HOSTNAME":
			if len(opts) > 0 {
				opts[0].Help = "This should be your system hostname."
			}

		case "DOCKER_VOLUME_CONFIG":
			opts = append(opts,
				VarOption{Display: "Home Folder", Value: "'~/.config/appdata'",
					Help: "Store config data inside your home directory."},
			)

		case "DOCKER_VOLUME_STORAGE":
			opts = append(opts,
				VarOption{Display: "Home Folder", Value: "'~/storage'",
					Help: "Store storage data inside your home directory."},
				VarOption{Display: "Mount Folder", Value: "'/mnt/storage'",
					Help: "Store storage data at /mnt/storage."},
			)

		case "GLOBAL_LAN_NETWORK":
			if len(opts) > 0 {
				opts[0].Help = "Defines your home LAN network (CIDR notation). Do not confuse with your router or server IP address."
			}

		case "PGID":
			if len(opts) > 0 {
				opts[0].Help = "This should be your user group ID."
			}

		case "PUID":
			if len(opts) > 0 {
				opts[0].Help = "This should be your user account ID."
			}

		case "TZ":
			if len(opts) > 0 {
				opts[0].Help = "If this is not the correct timezone, exit and set your system timezone first."
			}

		default:
			// Other globals: only Default Value (already prepended above)
		}
	}

	if len(opts) == 0 {
		return nil
	}
	return opts
}
