package semstyle

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

// Parse is a convenience alias for ToConsoleANSI (backwards compatibility)
func Parse(text string) string {
	return ToConsoleANSI(text)
}

// Translate is a convenience alias for ExpandConsoleTags (backwards compatibility)
func (st *Styler) Translate(text string) string {
	return st.ExpandConsoleTags(text)
}

// ExpandSemanticTags is a convenience alias for ExpandConsoleTags (backwards compatibility)
func (st *Styler) ExpandSemanticTags(text string) string {
	return st.ExpandConsoleTags(text)
}

// TranslateToTagged is a convenience alias for ExpandConsoleTags
func (st *Styler) TranslateToTagged(text string) string {
	return st.ExpandConsoleTags(text)
}

// RegisterColor is a legacy alias for RegisterSemanticTag
func (st *Styler) RegisterColor(name, value string) {
	// Strip underscore wrapper if present (legacy format)
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	st.RegisterSemanticTag(name, value)
}

// ToCviewTag is a no-op for compatibility (tags are already in proper format)
func (st *Styler) ToCviewTag(tag string) string {
	return tag
}

// --- package-level delegators to Default ---
func Translate(text string) string {
	return Default.Translate(text)
}

func ExpandSemanticTags(text string) string {
	return Default.ExpandSemanticTags(text)
}

func TranslateToTagged(text string) string {
	return Default.TranslateToTagged(text)
}

func RegisterColor(name, value string) {
	Default.RegisterColor(name, value)
}

func ToCviewTag(tag string) string {
	return Default.ToCviewTag(tag)
}
