package strutil

import (
	"charm.land/lipgloss/v2"
)

// Limit truncates a string to a specific width, accounting for ANSI codes
func Limit(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}

	// Simple truncation for now, could be improved to handle ANSI better if needed
	// BUT since RenderThemeText is used AFTER this in some places,
	// or we are truncating plain text, this is usually safe.
	// If the string has ANSI, lipgloss.Width(s) will be > width
	// and we return a truncated version.

	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// Repeat returns a string consisting of n copies of the string s.
// It's a safer version of strings.Repeat that handles negative counts.
func Repeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
