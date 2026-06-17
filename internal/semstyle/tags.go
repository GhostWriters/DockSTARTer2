package semstyle

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// Standard tag delimiters (library default). These are the values New() copies into each
// Styler and the "standard" delimiters external packages reference. Override a single
// Styler's delimiters with (*Styler).SetDelimiters, or change the process-wide standard
// (and the Default styler) with the package-level SetDelimiters.
var (
	SemanticPrefix = "{{|"
	SemanticSuffix = "|}}"
	DirectPrefix   = "{{["
	DirectSuffix   = "]}}"
)

// rebuildRegexes recompiles this Styler's tag regexes from its current delimiters.
func (st *Styler) rebuildRegexes() {
	semEscPre := regexp.QuoteMeta(st.semPre)
	semEscSuf := regexp.QuoteMeta(st.semSuf)
	dirEscPre := regexp.QuoteMeta(st.dirPre)
	dirEscSuf := regexp.QuoteMeta(st.dirSuf)

	// Allow either a named tag (optionally with modifiers) or bare modifiers with no name (e.g. {{|:red:black:B|}})
	st.semanticRegex = regexp.MustCompile(semEscPre + `(?P<content>[A-Za-z0-9_]+(?::[A-Za-z0-9_:\-#;~]*)?|:[A-Za-z0-9_:\-#;~]+)` + semEscSuf)
	st.directRegex = regexp.MustCompile(dirEscPre + `(?P<content>[A-Za-z0-9_:\-#;~]+)` + dirEscSuf)
}

// GetDelimitedRegex returns a regex matching both semantic and direct tags for this Styler.
func (st *Styler) GetDelimitedRegex() *regexp.Regexp {
	semEscPre := regexp.QuoteMeta(st.semPre)
	semEscSuf := regexp.QuoteMeta(st.semSuf)
	dirEscPre := regexp.QuoteMeta(st.dirPre)
	dirEscSuf := regexp.QuoteMeta(st.dirSuf)

	// Group 1: Semantic, Group 2: Direct
	pattern := fmt.Sprintf(`(?:%s(?P<semantic>[A-Za-z0-9_]+(?::[A-Za-z0-9_:\-#;~]*)?)%s|%s(?P<direct>[A-Za-z0-9_:\-#;~]+)%s)`,
		semEscPre, semEscSuf, dirEscPre, dirEscSuf)
	return regexp.MustCompile(pattern)
}

// GetDirectRegex returns the direct-tag regex for this Styler.
func (st *Styler) GetDirectRegex() *regexp.Regexp {
	return st.directRegex
}

// StripSemanticTags removes all semantic and direct tags from s, returning plain text.
func (st *Styler) StripSemanticTags(s string) string {
	s = st.semanticRegex.ReplaceAllString(s, "")
	s = st.directRegex.ReplaceAllString(s, "")
	return s
}

// WrapSemantic wraps a tag name in this Styler's semantic delimiters.
func (st *Styler) WrapSemantic(name string) string {
	return st.semPre + name + st.semSuf
}

// WrapDirect wraps a style code in this Styler's direct delimiters.
func (st *Styler) WrapDirect(code string) string {
	return st.dirPre + code + st.dirSuf
}

// SetDelimiters changes the process-wide standard delimiters and applies them to the
// Default styler. Per-instance overrides use (*Styler).SetDelimiters.
func SetDelimiters(semPre, semSuf, dirPre, dirSuf string) {
	SemanticPrefix, SemanticSuffix, DirectPrefix, DirectSuffix = semPre, semSuf, dirPre, dirSuf
	Default.SetDelimiters(semPre, semSuf, dirPre, dirSuf)
}

// Package-level delimiter helpers delegate to Default.
func GetDelimitedRegex() *regexp.Regexp { return Default.GetDelimitedRegex() }
func GetDirectRegex() *regexp.Regexp    { return Default.GetDirectRegex() }
func StripSemanticTags(s string) string { return Default.StripSemanticTags(s) }
func WrapSemantic(name string) string   { return Default.WrapSemantic(name) }
func WrapDirect(code string) string     { return Default.WrapDirect(code) }

