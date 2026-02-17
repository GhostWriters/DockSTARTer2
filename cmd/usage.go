package cmd

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/version"
	"fmt"
	"strings"
)

// PrintHelp prints usage information.
// If target is empty, prints global usage.
// If target is specified, prints usage for that specific flag/command.
func PrintHelp(target string) {
	fmt.Println(console.ToANSI(GetUsage(target)))
}

// GetUsage returns usage information as a string.
// If target is empty, returns global usage.
// If target is specified, returns usage for that specific flag/command.
func GetUsage(target string) string {
	var sb strings.Builder
	printStr := func(lines ...string) {
		for _, s := range lines {
			sb.WriteString(s + "\n")
		}
	}

	// Helper for header info
	appName := version.ApplicationName
	appCmd := version.CommandName

	// If target is empty, print intro
	if target == "" {
		// Mimic the header section
		printStr(
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", appName, version.Version),
			"{{|ApplicationName|}}DockSTARTer-Templates{{[-]}} [{{|Version|}}Unknown Version{{[-]}}]",
			"",
			fmt.Sprintf("Usage: {{|UsageCommand|}}%s{{[-]}} [{{|UsageCommand|}}<Flags>{{[-]}}] [{{|UsageCommand|}}<Command>{{[-]}}] ...", appCmd),
			"",
			fmt.Sprintf("This is the main {{|ApplicationName|}}%s{{[-]}} script.", appName),
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
			"Flags:",
			"",
		)
	}

	// Flags section and Command section.
	showAll := target == ""

	match := func(opts ...string) bool {
		if showAll {
			return true
		}
		for _, o := range opts {
			if o == target {
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

	if showAll {
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
			"{{|UsageCommand|}}-c --compose{{[-]}} < {{|UsageOption|}}pull{{[-]}} | {{|UsageOption|}}up{{[-]}} | {{|UsageOption|}}down{{[-]}} | {{|UsageOption|}}stop{{[-]}} | {{|UsageOption|}}restart{{[-]}} | {{|UsageOption|}}update{{[-]}} > [{{|UsageApp|}}<app>{{[-]}} ...]{{[-]}}",
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
	if match("-h", "--help") {
		printStr(
			"{{|UsageCommand|}}-h --help{{[-]}}",
			"	Show this usage information",
			"{{|UsageCommand|}}-h --help{{[-]}} {{|UsageOption|}}<option>{{[-]}}",
			"	Show the usage of the specified option",
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
	if match("-T", "--theme", "--theme-list", "--theme-table", "--theme-lines", "--theme-no-lines", "--theme-borders", "--theme-no-borders", "--theme-shadows", "--theme-no-shadows", "--theme-shadow-level", "--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color") {
		printStr(
			"{{|UsageCommand|}}-T --theme{{[-]}}",
			"	Shows the current theme",
			"{{|UsageCommand|}}-T --theme{{[-]}} {{|UsageTheme|}}<themename>{{[-]}}",
			"	Saves and applies the specified theme to the GUI",
			"{{|UsageCommand|}}--theme-list{{[-]}}",
			"	Lists the available themes",
			"{{|UsageCommand|}}--theme-table{{[-]}}",
			"	Lists the available themes in a table format",
			"{{|UsageCommand|}}--theme-lines{{[-]}}",
			"{{|UsageCommand|}}--theme-no-lines{{[-]}}",
			"	Turn the line drawing characters on or off in the GUI",
			"{{|UsageCommand|}}--theme-borders{{[-]}}",
			"{{|UsageCommand|}}--theme-no-borders{{[-]}}",
			"	Turn the borders on and off in the GUI",
			"{{|UsageCommand|}}--theme-shadows{{[-]}}",
			"{{|UsageCommand|}}--theme-no-shadows{{[-]}}",
			"	Turn the shadows on or off in the GUI",
			"{{|UsageCommand|}}--theme-shadow-level{{[-]}} {{|UsageOption|}}<level>{{[-]}}",
			"	Set the shadow level (0-4 or off/light/medium/dark/solid)",
			"{{|UsageCommand|}}--theme-scrollbar{{[-]}}",
			"{{|UsageCommand|}}--theme-no-scrollbar{{[-]}}",
			"	Turn the scrollbar on or off in the GUI",
			"{{|UsageCommand|}}--theme-border-color{{[-]}} {{|UsageOption|}}<level>{{[-]}}",
			"	Set the border color (1=Border, 2=Border2, 3=Both)",
		)
	}
	if match("-u", "--update", "--update-app", "--update-templates") {
		printStr(
			"{{|UsageCommand|}}-u --update{{[-]}}",
			fmt.Sprintf("	Update {{|ApplicationName|}}%s{{[-]}} using GitHub Releases and {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} using git.", appName),
			"{{|UsageCommand|}}-u --update{{[-]}} {{|UsageBranch|}}<AppVersionOrChannel>{{[-]}} {{|UsageBranch|}}<TemplateBranch>{{[-]}}",
			"	Update to specific versions. <AppVersionOrChannel> can be a version like 'v2.0.0.1' or a channel like 'testing'.",
			"{{|UsageCommand|}}--update-app{{[-]}}",
			fmt.Sprintf("	Update {{|ApplicationName|}}%s{{[-]}} only.", appName),
			"{{|UsageCommand|}}--update-app{{[-]}} {{|UsageBranch|}}<AppVersionOrChannel>{{[-]}}",
			fmt.Sprintf("	Update {{|ApplicationName|}}%s{{[-]}} to the specified version or channel.", appName),
			"{{|UsageCommand|}}--update-templates{{[-]}}",
			"	Update {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} only.",
			"{{|UsageCommand|}}--update-templates{{[-]}} {{|UsageBranch|}}<TemplateBranch>{{[-]}}",
			"	Update {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} to the specified branch.",
		)
	}
	if match("-V", "--version") {
		printStr(
			"{{|UsageCommand|}}-V --version{{[-]}}",
			"	Display version information",
			"{{|UsageCommand|}}-V --version{{[-]}} {{|UsageBranch|}}<AppBranch>{{[-]}} {{|UsageBranch|}}<TemplateBranch>{{[-]}}",
			"	Display version information for the specified branches",
		)
	}

	if showAll {
		printStr(
			"",
			"Menu Commands:",
			"",
		)
	}

	// Menu Commands
	if match("-M", "--menu") {
		printStr(
			"{{|UsageCommand|}}-M --menu{{[-]}}",
			"	Start the menu system.",
			fmt.Sprintf("	This is the same as typing '{{|UsageCommand|}}%s{{[-]}}'.", appCmd),
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}main{{[-]}} | {{|UsageOption|}}config{{[-]}} | {{|UsageOption|}}options{{[-]}} >{{[-]}}",
			"	Load the specified page in the menu.",
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}options-display{{[-]}} | {{|UsageOption|}}display{{[-]}} >{{[-]}}",
			"	Load the {{|UsagePage|}}Display Options{{[-]}} page in the menu.",
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}options-theme{{[-]}} | {{|UsageOption|}}theme{{[-]}} >{{[-]}}",
			"	Load the {{|UsagePage|}}Theme Chooser{{[-]}} page in the menu.",
			"{{|UsageCommand|}}-M --menu{{[-]}} < {{|UsageOption|}}config-app-select{{[-]}} | {{|UsageOption|}}app-select{{[-]}} | {{|UsageOption|}}select{{[-]}} >{{[-]}}",
			"	Load the {{|UsagePage|}}Application Selection{{[-]}} page in the menu.",
		)
	}
	if match("-S", "--select", "--menu-config-app-select", "--menu-app-select") {
		printStr(
			"{{|UsageCommand|}}-S --select{{[-]}}",
			"	Load the {{|UsagePage|}}Application Selection{{[-]}} page in the menu.",
		)
	}

	return strings.TrimRight(sb.String(), "\n")
}
