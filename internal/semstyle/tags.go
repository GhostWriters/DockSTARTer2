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

	// Semantic tag: name + optional modifier overrides (fg:bg:flags) + optional label.
	// Parsed as a single content group; label extraction done in code by counting colon fields.
	// {{|Name|}} {{|Name:fg:bg:flags|}} {{|Name:fg:bg:flags:Display Label|}} {{|Name::::Label|}}
	semClosingChar := regexp.QuoteMeta(string(st.semSuf[0]))
	st.semanticRegex = regexp.MustCompile(semEscPre +
		`(?P<content>[^` + semClosingChar + `]+)` +
		semEscSuf)
	// content = up to three colon-separated fields (fg:bg:flags), no spaces.
	// An optional fourth field ":label" follows; label may contain spaces but not the closing delimiter char.
	field := `[A-Za-z0-9_\-#;~]*`
	closingChar := regexp.QuoteMeta(string(st.dirSuf[0]))
	st.directRegex = regexp.MustCompile(dirEscPre +
		`(?P<content>` + field + `(?::` + field + `(?::` + field + `)?)?)` +
		`(?::(?P<label>[^` + closingChar + `]*))?` +
		dirEscSuf)
}

// GetDelimitedRegex returns a regex matching both semantic and direct tags for this Styler.
func (st *Styler) GetDelimitedRegex() *regexp.Regexp {
	semEscPre := regexp.QuoteMeta(st.semPre)
	semEscSuf := regexp.QuoteMeta(st.semSuf)
	dirEscPre := regexp.QuoteMeta(st.dirPre)
	dirEscSuf := regexp.QuoteMeta(st.dirSuf)

	pattern := fmt.Sprintf(`(?:%s(?P<semantic>[A-Za-z0-9_]+(?::[A-Za-z0-9_:\-#;~]*)?)%s|%s(?P<direct>[A-Za-z0-9_:\-#;~]+)%s)`,
		semEscPre, semEscSuf, dirEscPre, dirEscSuf)
	return regexp.MustCompile(pattern)
}

// GetDirectRegex returns the direct-tag regex for this Styler.
func (st *Styler) GetDirectRegex() *regexp.Regexp {
	return st.directRegex
}

// StripTags removes all semantic and direct tags from s, leaving plain text and any
// existing ANSI sequences untouched.
func (st *Styler) StripTags(s string) string {
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
func WrapSemantic(name string) string   { return Default.WrapSemantic(name) }
func WrapDirect(code string) string     { return Default.WrapDirect(code) }

// ToTags resolves semantic tags to direct tags using the console map (no prefix), or the
// theme map when a prefix is provided. Stops short of ANSI conversion — useful when the
// output will be passed to a compositor or TUI renderer that understands direct tags.
//
// stripSemanticLabel removes the label (5th colon-field) from a semantic tag content string,
// returning only the name + up to 3 modifier fields (fg:bg:flags). Label presence requires
// exactly 4 colons: name:fg:bg:flags:label — fewer colons means no label.
func stripSemanticLabel(content string) string {
	// Count colons; label only exists at the 5th field (index 4).
	idx := 0
	colons := 0
	for i, c := range content {
		if c == ':' {
			colons++
			if colons == 4 {
				idx = i
				break
			}
		}
	}
	if colons < 4 {
		return content
	}
	return content[:idx]
}

// semLabelFrom extracts the label from a semantic tag content string (the 5th colon-field).
// Returns ("", false) when no label is present.
func semLabelFrom(content string) (string, bool) {
	colons := 0
	for i, c := range content {
		if c == ':' {
			colons++
			if colons == 4 {
				return content[i+1:], true
			}
		}
	}
	return "", false
}

// ToTags(s)         — console map, no prefix
// ToTags(s, "")     — theme map, no prefix (theme-first with console fallback)
// ToTags(s, "pfx")  — theme map, prefix-qualified lookup
func (st *Styler) ToTags(s string, prefix ...string) string {
	if len(prefix) == 0 {
		return st.ExpandTagsWithMap(s, st.consoleMap, true, "")
	}
	return st.ExpandTagsWithMap(s, st.themeMap, true, prefix[0])
}

// ExpandTagsWithMap is the base tag-expansion routine. If styleMap is nil it uses the
// theme map with console fallback. Expansion repeats up to 8 passes so tag values that
// themselves reference other tags resolve correctly.
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

			// Strip any label field (5th colon-field) before resolving — labels are for
			// hyperlinks only and must not be passed to the style resolver.
			semanticName, modifiers, _ := strings.Cut(stripSemanticLabel(fullContent), ":")
			content := strings.ToLower(semanticName)

			var rawCode string
			var ok bool

			if styleMap != nil {
				if prefix != "" {
					rawCode, ok = styleMap[prefix+content]
				}
				if !ok {
					rawCode, ok = styleMap[content]
				}
			} else {
				if prefix != "" {
					rawCode, ok = st.themeMap[prefix+content]
				}
				if !ok {
					rawCode, ok = st.themeMap[content]
				}
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

			if modifiers != "" {
				return st.WrapDirect(modifiers)
			}
			if strip {
				return ""
			}
			return match
		})
	}

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