// ExpandConsoleTags converts semantic tags to standardized direct format using only the console map.
func (st *Styler) ExpandConsoleTags(text string) string {
	return st.ExpandTagsWithMap(text, st.consoleMap, true, "")
}

// ExpandThemeTags converts semantic tags to standardized direct format using only the theme map.
func (st *Styler) ExpandThemeTags(text string, prefix string) string {
	return st.ExpandTagsWithMap(text, st.themeMap, true, prefix)
}

// ExpandTagsWithMap is the base routine for expanding semantic tags.
// If styleMap is nil, it uses themeMap with fallback to consoleMap (Legacy behavior).
// Expansion is repeated until stable (up to 8 passes) so that tag values which
// themselves reference other semantic tags resolve correctly.
func (st *Styler) ExpandTagsWithMap(text string, styleMap map[string]string, stripUnresolvable bool, prefix string) string {
	st.ensureMaps()
	prefix = strings.ToLower(prefix)

	st.mu.RLock()
	defer st.mu.RUnlock()

	expandOnce := func(s string, strip bool) string {
		return st.semanticRegex.ReplaceAllStringFunc(s, func(match string) string {
			groupIndex := st.semanticRegex.SubexpIndex("content")
			subMatch := st.semanticRegex.FindStringSubmatch(match)
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
					rawCode, ok = st.themeMap[prefixed]
				}
				// 2. Try raw in Theme
				if !ok {
					rawCode, ok = st.themeMap[content]
				}
				// 3. Try fallback to Console (No prefix for Console colors)
				if !ok {
					rawCode, ok = st.consoleMap[content]
				}
			}

			if ok {
				result := st.WrapDirect(rawCode)
				if modifiers != "" {
					result += st.WrapDirect(modifiers)
				}
				return result
			}

			// Name didn't resolve (or was empty) — if modifiers are present, emit them as a
			// direct tag so {{|:fg:bg:flags|}} and {{|Unknown:fg:bg:flags|}} still apply styling.
			if modifiers != "" {
				return st.WrapDirect(modifiers)
			}
			if strip {
				return ""
			}
			return match
		})
	}

	// First passes without stripping so unresolved tags can resolve in later passes.
	// Final pass applies stripUnresolvable.
	const maxPasses = 8
	for range maxPasses - 1 {
		expanded := expandOnce(text, false)
		if expanded == text {
			break
		}
		text = expanded
	}
	return expandOnce(text, stripUnresolvable)
}

// processHyperlinks wraps the content of any registered hyperlink tag (see
// RegisterHyperlinkTag) — from the tag up to the next reset — in a terminal hyperlink, using
// the content's plain text as the destination. No-op when no hyperlink tags are registered.
func (st *Styler) processHyperlinks(text string) string {
	st.mu.RLock()
	n := len(st.hyperlinkTags)
	names := make([]string, 0, n)
	for name := range st.hyperlinkTags {
		names = append(names, regexp.QuoteMeta(name))
	}
	st.mu.RUnlock()
	if n == 0 {
		return text
	}

	semEscPre := regexp.QuoteMeta(st.semPre)
	semEscSuf := regexp.QuoteMeta(st.semSuf)
	dirEscPre := regexp.QuoteMeta(st.dirPre)
	dirEscSuf := regexp.QuoteMeta(st.dirSuf)

	// Start marker: any registered hyperlink tag (case-insensitive).
	// End marker: a reset, either direct ({{[-]}}) or semantic ({{|reset|}} / {{|-|}}).
	tagAlt := strings.Join(names, "|")
	pattern := fmt.Sprintf(`((?i)%s(?:%s)%s)(.*?)(%s-%s|%sreset%s|%s-%s)`,
		semEscPre, tagAlt, semEscSuf,
		dirEscPre, dirEscSuf,
		semEscPre, semEscSuf,
		semEscPre, semEscSuf)

	re := regexp.MustCompile(pattern)

	return re.ReplaceAllStringFunc(text, func(match string) string {
		subMatch := re.FindStringSubmatch(match)
		if len(subMatch) < 4 {
			return match
		}
		// Group 2 is the content between the start tag and the terminator. The link
		// destination is that content with all tags stripped.
		urlDestination := st.Strip(subMatch[2])
		linkStyle := lipgloss.NewStyle().Hyperlink(urlDestination)
		return linkStyle.Render(match)
	})
}

