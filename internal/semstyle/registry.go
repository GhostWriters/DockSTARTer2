package semstyle

import (
	"strings"
)

// Per-Styler map state lives on the Styler struct (see styler.go). The maps are created in
// New(); ensureMaps is a defensive guard for zero-value access.

// ensureMaps ensures color maps are built if they were missed by init
func (st *Styler) ensureMaps() {
	if len(st.ansiMap) == 0 {
		st.BuildColorMap()
	}
}

// BuildColorMap initializes the ANSI code and attribute name mappings.
// Default semantic tag registrations are handled separately by RegisterBaseTags.
func (st *Styler) BuildColorMap() {
	if st.ansiMap == nil {
		st.ansiMap = make(map[string]string)
	}
	if st.consoleMap == nil {
		st.consoleMap = make(map[string]string)
	}
	if st.themeMap == nil {
		st.themeMap = make(map[string]string)
	}
	if st.attributeMap == nil {
		st.attributeMap = make(map[string]string)
	}

	// Standard ANSI mappings
	st.ansiMap["-"] = CodeReset
	st.ansiMap["reset"] = CodeReset
	st.ansiMap["B"] = CodeBold
	st.ansiMap["b"] = CodeBoldOff
	st.ansiMap["D"] = CodeDim
	st.ansiMap["d"] = CodeDimOff
	st.ansiMap["U"] = CodeUnderline
	st.ansiMap["u"] = CodeUnderlineOff
	st.ansiMap["L"] = CodeBlink
	st.ansiMap["l"] = CodeBlinkOff
	st.ansiMap["R"] = CodeReverse
	st.ansiMap["r"] = CodeReverseOff
	st.ansiMap["I"] = CodeItalic
	st.ansiMap["i"] = CodeItalicOff
	st.ansiMap["S"] = CodeStrikethrough
	st.ansiMap["s"] = CodeStrikethroughOff

	// Attribute mappings — reset only; per-attribute on/off is handled by ansiMap in the flags field
	st.attributeMap["reset"] = CodeReset
	st.attributeMap["-"] = CodeReset

	// Colors...
	st.ansiMap["black"] = CodeBlack
	st.ansiMap["red"] = CodeRed
	st.ansiMap["green"] = CodeGreen
	st.ansiMap["yellow"] = CodeYellow
	st.ansiMap["blue"] = CodeBlue
	st.ansiMap["magenta"] = CodeMagenta
	st.ansiMap["cyan"] = CodeCyan
	st.ansiMap["white"] = CodeWhite
	st.ansiMap["bright-black"] = CodeBrightBlack
	st.ansiMap["bright-red"] = CodeBrightRed
	st.ansiMap["bright-green"] = CodeBrightGreen
	st.ansiMap["bright-yellow"] = CodeBrightYellow
	st.ansiMap["bright-blue"] = CodeBrightBlue
	st.ansiMap["bright-magenta"] = CodeBrightMagenta
	st.ansiMap["bright-cyan"] = CodeBrightCyan
	st.ansiMap["bright-white"] = CodeBrightWhite

	st.ansiMap["blackbg"] = CodeBlackBg
	st.ansiMap["redbg"] = CodeRedBg
	st.ansiMap["greenbg"] = CodeGreenBg
	st.ansiMap["yellowbg"] = CodeYellowBg
	st.ansiMap["bluebg"] = CodeBlueBg
	st.ansiMap["magentabg"] = CodeMagentaBg
	st.ansiMap["cyanbg"] = CodeCyanBg
	st.ansiMap["whitebg"] = CodeWhiteBg
	st.ansiMap["bright-blackbg"] = CodeBrightBlackBg
	st.ansiMap["bright-redbg"] = CodeBrightRedBg
	st.ansiMap["bright-greenbg"] = CodeBrightGreenBg
	st.ansiMap["bright-yellowbg"] = CodeBrightYellowBg
	st.ansiMap["bright-bluebg"] = CodeBrightBlueBg
	st.ansiMap["bright-magentabg"] = CodeBrightMagentaBg
	st.ansiMap["bright-cyanbg"] = CodeBrightCyanBg
	st.ansiMap["bright-whitebg"] = CodeBrightWhiteBg

}

// RegisterConsoleTag registers a semantic tag with its standardized tag value in the console map.
func (st *Styler) RegisterConsoleTag(name, taggedValue string) {
	st.RegisterConsoleTagRaw(name, st.StripDelimiters(taggedValue))
}

// RegisterConsoleTagRaw registers a semantic tag with a raw style code in the console map.
func (st *Styler) RegisterConsoleTagRaw(name, rawValue string) {
	st.ensureMaps()
	st.mu.Lock()
	st.consoleMap[strings.ToLower(name)] = rawValue
	st.mu.Unlock()
}

// RegisterThemeTag registers a semantic tag with its standardized tag value in the theme map.
func (st *Styler) RegisterThemeTag(name, taggedValue string) {
	st.RegisterThemeTagRaw(name, st.StripDelimiters(taggedValue))
}

// RegisterThemeTagRaw registers a semantic tag with a raw style code in the theme map.
func (st *Styler) RegisterThemeTagRaw(name, rawValue string) {
	st.ensureMaps()
	st.mu.Lock()
	st.themeMap[strings.ToLower(name)] = rawValue
	st.mu.Unlock()
}

