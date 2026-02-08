package cmd

import (
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
	cmdLineParts = append(cmdLineParts, fmt.Sprintf("{{_UserCommand_}}%s{{|-|}}", version.CommandName))

	for i := 0; i <= e.Index && i < len(e.Args); i++ {
		str := e.Args[i]
		if i == e.Index {
			// Highlight failing option
			str = fmt.Sprintf("{{_UserCommandError_}}%s{{|-|}}", str)
		} else {
			str = fmt.Sprintf("{{_UserCommand_}}%s{{|-|}}", str)
		}
		cmdLineParts = append(cmdLineParts, str)
	}

	// Join parts and wrap in single quotes as a whole visual unit (bash style)
	// bash: 'previous parts failing_part'
	cmdLineStr := "'" + strings.Join(cmdLineParts, " ") + "'"
	// Indent + ' + ds2 + space + previous args + spaces
	caretOffset := len(indent) + 1 + len(version.CommandName) + 1 // "   " + "'" + "ds2" + " "
	for i := 0; i < e.Index && i < len(e.Args); i++ {
		caretOffset += len(e.Args[i]) + 1 // arg + space
	}
	pointerLine := strings.Repeat(" ", caretOffset) + "{{_UserCommandErrorMarker_}}^{{|-|}}"

	// Format Message
	// Message might contain %c (command) or %o (option)
	failingOpt := ""
	if e.Index < len(e.Args) {
		failingOpt = e.Args[e.Index]
	}

	// Bash uses UserCommand (yellow) for %c and %o in the message text itself
	formattedCmd := fmt.Sprintf("'{{_UserCommand_}}%s{{|-|}}'", e.FailingCommand)
	formattedOpt := fmt.Sprintf("'{{_UserCommand_}}%s{{|-|}}'", failingOpt)

	replacer := strings.NewReplacer(
		"%c", formattedCmd,
		"%o", formattedOpt,
	)
	formattedMsg := replacer.Replace(e.Message) // Parse checks will handle tags

	out := fmt.Sprintf("Error in command line:\n\n%s%s\n%s\n\n%s%s\n", indent, cmdLineStr, pointerLine, indent, formattedMsg)

	// Add Usage if applicable
	// Add Usage if applicable
	if e.FailingCommand != "" {
		out += fmt.Sprintf("\n%sUsage is:\n", indent)
		// Use rich usage text
		usageStr := GetUsage(e.FailingCommand)
		lines := strings.Split(usageStr, "\n")
		for _, line := range lines {
			out += fmt.Sprintf("%s%s\n", indent, line)
		}
	} else {
		out += fmt.Sprintf("\n%sRun '{{_UserCommand_}}%s --help{{|-|}}' for usage.\n", indent, version.CommandName)
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
func Parse(args []string) ([]CommandGroup, error) {
	// Initialize flags to make sure help text is available
	InitFlags()

	// Lists based on cmdline.sh
	modifiers := map[string]bool{
		"-f": true, "--force": true,
		"-g": true, "--gui": true,
		"-v": true, "--verbose": true,
		"-x": true, "--debug": true,
		"-y": true, "--yes": true,
	}

	// IsModifier checks if a string is a command arg (starts with -) but not a modifier
	// In the bash script, anything starting with - that isn't a modifier is treated as a command
	// (or part of a command sequence).
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
			// Non-flag argument at top level?
			return nil, &ParseError{Args: expandedArgs, Index: i, Message: fmt.Sprintf("invalid option '%s'", arg), FailingCommand: lastCommand}
		}

		// Combined shorts are already expanded above

		if IsModifier(arg) {
			currentGroup.Flags = append(currentGroup.Flags, arg)
			lastCommand = arg
			i++
			continue
		}

		// If not a modifier, and starts with -, it's a command.

		// Validate that the command is a known flag
		// Handle potential key=value formats (e.g. --env-get=VAR)
		cmdToCheck := arg
		if strings.Contains(cmdToCheck, "=") {
			cmdToCheck = strings.SplitN(cmdToCheck, "=", 2)[0]
		}

		cmdName := strings.TrimLeft(cmdToCheck, "-")
		var validFlag *pflag.Flag
		if strings.HasPrefix(cmdToCheck, "--") {
			validFlag = pflag.Lookup(cmdName)
		} else if len(cmdName) == 1 {
			validFlag = pflag.CommandLine.ShorthandLookup(cmdName)
		}

		if validFlag == nil {
			return nil, &ParseError{Args: expandedArgs, Index: i, Message: "Invalid option %o"}
		}

		// Set command
		currentGroup.Command = arg
		lastCommand = arg
		cmd := arg // The command flag
		i++

		// Consume arguments for this command
		// Default behavior: Do NOT consume arguments unless specified
		consumesUntilDash := false

		switch cmd {
		// Commands that take unlimited arguments (until next flag)
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

				// generate and merge take NO more args
				if sub != "generate" && sub != "merge" {
					consumesUntilDash = true
				}
			}

		// Commands that require exactly ONE argument
		case "-t", "--test", "--config-pm":
			if i >= len(expandedArgs) || strings.HasPrefix(expandedArgs[i], "-") {
				return nil, &ParseError{Args: expandedArgs, Index: i - 1, FailingCommand: cmd, Message: fmt.Sprintf("Command %s requires an argument.", cmd)}
			}
			currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
			i++

		// Commands that accept OPTIONAL arguments (Max 1)
		case "-T", "--theme", "-S", "--select", "--menu-config-app-select", "--menu-app-select", "--theme-shadow-level":
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
					"options-theme": true, "theme": true,
					"config-app-select": true, "app-select": true, "select": true,
				}
				if !validSubs[sub] {
					return nil, &ParseError{Args: expandedArgs, Index: i, FailingCommand: cmd, Message: "Invalid option %o"}
				}
				currentGroup.Args = append(currentGroup.Args, sub)
				i++
			}

		// Commands that accept OPTIONAL arguments (Max 2)
		case "-u", "--update", "--update-app", "--update-templates", "-V", "--version":
			// Helper to consume up to N args
			for count := 0; count < 2; count++ {
				if i < len(expandedArgs) && !strings.HasPrefix(expandedArgs[i], "-") {
					currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
					i++
				} else {
					break
				}
			}

		// Commands with special parsing logic
		case "-h", "--help":
			// Help allows optional argument (next flag/cmd)
			// Check if next arg exists and starts with "-"
			if i < len(expandedArgs) && strings.HasPrefix(expandedArgs[i], "-") {
				currentGroup.Args = append(currentGroup.Args, expandedArgs[i])
				i++
			}

		// Commands that take NO arguments (Explicitly logic is same as default, but listed for clarity)
		case "-i", "--install",
			"-p", "--prune",
			"-R", "--reset",
			"-e", "--env",
			"-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced",
			"--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table",
			"--config-show", "--show-config",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow",
			"--theme-scrollbar", "--theme-no-scrollbar":
			// Do nothing, consumesUntilDash is false

		default:
			// If we missed a command, default is strict (consumes nothing).
			// This effectively makes unknown commands that were parsed as "starts with -" but aren't in this list behave as tags.
			// But wait, the IsModifier check happens before.
			// If it's a known command (in this switch or not), it's treated as a command.
			// If it's not in THIS switch, it consumes 0 args.
			// If the user provided args, they will be caught as "Invalid Option" in the next loop iteration.
		}

		if consumesUntilDash {
			for i < len(expandedArgs) {
				nextArg := expandedArgs[i]
				if strings.HasPrefix(nextArg, "-") {
					// Next flag starts
					break
				}
				currentGroup.Args = append(currentGroup.Args, nextArg)
				i++
			}
		}

		// Command group finished
		groups = append(groups, currentGroup)
		currentGroup = CommandGroup{} // Reset for next group
	}

	// Implicit menu command if only modifiers?
	// We just return the group as is. The caller logic will decide behavior.
	if len(currentGroup.Flags) > 0 {
		// Implicit menu command if only modifiers?
		// We just return the group as is. The caller logic will decide behavior.
		groups = append(groups, currentGroup)
	}

	return groups, nil
}
