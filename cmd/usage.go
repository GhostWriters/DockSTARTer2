package cmd

import (
	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"fmt"
)

// PrintHelp prints usage information to stdout.
// Used by the standalone CLI.
func PrintHelp(target string) {
	fmt.Println(console.ToConsoleANSI(commands.GetUsage(target, false)))
}

// GetUsage returns usage information as a string.
// Delegates to the shared internal/commands implementation.
func GetUsage(target string, noHeading bool) string {
	return commands.GetUsage(target, noHeading)
}
