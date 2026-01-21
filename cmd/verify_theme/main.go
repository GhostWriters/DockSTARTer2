package main

import (
	"DockSTARTer2/internal/theme"
	"fmt"
	"log"
)

func main() {
	fmt.Println("Loading DockSTARTer theme...")
	err := theme.Load("DockSTARTer")
	if err != nil {
		log.Fatalf("Failed to load theme: %v", err)
	}

	fmt.Printf("ScreenFG: %v (Name: %s)\n", theme.Current.ScreenFG, theme.Current.ScreenFG.Name())
	fmt.Printf("ScreenBG: %v (Name: %s)\n", theme.Current.ScreenBG, theme.Current.ScreenBG.Name()) // Reverted

	// Also check Default
	theme.Default()
	fmt.Printf("Default ScreenFG: %v (Name: %s)\n", theme.Current.ScreenFG, theme.Current.ScreenFG.Name())
	fmt.Printf("Default ScreenBG: %v (Name: %s)\n", theme.Current.ScreenBG, theme.Current.ScreenBG.Name())
}
