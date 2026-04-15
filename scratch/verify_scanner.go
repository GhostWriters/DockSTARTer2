package main

import (
	"fmt"
	"regexp"
)

func main() {
	// Test Regex for ScanForHyperlinks
	re := regexp.MustCompile(`\x1b\]8;.*?;(.*?)(?:\x07|\x1b\\)(.*?)\x1b\]8;;(?:\x07|\x1b\\)`)
	
	// Test 1: BEL terminator (standard lipgloss)
	s1 := "\x1b]8;;http://google.com\x07Google\x1b]8;;\x07"
	m1 := re.FindStringSubmatch(s1)
	if len(m1) > 2 {
		fmt.Printf("Test 1 (BEL): Link=%s Content=%s SUCCESS\n", m1[1], m1[2])
	} else {
		fmt.Printf("Test 1 (BEL): FAILED\n")
	}

	// Test 2: ST terminator (common in glamour/markdown)
	s2 := "\x1b]8;;http://example.com\x1b\\Example\x1b]8;;\x1b\\"
	m2 := re.FindStringSubmatch(s2)
	if len(m2) > 2 {
		fmt.Printf("Test 2 (ST): Link=%s Content=%s SUCCESS\n", m2[1], m2[2])
	} else {
		fmt.Printf("Test 2 (ST): FAILED\n")
	}
}
