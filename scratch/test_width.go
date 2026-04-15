package main

import (
	"fmt"
	"charm.land/lipgloss/v2"
)

func main() {
	s := "Prefix " + lipgloss.NewStyle().Hyperlink("http://google.com").Render("Link") + " Suffix"
	fmt.Printf("String: %q\n", s)
	fmt.Printf("Visual Width: %d\n", lipgloss.Width(s))
}
