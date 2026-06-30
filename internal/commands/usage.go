package commands

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

// PrintHelp prints usage information.
// If target is empty, prints global usage.
// If target is specified, prints usage for that specific flag/command.
func PrintHelp(ctx context.Context, target string) {
	logger.Display(ctx, GetUsage(target, false))
}

// GetUsage returns usage information as a string.
// If target is empty, returns global usage.
// If target is specified, returns usage for that specific flag/command.
func GetUsage(target string, noHeading bool) string {
	var sb strings.Builder
	printStr := func(lines ...string) {
		for _, s := range lines {
			sb.WriteString(s + "\n")
		}
	}

	// Helper for header info
	appName := version.ApplicationName
	appCmd := version.CommandName

	// Track if we found a match for a specific target
	found := false

	// If target is empty, print intro
	if !noHeading {
		// Mimic the header section
		printStr(
			update.GetAppVersionDisplay(),
			update.GetTmplVersionDisplay(),
			update.GetComposeSdkVersionDisplay(),
			"",
			fmt.Sprintf("Usage: {{|UsageCommand|}}%s{{[-]}} [{{|UsageCommand|}}<Flags>{{[-]}}] [{{|UsageCommand|}}<Command>{{[-]}}] ...", appCmd),
			"",
			fmt.Sprintf("This is the main {{|ApplicationName|}}%s{{[-]}} application.", appName),
			"For regular usage you can run without providing any options.",
			"",
			"You may include multiple commands on the command-line, and they will be executed in",
			"the order given, only stopping on an error. Any flags included only apply to the",
			"following command, and get reset before the next command.",
			"",
			"Any command that takes a variable name, the variable will by default be looked for",
			"in the global '{{|UsageFile|}}.env{{[-]}}' file. If the variable name used is in form of '{{|UsageVar|}}app:var{{[-]}}', it",
			"will instead refer to the variable '{{|UsageVar|}}<var>{{[-]}}' in '{{|UsageFile|}}.env.app.<app>{{[-]}}'.  Some commands",
			"that take app names can use the form '{{|UsageApp|}}app:{{[-]}}' to refer to the same file.",
			"",
		)
		if target == "" {
			printStr(
				"Flags:",
				"",
			)
		}
	}

	// Flags section and Command section.
	showAll := target == ""

	match := func(opts ...string) bool {
		if showAll {
			return true
		}
		for _, o := range opts {
			if o == target {
				found = true
				return true
			}
		}
		return false
	}

	// Flags
	if match("-f", "--force") {
		printStr(
			"{{|UsageCommand|}}-f --force{{[-]}}",
			"	Force certain install/upgrade actions to run even if they would not be needed.",
		)
	}
	if match("-g", "--gui") {
		printStr(
			"{{|UsageCommand|}}-g --gui{{[-]}}",
			"	Use dialog boxes",
		)
	}
	if match("-v", "--verbose") {
		printStr(
			"{{|UsageCommand|}}-v --verbose{{[-]}}",
			"	Verbose",
		)
	}
	if match("-x", "--debug") {
		printStr(
			"{{|UsageCommand|}}-x --debug{{[-]}}",
			"	Debug",
		)
	}
	if match("-y", "--yes") {
		printStr(
			"{{|UsageCommand|}}-y --yes{{[-]}}",
			"	Assume Yes for all prompts",
		)
	}

	if showAll && !noHeading {
		printStr(
			"",
			"CLI Commands:",
			"",
		)
	}

	// CLI Commands
	if match("-a", "--add") {
		printStr(
			"{{|UsageCommand|}}-a --add{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Add the default variables for the app(s) specified",
		)
	}
	if match("-c", "--compose") {
		printStr(
			"{{|UsageCommand|}}-c --compose{{[-]}} < {{|UsageOption|}}pull{{[-]}} | {{|UsageOption|}}up{{[-]}} | {{|UsageOption|}}down{{[-]}} | {{|UsageOption|}}stop{{[-]}} | {{|UsageOption|}}start{{[-]}} | {{|UsageOption|}}restart{{[-]}} | {{|UsageOption|}}create{{[-]}} | {{|UsageOption|}}rm{{[-]}} | {{|UsageOption|}}kill{{[-]}} | {{|UsageOption|}}pause{{[-]}} | {{|UsageOption|}}unpause{{[-]}} | {{|UsageOption|}}update{{[-]}} > [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Run docker compose commands. If no command is given, it does an '{{|UsageOption|}}update{{[-]}}'.",
			"	The '{{|UsageOption|}}update{{[-]}}' command is the same as a '{{|UsageOption|}}pull{{[-]}}' followed by an '{{|UsageOption|}}up{{[-]}}'",
			"{{|UsageCommand|}}-c --compose{{[-]}} < {{|UsageOption|}}generate{{[-]}} | {{|UsageOption|}}merge{{[-]}} >{{[-]}}",
			"	Generates the '{{|UsageFile|}}docker-compose.yml{{[-]}} file",
		)
	}

	if match("--config-show", "--show-config", "--config-folder", "--config-compose-folder") {
		printStr(
			"{{|UsageCommand|}}--config-show{{[-]}}",
			"{{|UsageCommand|}}--show-config{{[-]}}",
			"	Shows the current configuration options",
			"{{|UsageCommand|}}--config-folder{{[-]}} {{|UsageFile|}}<path>{{[-]}}",
			"	Sets the folder where application variables are stored.",
			"{{|UsageCommand|}}--config-compose-folder{{[-]}} {{|UsageFile|}}<path>{{[-]}}",
			"	Sets the folder where the docker-compose.yml file is stored.",
		)
	}
	if match("--disconnect", "--server") {
		printStr(
			"{{|UsageCommand|}}--disconnect{{[-]}}",
			"	Release the active editor session edit lock (graceful, waits up to 10s).",
			"{{|UsageCommand|}}--disconnect{{[-]}} [{{|UsageOption|}}all{{[-]}}|{{|UsageOption|}}web{{[-]}}|{{|UsageOption|}}ssh{{[-]}}|{{|UsageVar|}}<port>{{[-]}}|{{|UsageVar|}}<ip:port>{{[-]}}]",
			"	Disconnect sessions matching the target. A bare port number (SSH or web) disconnects",
			"	all sessions on the server instance using that port.",
		)
	}
	if match("-e", "--env") {
		printStr(
			"{{|UsageCommand|}}-e --env{{[-]}}",
			"	Update your '{{|UsageFile|}}.env{{[-]}}' files with new variables",
		)
	}
	if match("--env-appvars") {
		printStr(
			"{{|UsageCommand|}}--env-appvars{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	List all variable names for the app(s) specified",
		)
	}
	if match("--env-appvars-lines") {
		printStr(
			"{{|UsageCommand|}}--env-appvars-lines{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	List all variables and values for the app(s) specified",
		)
	}
	if match("--env-get", "--env-get=") {
		printStr(
			"{{|UsageCommand|}}--env-get{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]{{[-]}}",
			"{{|UsageCommand|}}--env-get={{[-]}}{{|UsageVar|}}<var>{{[-]}}",
			"	Get the value of a {{|UsageVar|}}<var>{{[-]}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-line", "--env-get-line=") {
		printStr(
			"{{|UsageCommand|}}--env-get-line{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]{{[-]}}",
			"{{|UsageCommand|}}--env-get-line={{[-]}}{{|UsageVar|}}<var>{{[-]}}",
			"	Get the line of a {{|UsageVar|}}<var>{{[-]}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-literal", "--env-get-literal=") {
		printStr(
			"{{|UsageCommand|}}--env-get-literal{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]{{[-]}}",
			"{{|UsageCommand|}}--env-get-literal{{[-]}}={{|UsageVar|}}<var>{{[-]}}",
			"	Get the literal value (including quotes) of a {{|UsageVar|}}<var>{{[-]}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-lower", "--env-get-lower=") {
		printStr(
			"{{|UsageCommand|}}--env-get-lower{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]{{[-]}}",
			"{{|UsageCommand|}}--env-get-lower{{[-]}}={{|UsageVar|}}<var>{{[-]}}",
			"	Get the value of a {{|UsageVar|}}<var>{{[-]}}iable",
		)
	}
	if match("--env-get-lower-line", "--env-get-lower-line=") {
		printStr(
			"{{|UsageCommand|}}--env-get-lower-line{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]",
			"{{|UsageCommand|}}--env-get-lower-line={{[-]}}{{|UsageVar|}}<var>{{[-]}}",
			"	Get the line of a {{|UsageVar|}}<var>{{[-]}}iable",
		)
	}
	if match("--env-get-lower-literal", "--env-get-lower-literal=") {
		printStr(
			"{{|UsageCommand|}}--env-get-lower-literal{{[-]}} {{|UsageVar|}}<var>{{[-]}} [{{|UsageVar|}}<var>{{[-]}} ...]{{[-]}}",
			"{{|UsageCommand|}}--env-get-lower-literal={{[-]}}{{|UsageVar|}}<var>{{[-]}}",
			"	Get the literal value (including quotes) of a {{|UsageVar|}}<var>{{[-]}}iable",
		)
	}
	if match("--env-set", "--env-set=") {
		printStr(
			"{{|UsageCommand|}}--env-set{{[-]}} {{|UsageVar|}}<var>=<val>{{[-]}}",
			"{{|UsageCommand|}}--env-set={{[-]}}{{|UsageVar|}}<var>,<val>{{[-]}}",
			"	Set the {{|UsageVar|}}<val>{{[-]}}ue of a {{|UsageVar|}}<var>{{[-]}}iable (variable name is forced to UPPER CASE).",
		)
	}
	if match("--env-set-lower", "--env-set-lower=") {
		printStr(
			"{{|UsageCommand|}}--env-set-lower{{[-]}} {{|UsageVar|}}<var>=<val>{{[-]}}",
			"{{|UsageCommand|}}--env-set-lower={{[-]}}{{|UsageVar|}}<var>,<val>{{[-]}}",
			"	Set the {{|UsageVar|}}<val>{{[-]}}ue of a {{|UsageVar|}}<var>{{[-]}}iable",
		)
	}
	if match("--env-edit", "--env-edit-lower") {
		printStr(
			"{{|UsageCommand|}}--env-edit{{[-]}} {{|UsageVar|}}<var>{{[-]}}",
			"{{|UsageCommand|}}--env-edit{{[-]}} {{|UsageVar|}}<app>:<var>{{[-]}}",
			"	Open the value picker TUI for a {{|UsageVar|}}<var>{{[-]}}iable (upper-cased)",
			"{{|UsageCommand|}}--env-edit-lower{{[-]}} {{|UsageVar|}}<var>{{[-]}}",
			"{{|UsageCommand|}}--env-edit-lower{{[-]}} {{|UsageVar|}}<app>:<var>{{[-]}}",
			"	Open the value picker TUI for a {{|UsageVar|}}<var>{{[-]}}iable (preserve case)",
		)
	}
	if match("-h", "--help") {
		printStr(
			"{{|UsageCommand|}}-h --help{{[-]}}",
			"	Show this usage information",
			"{{|UsageCommand|}}-h --help{{[-]}} {{|UsageOption|}}<option>{{[-]}}",
			"	Show the usage of the specified option",
		)
	}
	if match("--man") {
		printStr(
			"{{|UsageCommand|}}--man{{[-]}} {{|UsageApp|}}<app>{{[-]}}",
			"	Show documentation for the app specified",
		)
	}
	if match("-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced") {
		printStr(
			"{{|UsageCommand|}}-l --list{{[-]}}",
			"	List all apps",
			"{{|UsageCommand|}}--list-added{{[-]}}",
			"	List added apps",
			"{{|UsageCommand|}}--list-builtin{{[-]}}",
			"	List builtin apps",
			"{{|UsageCommand|}}--list-deprecated{{[-]}}",
			"	List deprecated apps",
			"{{|UsageCommand|}}--list-enabled{{[-]}}",
			"	List enabled apps",
			"{{|UsageCommand|}}--list-disabled{{[-]}}",
			"	List disabled apps",
			"{{|UsageCommand|}}--list-nondeprecated{{[-]}}",
			"	List non-deprecated apps",
			"{{|UsageCommand|}}--list-referenced{{[-]}}",
			"	List referenced apps",
		)
	}
	if match("-p", "--prune") {
		printStr(
			"{{|UsageCommand|}}-p --prune{{[-]}}",
			"	Remove unused docker resources",
		)
	}
	if match("-r", "--remove") {
		printStr(
			"{{|UsageCommand|}}-r --remove{{[-]}}",
			"	Prompt to remove variables for all disabled apps",
			"{{|UsageCommand|}}-r --remove{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Prompt to remove the variables for the app specified",
		)
	}
	if match("-R", "--reset") {
		printStr(
			"{{|UsageCommand|}}-R --reset{{[-]}}",
			fmt.Sprintf("	Resets {{|ApplicationName|}}%s to always process environment files.", appName),
			"	This is usually not needed unless you have modified application templates yourself.",
		)
	}
	if match("--server", "--disconnect") {
		printStr(
			"{{|UsageCommand|}}--server{{[-]}}",
			"	Show the server status (default when no subcommand is given).",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}start{{[-]}} [{{|UsageVar|}}<sshPort>{{[-]}} [{{|UsageVar|}}<webPort>{{[-]}}]]",
			"	Start the server daemon in the background. Port args override the config file.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}stop{{[-]}} [{{|UsageVar|}}<port>{{[-]}}]",
			"	Stop the running server daemon. Specify a port (SSH or web) to stop a specific instance.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}restart{{[-]}} [{{|UsageVar|}}<port>{{[-]}} [{{|UsageVar|}}<newSshPort>{{[-]}} [{{|UsageVar|}}<newWebPort>{{[-]}}]]]",
			"	Stop and restart the server daemon. Optionally target a specific instance by port",
			"	(SSH or web) and override ports for the new instance. Use 0 or \"\" to target the",
			"	single running instance when changing ports without specifying a target port.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}disconnect{{[-]}} [{{|UsageOption|}}all{{[-]}}|{{|UsageOption|}}web{{[-]}}|{{|UsageOption|}}ssh{{[-]}}|{{|UsageVar|}}<port>{{[-]}}|{{|UsageVar|}}<ip:port>{{[-]}}]",
			"	Alias for --disconnect. See --disconnect for full details.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}install{{[-]}} [{{|UsageVar|}}<sshPort>{{[-]}} [{{|UsageVar|}}<webPort>{{[-]}}]]",
			"	Install the OS service unit for the server daemon. Port args are written into",
			"	the service unit and override the config file.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}uninstall{{[-]}}",
			"	Remove the OS service unit for the server daemon.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}enable{{[-]}} [{{|UsageVar|}}<sshPort>{{[-]}} [{{|UsageVar|}}<webPort>{{[-]}}]]",
			"	Install (if needed) and enable the server to start at boot.",
			"{{|UsageCommand|}}--server{{[-]}} {{|UsageOption|}}disable{{[-]}}",
			"	Disable the server from starting at boot (keeps it installed).",
			"{{|UsageCommand|}}--server-daemon{{[-]}} [{{|UsageVar|}}<sshPort>{{[-]}} [{{|UsageVar|}}<webPort>{{[-]}}]]",
			"	Run the server daemon directly in the foreground (blocking). Useful if you",
			"	want to manage the process yourself rather than using '{{|UsageOption|}}--server start{{[-]}}'.",
		)
	}
	if match("-s", "--status") {
		printStr(
			"{{|UsageCommand|}}-s --status{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Returns the enabled/disabled status for the app specified",
		)
	}
	if match("--status-disable", "--status-enable") {
		printStr(
			"{{|UsageCommand|}}--status-disable{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Disable the app specified",
			"{{|UsageCommand|}}--status-enable{{[-]}} {{|UsageApp|}}<app>{{[-]}} [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
			"	Enable the app specified",
		)
	}
	if match("-t", "--test") {
		printStr(
			"{{|UsageCommand|}}-t --test{{[-]}} {{|UsageFile|}}<test_name>{{[-]}}",
			"	Run tests to check the program",
		)
	}
	if match("-T", "--theme", "--theme-list", "--theme-table",
		"--theme-lines", "--theme-no-lines",
		"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
		"--theme-large-buttons", "--theme-no-large-buttons",
		"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
		"--theme-scrollbar", "--theme-no-scrollbar", "--theme-scrollbars", "--theme-no-scrollbars",
		"--theme-spinner", "--theme-no-spinner", "--theme-spinners", "--theme-no-spinners", "--theme-spinner-speed",
		"--theme-border-color",
		"--theme-dialog-title", "--theme-submenu-title", "--theme-panel-title",
		"--theme-extract", "--theme-extract-all") {
		printStr(
			"{{|UsageCommand|}}-T --theme{{[-]}} [{{|UsageTheme|}}<themename>{{[-]}} | {{|UsageTheme|}}user:<themename>{{[-]}} | {{|UsageTheme|}}<path>.ds2theme{{[-]}} | {{|UsageTheme|}}file:<path>{{[-]}}]",
			"	Apply a theme. No arg shows the current theme.",
			"	  {{|UsageTheme|}}<themename>{{[-]}}: embedded theme   {{|UsageTheme|}}user:<themename>{{[-]}}: user themes folder",
			"	  {{|UsageTheme|}}<path>.ds2theme{{[-]}} or {{|UsageTheme|}}file:<path>{{[-]}}: arbitrary file path",
			"{{|UsageCommand|}}--theme-list{{[-]}}",
			"	Lists the available themes",
			"{{|UsageCommand|}}--theme-table{{[-]}}",
			"	Lists the available themes in a table format",
			"{{|UsageCommand|}}--theme-lines{{[-]}} | {{|UsageCommand|}}--theme-no-lines{{[-]}}",
			"	Turn line drawing characters on or off in the GUI",
			"{{|UsageCommand|}}--theme-borders{{[-]}} | {{|UsageCommand|}}--theme-no-borders{{[-]}}",
			"	Turn borders on or off in the GUI",
			"{{|UsageCommand|}}--theme-large-buttons{{[-]}} | {{|UsageCommand|}}--theme-no-large-buttons{{[-]}}",
			"	Turn large (bordered) button style on or off in the GUI",
			"{{|UsageCommand|}}--theme-shadows{{[-]}} | {{|UsageCommand|}}--theme-no-shadows{{[-]}}",
			"	Turn shadows on or off in the GUI",
			"{{|UsageCommand|}}--theme-shadow-level{{[-]}} {{|UsageOption|}}<level>{{[-]}}",
			"	Set the shadow level (0-4 or off/light/medium/dark/solid)",
			"{{|UsageCommand|}}--theme-scrollbars{{[-]}} | {{|UsageCommand|}}--theme-no-scrollbars{{[-]}}",
			"	Turn the scrollbar on or off in the GUI",
			"{{|UsageCommand|}}--theme-spinners{{[-]}} | {{|UsageCommand|}}--theme-no-spinners{{[-]}}",
			"	Turn the CLI spinner on or off",
			"{{|UsageCommand|}}--theme-spinner-speed{{[-]}} {{|UsageOption|}}<ms>{{[-]}}",
			fmt.Sprintf("	Set spinner frame speed in milliseconds (50-5000, default %d)", config.DefaultConfig().UI.SpinnerSpeed),
			"{{|UsageCommand|}}--theme-border-color{{[-]}} {{|UsageOption|}}<level>{{[-]}}",
			"	Set the border color (1=Border, 2=Border2, 3=Both)",
			"{{|UsageCommand|}}--theme-dialog-title{{[-]}} {{|UsageOption|}}<align>{{[-]}}",
			"	Set dialog title alignment ({{|UsageOption|}}left{{[-]}} or {{|UsageOption|}}center{{[-]}})",
			"{{|UsageCommand|}}--theme-submenu-title{{[-]}} {{|UsageOption|}}<align>{{[-]}}",
			"	Set submenu title alignment ({{|UsageOption|}}left{{[-]}} or {{|UsageOption|}}center{{[-]}})",
			"{{|UsageCommand|}}--theme-panel-title{{[-]}} {{|UsageOption|}}<align>{{[-]}}",
			"	Set log panel title alignment ({{|UsageOption|}}left{{[-]}} or {{|UsageOption|}}center{{[-]}})",
			"{{|UsageCommand|}}--theme-extract{{[-]}} {{|UsageTheme|}}<themename>{{[-]}} {{|UsageOption|}}<destdir>{{[-]}} {{|UsageOption|}}<filename>{{[-]}}",
			"	Extract a theme to a file (use {{|UsageTheme|}}user:<name>{{[-]}} for user themes; {{|UsageOption|}}user:{{[-]}} as destdir for the user themes folder)",
			"{{|UsageCommand|}}--theme-extract-all{{[-]}} {{|UsageOption|}}<destdir>{{[-]}}",
			"	Extract all embedded themes to a directory (use {{|UsageOption|}}user:{{[-]}} for the user themes folder)",
		)
	}
	if match("--theme-spinner-speed") {
		printStr(
			"{{|UsageCommand|}}--theme-spinner-speed{{[-]}} {{|UsageOption|}}<ms>{{[-]}}",
			fmt.Sprintf("	Set spinner frame speed in milliseconds (50-5000, default %d)", config.DefaultConfig().UI.SpinnerSpeed),
		)
	}
	if match("-u", "--update", "--update-app", "--update-templates") {
		printStr(
			"{{|UsageCommand|}}-u --update{{[-]}} [{{|UsageBranch|}}<AppVersionOrChannel>{{[-]}} [{{|UsageBranch|}}<TemplateBranch>{{[-]}}]]",
			fmt.Sprintf("	Update {{|ApplicationName|}}%s{{[-]}} and {{|ApplicationName|}}DockSTARTer-Templates{{[-]}}. Optionally specify version/channel and template branch.", appName),
			"{{|UsageCommand|}}--update-app{{[-]}} [{{|UsageBranch|}}<AppVersionOrChannel>{{[-]}}]",
			fmt.Sprintf("	Update {{|ApplicationName|}}%s{{[-]}} only. Optionally specify a version like 'v2.0.0.1' or a channel like 'testing'.", appName),
			"{{|UsageCommand|}}--update-templates{{[-]}} [{{|UsageBranch|}}<TemplateBranch>{{[-]}}]",
			"	Update {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} only. Optionally specify a branch.",
		)
	}
	if match("-V", "--version") {
		printStr(
			"{{|UsageCommand|}}-V --version{{[-]}} [{{|UsageBranch|}}<AppBranch>{{[-]}} [{{|UsageBranch|}}<TemplateBranch>{{[-]}}]]",
			"	Display version information. Optionally specify branches to check remote versions.",
		)
	}
	if match("--print-version") {
		printStr(
			"{{|UsageCommand|}}--print-version{{[-]}}",
			fmt.Sprintf("	Print the raw {{|ApplicationName|}}%s{{[-]}} version string and exit. Suitable for scripting.", appName),
		)
	}
	if match("--print-templates-version") {
		printStr(
			"{{|UsageCommand|}}--print-templates-version{{[-]}}",
			"	Print the raw {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} version string and exit. Suitable for scripting.",
		)
	}

	if showAll && !noHeading {
		printStr(
			"",
			"Menu Commands:",
			"",
		)
	}

	// Menu Commands
	if match("--edit-global", "--start-edit-global") {
		printStr(
			"{{|UsageCommand|}}--edit-global{{[-]}}",
			"	Open the global environment variables editor (root session).",
			"{{|UsageCommand|}}--start-edit-global{{[-]}}",
			"	Open the global environment variables editor (restore nav stack).",
		)
	}
	if match("--edit-app", "--start-edit-app") {
		printStr(
			"{{|UsageCommand|}}--edit-app{{[-]}} {{|UsageApp|}}<app>{{[-]}}",
			"	Open the environment variables editor for the specified app (root session).",
			"{{|UsageCommand|}}--start-edit-app{{[-]}} {{|UsageApp|}}<app>{{[-]}}",
			"	Open the environment variables editor for the specified app (restore nav stack).",
		)
	}
	if match("-M", "--menu") {
		printStr(
			"{{|UsageCommand|}}-M --menu{{[-]}}",
			"	Start the menu system.",
			fmt.Sprintf("	This is the same as typing '{{|UsageCommand|}}%s{{[-]}}'.", appCmd),
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}page{{[-]}} >{{[-]}}",
			"	Load the page as the root (no Back button, Exit goes to shell).",
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}start-page{{[-]}} >{{[-]}}",
			"	Load the page with full navigation history (Back returns to parent).",
			"	Pages: {{|UsageOption|}}main{{[-]}} | {{|UsageOption|}}config{{[-]}} | {{|UsageOption|}}options{{[-]}}",
			"	       {{|UsageOption|}}appearance{{[-]}} (also: {{|UsageOption|}}display{{[-]}} | {{|UsageOption|}}theme{{[-]}} | {{|UsageOption|}}options-display{{[-]}} | {{|UsageOption|}}options-theme{{[-]}})",
			"	       {{|UsageOption|}}app-select{{[-]}} (also: {{|UsageOption|}}select{{[-]}} | {{|UsageOption|}}config-app-select{{[-]}})",
		)
	}
	if match("-S", "--select", "--menu-config-app-select", "--menu-app-select") {
		printStr(
			"{{|UsageCommand|}}-S --select{{[-]}}",
			"	Load the {{|UsagePage|}}Application Selection{{[-]}} page in the menu.",
		)
	}

	if target != "" && !found {
		printStr(
			fmt.Sprintf("Unknown option '{{|UsageCommand|}}%s{{[-]}}'.", target),
		)
	}

	return strings.TrimRight(sb.String(), "\n")
}