// processHyperlinks wraps the content of any registered hyperlink tag in a terminal
// hyperlink, using the content's plain text as the destination.
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
		urlDestination := st.ToPlain(subMatch[2])
		linkStyle := lipgloss.NewStyle().Hyperlink(urlDestination)
		return linkStyle.Render(match)
	})
}

// RenderPolicy, when set, is consulted by ToANSI: if it returns false the text is
// stripped instead of rendered. The host app sets this to encode its TTY/TUI policy;
// when nil the engine always renders.
var RenderPolicy func() bool

// ToANSI converts semantic and direct tags to ANSI escape sequences.
//
// ToANSI(s)         — resolves using the console map
// ToANSI(s, "")     — resolves using the theme map (theme-first, console fallback), no prefix
// ToANSI(s, "pfx")  — resolves using the theme map with a prefix qualifier
func (st *Styler) ToANSI(s string, prefix ...string) string {
	if RenderPolicy != nil && !RenderPolicy() {
		return st.ToPlain(s)
	}
	s = st.processHyperlinks(s)
	s = st.processInlineHyperlinks(s)
	s = st.ToTags(s, prefix...)
	return st.processDirectTags(s)
}

// ToPlain removes all semantic tags, direct tags, and ANSI escape sequences, returning
// plain undecorated text.
func (st *Styler) ToPlain(s string) string {
	s = st.semanticRegex.ReplaceAllString(s, "")
	s = st.directRegex.ReplaceAllString(s, "")
	return StripANSI(s)
}

