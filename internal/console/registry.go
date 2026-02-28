package console

import (
	"reflect"
	"strings"
	"sync"
)

var (
	// semanticMu guards all reads and writes of semanticMap, which is written
	// concurrently by theme-loading goroutines and read by the render goroutine.
	semanticMu sync.RWMutex

	// semanticMap stores semantic tag -> raw style code mappings (e.g., "version" -> "cyan")
	semanticMap map[string]string

	// ansiMap stores color/modifier names -> ANSI code mappings
	ansiMap map[string]string

	// attributeMap stores non-color attribute names -> ANSI code mappings
	attributeMap map[string]string
)

func init() {
	// Initialize maps
	semanticMap = make(map[string]string)
	ansiMap = make(map[string]string)
	attributeMap = make(map[string]string)
}

// ensureMaps ensures color maps are built if they were missed by init
func ensureMaps() {
	if len(ansiMap) == 0 {
		BuildColorMap()
	}
}

// BuildColorMap initializes the ANSI code mappings and semantic tag definitions.
func BuildColorMap() {
	if ansiMap == nil {
		ansiMap = make(map[string]string)
	}
	if semanticMap == nil {
		semanticMap = make(map[string]string)
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

	// Build semantic map from Colors struct
	val := reflect.ValueOf(Colors)
	typ := val.Type()

	baseKeys := map[string]bool{
		"reset": true, "bold": true, "dim": true, "underline": true, "blink": true, "reverse": true,
		"black": true, "red": true, "green": true, "yellow": true, "blue": true, "magenta": true, "cyan": true, "white": true,
		"blackbg": true, "redbg": true, "greenbg": true, "yellowbg": true, "bluebg": true, "magentabg": true, "cyanbg": true, "whitebg": true,
	}

	semanticMu.Lock()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := strings.ToLower(field.Name)
		if !baseKeys[key] {
			valStr := val.Field(i).String()
			// Store raw value (strip brackets if present)
			semanticMap[key] = StripDelimiters(valStr)
		}
	}
	semanticMu.Unlock()
}

// RegisterSemanticTag registers a semantic tag with its standardized tag value.
// It automatically strips delimiters if present to store the raw value.
func RegisterSemanticTag(name, taggedValue string) {
	RegisterSemanticTagRaw(name, StripDelimiters(taggedValue))
}

// RegisterSemanticTagRaw registers a semantic tag with a raw style code (no brackets).
func RegisterSemanticTagRaw(name, rawValue string) {
	ensureMaps()
	semanticMu.Lock()
	semanticMap[strings.ToLower(name)] = rawValue
	semanticMu.Unlock()
}

// GetColorDefinition returns the formatted tag value (with brackets) for a semantic tag.
// This is used for expansion and backward compatibility.
func GetColorDefinition(name string) string {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	semanticMu.RLock()
	raw := semanticMap[strings.ToLower(name)]
	semanticMu.RUnlock()
	if raw == "" {
		return ""
	}
	return WrapDirect(raw)
}

// UnregisterColor removes a semantic tag
func UnregisterColor(name string) {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	semanticMu.Lock()
	delete(semanticMap, strings.ToLower(name))
	semanticMu.Unlock()
}

// UnregisterPrefix removes all semantic tags that start with the given prefix
func UnregisterPrefix(prefix string) {
	ensureMaps()
	searchPrefix := strings.ToLower(strings.TrimSuffix(prefix, "_") + "_")
	semanticMu.Lock()
	for key := range semanticMap {
		if strings.HasPrefix(key, searchPrefix) {
			delete(semanticMap, key)
		}
	}
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
	// Fallback to standard delimiters if they differ
	if SemanticPrefix != "{{|" {
		if strings.HasPrefix(text, "{{|") && strings.HasSuffix(text, "|}}") {
			return text[3 : len(text)-3]
		}
	}
	// Also fallback to the previous legacy semantic delimiter
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
