package commands

import "github.com/spf13/pflag"

// NewFlagSet returns a fresh, isolated FlagSet with all ds2 flags registered.
// Each call to Parse gets its own FlagSet so console parses never share state
// with the global pflag.CommandLine or with each other.
func NewFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("ds2", pflag.ContinueOnError)

	// Modifiers
	fs.BoolP("force", "f", false, "Force execution")
	fs.BoolP("gui", "g", false, "Show GUI")
	fs.BoolP("verbose", "v", false, "Verbose output")
	fs.BoolP("debug", "x", false, "Debug output")
	fs.BoolP("yes", "y", false, "Assume yes")
	fs.BoolP("help", "h", false, "Show help")

	// App Management
	fs.StringP("add", "a", "", "Add application(s)")
	fs.StringP("remove", "r", "", "Remove application(s)")
	fs.StringP("select", "S", "", "Select apps (Application Selection menu)")
	fs.StringP("status", "s", "", "Application status")
	fs.String("status-enable", "", "Enable app(s)")
	fs.String("status-disable", "", "Disable app(s)")

	// Listing
	fs.BoolP("list", "l", false, "List all apps")
	fs.Bool("list-added", false, "List added apps")
	fs.Bool("list-builtin", false, "List builtin apps")
	fs.Bool("list-deprecated", false, "List deprecated apps")
	fs.Bool("list-enabled", false, "List enabled apps")
	fs.Bool("list-disabled", false, "List disabled apps")
	fs.Bool("list-nondeprecated", false, "List nondeprecated apps")
	fs.Bool("list-referenced", false, "List referenced apps")

	// Docker Compose
	fs.StringP("compose", "c", "", "Docker Compose operations (up, down, pull, etc.)")
	fs.StringP("prune", "p", "", "Prune docker resources")

	// Installation / Update
	fs.BoolP("install", "i", false, "Install/update dependencies")
	fs.StringP("update", "u", "", "Update DockSTARTer and Templates (can specify tag/branch)")
	fs.String("update-app", "", "Update DockSTARTer only (can specify tag)")
	fs.String("update-templates", "", "Update Templates only (can specify tag/branch)")
	fs.StringP("version", "V", "", "Show version")
	fs.BoolP("reset", "R", false, "Reset DockSTARTer to process all actions")

	// Environment Variables
	fs.BoolP("env", "e", false, "Update .env files")
	fs.String("env-appvars", "", "List variable names for app")
	fs.String("env-appvars-lines", "", "List variable lines for app")
	fs.String("env-get", "", "Get variable value")
	fs.String("env-get-line", "", "Get variable line")
	fs.String("env-get-literal", "", "Get variable literal value")
	fs.String("env-get-lower", "", "Get variable value (lowercase)")
	fs.String("env-get-lower-line", "", "Get variable line (lowercase)")
	fs.String("env-get-lower-literal", "", "Get variable literal value (lowercase)")
	fs.String("env-set", "", "Set variable value")
	fs.String("env-set-literal", "", "Set variable literal value")
	fs.String("env-set-lower", "", "Set variable value (lowercase)")
	fs.String("env-set-lower-literal", "", "Set variable literal value (lowercase)")
	fs.String("env-edit", "", "Open the value picker TUI for a variable (APP:VAR or VAR)")
	fs.String("env-edit-lower", "", "Open the value picker TUI for a variable (preserve case)")

	// Editor
	fs.Bool("edit-global", false, "Open the global environment variables editor")
	fs.Bool("start-edit-global", false, "Open the global environment variables editor (restore nav stack)")
	fs.String("edit-app", "", "Open the environment variables editor for a specific app")
	fs.String("start-edit-app", "", "Open the environment variables editor for a specific app (restore nav stack)")

	// Configuration / Menu
	fs.StringP("menu", "M", "", "Show menu (main, config, options, etc.)")
	fs.String("config-pm", "", "Config package manager")
	fs.Bool("config-pm-auto", false, "Auto-detect package manager")
	fs.Bool("config-show", false, "Show configuration")
	fs.String("config-folder", "", "Set config folder path")
	fs.String("config-compose-folder", "", "Set compose folder path")
	fs.Bool("show-config", false, "Show configuration (alias)")

	// Theme
	fs.StringP("theme", "T", "", "Set theme by name or import a .ds2theme file path")
	fs.Bool("theme-list", false, "List themes")
	fs.Bool("theme-table", false, "List themes table")
	fs.Bool("theme-lines", false, "Turn line drawing characters on")
	fs.Bool("theme-no-lines", false, "Turn line drawing characters off")
	fs.Bool("theme-line", false, "Turn line drawing characters on")
	fs.Bool("theme-no-line", false, "Turn line drawing characters off")
	fs.Bool("theme-borders", false, "Turn borders on")
	fs.Bool("theme-no-borders", false, "Turn borders off")
	fs.Bool("theme-border", false, "Turn borders on")
	fs.Bool("theme-no-border", false, "Turn borders off")
	fs.Bool("theme-button-borders", false, "Turn button borders on")
	fs.Bool("theme-no-button-borders", false, "Turn button borders off")
	fs.Bool("theme-shadows", false, "Turn shadows on")
	fs.Bool("theme-no-shadows", false, "Turn shadows off")
	fs.Bool("theme-shadow", false, "Turn shadows on")
	fs.Bool("theme-no-shadow", false, "Turn shadows off")
	fs.String("theme-shadow-level", "", "Set shadow level (0-4 or off/light/medium/dark/solid)")
	fs.Bool("theme-scrollbar", false, "Turn scrollbar on")
	fs.Bool("theme-no-scrollbar", false, "Turn scrollbar off")
	fs.String("theme-border-color", "", "Set border color (1=Border, 2=Border2, 3=Both)")
	fs.String("theme-dialog-title", "", "Set dialog title alignment (left/center)")
	fs.String("theme-submenu-title", "", "Set submenu title alignment (left/center)")
	fs.String("theme-log-title", "", "Set log title alignment (left/center)")
	fs.String("theme-extract", "", "Extract a theme to a directory for customization")
	fs.Bool("theme-extract-all", false, "Extract all embedded themes to a directory")

	// Testing
	fs.StringP("test", "t", "", "Run test script")
	fs.String("man", "", "Show documentation for application")

	// Server
	fs.String("server", "", "Server management (status, start, stop, restart, disconnect, install, uninstall, enable, disable)")

	// Internal — used by --server start to re-exec as a background daemon.
	fs.Bool("server-daemon", false, "")
	_ = fs.MarkHidden("server-daemon")

	return fs
}

// InitFlags registers all ds2 flags on the global pflag.CommandLine.
// Called once at startup by the cmd package for CLI flag parsing.
func InitFlags() {
	fs := NewFlagSet()
	fs.VisitAll(func(f *pflag.Flag) {
		if pflag.CommandLine.Lookup(f.Name) == nil {
			pflag.CommandLine.AddFlag(f)
		}
	})
}
