package main

import (
	"fmt"
	"charm.land/glamour/v2"
)

func main() {
	r, _ := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"))
	out, _ := r.Render("[Link](http://google.com)")
	fmt.Printf("Glamour: %q\n", out)
}
