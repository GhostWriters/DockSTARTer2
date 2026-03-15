package appenv

import (
	"regexp"
	"strings"
)

// VarOption represents a single predefined selectable value for a variable,
// ported from DockSTARTer's menu_value_prompt.sh option tables.
type VarOption struct {
	Display string // Human-readable label (e.g. "Enabled")
	Value   string // Actual value to set, unquoted (e.g. "true")
	Help    string // Optional short help text
}

// portRegexp matches variables like APPNAME__PORT_0, APPNAME__PORT_1, etc.
var portRegexp = regexp.MustCompile(`^[A-Z0-9_]+__PORT_[0-9]+$`)

// GetVarOptions returns the predefined selectable options for a variable.
//
// varName is the full variable name (e.g. "WATCHTOWER__ENABLED", "DOCKER_VOLUME_STORAGE").
// appName is the uppercase app-name prefix (e.g. "WATCHTOWER"), or "" for global variables.
// defaultValue is the built-in default from builtinDefaults, or "" if unknown.
//
// Returns nil when no predefined options exist (the caller should allow free-form editing).
// Always includes a "Default Value" option when defaultValue is non-empty.
func GetVarOptions(varName, appName, defaultValue string) []VarOption {
	var opts []VarOption

	if appName != "" {
		// --- App-specific variables ---
		switch {
		case varName == appName+"__ENABLED":
			opts = append(opts,
				VarOption{Display: "Enabled", Value: "true",
					Help: "Enable this application."},
				VarOption{Display: "Disabled", Value: "false",
					Help: "Disable this application."},
			)

		case varName == appName+"__NETWORK_MODE":
			opts = append(opts,
				VarOption{Display: "Bridge Network", Value: "bridge",
					Help: "Connect to the internal Docker bridge network (same as leaving value empty)."},
				VarOption{Display: "Host Network", Value: "host",
					Help: "Connect to the host OS network stack."},
				VarOption{Display: "No Network", Value: "none",
					Help: "Leave the container without a network connection."},
				VarOption{Display: "Use Gluetun", Value: "service:gluetun",
					Help: "Route traffic through the Gluetun VPN container."},
				VarOption{Display: "Use PrivoxyVPN", Value: "service:privoxyvpn",
					Help: "Route traffic through the PrivoxyVPN container."},
			)

		case varName == appName+"__RESTART":
			opts = append(opts,
				VarOption{Display: "Unless Stopped", Value: "unless-stopped",
					Help: "Restart unless manually stopped (recommended)."},
				VarOption{Display: "Never Restart", Value: "no",
					Help: "Never automatically restart."},
				VarOption{Display: "Always Restart", Value: "always",
					Help: "Always restart, even after a manual stop."},
				VarOption{Display: "On Failure", Value: "on-failure",
					Help: "Restart only when the container exits with a non-zero code."},
			)

		case strings.HasPrefix(varName, appName+"__VOLUME_"):
			// Volume paths: only offer Default

		case portRegexp.MatchString(varName):
			// Ports: only offer Default

		case varName == appName+"__TAG":
			// Tag: only offer Default

		default:
			// Other app variables: only Default
		}

	} else {
		// --- Global variables ---
		switch varName {
		case "DOCKER_VOLUME_CONFIG":
			opts = append(opts,
				VarOption{Display: "Home Folder", Value: "~/.config/appdata",
					Help: "Store config data inside your home directory."},
			)

		case "DOCKER_VOLUME_STORAGE":
			opts = append(opts,
				VarOption{Display: "Home Folder", Value: "~/storage",
					Help: "Store storage data inside your home directory."},
				VarOption{Display: "Mount Folder", Value: "/mnt/storage",
					Help: "Store storage data at /mnt/storage."},
			)

		// System-detected globals — no curated value list; only Default is useful.
		case "DOCKER_GID", "DOCKER_HOSTNAME", "GLOBAL_LAN_NETWORK", "PGID", "PUID", "TZ":
			// Only Default offered

		default:
			// Other globals: only Default
		}
	}

	// Append "Default Value" option when a default is known.
	if defaultValue != "" {
		opts = append(opts, VarOption{
			Display: "Default Value",
			Value:   defaultValue,
			Help:    "Restore the built-in default value.",
		})
	}

	return opts
}
