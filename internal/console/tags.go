package console

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// semanticRegex matches {{_content_}} format for semantic tags
	semanticRegex = regexp.MustCompile(`\{\{_([A-Za-z0-9_]+)_\}\}`)

	// directRegex matches {{|content|}} format for direct tview-style codes
	directRegex = regexp.MustCompile(`\{\{\|([A-Za-z0-9_:\-#]+)\|\}\}`)
)

// ExpandTags converts semantic and direct tags to standardized {{|style|}} format
// - {{_Tag_}} : Semantic lookup
// - {{|code|}} : Direct style (no-op, just for consistency)
func ExpandTags(text string) string {
	ensureMaps()

	// 1. Process semantic tags {{_Tag_}}
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{_" and "_}}"
		content = strings.ToLower(content)

		// Check semantic map
		if tag, ok := semanticMap[content]; ok {
			return tag
		}

		// Unknown semantic tag - strip it
		return ""
	})

	return text
}

// ToANSI converts semantic and direct tags to ANSI escape sequences
// - {{_Tag_}} : Semantic lookup -> ANSI
// - {{|code|}} : Direct tview-style -> ANSI
func ToANSI(text string) string {
	ensureMaps()
	// FORCE TTY for debugging
	if !isTTYGlobal {
		// Not a TTY, strip all codes
		return Strip(text)
	}

	// 1. Expand all semantic tags first (Pass 1)
	// This ensures that multi-tag definitions like {{|-|}}{{|blue|}} are fully expanded
	text = ExpandTags(text)

	// 2. Process all direct tags {{|code|}} -> ANSI (Pass 2)
	text = directRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{|" and "|}}"
		return parseStyleCodeToANSI(content)
	})

	return text
}

// Strip removes all semantic and direct tags from text, as well as ANSI escape sequences
func Strip(text string) string {
	text = semanticRegex.ReplaceAllString(text, "")
	text = directRegex.ReplaceAllString(text, "")
	return StripANSI(text)
}

// ForTUI prepares text for display with standardized tags.
// Literal brackets [text] are now treated as plain text and do NOT need escaping.
func ForTUI(text string) string {
	return ExpandTags(text)
}

// Sprintf formats according to a format specifier and returns the string with ANSI codes
func Sprintf(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	return ToANSI(msg)
}

// Println prints a line with ANSI color codes parsed
func Println(a ...any) {
	msg := fmt.Sprint(a...)
	fmt.Println(ToANSI(msg))
}