// RenderPolicy, when set, is consulted by ToConsoleANSI: if it returns false the text
// is stripped (no ANSI) instead of rendered. The host app sets this to encode its
// TTY/TUI policy; when nil the engine always renders. Keeps the engine free of TTY state.
var RenderPolicy func() bool

// ToConsoleANSI converts semantic and direct tags to ANSI escape sequences using only console colors.
func (st *Styler) ToConsoleANSI(text string) string {
	if RenderPolicy != nil && !RenderPolicy() {
		return st.Strip(text)
	}

	// 0. Process Hyperlinks
	text = st.processHyperlinks(text)

	// 1. Expand only console tags
	text = st.ExpandConsoleTags(text)

	// 2. Process all direct tags -> ANSI
	return st.processDirectTags(text)
}

// ToThemeANSI converts semantic and direct tags to ANSI escape sequences using only theme colors.
func (st *Styler) ToThemeANSI(text string) string {
	return st.ToThemeANSIWithPrefix(text, "")
}

// ToThemeANSIWithPrefix converts semantic and direct tags to ANSI escape sequences using only theme colors with a prefix.
func (st *Styler) ToThemeANSIWithPrefix(text string, prefix string) string {
	// 0. Process Hyperlinks
	text = st.processHyperlinks(text)

	// 1. Expand theme tags
	text = st.ExpandThemeTags(text, prefix)

	// 2. Process all direct tags -> ANSI
	return st.processDirectTags(text)
}

// processDirectTags is a helper to convert direct tags {{[code]}} to ANSI sequences.
func (st *Styler) processDirectTags(text string) string {
	re := GetDirectRegex()
	return re.ReplaceAllStringFunc(text, func(match string) string {
		groupIndex := re.SubexpIndex("content")
		subMatch := re.FindStringSubmatch(match)
		if len(subMatch) <= groupIndex {
			return ""
		}
		return st.parseStyleCodeToANSI(subMatch[groupIndex])
	})
}

// Strip removes all semantic and direct tags from text, as well as ANSI escape sequences
func (st *Styler) Strip(text string) string {
	text = st.semanticRegex.ReplaceAllString(text, "")
	text = st.directRegex.ReplaceAllString(text, "")
	return StripANSI(text)
}

// ForTUI prepares text for display with standardized theme tags
func (st *Styler) ForTUI(text string) string {
	return st.ExpandThemeTags(text, "")
}

// Sprintf formats according to a format specifier and returns the string with Console ANSI codes
func (st *Styler) Sprintf(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	return st.ToConsoleANSI(msg)
}

// --- package-level delegators to Default ---
func ExpandConsoleTags(text string) string {
	return Default.ExpandConsoleTags(text)
}

func ExpandThemeTags(text string, prefix string) string {
	return Default.ExpandThemeTags(text, prefix)
}

func ExpandTagsWithMap(text string, styleMap map[string]string, stripUnresolvable bool, prefix string) string {
	return Default.ExpandTagsWithMap(text, styleMap, stripUnresolvable, prefix)
}

func ToConsoleANSI(text string) string {
	return Default.ToConsoleANSI(text)
}

func ToThemeANSI(text string) string {
	return Default.ToThemeANSI(text)
}

func ToThemeANSIWithPrefix(text string, prefix string) string {
	return Default.ToThemeANSIWithPrefix(text, prefix)
}

func processDirectTags(text string) string {
	return Default.processDirectTags(text)
}

func Strip(text string) string {
	return Default.Strip(text)
}

func ForTUI(text string) string {
	return Default.ForTUI(text)
}

func Sprintf(format string, a ...any) string {
	return Default.Sprintf(format, a...)
}
