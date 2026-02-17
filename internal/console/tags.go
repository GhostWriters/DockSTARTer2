package console

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/muesli/termenv"
)

var (
	// Delimiters for semantic and direct tags (Library Standard)
	SemanticPrefix = "{{|"
	SemanticSuffix = "|}}"
	DirectPrefix   = "{{["
	DirectSuffix   = "]}}"

	// Precompiled regexes (Locked to standard)
	semanticRegex *regexp.Regexp
	directRegex   *regexp.Regexp
)

func init() {
	rebuildRegexes()
}

func rebuildRegexes() {
	// Re-compile based on static defaults
	// We use QuoteMeta to be safe, even though we know the defaults.
	semEscPre := regexp.QuoteMeta(SemanticPrefix)
	semEscSuf := regexp.QuoteMeta(SemanticSuffix)
	dirEscPre := regexp.QuoteMeta(DirectPrefix)
	dirEscSuf := regexp.QuoteMeta(DirectSuffix)

	semanticRegex = regexp.MustCompile(semEscPre + `(?P<content>[A-Za-z0-9_]+)` + semEscSuf)
	directRegex = regexp.MustCompile(dirEscPre + `(?P<content>[A-Za-z0-9_:\-#;]+)` + dirEscSuf)
}

// GetDelimitedRegex returns the standard regex for both semantic and direct tags.
func GetDelimitedRegex() *regexp.Regexp {
	semEscPre := regexp.QuoteMeta(SemanticPrefix)
	semEscSuf := regexp.QuoteMeta(SemanticSuffix)
	dirEscPre := regexp.QuoteMeta(DirectPrefix)
	dirEscSuf := regexp.QuoteMeta(DirectSuffix)

	// Group 1: Semantic, Group 2: Direct
	pattern := fmt.Sprintf(`(?:%s(?P<semantic>[A-Za-z0-9_]+)%s|%s(?P<direct>[A-Za-z0-9_:\-#;]+)%s)`,
		semEscPre, semEscSuf, dirEscPre, dirEscSuf)
	return regexp.MustCompile(pattern)
}

// GetDirectRegex returns the standard regex for direct tags only.
func GetDirectRegex() *regexp.Regexp {
	return directRegex
}

// WrapSemantic wraps a tag name in standard semantic delimiters
func WrapSemantic(name string) string {
	return SemanticPrefix + name + SemanticSuffix
}

// WrapDirect wraps a style code in standard direct delimiters
func WrapDirect(code string) string {
	return DirectPrefix + code + DirectSuffix
}

// ExpandTags converts semantic tags to standardized direct format using raw style codes
func ExpandTags(text string) string {
	ensureMaps()

	// 1. Process semantic tags
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Extract content using the named group
		groupIndex := semanticRegex.SubexpIndex("content")
		subMatch := semanticRegex.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		content := strings.ToLower(subMatch[groupIndex])

		// Check semantic map (stores RAW codes)
		if rawCode, ok := semanticMap[content]; ok {
			// Wrap the raw code in standard direct delimiters
			return WrapDirect(rawCode)
		}

		// Unknown semantic tag - strip it
		return ""
	})

	return text
}

// ToANSI converts semantic and direct tags to ANSI escape sequences.
// It uses the default stdout profile.
func ToANSI(text string) string {
	return ToANSIWithProfile(text)
}

// ToANSIWithProfile allows specifying a profile (e.g. for TUI vs CLI).
func ToANSIWithProfile(text string, profile ...termenv.Profile) string {
	ensureMaps()

	p := preferredProfile
	if len(profile) > 0 {
		p = profile[0]
	} else if !isTTYGlobal && !TUIMode {
		// Only check globals if no specific profile was requested
		return Strip(text)
	}

	// If the profile is ASCII, just strip
	if p == termenv.Ascii {
		return Strip(text)
	}

	// 1. Expand all semantic tags first (Pass 1)
	text = ExpandTags(text)

	// 2. Process all direct tags -> ANSI (Pass 2)
	re := GetDirectRegex()
	return re.ReplaceAllStringFunc(text, func(match string) string {
		groupIndex := re.SubexpIndex("content")
		subMatch := re.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		return parseStyleCodeToANSI(subMatch[groupIndex], p)
	})
}

// Strip removes all semantic and direct tags from text, as well as ANSI escape sequences
func Strip(text string) string {
	text = semanticRegex.ReplaceAllString(text, "")
	text = directRegex.ReplaceAllString(text, "")
	return StripANSI(text)
}

// ForTUI prepares text for display with standardized tags
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
