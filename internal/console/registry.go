package console

import (
	"strings"
	"sync"
)

var (
	// semanticMu guards all reads and writes of semanticMap, which is written
	// concurrently by theme-loading goroutines and read by the render goroutine.
	semanticMu sync.RWMutex

	// consoleMap stores hardcoded console semantic tag -> raw style code mappings
	consoleMap map[string]string

	// themeMap stores theme-loaded semantic tag -> raw style code mappings
	themeMap map[string]string

	// ansiMap stores color/modifier names -> ANSI code mappings
	ansiMap map[string]string

	// attributeMap stores non-color attribute names -> ANSI code mappings
	attributeMap map[string]string
)

func init() {
	// Initialize maps
	consoleMap = make(map[string]string)
	themeMap = make(map[string]string)
	ansiMap = make(map[string]string)
	attributeMap = make(map[string]string)
}

// ensureMaps ensures color maps are built if they were missed by init
func ensureMaps() {
	if len(ansiMap) == 0 {
		BuildColorMap()
	}
}

// BuildColorMap initializes the ANSI code and attribute name mappings.
// Default semantic tag registrations are handled separately by RegisterBaseTags.
func BuildColorMap() {
	if ansiMap == nil {
		ansiMap = make(map[string]string)
	}
	if consoleMap == nil {
		consoleMap = make(map[string]string)
	}
	if themeMap == nil {
		themeMap = make(map[string]string)
	}
	if attributeMap == nil {
		attributeMap = make(map[string]string)
	}

	// Standard ANSI mappings
	ansiMap["-"] = CodeReset
	ansiMap["reset"] = CodeReset
	ansiMap["B"] = CodeBold
	ansiMap["b"] = CodeBoldOff
	ansiMap["D"] = CodeDim
	ansiMap["d"] = CodeDimOff
	ansiMap["U"] = CodeUnderline
	ansiMap["u"] = CodeUnderlineOff
	ansiMap["L"] = CodeBlink
	ansiMap["l"] = CodeBlinkOff
	ansiMap["R"] = CodeReverse
	ansiMap["r"] = CodeReverseOff
	ansiMap["I"] = CodeItalic
	ansiMap["i"] = CodeItalicOff
	ansiMap["S"] = CodeStrikethrough
	ansiMap["s"] = CodeStrikethroughOff

	// Attribute mappings (normalized names)
	attributeMap["reset"] = CodeReset
	attributeMap["-"] = CodeReset
	attributeMap["bold"] = CodeBold
	attributeMap["b"] = CodeBold
	attributeMap["dim"] = CodeDim
	attributeMap["d"] = CodeDim
	attributeMap["underline"] = CodeUnderline
	attributeMap["u"] = CodeUnderline
	attributeMap["blink"] = CodeBlink
	attributeMap["l"] = CodeBlink
	attributeMap["reverse"] = CodeReverse
	attributeMap["r"] = CodeReverse
	attributeMap["italic"] = CodeItalic
	attributeMap["i"] = CodeItalic
	attributeMap["strikethrough"] = CodeStrikethrough
	attributeMap["s"] = CodeStrikethrough
	attributeMap["-bold"] = CodeBoldOff
	attributeMap["-b"] = CodeBoldOff
	attributeMap["-dim"] = CodeDimOff
	attributeMap["-d"] = CodeDimOff
	attributeMap["-underline"] = CodeUnderlineOff
	attributeMap["-u"] = CodeUnderlineOff
	attributeMap["-blink"] = CodeBlinkOff
	attributeMap["-l"] = CodeBlinkOff
	attributeMap["-reverse"] = CodeReverseOff
	attributeMap["-r"] = CodeReverseOff
	attributeMap["-italic"] = CodeItalicOff
	attributeMap["-i"] = CodeItalicOff
	attributeMap["-strikethrough"] = CodeStrikethroughOff
	attributeMap["-s"] = CodeStrikethroughOff

	// Colors...
	ansiMap["black"] = CodeBlack
	ansiMap["red"] = CodeRed
	ansiMap["green"] = CodeGreen
	ansiMap["yellow"] = CodeYellow
	ansiMap["blue"] = CodeBlue
	ansiMap["magenta"] = CodeMagenta
	ansiMap["cyan"] = CodeCyan
	ansiMap["white"] = CodeWhite
	ansiMap["bright-black"] = CodeBrightBlack
	ansiMap["bright-red"] = CodeBrightRed
	ansiMap["bright-green"] = CodeBrightGreen
	ansiMap["bright-yellow"] = CodeBrightYellow
	ansiMap["bright-blue"] = CodeBrightBlue
	ansiMap["bright-magenta"] = CodeBrightMagenta
	ansiMap["bright-cyan"] = CodeBrightCyan
	ansiMap["bright-white"] = CodeBrightWhite

	ansiMap["blackbg"] = CodeBlackBg
	ansiMap["redbg"] = CodeRedBg
	ansiMap["greenbg"] = CodeGreenBg
	ansiMap["yellowbg"] = CodeYellowBg
	ansiMap["bluebg"] = CodeBlueBg
	ansiMap["magentabg"] = CodeMagentaBg
	ansiMap["cyanbg"] = CodeCyanBg
	ansiMap["whitebg"] = CodeWhiteBg
	ansiMap["bright-blackbg"] = CodeBrightBlackBg
	ansiMap["bright-redbg"] = CodeBrightRedBg
	ansiMap["bright-greenbg"] = CodeBrightGreenBg
	ansiMap["bright-yellowbg"] = CodeBrightYellowBg
	ansiMap["bright-bluebg"] = CodeBrightBlueBg
	ansiMap["bright-magentabg"] = CodeBrightMagentaBg
	ansiMap["bright-cyanbg"] = CodeBrightCyanBg
	ansiMap["bright-whitebg"] = CodeBrightWhiteBg

}

