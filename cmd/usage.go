package cmd

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/version"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// PrintHelp prints usage information.
// If target is empty, prints global usage.
// If target is specified, prints usage for that specific flag/command.
func PrintHelp(target string) {
	fmt.Println(console.Parse(GetUsage(target)))
}

// GetUsage returns usage information as a string.
// If target is empty, returns global usage.
// If target is specified, returns usage for that specific flag/command.
func GetUsage(target string) string {
	var sb strings.Builder
	printStr := func(s string) {
		sb.WriteString(s + "\n")
	}

	// Helper for header info
	appName := version.ApplicationName
	appCmd := version.CommandName

	// If target is empty, print intro
	if target == "" {
		// Mimic the header section
		printStr(fmt.Sprintf("Usage: [_UsageCommand_]%s[-] [[_UsageCommand_]<Flags>[-]] [[_UsageCommand_]<Command>[-]] ...", appCmd))
		printStr("")
		printStr(fmt.Sprintf("[_ApplicationName_]%s[-] [[_Version_]%s[-]]", appName, version.Version))
		printStr("[_ApplicationName_]DockSTARTer-Templates[-] [[_Version_]Unknown Version[-]]")
		printStr(fmt.Sprintf("This is the main [_ApplicationName_]%s[-] script.", appName))
		printStr("For regular usage you can run without providing any options.")
		printStr("")
		printStr("You may include multiple commands on the command-line, and they will be executed in")
		printStr("the order given, only stopping on an error. Any flags included only apply to the")
		printStr("following command, and get reset before the next command.")
		printStr("")
		printStr("Any command that takes a variable name, the variable will by default be looked for")
		printStr("in the global '[_UsageFile_].env[-]' file. If the variable name used is in form of '[_UsageVar_]app:var[-]', it")
		printStr("will instead refer to the variable '[_UsageVar_]<var>[-]' in '[_UsageFile_].env.app.<app>[-]'.  Some commands")
		printStr("that take app names can use the form '[_UsageApp_]app:[-]' to refer to the same file.")
		printStr("")
		printStr("Flags:")
		printStr("")
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
		printStr("[_UsageCommand_]-f --force[-]")
		printStr("	Force certain install/upgrade actions to run even if they would not be needed.")
	}
	if match("-g", "--gui") {
		printStr("[_UsageCommand_]-g --gui[-]")
		printStr("	Use dialog boxes")
	}
	if match("-v", "--verbose") {
		printStr("[_UsageCommand_]-v --verbose[-]")
		printStr("	Verbose")
	}
	if match("-x", "--debug") {
		printStr("[_UsageCommand_]-x --debug[-]")
		printStr("	Debug")
	}
	if match("-y", "--yes") {
		printStr("[_UsageCommand_]-y --yes[-]")
		printStr("	Assume Yes for all prompts")
	}

	if showAll {
		printStr("")
		printStr("CLI Commands:")
		printStr("")
	}

	// CLI Commands
	if match("-a", "--add") {
		printStr("[_UsageCommand_]-a --add[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Add the default variables for the app(s) specified")
	}
	if match("-c", "--compose") {
		printStr("[_UsageCommand_]-c --compose[-] < [_UsageOption_]pull[-] | [_UsageOption_]up[-] | [_UsageOption_]down[-] | [_UsageOption_]stop[-] | [_UsageOption_]restart[-] | [_UsageOption_]update[-] > [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Run docker compose commands. If no command is given, it does an '[_UsageOption_]update[-]'.")
		printStr("	The '[_UsageOption_]update[-]' command is the same as a '[_UsageOption_]pull[-]' followed by an '[_UsageOption_]up[-]'")
		printStr("[_UsageCommand_]-c --compose[-] < [_UsageOption_]generate[-] | [_UsageOption_]merge[-] >[-]")
		printStr("	Generates the '[_UsageFile_]docker-compose.yml[-] file")
	}
	if match("--config-pm", "--config-pm-auto") {
		printStr("[_UsageCommand_]--config-pm[-] [_UsageOption_]<package manager>[-]")
		printStr("	Select the specified package manager to install dependencies")
		printStr("[_UsageCommand_]--config-pm-auto[-]")
		printStr("	Autodetect the package manager")
	}
	if match("--config-pm-list") {
		printStr("[_UsageCommand_]--config-pm-list[-]")
		printStr("	Lists the compatible package managers")
	}
	if match("--config-pm-table") {
		printStr("[_UsageCommand_]--config-pm-table[-]")
		printStr("	Lists the compatible package managers in a table format")
	}
	if match("--config-pm-existing-list") {
		printStr("[_UsageCommand_]--config-pm-existing-list[-]")
		printStr("	Lists the existing package managers")
	}
	if match("--config-pm-existing-table") {
		printStr("[_UsageCommand_]--config-pm-existing-table[-]")
		printStr("	Lists the existing package managers in a table format")
	}
	if match("--config-show", "--show-config") {
		printStr("[_UsageCommand_]--config-show[-]")
		printStr("[_UsageCommand_]--show-config[-]")
		printStr("	Shows the current configuration options")
	}
	if match("-e", "--env") {
		printStr("[_UsageCommand_]-e --env[-]")
		printStr("	Update your '[_UsageFile_].env[-]' files with new variables")
	}
	if match("--env-appvars") {
		printStr("[_UsageCommand_]--env-appvars[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	List all variable names for the app(s) specified")
	}
	if match("--env-appvars-lines") {
		printStr("[_UsageCommand_]--env-appvars-lines[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	List all variables and values for the app(s) specified")
	}
	if match("--env-get", "--env-get=") {
		printStr("[_UsageCommand_]--env-get[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...][-]")
		printStr("[_UsageCommand_]--env-get=[-][_UsageVar_]<var>[-]")
		printStr("	Get the value of a [_UsageVar_]<var>[-]iable (variable name is forced to UPPER CASE)")
	}
	if match("--env-get-line", "--env-get-line=") {
		printStr("[_UsageCommand_]--env-get-line[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...][-]")
		printStr("[_UsageCommand_]--env-get-line=[-][_UsageVar_]<var>[-]")
		printStr("	Get the line of a [_UsageVar_]<var>[-]iable (variable name is forced to UPPER CASE)")
	}
	if match("--env-get-literal", "--env-get-literal=") {
		printStr("[_UsageCommand_]--env-get-literal[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...][-]")
		printStr("[_UsageCommand_]--env-get-literal[-]=[_UsageVar_]<var>[-]")
		printStr("	Get the literal value (including quotes) of a [_UsageVar_]<var>[-]iable (variable name is forced to UPPER CASE)")
	}
	if match("--env-get-lower", "--env-get-lower=") {
		printStr("[_UsageCommand_]--env-get-lower[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...][-]")
		printStr("[_UsageCommand_]--env-get-lower[-]=[_UsageVar_]<var>[-]")
		printStr("	Get the value of a [_UsageVar_]<var>[-]iable")
	}
	if match("--env-get-lower-line", "--env-get-lower-line=") {
		printStr("[_UsageCommand_]--env-get-lower-line[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...]")
		printStr("[_UsageCommand_]--env-get-lower-line=[-][_UsageVar_]<var>[-]")
		printStr("	Get the line of a [_UsageVar_]<var>[-]iable")
	}
	if match("--env-get-lower-literal", "--env-get-lower-literal=") {
		printStr("[_UsageCommand_]--env-get-lower-literal[-] [_UsageVar_]<var>[-] [[_UsageVar_]<var>[-] ...][-]")
		printStr("[_UsageCommand_]--env-get-lower-literal=[-][_UsageVar_]<var>[-]")
		printStr("	Get the literal value (including quotes) of a [_UsageVar_]<var>[-]iable")
	}
	if match("--env-set", "--env-set=") {
		printStr("[_UsageCommand_]--env-set[-] [_UsageVar_]<var>=<val>[-]")
		printStr("[_UsageCommand_]--env-set=[-][_UsageVar_]<var>,<val>[-]")
		printStr("	Set the [_UsageVar_]<val>[-]ue of a [_UsageVar_]<var>[-]iable (variable name is forced to UPPER CASE).")
	}
	if match("--env-set-lower", "--env-set-lower=") {
		printStr("[_UsageCommand_]--env-set-lower[-] [_UsageVar_]<var>=<val>[-]")
		printStr("[_UsageCommand_]--env-set-lower=[-][_UsageVar_]<var>,<val>[-]")
		printStr("	Set the [_UsageVar_]<val>[-]ue of a [_UsageVar_]<var>[-]iable")
	}
	if match("-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced") {
		printStr("[_UsageCommand_]-l --list[-]")
		printStr("	List all apps")
		printStr("[_UsageCommand_]--list-added[-]")
		printStr("	List added apps")
		printStr("[_UsageCommand_]--list-builtin[-]")
		printStr("	List builtin apps")
		printStr("[_UsageCommand_]--list-deprecated[-]")
		printStr("	List deprecated apps")
		printStr("[_UsageCommand_]--list-enabled[-]")
		printStr("	List enabled apps")
		printStr("[_UsageCommand_]--list-disabled[-]")
		printStr("	List disabled apps")
		printStr("[_UsageCommand_]--list-nondeprecated[-]")
		printStr("	List non-deprecated apps")
		printStr("[_UsageCommand_]--list-referenced[-]")
		printStr("	List referenced apps")
	}
	if match("-h", "--help") {
		printStr("[_UsageCommand_]-h --help[-]")
		printStr("	Show this usage information")
		printStr("[_UsageCommand_]-h --help[-] [_UsageOption_]<option>[-]")
		printStr("	Show the usage of the specified option")
	}
	if match("-i", "--install") {
		printStr("[_UsageCommand_]-i --install[-]")
		printStr("	Install/update all dependencies")
	}
	if match("-p", "--prune") {
		printStr("[_UsageCommand_]-p --prune[-]")
		printStr("	Remove unused docker resources")
	}
	if match("-r", "--remove") {
		printStr("[_UsageCommand_]-r --remove[-]")
		printStr("	Prompt to remove variables for all disabled apps")
		printStr("[_UsageCommand_]-r --remove[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Prompt to remove the variables for the app specified")
	}
	if match("-R", "--reset") {
		printStr("[_UsageCommand_]-R --reset[-]")
		printStr(fmt.Sprintf("	Resets [_ApplicationName_]%s to always process environment files.", appName))
		printStr("	This is usually not needed unless you have modified application templates yourself.")
	}
	if match("-s", "--status") {
		printStr("[_UsageCommand_]-s --status[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Returns the enabled/disabled status for the app specified")
	}
	if match("--status-disable", "--status-enable") {
		printStr("[_UsageCommand_]--status-disable[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Disable the app specified")
		printStr("[_UsageCommand_]--status-enable[-] [_UsageApp_]<app>[-] [[_UsageApp_]<app>[-] ...][-]")
		printStr("	Enable the app specified")
	}
	if match("-t", "--test") {
		printStr("[_UsageCommand_]-t --test[-] [_UsageFile_]<test_name>[-]")
		printStr("	Run tests to check the program")
	}
	if match("-T", "--theme", "--theme-list", "--theme-table", "--theme-lines", "--theme-no-lines", "--theme-borders", "--theme-no-borders", "--theme-shadows", "--theme-no-shadows", "--theme-scrollbar", "--theme-no-scrollbar") {
		printStr("[_UsageCommand_]-T --theme[-]")
		printStr("	Shows the current theme")
		printStr("[_UsageCommand_]-T --theme[-] [_UsageTheme_]<themename>[-]")
		printStr("	Saves and applies the specified theme to the GUI")
		printStr("[_UsageCommand_]--theme-list[-]")
		printStr("	Lists the available themes")
		printStr("[_UsageCommand_]--theme-table[-]")
		printStr("	Lists the available themes in a table format")
		printStr("[_UsageCommand_]--theme-lines[-]")
		printStr("[_UsageCommand_]--theme-no-lines[-]")
		printStr("	Turn the line drawing characters on or off in the GUI")
		printStr("[_UsageCommand_]--theme-borders[-]")
		printStr("[_UsageCommand_]--theme-no-borders[-]")
		printStr("	Turn the borders on and off in the GUI")
		printStr("[_UsageCommand_]--theme-shadows[-]")
		printStr("[_UsageCommand_]--theme-no-shadows[-]")
		printStr("	Turn the shadows on or off in the GUI")
		printStr("[_UsageCommand_]--theme-scrollbar[-]")
		printStr("[_UsageCommand_]--theme-no-scrollbar[-]")
		printStr("	Turn the scrollbar on or off in the GUI")
	}
	if match("-u", "--update", "--update-app", "--update-templates") {
		printStr("[_UsageCommand_]-u --update[-]")
		printStr("	Update [_ApplicationName_]DockSTARTer[-] using GitHub Releases and [_ApplicationName_]DockSTARTer-Templates[-] using git.")
		printStr("[_UsageCommand_]-u --update[-] [_UsageBranch_]<AppVersionOrChannel>[-] [_UsageBranch_]<TemplateBranch>[-]")
		printStr("	Update to specific versions. <AppVersionOrChannel> can be a version like 'v2.0.0.1' or a channel like 'testing'.")
		printStr("[_UsageCommand_]--update-app[-]")
		printStr("	Update [_ApplicationName_]DockSTARTer[-] only.")
		printStr("[_UsageCommand_]--update-app[-] [_UsageBranch_]<AppVersionOrChannel>[-]")
		printStr("	Update [_ApplicationName_]DockSTARTer[-] to the specified version or channel.")
		printStr("[_UsageCommand_]--update-templates[-]")
		printStr("	Update [_ApplicationName_]DockSTARTer-Templates[-] only.")
		printStr("[_UsageCommand_]--update-templates[-] [_UsageBranch_]<TemplateBranch>[-]")
		printStr("	Update [_ApplicationName_]DockSTARTer-Templates[-] to the specified branch.")
	}
	if match("-V", "--version") {
		printStr("[_UsageCommand_]-V --version[-]")
		printStr("	Display version information")
		printStr("[_UsageCommand_]-V --version[-] [_UsageBranch_]<AppBranch>[-] [_UsageBranch_]<TemplateBranch>[-]")
		printStr("	Display version information for the specified branches")
	}

	if showAll {
		printStr("")
		printStr("Menu Commands:")
		printStr("")
	}

	// Menu Commands
	if match("-M", "--menu") {
		printStr("[_UsageCommand_]-M --menu[-]")
		printStr("	Start the menu system.")
		printStr(fmt.Sprintf("	This is the same as typing '[_UsageCommand_]%s[-]'.", appCmd))
		printStr("[_UsageCommand_]-M --menu[-] < [_UsageOption_]main[-] | [_UsageOption_]config[-] | [_UsageOption_]options[-] >[-]")
		printStr("	Load the specified page in the menu.")
		printStr("[_UsageCommand_]-M --menu[-] < [_UsageOption_]options-display[-] | [_UsageOption_]display[-] >[-]")
		printStr("	Load the [_UsagePage_]Display Options[-] page in the menu.")
		printStr("[_UsageCommand_]-M --menu[-] < [_UsageOption_]options-theme[-] | [_UsageOption_]theme[-] >[-]")
		printStr("	Load the [_UsagePage_]Theme Chooser[-] page in the menu.")
		printStr("[_UsageCommand_]-M --menu[-] < [_UsageOption_]config-app-select[-] | [_UsageOption_]app-select[-] | [_UsageOption_]select[-] >[-]")
		printStr("	Load the [_UsagePage_]Application Selection[-] page in the menu.")
	}
	if match("-S", "--select", "--menu-config-app-select", "--menu-app-select") {
		printStr("[_UsageCommand_]-S --select[-]")
		printStr("	Load the [_UsagePage_]Application Selection[-] page in the menu.")
	}

	return strings.TrimRight(sb.String(), "\n")
}

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

	// Environment Variables
	pflag.BoolP("env", "e", false, "Update .env files")
	pflag.String("env-appvars", "", "List variable names for app")
	pflag.String("env-appvars-lines", "", "List variable lines for app")
	pflag.String("env-get", "", "Get variable value")
	pflag.String("env-get-line", "", "Get variable line")
	pflag.String("env-get-literal", "", "Get variable literal value")
	pflag.String("env-set", "", "Set variable value")

	// Configuration / Menu
	pflag.StringP("menu", "M", "", "Show menu (main, config, options, etc.)")
	pflag.String("config-pm", "", "Config package manager")
	pflag.Bool("config-pm-auto", false, "Auto-detect package manager")
	pflag.Bool("config-show", false, "Show configuration")

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
	pflag.Bool("theme-scrollbar", false, "Turn scrollbar on")
	pflag.Bool("theme-no-scrollbar", false, "Turn scrollbar off")

	// Testing
	pflag.StringP("test", "t", "", "Run test script")
}
