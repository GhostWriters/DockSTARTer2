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
			fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} [{{_Version_}}%s{{|-|}}]", appName, version.Version),
			"{{_ApplicationName_}}DockSTARTer-Templates{{|-|}} [{{_Version_}}Unknown Version{{|-|}}]",
			"",
			fmt.Sprintf("Usage: {{_UsageCommand_}}%s{{|-|}} [{{_UsageCommand_}}<Flags>{{|-|}}] [{{_UsageCommand_}}<Command>{{|-|}}] ...", appCmd),
			"",
			fmt.Sprintf("This is the main {{_ApplicationName_}}%s{{|-|}} script.", appName),
			"For regular usage you can run without providing any options.",
			"",
			"You may include multiple commands on the command-line, and they will be executed in",
			"the order given, only stopping on an error. Any flags included only apply to the",
			"following command, and get reset before the next command.",
			"",
			"Any command that takes a variable name, the variable will by default be looked for",
			"in the global '{{_UsageFile_}}.env{{|-|}}' file. If the variable name used is in form of '{{_UsageVar_}}app:var{{|-|}}', it",
			"will instead refer to the variable '{{_UsageVar_}}<var>{{|-|}}' in '{{_UsageFile_}}.env.app.<app>{{|-|}}'.  Some commands",
			"that take app names can use the form '{{_UsageApp_}}app:{{|-|}}' to refer to the same file.",
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
			"{{_UsageCommand_}}-f --force{{|-|}}",
			"	Force certain install/upgrade actions to run even if they would not be needed.",
		)
	}
	if match("-g", "--gui") {
		printStr(
			"{{_UsageCommand_}}-g --gui{{|-|}}",
			"	Use dialog boxes",
		)
	}
	if match("-v", "--verbose") {
		printStr(
			"{{_UsageCommand_}}-v --verbose{{|-|}}",
			"	Verbose",
		)
	}
	if match("-x", "--debug") {
		printStr(
			"{{_UsageCommand_}}-x --debug{{|-|}}",
			"	Debug",
		)
	}
	if match("-y", "--yes") {
		printStr(
			"{{_UsageCommand_}}-y --yes{{|-|}}",
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
			"{{_UsageCommand_}}-a --add{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Add the default variables for the app(s) specified",
		)
	}
	if match("-c", "--compose") {
		printStr(
			"{{_UsageCommand_}}-c --compose{{|-|}} < {{_UsageOption_}}pull{{|-|}} | {{_UsageOption_}}up{{|-|}} | {{_UsageOption_}}down{{|-|}} | {{_UsageOption_}}stop{{|-|}} | {{_UsageOption_}}restart{{|-|}} | {{_UsageOption_}}update{{|-|}} > [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Run docker compose commands. If no command is given, it does an '{{_UsageOption_}}update{{|-|}}'.",
			"	The '{{_UsageOption_}}update{{|-|}}' command is the same as a '{{_UsageOption_}}pull{{|-|}}' followed by an '{{_UsageOption_}}up{{|-|}}'",
			"{{_UsageCommand_}}-c --compose{{|-|}} < {{_UsageOption_}}generate{{|-|}} | {{_UsageOption_}}merge{{|-|}} >{{|-|}}",
			"	Generates the '{{_UsageFile_}}docker-compose.yml{{|-|}} file",
		)
	}

	if match("--config-show", "--show-config") {
		printStr(
			"{{_UsageCommand_}}--config-show{{|-|}}",
			"{{_UsageCommand_}}--show-config{{|-|}}",
			"	Shows the current configuration options",
		)
	}
	if match("-e", "--env") {
		printStr(
			"{{_UsageCommand_}}-e --env{{|-|}}",
			"	Update your '{{_UsageFile_}}.env{{|-|}}' files with new variables",
		)
	}
	if match("--env-appvars") {
		printStr(
			"{{_UsageCommand_}}--env-appvars{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	List all variable names for the app(s) specified",
		)
	}
	if match("--env-appvars-lines") {
		printStr(
			"{{_UsageCommand_}}--env-appvars-lines{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	List all variables and values for the app(s) specified",
		)
	}
	if match("--env-get", "--env-get=") {
		printStr(
			"{{_UsageCommand_}}--env-get{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]{{|-|}}",
			"{{_UsageCommand_}}--env-get={{|-|}}{{_UsageVar_}}<var>{{|-|}}",
			"	Get the value of a {{_UsageVar_}}<var>{{|-|}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-line", "--env-get-line=") {
		printStr(
			"{{_UsageCommand_}}--env-get-line{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]{{|-|}}",
			"{{_UsageCommand_}}--env-get-line={{|-|}}{{_UsageVar_}}<var>{{|-|}}",
			"	Get the line of a {{_UsageVar_}}<var>{{|-|}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-literal", "--env-get-literal=") {
		printStr(
			"{{_UsageCommand_}}--env-get-literal{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]{{|-|}}",
			"{{_UsageCommand_}}--env-get-literal{{|-|}}={{_UsageVar_}}<var>{{|-|}}",
			"	Get the literal value (including quotes) of a {{_UsageVar_}}<var>{{|-|}}iable (variable name is forced to UPPER CASE)",
		)
	}
	if match("--env-get-lower", "--env-get-lower=") {
		printStr(
			"{{_UsageCommand_}}--env-get-lower{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]{{|-|}}",
			"{{_UsageCommand_}}--env-get-lower{{|-|}}={{_UsageVar_}}<var>{{|-|}}",
			"	Get the value of a {{_UsageVar_}}<var>{{|-|}}iable",
		)
	}
	if match("--env-get-lower-line", "--env-get-lower-line=") {
		printStr(
			"{{_UsageCommand_}}--env-get-lower-line{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]",
			"{{_UsageCommand_}}--env-get-lower-line={{|-|}}{{_UsageVar_}}<var>{{|-|}}",
			"	Get the line of a {{_UsageVar_}}<var>{{|-|}}iable",
		)
	}
	if match("--env-get-lower-literal", "--env-get-lower-literal=") {
		printStr(
			"{{_UsageCommand_}}--env-get-lower-literal{{|-|}} {{_UsageVar_}}<var>{{|-|}} [{{_UsageVar_}}<var>{{|-|}} ...]{{|-|}}",
			"{{_UsageCommand_}}--env-get-lower-literal={{|-|}}{{_UsageVar_}}<var>{{|-|}}",
			"	Get the literal value (including quotes) of a {{_UsageVar_}}<var>{{|-|}}iable",
		)
	}
	if match("--env-set", "--env-set=") {
		printStr(
			"{{_UsageCommand_}}--env-set{{|-|}} {{_UsageVar_}}<var>=<val>{{|-|}}",
			"{{_UsageCommand_}}--env-set={{|-|}}{{_UsageVar_}}<var>,<val>{{|-|}}",
			"	Set the {{_UsageVar_}}<val>{{|-|}}ue of a {{_UsageVar_}}<var>{{|-|}}iable (variable name is forced to UPPER CASE).",
		)
	}
	if match("--env-set-lower", "--env-set-lower=") {
		printStr(
			"{{_UsageCommand_}}--env-set-lower{{|-|}} {{_UsageVar_}}<var>=<val>{{|-|}}",
			"{{_UsageCommand_}}--env-set-lower={{|-|}}{{_UsageVar_}}<var>,<val>{{|-|}}",
			"	Set the {{_UsageVar_}}<val>{{|-|}}ue of a {{_UsageVar_}}<var>{{|-|}}iable",
		)
	}
	if match("-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced") {
		printStr(
			"{{_UsageCommand_}}-l --list{{|-|}}",
			"	List all apps",
			"{{_UsageCommand_}}--list-added{{|-|}}",
			"	List added apps",
			"{{_UsageCommand_}}--list-builtin{{|-|}}",
			"	List builtin apps",
			"{{_UsageCommand_}}--list-deprecated{{|-|}}",
			"	List deprecated apps",
			"{{_UsageCommand_}}--list-enabled{{|-|}}",
			"	List enabled apps",
			"{{_UsageCommand_}}--list-disabled{{|-|}}",
			"	List disabled apps",
			"{{_UsageCommand_}}--list-nondeprecated{{|-|}}",
			"	List non-deprecated apps",
			"{{_UsageCommand_}}--list-referenced{{|-|}}",
			"	List referenced apps",
		)
	}
	if match("-h", "--help") {
		printStr(
			"{{_UsageCommand_}}-h --help{{|-|}}",
			"	Show this usage information",
			"{{_UsageCommand_}}-h --help{{|-|}} {{_UsageOption_}}<option>{{|-|}}",
			"	Show the usage of the specified option",
		)
	}

	if match("-p", "--prune") {
		printStr(
			"{{_UsageCommand_}}-p --prune{{|-|}}",
			"	Remove unused docker resources",
		)
	}
	if match("-r", "--remove") {
		printStr(
			"{{_UsageCommand_}}-r --remove{{|-|}}",
			"	Prompt to remove variables for all disabled apps",
			"{{_UsageCommand_}}-r --remove{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Prompt to remove the variables for the app specified",
		)
	}
	if match("-R", "--reset") {
		printStr(
			"{{_UsageCommand_}}-R --reset{{|-|}}",
			fmt.Sprintf("	Resets {{_ApplicationName_}}%s to always process environment files.", appName),
			"	This is usually not needed unless you have modified application templates yourself.",
		)
	}
	if match("-s", "--status") {
		printStr(
			"{{_UsageCommand_}}-s --status{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Returns the enabled/disabled status for the app specified",
		)
	}
	if match("--status-disable", "--status-enable") {
		printStr(
			"{{_UsageCommand_}}--status-disable{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Disable the app specified",
			"{{_UsageCommand_}}--status-enable{{|-|}} {{_UsageApp_}}<app>{{|-|}} [{{_UsageApp_}}<app>{{|-|}} ...]{{|-|}}",
			"	Enable the app specified",
		)
	}
	if match("-t", "--test") {
		printStr(
			"{{_UsageCommand_}}-t --test{{|-|}} {{_UsageFile_}}<test_name>{{|-|}}",
			"	Run tests to check the program",
		)
	}
	if match("-T", "--theme", "--theme-list", "--theme-table", "--theme-lines", "--theme-no-lines", "--theme-borders", "--theme-no-borders", "--theme-shadows", "--theme-no-shadows", "--theme-scrollbar", "--theme-no-scrollbar") {
		printStr(
			"{{_UsageCommand_}}-T --theme{{|-|}}",
			"	Shows the current theme",
			"{{_UsageCommand_}}-T --theme{{|-|}} {{_UsageTheme_}}<themename>{{|-|}}",
			"	Saves and applies the specified theme to the GUI",
			"{{_UsageCommand_}}--theme-list{{|-|}}",
			"	Lists the available themes",
			"{{_UsageCommand_}}--theme-table{{|-|}}",
			"	Lists the available themes in a table format",
			"{{_UsageCommand_}}--theme-lines{{|-|}}",
			"{{_UsageCommand_}}--theme-no-lines{{|-|}}",
			"	Turn the line drawing characters on or off in the GUI",
			"{{_UsageCommand_}}--theme-borders{{|-|}}",
			"{{_UsageCommand_}}--theme-no-borders{{|-|}}",
			"	Turn the borders on and off in the GUI",
			"{{_UsageCommand_}}--theme-shadows{{|-|}}",
			"{{_UsageCommand_}}--theme-no-shadows{{|-|}}",
			"	Turn the shadows on or off in the GUI",
			"{{_UsageCommand_}}--theme-scrollbar{{|-|}}",
			"{{_UsageCommand_}}--theme-no-scrollbar{{|-|}}",
			"	Turn the scrollbar on or off in the GUI",
		)
	}
	if match("-u", "--update", "--update-app", "--update-templates") {
		printStr(
			"{{_UsageCommand_}}-u --update{{|-|}}",
			fmt.Sprintf("	Update {{_ApplicationName_}}%s{{|-|}} using GitHub Releases and {{_ApplicationName_}}DockSTARTer-Templates{{|-|}} using git.", appName),
			"{{_UsageCommand_}}-u --update{{|-|}} {{_UsageBranch_}}<AppVersionOrChannel>{{|-|}} {{_UsageBranch_}}<TemplateBranch>{{|-|}}",
			"	Update to specific versions. <AppVersionOrChannel> can be a version like 'v2.0.0.1' or a channel like 'testing'.",
			"{{_UsageCommand_}}--update-app{{|-|}}",
			fmt.Sprintf("	Update {{_ApplicationName_}}%s{{|-|}} only.", appName),
			"{{_UsageCommand_}}--update-app{{|-|}} {{_UsageBranch_}}<AppVersionOrChannel>{{|-|}}",
			fmt.Sprintf("	Update {{_ApplicationName_}}%s{{|-|}} to the specified version or channel.", appName),
			"{{_UsageCommand_}}--update-templates{{|-|}}",
			"	Update {{_ApplicationName_}}DockSTARTer-Templates{{|-|}} only.",
			"{{_UsageCommand_}}--update-templates{{|-|}} {{_UsageBranch_}}<TemplateBranch>{{|-|}}",
			"	Update {{_ApplicationName_}}DockSTARTer-Templates{{|-|}} to the specified branch.",
		)
	}
	if match("-V", "--version") {
		printStr(
			"{{_UsageCommand_}}-V --version{{|-|}}",
			"	Display version information",
			"{{_UsageCommand_}}-V --version{{|-|}} {{_UsageBranch_}}<AppBranch>{{|-|}} {{_UsageBranch_}}<TemplateBranch>{{|-|}}",
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
			"{{_UsageCommand_}}-M --menu{{|-|}}",
			"	Start the menu system.",
			fmt.Sprintf("	This is the same as typing '{{_UsageCommand_}}%s{{|-|}}'.", appCmd),
			"{{_UsageCommand_}}-M --menu{{|-|}} < {{_UsageOption_}}main{{|-|}} | {{_UsageOption_}}config{{|-|}} | {{_UsageOption_}}options{{|-|}} >{{|-|}}",
			"	Load the specified page in the menu.",
			"{{_UsageCommand_}}-M --menu{{|-|}} < {{_UsageOption_}}options-display{{|-|}} | {{_UsageOption_}}display{{|-|}} >{{|-|}}",
			"	Load the {{_UsagePage_}}Display Options{{|-|}} page in the menu.",
			"{{_UsageCommand_}}-M --menu{{|-|}} < {{_UsageOption_}}options-theme{{|-|}} | {{_UsageOption_}}theme{{|-|}} >{{|-|}}",
			"	Load the {{_UsagePage_}}Theme Chooser{{|-|}} page in the menu.",
			"{{_UsageCommand_}}-M --menu{{|-|}} < {{_UsageOption_}}config-app-select{{|-|}} | {{_UsageOption_}}app-select{{|-|}} | {{_UsageOption_}}select{{|-|}} >{{|-|}}",
			"	Load the {{_UsagePage_}}Application Selection{{|-|}} page in the menu.",
		)
	}
	if match("-S", "--select", "--menu-config-app-select", "--menu-app-select") {
		printStr(
			"{{_UsageCommand_}}-S --select{{|-|}}",
			"	Load the {{_UsagePage_}}Application Selection{{|-|}} page in the menu.",
		)
	}

	return strings.TrimRight(sb.String(), "\n")
}