// RegisterConsoleTag registers a semantic tag with its standardized tag value in the console map.
func RegisterConsoleTag(name, taggedValue string) {
	RegisterConsoleTagRaw(name, StripDelimiters(taggedValue))
}

// RegisterConsoleTagRaw registers a semantic tag with a raw style code in the console map.
func RegisterConsoleTagRaw(name, rawValue string) {
	ensureMaps()
	semanticMu.Lock()
	consoleMap[strings.ToLower(name)] = rawValue
	semanticMu.Unlock()
}

// RegisterThemeTag registers a semantic tag with its standardized tag value in the theme map.
func RegisterThemeTag(name, taggedValue string) {
	RegisterThemeTagRaw(name, StripDelimiters(taggedValue))
}

// RegisterThemeTagRaw registers a semantic tag with a raw style code in the theme map.
func RegisterThemeTagRaw(name, rawValue string) {
	ensureMaps()
	semanticMu.Lock()
	themeMap[strings.ToLower(name)] = rawValue
	semanticMu.Unlock()
}

// RegisterSemanticTag is a legacy wrapper that registers to BOTH maps for backward compatibility during transition.
// TODO: Remove after all calls are migrated.
func RegisterSemanticTag(name, taggedValue string) {
	RegisterConsoleTag(name, taggedValue)
	RegisterThemeTag(name, taggedValue)
}

// RegisterSemanticTagRaw is a legacy wrapper that registers to BOTH maps for backward compatibility during transition.
// TODO: Remove after all calls are migrated.
func RegisterSemanticTagRaw(name, rawValue string) {
	RegisterConsoleTagRaw(name, rawValue)
	RegisterThemeTagRaw(name, rawValue)
}

// GetColorDefinition returns the formatted tag value (with brackets) for a semantic tag.
// It searches the theme map first, then console map.
func GetColorDefinition(name string) string {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	content := strings.ToLower(name)

	semanticMu.RLock()
	raw, ok := themeMap[content]
	if !ok {
		raw = consoleMap[content]
	}
	semanticMu.RUnlock()

	if raw == "" {
		return ""
	}
	return WrapDirect(raw)
}

// UnregisterColor removes a semantic tag from both maps
func UnregisterColor(name string) {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	content := strings.ToLower(name)

	semanticMu.Lock()
	delete(consoleMap, content)
	delete(themeMap, content)
	semanticMu.Unlock()
}

// UnregisterPrefix removes all semantic tags that start with the given prefix from both maps
func UnregisterPrefix(prefix string) {
	ensureMaps()
	searchPrefix := strings.ToLower(strings.TrimSuffix(prefix, "_") + "_")
	semanticMu.Lock()
	for key := range consoleMap {
		if strings.HasPrefix(key, searchPrefix) {
			delete(consoleMap, key)
		}
	}
	for key := range themeMap {
		if strings.HasPrefix(key, searchPrefix) {
			delete(themeMap, key)
		}
	}
	semanticMu.Unlock()
}

// ClearThemeMap removes all entries from the theme map.
func ClearThemeMap() {
	semanticMu.Lock()
	themeMap = make(map[string]string)
	semanticMu.Unlock()
}

// ResetCustomColors clears all semantic tags and rebuilds from Colors struct
func ResetCustomColors() {
	BuildColorMap()
}

// StripDelimiters removes any known library delimiters from a style string to get the raw content
func StripDelimiters(text string) string {
	// Check current semantic delimiters
	if strings.HasPrefix(text, SemanticPrefix) && strings.HasSuffix(text, SemanticSuffix) {
		return text[len(SemanticPrefix) : len(text)-len(SemanticSuffix)]
	}
	// Check current direct delimiters
	if strings.HasPrefix(text, DirectPrefix) && strings.HasSuffix(text, DirectSuffix) {
		return text[len(DirectPrefix) : len(text)-len(DirectSuffix)]
	}
	// Fallback to standard delimiters if the globals have been customised
	if SemanticPrefix != "{{|" {
		if strings.HasPrefix(text, "{{|") && strings.HasSuffix(text, "|}}") {
			return text[3 : len(text)-3]
		}
	}
	if DirectPrefix != "{{[" {
		if strings.HasPrefix(text, "{{[") && strings.HasSuffix(text, "]}}") {
			return text[3 : len(text)-3]
		}
	}
	return text
}
