package console

import (
	"reflect"
	"strings"
)

var (
	// semanticMap stores semantic tag -> standardized tag mappings (e.g., "version" -> "{{|cyan|}}")
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
	// Modifiers/Attributes map
	// We only store non-color attributes here. Colors are handled via preferredProfile.Color()
	attributeMap = map[string]string{
		"reset":          CodeReset,
		"-":              CodeReset,
		"bold":           CodeBold,
		"b":              CodeBold,
		"dim":            CodeDim,
		"d":              CodeDim,
		"underline":      CodeUnderline,
		"u":              CodeUnderline,
		"blink":          CodeBlink,
		"l":              CodeBlink,
		"reverse":        CodeReverse,
		"r":              CodeReverse,
		"italic":         CodeItalic,
		"i":              CodeItalic,
		"strikethrough":  CodeStrikethrough,
		"s":              CodeStrikethrough,
		"-bold":          CodeBoldOff,
		"-b":             CodeBoldOff,
		"-dim":           CodeDimOff,
		"-d":             CodeDimOff,
		"-underline":     CodeUnderlineOff,
		"-u":             CodeUnderlineOff,
		"-blink":         CodeBlinkOff,
		"-l":             CodeBlinkOff,
		"-reverse":       CodeReverseOff,
		"-r":             CodeReverseOff,
		"-italic":        CodeItalicOff,
		"-i":             CodeItalicOff,
		"-strikethrough": CodeStrikethroughOff,
		"-s":             CodeStrikethroughOff,
	}
}

// ensureMaps ensures color maps are built if they were missed by init
func ensureMaps() {
	if len(ansiMap) == 0 {
		BuildColorMap()
	}
}

// BuildColorMap initializes the ANSI code mappings and semantic tag definitions.
// NOTE: This preserves existing semantic tags to allow theme registration before/after this call.
func BuildColorMap() {
	ansiMap = make(map[string]string)
	if semanticMap == nil {
		semanticMap = make(map[string]string)
	}

	// Standard ANSI color/modifier mappings (Case-sensitive for tags)
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

	// Foreground colors
	ansiMap["black"] = CodeBlack
	ansiMap["red"] = CodeRed
	ansiMap["green"] = CodeGreen
	ansiMap["yellow"] = CodeYellow
	ansiMap["blue"] = CodeBlue
	ansiMap["magenta"] = CodeMagenta
	ansiMap["cyan"] = CodeCyan
	ansiMap["white"] = CodeWhite

	// Foreground colors (Bright)
	ansiMap["bright-black"] = CodeBrightBlack
	ansiMap["bright-red"] = CodeBrightRed
	ansiMap["bright-green"] = CodeBrightGreen
	ansiMap["bright-yellow"] = CodeBrightYellow
	ansiMap["bright-blue"] = CodeBrightBlue
	ansiMap["bright-magenta"] = CodeBrightMagenta
	ansiMap["bright-cyan"] = CodeBrightCyan
	ansiMap["bright-white"] = CodeBrightWhite

	// Background colors (with "bg" suffix for fg:bg parsing)
	ansiMap["blackbg"] = CodeBlackBg
	ansiMap["redbg"] = CodeRedBg
	ansiMap["greenbg"] = CodeGreenBg
	ansiMap["yellowbg"] = CodeYellowBg
	ansiMap["bluebg"] = CodeBlueBg
	ansiMap["magentabg"] = CodeMagentaBg
	ansiMap["cyanbg"] = CodeCyanBg
	ansiMap["whitebg"] = CodeWhiteBg

	// Background colors (Bright)
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

	// Whitelist of base codes that are NOT semantic
	baseKeys := map[string]bool{
		"reset": true, "bold": true, "dim": true, "underline": true, "blink": true, "reverse": true,
		"black": true, "red": true, "green": true, "yellow": true, "blue": true, "magenta": true, "cyan": true, "white": true,
		"blackbg": true, "redbg": true, "greenbg": true, "yellowbg": true, "bluebg": true, "magentabg": true, "cyanbg": true, "whitebg": true,
	}

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := strings.ToLower(field.Name)
		if !baseKeys[key] {
			valStr := val.Field(i).String()
			// Store the tview-format value (e.g., "[cyan::b]")
			semanticMap[key] = valStr
		}
	}
}

// RegisterSemanticTag registers a semantic tag with its standardized tag value
func RegisterSemanticTag(name, taggedValue string) {
	ensureMaps()
	semanticMap[strings.ToLower(name)] = taggedValue
}

// GetColorDefinition returns the tview-format value for a semantic tag
func GetColorDefinition(name string) string {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	return semanticMap[strings.ToLower(name)]
}

// UnregisterColor removes a semantic tag
func UnregisterColor(name string) {
	ensureMaps()
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	delete(semanticMap, strings.ToLower(name))
}

// ResetCustomColors clears all semantic tags and rebuilds from Colors struct
func ResetCustomColors() {
	BuildColorMap()
}
