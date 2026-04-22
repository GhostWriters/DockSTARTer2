package commands

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/version"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

var ErrHelp = errors.New("help shown")

// ParseError wraps argument parsing errors to provide rich output matching bash script style
type ParseError struct {
	Args           []string // The full argument list passed to Parse
	Index          int      // The index where the error occurred
	Message        string   // The specific error message
	FailingCommand string   // The command being processed (e.g. "--test")
}

func (e *ParseError) Error() string {
	indent := "   "

	// Build command line string
	var cmdLineParts []string

	// Prepend "ds2" as the command text
	cmdLineParts = append(cmdLineParts, fmt.Sprintf("{{|UserCommand|}}%s{{[-]}}", version.CommandName))

	for i := 0; i <= e.Index && i < len(e.Args); i++ {
		str := e.Args[i]
		if i == e.Index {
			// Highlight failing option
			str = fmt.Sprintf("{{|UserCommandError|}}%s{{[-]}}", str)
		} else {
			str = fmt.Sprintf("{{|UserCommand|}}%s{{[-]}}", str)
		}
		cmdLineParts = append(cmdLineParts, str)
	}

	// Join parts and wrap in single quotes as a whole visual unit (bash style)
	cmdLineStr := "'" + strings.Join(cmdLineParts, " ") + "'"
	caretOffset := len(indent) + 1 + len(version.CommandName) + 1
	for i := 0; i < e.Index && i < len(e.Args); i++ {
		caretOffset += len(e.Args[i]) + 1
	}
	pointerLine := strutil.Repeat(" ", caretOffset) + "{{|UserCommandErrorMarker|}}^{{[-]}}"

	// Format Message
	failingOpt := ""
	if e.Index < len(e.Args) {
		failingOpt = e.Args[e.Index]
	}

	formattedCmd := fmt.Sprintf("'{{|UserCommand|}}%s{{[-]}}'", e.FailingCommand)
	formattedOpt := fmt.Sprintf("'{{|UserCommand|}}%s{{[-]}}'", failingOpt)

	replacer := strings.NewReplacer(
		"%c", formattedCmd,
		"%o", formattedOpt,
	)
	formattedMsg := replacer.Replace(e.Message)

	out := fmt.Sprintf("Error in command line:\n\n%s%s\n%s\n\n%s%s\n", indent, cmdLineStr, pointerLine, indent, formattedMsg)

	if e.FailingCommand != "" {
		out += fmt.Sprintf("\n%sUsage is:\n", indent)
		usageStr := GetUsage(e.FailingCommand, true)
		lines := strings.Split(usageStr, "\n")
		for _, line := range lines {
			out += fmt.Sprintf("%s%s\n", indent, line)
		}
	} else {
		out += fmt.Sprintf("\n%sRun '{{|UserCommand|}}%s --help{{[-]}}' for usage.\n", indent, version.CommandName)
	}

	return out
}

// CommandGroup represents a parsed group of flags and a command with its arguments
type CommandGroup struct {
	Flags   []string
	Command string
	Args    []string
}

// FullSlice returns the reconstructed slice of strings for the group
func (cg CommandGroup) FullSlice() []string {
	var s []string
	s = append(s, cg.Flags...)
	if cg.Command != "" {
		s = append(s, cg.Command)
	}
	s = append(s, cg.Args...)
	return s
}

// CommandSlice returns the command and its arguments as a slice
func (cg CommandGroup) CommandSlice() []string {
	var s []string
	if cg.Command != "" {
		s = append(s, cg.Command)
	}
	s = append(s, cg.Args...)
	return s
}

// Flatten converts a slice of CommandGroups into a single slice of strings
func Flatten(groups []CommandGroup) []string {
	var s []string
	for _, g := range groups {
		s = append(s, g.FullSlice()...)
	}
	return s
}

