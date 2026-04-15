package main

import (
	"fmt"
	"DockSTARTer2/internal/console"
)

func main() {
	console.SetTUIEnabled(true)
	
	// Test case 1: Uppercase URL
	s1 := console.ToConsoleANSI("{{|URL|}}http://google.com{{[-]}}")
	fmt.Printf("Upper: %q\n", s1)
	
	// Test case 2: Lowercase url
	s2 := console.ToConsoleANSI("{{|url|}}http://example.com{{[-]}}")
	fmt.Printf("Lower: %q\n", s2)
}
