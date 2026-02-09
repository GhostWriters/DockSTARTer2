package console

import (
	"strings"
)

// This file provides legacy compatibility exports for backward compatibility.
// All actual implementations have been moved to more focused files:
// - color_parse.go: Color resolution and parsing
// - profile.go: Terminal profile detection
// - ansi.go: ANSI escape code generation
// - tags.go: Tag parsing and expansion
// - registry.go: Semantic tag registry management

// Parse is a convenience alias for ToANSI (backwards compatibility)
func Parse(text string) string {
	return ToANSI(text)
}

// Translate is a convenience alias for ExpandTags (backwards compatibility)
func Translate(text string) string {
	return ExpandTags(text)
}

// ExpandSemanticTags is a convenience alias for ExpandTags (backwards compatibility)
func ExpandSemanticTags(text string) string {
	return ExpandTags(text)
}

// TranslateToTagged is a convenience alias for ExpandTags
func TranslateToTagged(text string) string {
	return ExpandTags(text)
}

// RegisterColor is a legacy alias for RegisterSemanticTag
func RegisterColor(name, value string) {
	// Strip underscore wrapper if present (legacy format)
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	RegisterSemanticTag(name, value)
}

// ToCviewTag is a no-op for compatibility (tags are already in proper format)
func ToCviewTag(tag string) string {
	return tag
}
