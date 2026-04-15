package main

import (
	"fmt"
	"charm.land/glamour/v2"
)

func main() {
	r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"))
	out, _ := r.Render("[Link](http://example.com)")
	
	for _, c := range out {
		if c < 32 || c > 126 {
			fmt.Printf("\\x%02x", c)
		} else {
			fmt.Printf("%c", c)
		}
	}
	fmt.Println()
}
