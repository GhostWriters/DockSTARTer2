package main

import (
	"fmt"
	"regexp"
	"strings"
	"charm.land/lipgloss/v2"
)

// Simplified HitRegion for testing
type HitRegion struct {
	ID     string
	X, Y   int
	Width  int
}

func ScanForHyperlinks(rendered string, offsetX, offsetY int) []HitRegion {
	var regions []HitRegion
	lines := strings.Split(rendered, "\n")

	// OSC 8 Regex: \x1b]8;[params];[url]\a[content]\x1b]8;;\a
	re := regexp.MustCompile(`\x1b\]8;.*?;(.*?)\x07(.*?)\x1b\]8;;\x07`)

	for y, line := range lines {
		matches := re.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) < 6 {
				continue
			}
			url := line[match[2]:match[3]]
			content := line[match[4]:match[5]]

			prefix := line[:match[0]]
			visualX := lipgloss.Width(prefix)
			visualW := lipgloss.Width(content)

			regions = append(regions, HitRegion{
				ID:    "link:" + url,
				X:     offsetX + visualX,
				Y:     offsetY + y,
				Width: visualW,
			})
		}
	}
	return regions
}

func main() {
	// Simulate a colored string with a link
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	link := lipgloss.NewStyle().Hyperlink("http://google.com").Render("Google")
	content := style.Render("Check out " + link + " now!")
	
	fmt.Printf("Rendered: %q\n", content)
	regions := ScanForHyperlinks(content, 0, 0)
	for _, r := range regions {
		fmt.Printf("HitRegion: ID=%s X=%d W=%d\n", r.ID, r.X, r.Width)
	}
}
