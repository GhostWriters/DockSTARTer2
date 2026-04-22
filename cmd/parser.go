package cmd

import (
	"DockSTARTer2/internal/commands"
)

// ParseError is a wrapper for commands.ParseError
type ParseError = commands.ParseError

// CommandGroup is a wrapper for commands.CommandGroup
type CommandGroup = commands.CommandGroup

// ErrHelp is a wrapper for commands.ErrHelp
var ErrHelp = commands.ErrHelp

// Parse parses the raw command line arguments into groups.
// Delegates to the shared internal/commands implementation.
func Parse(args []string) ([]CommandGroup, error) {
	return commands.Parse(args)
}

// Flatten converts a slice of CommandGroups into a single slice of strings.
// Delegates to the shared internal/commands implementation.
func Flatten(groups []CommandGroup) []string {
	return commands.Flatten(groups)
}
