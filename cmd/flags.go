package cmd

import (
	"github.com/spf13/pflag"
)

// InitFlags defines the pflags used for argument validation and help.
func InitFlags() {
	// Modifiers
	pflag.BoolP("force", "f", false, "Force execution")
	pflag.BoolP("gui", "g", false, "Show GUI")
	pflag.BoolP("verbose", "v", false, "Verbose output")
	pflag.BoolP("debug", "x", false, "Debug output")
	pflag.BoolP("yes", "y", false, "Assume yes")
	pflag.BoolP("help", "h", false, "Show help")

	// App Management
	pflag.StringP("add", "a", "", "Add application(s)")
	pflag.StringP("remove", "r", "", "Remove application(s)")
	pflag.StringP("select", "S", "", "Select apps (Application Selection menu)")
	pflag.StringP("status", "s", "", "Application status")
	pflag.String("status-enable", "", "Enable app(s)")
	pflag.String("status-disable", "", "Disable app(s)")

	// Listing
	pflag.BoolP("list", "l", false, "List all apps")
	pflag.Bool("list-added", false, "List added apps")
	pflag.Bool("list-builtin", false, "List builtin apps")
	pflag.Bool("list-deprecated", false, "List deprecated apps")
	pflag.Bool("list-enabled", false, "List enabled apps")
	pflag.Bool("list-disabled", false, "List disabled apps")
	pflag.Bool("list-nondeprecated", false, "List nondeprecated apps")
	pflag.Bool("list-referenced", false, "List referenced apps")

	// Docker Compose
	pflag.StringP("compose", "c", "", "Docker Compose operations (up, down, pull, etc.)")
	pflag.StringP("prune", "p", "", "Prune docker resources")

	// Installation / Update
	pflag.BoolP("install", "i", false, "Install/update dependencies")
	pflag.StringP("update", "u", "", "Update DockSTARTer and Templates (can specify tag/branch)")
	pflag.String("update-app", "", "Update DockSTARTer only (can specify tag)")
	pflag.String("update-templates", "", "Update Templates only (can specify tag/branch)")
	pflag.StringP("version", "V", "", "Show version")
	pflag.BoolP("reset", "R", false, "Reset DockSTARTer to process all actions")
	// Environment Variables
	pflag.BoolP("env", "e", false, "Update .env files")
	pflag.String("env-appvars", "", "List variable names for app")
	pflag.String("env-appvars-lines", "", "List variable lines for app")
	pflag.String("env-get", "", "Get variable value")
	pflag.String("env-get-line", "", "Get variable line")
	pflag.String("env-get-literal", "", "Get variable literal value")
	pflag.String("env-get-lower", "", "Get variable value (lowercase)")
	pflag.String("env-get-lower-line", "", "Get variable line (lowercase)")
	pflag.String("env-get-lower-literal", "", "Get variable literal value (lowercase)")
	pflag.String("env-set", "", "Set variable value")
	pflag.String("env-set-literal", "", "Set variable literal value")
	pflag.String("env-set-lower", "", "Set variable value (lowercase)")
	pflag.String("env-set-lower-literal", "", "Set variable literal value (lowercase)")

	// Configuration / Menu
	pflag.StringP("menu", "M", "", "Show menu (main, config, options, etc.)")
	pflag.String("config-pm", "", "Config package manager")
	pflag.Bool("config-pm-auto", false, "Auto-detect package manager")
	pflag.Bool("config-show", false, "Show configuration")
	pflag.String("config-folder", "", "Set config folder path")
	pflag.String("config-compose-folder", "", "Set compose folder path")
	pflag.Bool("show-config", false, "Show configuration (alias)")

	// Theme
	pflag.StringP("theme", "T", "", "Theme operations")
	pflag.Bool("theme-list", false, "List themes")
	pflag.Bool("theme-table", false, "List themes table")
	pflag.Bool("theme-lines", false, "Turn line drawing characters on")
	pflag.Bool("theme-no-lines", false, "Turn line drawing characters off")
	pflag.Bool("theme-line", false, "Turn line drawing characters on")
	pflag.Bool("theme-no-line", false, "Turn line drawing characters off")
	pflag.Bool("theme-borders", false, "Turn borders on")
	pflag.Bool("theme-no-borders", false, "Turn borders off")
	pflag.Bool("theme-border", false, "Turn borders on")
	pflag.Bool("theme-no-border", false, "Turn borders off")
	pflag.Bool("theme-shadows", false, "Turn shadows on")
	pflag.Bool("theme-no-shadows", false, "Turn shadows off")
	pflag.Bool("theme-shadow", false, "Turn shadows on")
	pflag.Bool("theme-no-shadow", false, "Turn shadows off")
	pflag.String("theme-shadow-level", "", "Set shadow level (0-4 or off/light/medium/dark/solid)")
	pflag.Bool("theme-scrollbar", false, "Turn scrollbar on")
	pflag.Bool("theme-no-scrollbar", false, "Turn scrollbar off")
	pflag.String("theme-border-color", "", "Set border color (1=Border, 2=Border2, 3=Both)")
	pflag.String("theme-dialog-title", "", "Set dialog title alignment (left/center)")
	pflag.String("theme-submenu-title", "", "Set submenu title alignment (left/center)")
	pflag.String("theme-log-title", "", "Set log title alignment (left/center)")

	// Testing
	pflag.StringP("test", "t", "", "Run test script")
}