// Parse parses the raw command line arguments into groups of command operations
// mimicking the logic in DockSTARTer's bash implementation.
// Each call uses a fresh FlagSet so repeated calls (e.g. from the console panel)
// never share or re-register flag state with each other or the CLI.
func Parse(args []string) ([]CommandGroup, error) {
	fs := NewFlagSet()

	// Lists based on cmdline.sh
	modifiers := map[string]bool{
		"-f": true, "--force": true,
		"-g": true, "--gui": true,
		"-v": true, "--verbose": true,
		"-x": true, "--debug": true,
		"-y": true, "--yes": true,
	}

	IsModifier := func(s string) bool {
		return modifiers[s]
	}

	// Pre-process args to expand combined short flags (e.g. -fc -> -f -c)
	var expandedArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 {
			chars := arg[1:]
			for _, c := range chars {
				expandedArgs = append(expandedArgs, fmt.Sprintf("-%c", c))
			}
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	var groups []CommandGroup
	var currentGroup CommandGroup
	var lastCommand string

	i := 0
	for i < len(expandedArgs) {
		arg := expandedArgs[i]

		if !strings.HasPrefix(arg, "-") {
			return nil, &ParseError{Args: expandedArgs, Index: i, Message: fmt.Sprintf("invalid option '%s'", arg), FailingCommand: lastCommand}
		}

		if IsModifier(arg) {
			currentGroup.Flags = append(currentGroup.Flags, arg)
			lastCommand = arg
			i++
			continue
		}

		// Validate that the command is a known flag
		cmdToCheck := arg
		if strings.Contains(cmdToCheck, "=") {
			cmdToCheck = strings.SplitN(cmdToCheck, "=", 2)[0]
		}

		cmdName := strings.TrimLeft(cmdToCheck, "-")
		var validFlag *pflag.Flag
		if strings.HasPrefix(cmdToCheck, "--") {
			validFlag = fs.Lookup(cmdName)
		} else if len(cmdName) == 1 {
			validFlag = fs.ShorthandLookup(cmdName)
		}

		if validFlag == nil {
			return nil, &ParseError{Args: expandedArgs, Index: i, Message: "Invalid option %o"}
		}

		// Set command
		currentGroup.Command = arg
		lastCommand = arg
		cmd := arg
		i++

		consumesUntilDash := false

		switch cmd {
		case "-a", "--add",
			"--env-appvars", "--env-appvars-lines",
			"--env-get", "--env-get-line", "--env-get-line-regex", "--env-get-literal",
			"--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal",
			"--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal",
			"-r", "--remove",
			"-s", "--status",
			"--status-enable", "--status-disable":
			consumesUntilDash = true

		case "-c", "--compose":
			if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
				sub := expandedArgs[i]
				validSubs := map[string]bool{
					"pull": true, "up": true, "down": true, "stop": true, "restart": true, "update": true,
					"generate": true, "merge": true,
				}
				if !validSubs[sub] {
					return nil, &ParseError{Args: expandedArgs, Index: i, FailingCommand: cmd, Message: "Invalid option %o"}
				}
				currentGroup.Args = append(currentGroup.Args, sub)
				i++
				if sub != "generate" && sub != "merge" {
					consumesUntilDash = true
				}
			}

		case "-t", "--test", "--man", "--config-pm", "--config-folder", "--config-compose-folder", "--theme-border-color",
			"--edit-app", "--start-edit-app",
			"--env-edit", "--env-edit-lower":
			if i >= len(expandedArgs) || strings.HasPrefix(expandedArgs[i], "-") {
				return nil, &ParseError{Args: expandedArgs, Index: i - 1, FailingCommand: cmd, Message: fmt.Sprintf("Command %s requires an argument.", cmd)}
			}
			currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
			i++

		case "-T", "--theme", "-S", "--select", "--menu-config-app-select", "--menu-app-select", "--theme-shadow-level",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title":
			if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
				currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
				i++
			}

		case "-M", "--menu":
			if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
				sub := expandedArgs[i]
				validSubs := map[string]bool{
					"main": true, "config": true, "options": true,
					"options-display": true, "display": true,
					"options-theme": true, "theme": true, "appearance": true,
					"config-app-select": true, "app-select": true, "select": true,
				}
				bare := strings.TrimPrefix(sub, "start-")
				if !validSubs[sub] && (!strings.HasPrefix(sub, "start-") || !validSubs[bare]) {
					return nil, &ParseError{Args: expandedArgs, Index: i, FailingCommand: cmd, Message: "Invalid option %o"}
				}
				currentGroup.Args = append(currentGroup.Args, sub)
				i++
			}

		case "--server":
			if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
				sub := expandedArgs[i]
				validSubs := map[string]bool{
					"status": true, "start": true, "stop": true, "restart": true,
					"disconnect": true, "install": true, "uninstall": true,
					"enable": true, "disable": true,
				}
				if !validSubs[sub] {
					return nil, &ParseError{Args: expandedArgs, Index: i, FailingCommand: cmd, Message: "Invalid option %o"}
				}
				currentGroup.Args = append(currentGroup.Args, sub)
				i++
			}

		case "--theme-extract":
			if i >= len(expandedArgs) || strings.HasPrefix(expandedArgs[i], "-") {
				return nil, &ParseError{Args: expandedArgs, Index: i - 1, FailingCommand: cmd, Message: fmt.Sprintf("Command %s requires a theme name.", cmd)}
			}
			currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
			i++
			for count := 0; count < 2; count++ {
				if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
					currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
					i++
				} else {
					break
				}
			}

		case "--theme-extract-all":
			if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
				currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
				i++
			}

		case "-u", "--update", "--update-app", "--update-templates", "-V", "--version":
			for count := 0; count < 2; count++ {
				if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
					currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
					i++
				} else {
					break
				}
			}

		case "-h", "--help":
			if i < len(expandedArgs) && strings.HasPrefix(expandedArgs[i], "-") {
				currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
				i++
			}

		case "--server-daemon":
			currentGroup.Args = append(currentGroup.Args, expandedArgs[i:]...)
			i = len(expandedArgs)

		case "-i", "--install",
			"-p", "--prune",
			"-R", "--reset",
			"-e", "--env",
			"--edit-global", "--start-edit-global",
			"-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced",
			"--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table",
			"--config-show", "--show-config",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-button-borders", "--theme-no-button-borders",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow",
			"--theme-scrollbar", "--theme-no-scrollbar":
			// Do nothing, consumesUntilDash is false

		default:
		}

		if consumesUntilDash {
			for i < len(expandedArgs) {
				nextArg := expandedArgs[i]
				if strings.HasPrefix(nextArg, "-") {
					break
				}
				currentGroup.Args = append(currentGroup.Args, nextArg)
				i++
			}
		}

		groups = append(groups, currentGroup)
		currentGroup = CommandGroup{}
	}

	if len(currentGroup.Flags) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups, nil
}
