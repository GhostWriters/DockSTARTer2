package console

import (
	"fmt"
	"regexp"
	"strings"
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

// ExpandConsoleTags converts semantic tags to standardized direct format using only the console map.
func ExpandConsoleTags(text string) string {
	return ExpandTagsWithMap(text, consoleMap, true, "")
}

// ExpandThemeTags converts semantic tags to standardized direct format using only the theme map.
func ExpandThemeTags(text string, prefix string) string {
	return ExpandTagsWithMap(text, themeMap, true, prefix)
}


// ExpandTagsWithMap is the base routine for expanding semantic tags.
// If styleMap is nil, it uses themeMap with fallback to consoleMap (Legacy behavior).
func ExpandTagsWithMap(text string, styleMap map[string]string, stripUnresolvable bool, prefix string) string {
	ensureMaps()
	prefix = strings.ToLower(prefix)

	// Process semantic tags
	semanticMu.RLock()
	defer semanticMu.RUnlock()

	return semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		groupIndex := semanticRegex.SubexpIndex("content")
		subMatch := semanticRegex.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		fullContent := subMatch[groupIndex]

		// Split semantic name from optional inline modifiers (e.g. "Title:::R")
		semanticName := fullContent
		modifiers := ""
		if idx := strings.IndexByte(fullContent, ':'); idx >= 0 {
			semanticName = fullContent[:idx]
			modifiers = fullContent[idx+1:]
		}
		content := strings.ToLower(semanticName)

		var rawCode string
		var ok bool

		if styleMap != nil {
			// Specific map requested (No fallback)
			// 1. Try with prefix if provided
			if prefix != "" {
				prefixed := prefix + content
				rawCode, ok = styleMap[prefixed]
			}
			// 2. Try raw name if prefix failed or wasn't provided
			if !ok {
				rawCode, ok = styleMap[content]
			}
		} else {
			// Legacy behavior: Theme preferred, then Console
			// 1. Try with prefix in Theme
			if prefix != "" {
				prefixed := prefix + content
				rawCode, ok = themeMap[prefixed]
			}
			// 2. Try raw in Theme
			if !ok {
				rawCode, ok = themeMap[content]
			}
			// 3. Try fallback to Console (No prefix for Console colors)
			if !ok {
				rawCode, ok = consoleMap[content]
			}
		}

		if ok {
			result := WrapDirect(rawCode)
			if modifiers != "" {
				result += WrapDirect(modifiers)
			}
			return result
		}

		if stripUnresolvable {
			return ""
		}
		return match
	})
}

// ToConsoleANSI converts semantic and direct tags to ANSI escape sequences using only console colors.
func ToConsoleANSI(text string) string {
	if !isTTYGlobal && !TUIMode && !IsTUIEnabled() {
		return Strip(text)
	}

	// 1. Expand only console tags
	text = ExpandConsoleTags(text)

	// 2. Process all direct tags -> ANSI
	return processDirectTags(text)
}

// ToThemeANSI converts semantic and direct tags to ANSI escape sequences using only theme colors.
func ToThemeANSI(text string) string {
	return ToThemeANSIWithPrefix(text, "")
}

// ToThemeANSIWithPrefix converts semantic and direct tags to ANSI escape sequences using only theme colors with a prefix.
func ToThemeANSIWithPrefix(text string, prefix string) string {
	// Prefix logic is currently handled during registration in theme.go,
	// so we just expand theme tags.
	text = ExpandThemeTags(text, prefix)

	// 2. Process all direct tags -> ANSI
	return processDirectTags(text)
}

// processDirectTags is a helper to convert direct tags {{[code]}} to ANSI sequences.
func processDirectTags(text string) string {
	re := GetDirectRegex()
	return re.ReplaceAllStringFunc(text, func(match string) string {
		groupIndex := re.SubexpIndex("content")
		subMatch := re.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		return parseStyleCodeToANSI(subMatch[groupIndex])
	})
}


// Strip removes all semantic and direct tags from text, as well as ANSI escape sequences
func Strip(text string) string {
	text = semanticRegex.ReplaceAllString(text, "")
	text = directRegex.ReplaceAllString(text, "")
	return StripANSI(text)
}

// ForTUI prepares text for display with standardized theme tags
func ForTUI(text string) string {
	return ExpandThemeTags(text, "")
}

// Sprintf formats according to a format specifier and returns the string with Console ANSI codes
func Sprintf(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	return ToConsoleANSI(msg)
}

// Println prints a line with Console ANSI color codes parsed
func Println(a ...any) {
	msg := fmt.Sprint(a...)
	fmt.Println(ToConsoleANSI(msg))
}