// GetRawTagCode returns the raw style code (fg:bg:flags) for the given tag name from the theme map.
// Returns "" if the tag is not registered.
func (st *Styler) GetRawTagCode(name string) string {
	st.ensureMaps()
	st.mu.RLock()
	raw := st.themeMap[strings.ToLower(name)]
	if raw == "" {
		raw = st.consoleMap[strings.ToLower(name)]
	}
	st.mu.RUnlock()
	return raw
}

// RegisterSemanticTag registers a tag into BOTH the console (base) and theme maps — a
// convenience for defining a style that should resolve identically whether or not a theme
// is active. Prefer RegisterConsoleTag / RegisterThemeTag when you want to target one map.
func (st *Styler) RegisterSemanticTag(name, taggedValue string) {
	st.RegisterConsoleTag(name, taggedValue)
	st.RegisterThemeTag(name, taggedValue)
}

// RegisterSemanticTagRaw is the raw-value form of RegisterSemanticTag (registers to both maps).
func (st *Styler) RegisterSemanticTagRaw(name, rawValue string) {
	st.RegisterConsoleTagRaw(name, rawValue)
	st.RegisterThemeTagRaw(name, rawValue)
}

// GetColorDefinition returns the formatted tag value (with brackets) for a semantic tag.
// It searches the theme map first, then console map.
func (st *Styler) GetColorDefinition(name string) string {
	st.ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	content := strings.ToLower(name)

	st.mu.RLock()
	raw, ok := st.themeMap[content]
	if !ok {
		raw = st.consoleMap[content]
	}
	st.mu.RUnlock()

	if raw == "" {
		return ""
	}
	return st.WrapDirect(raw)
}

// UnregisterColor removes a semantic tag from both maps
func (st *Styler) UnregisterColor(name string) {
	st.ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	content := strings.ToLower(name)

	st.mu.Lock()
	delete(st.consoleMap, content)
	delete(st.themeMap, content)
	st.mu.Unlock()
}

// UnregisterPrefix removes all semantic tags that start with the given prefix from both maps
func (st *Styler) UnregisterPrefix(prefix string) {
	st.ensureMaps()
	searchPrefix := strings.ToLower(strings.TrimSuffix(prefix, "_") + "_")
	st.mu.Lock()
	for key := range st.consoleMap {
		if strings.HasPrefix(key, searchPrefix) {
			delete(st.consoleMap, key)
		}
	}
	for key := range st.themeMap {
		if strings.HasPrefix(key, searchPrefix) {
			delete(st.themeMap, key)
		}
	}
	st.mu.Unlock()
}

// ClearThemeMap removes all entries from the theme map.
func (st *Styler) ClearThemeMap() {
	st.mu.Lock()
	st.themeMap = make(map[string]string)
	st.mu.Unlock()
}

// ResetCustomColors clears all semantic tags and rebuilds from Colors struct
func (st *Styler) ResetCustomColors() {
	st.BuildColorMap()
}

// StripDelimiters removes any of this Styler's known delimiters from a style string to get
// the raw content, falling back to the library-standard delimiters if customised.
func (st *Styler) StripDelimiters(text string) string {
	if strings.HasPrefix(text, st.semPre) && strings.HasSuffix(text, st.semSuf) {
		return text[len(st.semPre) : len(text)-len(st.semSuf)]
	}
	if strings.HasPrefix(text, st.dirPre) && strings.HasSuffix(text, st.dirSuf) {
		return text[len(st.dirPre) : len(text)-len(st.dirSuf)]
	}
	// Fallback to standard delimiters if this Styler uses custom ones
	if st.semPre != "{{|" {
		if strings.HasPrefix(text, "{{|") && strings.HasSuffix(text, "|}}") {
			return text[3 : len(text)-3]
		}
	}
	if st.dirPre != "{{[" {
		if strings.HasPrefix(text, "{{[") && strings.HasSuffix(text, "]}}") {
			return text[3 : len(text)-3]
		}
	}
	return text
}

// --- package-level delegators to Default ---
func StripDelimiters(text string) string { return Default.StripDelimiters(text) }

func BuildColorMap() {
	Default.BuildColorMap()
}

func RegisterConsoleTag(name, taggedValue string) {
	Default.RegisterConsoleTag(name, taggedValue)
}

func RegisterConsoleTagRaw(name, rawValue string) {
	Default.RegisterConsoleTagRaw(name, rawValue)
}

func RegisterThemeTag(name, taggedValue string) {
	Default.RegisterThemeTag(name, taggedValue)
}

func RegisterThemeTagRaw(name, rawValue string) {
	Default.RegisterThemeTagRaw(name, rawValue)
}

func GetRawTagCode(name string) string {
	return Default.GetRawTagCode(name)
}

func RegisterSemanticTag(name, taggedValue string) {
	Default.RegisterSemanticTag(name, taggedValue)
}

func RegisterSemanticTagRaw(name, rawValue string) {
	Default.RegisterSemanticTagRaw(name, rawValue)
}

func GetColorDefinition(name string) string {
	return Default.GetColorDefinition(name)
}

func UnregisterColor(name string) {
	Default.UnregisterColor(name)
}

func UnregisterPrefix(prefix string) {
	Default.UnregisterPrefix(prefix)
}

func ClearThemeMap() {
	Default.ClearThemeMap()
}

func ResetCustomColors() {
	Default.ResetCustomColors()
}