// processInlineHyperlinks handles tags with an explicit display-text (label) field:
//
//	Direct:   {{[fg:bg:flags:Display Label]}}https://url{{[-]}}
//	Semantic: {{|StyleName:fg:bg:flags:Display Label|}}https://url{{[-]}}
//
// The label is the visible link text; content up to the next reset tag is the URL.
// Empty label uses the URL as both link and display text.
// Tags without a label field are left untouched for later processing.
func (st *Styler) processInlineHyperlinks(text string) string {
	dirContentIdx := st.directRegex.SubexpIndex("content")
	dirLabelIdx := st.directRegex.SubexpIndex("label")
	semContentIdx := st.semanticRegex.SubexpIndex("content")
	if dirContentIdx < 0 || dirLabelIdx < 0 || semContentIdx < 0 {
		return text
	}

	resetPat := regexp.MustCompile(
		regexp.QuoteMeta(st.dirPre) + `(?:-|` + regexp.QuoteMeta(CodeReset) + `)` + regexp.QuoteMeta(st.dirSuf) + `|` +
			regexp.QuoteMeta(st.semPre) + `(?:-|reset)` + regexp.QuoteMeta(st.semSuf),
	)

	var out strings.Builder
	consumed := 0

	for {
		slice := text[consumed:]

		dirLoc := st.directRegex.FindStringSubmatchIndex(slice)
		semLoc := st.semanticRegex.FindStringSubmatchIndex(slice)

		if dirLoc == nil && semLoc == nil {
			break
		}

		// Prefer whichever tag starts earliest.
		useDir := dirLoc != nil && (semLoc == nil || dirLoc[0] <= semLoc[0])

		var tagStart, tagEnd int
		var label, styleCode string
		var isSemantic bool

		if useDir {
			tagStart, tagEnd = dirLoc[0], dirLoc[1]
			// Direct tag: label is a named regex group.
			if dirLoc[dirLabelIdx*2] < 0 {
				out.WriteString(slice[:tagEnd])
				consumed += tagEnd
				continue
			}
			label = slice[dirLoc[dirLabelIdx*2]:dirLoc[dirLabelIdx*2+1]]
			styleCode = slice[dirLoc[dirContentIdx*2]:dirLoc[dirContentIdx*2+1]]
		} else {
			tagStart, tagEnd = semLoc[0], semLoc[1]
			// Semantic tag: label is the 5th colon-field, extracted in code.
			fullContent := slice[semLoc[semContentIdx*2]:semLoc[semContentIdx*2+1]]
			semLabel, hasLabel := semLabelFrom(fullContent)
			if !hasLabel {
				out.WriteString(slice[:tagEnd])
				consumed += tagEnd
				continue
			}
			label = semLabel
			styleCode = stripSemanticLabel(fullContent)
			isSemantic = true
		}

		resetLoc := resetPat.FindStringIndex(slice[tagEnd:])
		if resetLoc == nil {
			out.WriteString(slice[:tagEnd])
			consumed += tagEnd
			continue
		}

		url := slice[tagEnd : tagEnd+resetLoc[0]]
		if label == "" {
			label = st.ToPlain(url)
		}

		var styleANSI string
		if isSemantic {
			styleANSI = st.ToANSI(st.semPre + styleCode + st.semSuf)
		} else {
			styleANSI = st.parseStyleCodeToANSI(styleCode)
		}

		hyperlink := fmt.Sprintf("\x1b]8;;%s\x1b\\%s%s%s\x1b]8;;\x1b\\", url, styleANSI, label, CodeReset)

		out.WriteString(slice[:tagStart])
		out.WriteString(hyperlink)
		consumed += tagEnd + resetLoc[1]
	}

	out.WriteString(text[consumed:])
	return out.String()
}

// processDirectTags converts remaining direct tags {{[code]}} to ANSI sequences.
// Inline hyperlink tags (those with a label field) are handled before this by
// processInlineHyperlinks and are not present in the text when this runs.
func (st *Styler) processDirectTags(text string) string {
	re := st.directRegex
	contentIdx := re.SubexpIndex("content")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		subMatch := re.FindStringSubmatch(match)
		if len(subMatch) <= contentIdx {
			return ""
		}
		return st.parseStyleCodeToANSI(subMatch[contentIdx])
	})
}

// Sprintf formats according to a format specifier and returns the result with ANSI codes
// applied via the console map.
func (st *Styler) Sprintf(format string, a ...any) string {
	return st.ToANSI(fmt.Sprintf(format, a...))
}

// --- package-level delegators to Default ---

func ToANSI(s string, prefix ...string) string { return Default.ToANSI(s, prefix...) }
func ToTags(s string, prefix ...string) string { return Default.ToTags(s, prefix...) }
func ToPlain(s string) string                  { return Default.ToPlain(s) }
func StripTags(s string) string                { return Default.StripTags(s) }
func ExpandTagsWithMap(text string, styleMap map[string]string, stripUnresolvable bool, prefix string) string {
	return Default.ExpandTagsWithMap(text, styleMap, stripUnresolvable, prefix)
}
func Sprintf(format string, a ...any) string { return Default.Sprintf(format, a...) }
