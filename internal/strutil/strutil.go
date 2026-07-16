package strutil

import (
	"net/url"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// FileURL builds a well-formed file:// URI for an absolute filesystem path,
// percent-encoding via net/url rather than naive "file://"+path
// concatenation. Lives here (not a UI-specific package) so both the
// CLI/console layer and the TUI's rendering layer can share it.
//
// path may use "\" or "/" (DS2 runs on both Windows and Linux); a Windows
// drive path (e.g. "C:\Users\x") gets a leading "/" added before the drive
// letter to form "file:///C:/Users/x" -- without it the drive letter would
// be misread as a URL scheme/host.
func FileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

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
