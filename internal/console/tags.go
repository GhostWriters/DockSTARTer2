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

// SetDelimiters updates the global tag delimiters and rebuilds the compiled regex patterns.
// Call this once at startup before any tag processing if non-standard delimiters are desired.
// semPre/semSuf wrap semantic tag names (e.g. "{{|" / "|}}").
// dirPre/dirSuf wrap direct style codes (e.g. "{{[" / "]}}").
func SetDelimiters(semPre, semSuf, dirPre, dirSuf string) {
	SemanticPrefix = semPre
	SemanticSuffix = semSuf
	DirectPrefix = dirPre
	DirectSuffix = dirSuf
	rebuildRegexes()
}

func rebuildRegexes() {
	semEscPre := regexp.QuoteMeta(SemanticPrefix)
	semEscSuf := regexp.QuoteMeta(SemanticSuffix)
	dirEscPre := regexp.QuoteMeta(DirectPrefix)
	dirEscSuf := regexp.QuoteMeta(DirectSuffix)

	semanticRegex = regexp.MustCompile(semEscPre + `(?P<content>[A-Za-z0-9_]+(?::[A-Za-z0-9_:\-#;~]*)?)` + semEscSuf)
	directRegex = regexp.MustCompile(dirEscPre + `(?P<content>[A-Za-z0-9_:\-#;~]+)` + dirEscSuf)
}

// GetDelimitedRegex returns the standard regex for both semantic and direct tags.
func GetDelimitedRegex() *regexp.Regexp {
	semEscPre := regexp.QuoteMeta(SemanticPrefix)
	semEscSuf := regexp.QuoteMeta(SemanticSuffix)
	dirEscPre := regexp.QuoteMeta(DirectPrefix)
	dirEscSuf := regexp.QuoteMeta(DirectSuffix)

	// Group 1: Semantic, Group 2: Direct
	pattern := fmt.Sprintf(`(?:%s(?P<semantic>[A-Za-z0-9_]+(?::[A-Za-z0-9_:\-#;~]*)?)%s|%s(?P<direct>[A-Za-z0-9_:\-#;~]+)%s)`,
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
	return ExpandTagsWithPrefix(text, "")
}

// ExpandTagsWithPrefix converts semantic tags to standardized direct format,
// attempting to resolve with the given prefix first (e.g., "Preview_").
func ExpandTagsWithPrefix(text string, prefix string) string {
	ensureMaps()
	prefix = strings.ToLower(prefix)

	// Process semantic tags
	semanticMu.RLock()
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		groupIndex := semanticRegex.SubexpIndex("content")
		subMatch := semanticRegex.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		fullContent := subMatch[groupIndex]

		// Split semantic name from optional inline modifiers (e.g. "Theme_Title:::R" -> "Theme_Title" + "::R")
		semanticName := fullContent
		modifiers := ""
		if idx := strings.IndexByte(fullContent, ':'); idx >= 0 {
			semanticName = fullContent[:idx]
			modifiers = fullContent[idx+1:]
		}
		content := strings.ToLower(semanticName)

		// 1. Try with prefix if provided
		if prefix != "" {
			prefixed := prefix + content
			if rawCode, ok := semanticMap[prefixed]; ok {
				result := WrapDirect(rawCode)
				if modifiers != "" {
					result += WrapDirect(modifiers)
				}
				return result
			}
		}

		// 2. Try raw name
		if rawCode, ok := semanticMap[content]; ok {
			result := WrapDirect(rawCode)
			if modifiers != "" {
				result += WrapDirect(modifiers)
			}
			return result
		}

		return ""
	})
	semanticMu.RUnlock()

	return text
}

// ToANSI converts semantic and direct tags to ANSI escape sequences.
// It uses the default stdout profile.
func ToANSI(text string) string {
	return ToANSIWithProfile(text)
}

// ToANSIWithProfile allows specifying a profile (e.g. for TUI vs CLI).
func ToANSIWithProfile(text string, profile ...termenv.Profile) string {
	return ToANSIWithPrefix(text, "", profile...)
}

// ToANSIWithPrefix allows specifying a prefix for namespaced tag resolution and a profile.
func ToANSIWithPrefix(text string, prefix string, profile ...termenv.Profile) string {
	ensureMaps()

	p := preferredProfile
	if len(profile) > 0 {
		p = profile[0]
	} else if !isTTYGlobal && !TUIMode {
		return Strip(text)
	}

	if p == termenv.Ascii {
		return Strip(text)
	}

	// 1. Expand all semantic tags (Pass 1)
	text = ExpandTagsWithPrefix(text, prefix)

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
