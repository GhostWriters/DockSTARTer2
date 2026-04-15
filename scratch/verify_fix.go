package main

import (
	"fmt"
	"DockSTARTer2/internal/console"
)

func main() {
	// Enable TUI mode to ensure hyperlinks are processed
	console.SetTUIEnabled(true)
	
	// Test case: URL with internal bold direct style
	input := "{{|URL|}}https://github.com/{{[bold]}}GhostWriters{{[-]}}/DockSTARTer2{{[-]}}"
	output := console.ToConsoleANSI(input)
	
	fmt.Printf("Input:  %s\n", input)
	fmt.Printf("Output (Raw): %q\n", output)
}
