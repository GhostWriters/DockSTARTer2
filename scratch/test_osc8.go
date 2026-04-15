package main

import (
	"fmt"
	"charm.land/lipgloss/v2"
)

func main() {
	s := lipgloss.NewStyle().Hyperlink("http://google.com").Render("Link")
	fmt.Printf("%q\n", s)
}
