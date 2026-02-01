package console

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"codeberg.org/tslocum/cview"
)

var (
	// semanticMap stores semantic tag -> tview format mappings (e.g., "version" -> "[cyan]")
	semanticMap map[string]string

	// ansiMap stores color/modifier names -> ANSI code mappings
	ansiMap map[string]string

	// semanticRegex matches {{_content_}} format for semantic tags
	semanticRegex = regexp.MustCompile(`\{\{_([A-Za-z0-9_]+)_\}\}`)

	// directRegex matches {{|content|}} format for direct tview-style codes
	directRegex = regexp.MustCompile(`\{\{\|([A-Za-z0-9_:\-#]+)\|\}\}`)

	isTTYGlobal bool
)

func init() {
	// Initialize maps
	semanticMap = make(map[string]string)
	ansiMap = make(map[string]string)

	// Check TTY once
	stat, _ := os.Stdout.Stat()
	isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0

	// Build color maps lazily via ensureMaps()
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

	// Standard ANSI color/modifier mappings
	ansiMap["-"] = CodeReset
	ansiMap["reset"] = CodeReset
	ansiMap["bold"] = CodeBold
	ansiMap["dim"] = CodeDim
	ansiMap["underline"] = CodeUnderline
	ansiMap["blink"] = CodeBlink
	ansiMap["reverse"] = CodeReverse

	// Foreground colors
	ansiMap["black"] = CodeBlack
	ansiMap["red"] = CodeRed
	ansiMap["green"] = CodeGreen
	ansiMap["yellow"] = CodeYellow
	ansiMap["blue"] = CodeBlue
	ansiMap["magenta"] = CodeMagenta
	ansiMap["cyan"] = CodeCyan
	ansiMap["white"] = CodeWhite

	// Background colors (with "bg" suffix for fg:bg parsing)
	ansiMap["blackbg"] = CodeBlackBg
	ansiMap["redbg"] = CodeRedBg
	ansiMap["greenbg"] = CodeGreenBg
	ansiMap["yellowbg"] = CodeYellowBg
	ansiMap["bluebg"] = CodeBlueBg
	ansiMap["magentabg"] = CodeMagentaBg
	ansiMap["cyanbg"] = CodeCyanBg
	ansiMap["whitebg"] = CodeWhiteBg

	// Flag character mappings
	ansiMap["b"] = CodeBold
	ansiMap["d"] = CodeDim
	ansiMap["u"] = CodeUnderline
	ansiMap["l"] = CodeBlink
	ansiMap["r"] = CodeReverse

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

// RegisterSemanticTag registers a semantic tag with its tview-format value
func RegisterSemanticTag(name, tviewValue string) {
	ensureMaps()
	semanticMap[strings.ToLower(name)] = tviewValue
}

// ToTview converts semantic and direct tags to tview [color] format
// - {{_Tag_}} : Semantic lookup
// - {{|code|}} : Direct tview-style (just replace delimiters)
func ToTview(text string) string {
	ensureMaps()

	// 1. Process semantic tags {{_Tag_}}
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{_" and "_}}"
		content = strings.ToLower(content)

		// Check semantic map
		if tviewTag, ok := semanticMap[content]; ok {
			return tviewTag
		}

		// Unknown semantic tag - leave as-is for debugging
		return match
	})

	// 2. Process direct tags {{|code|}} -> [code]
	text = directRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{|" and "|}}"
		return "[" + content + "]"
	})

	return text
}

// ToANSI converts semantic and direct tags to ANSI escape sequences
// - {{_Tag_}} : Semantic lookup -> ANSI
// - {{|code|}} : Direct tview-style -> ANSI
func ToANSI(text string) string {
	ensureMaps()
	if !isTTYGlobal {
		// Not a TTY, strip all codes
		return Strip(text)
	}

	// 1. Process semantic tags {{_Tag_}}
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{_" and "_}}"
		content = strings.ToLower(content)

		// Check semantic map, then resolve tview tag to ANSI
		if tviewTag, ok := semanticMap[content]; ok {
			return tviewTagToANSI(tviewTag)
		}

		// Unknown semantic tag - strip it
		return ""
	})

	// 2. Process direct tags {{|code|}} -> ANSI
	text = directRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{|" and "|}}"
		content = strings.ToLower(content)
		return parseTviewStyleToANSI(content)
	})

	return text
}

// Strip removes all semantic and direct tags from text, leaving plain text
func Strip(text string) string {
	text = semanticRegex.ReplaceAllString(text, "")
	text = directRegex.ReplaceAllString(text, "")
	return text
}

// tviewTagToANSI converts a tview-format tag like "[cyan::b]" to ANSI codes
func tviewTagToANSI(tviewTag string) string {
	// Remove brackets: "[cyan::b]" -> "cyan::b"
	if len(tviewTag) < 2 || tviewTag[0] != '[' || tviewTag[len(tviewTag)-1] != ']' {
		return ""
	}
	content := tviewTag[1 : len(tviewTag)-1]
	return parseTviewStyleToANSI(content)
}

// parseTviewStyleToANSI parses fg:bg:flags format and returns ANSI codes
func parseTviewStyleToANSI(content string) string {
	if content == "-" {
		return CodeReset
	}

	// Split by colons: fg:bg:flags
	parts := strings.Split(content, ":")
	var codes strings.Builder

	// Part 0: Foreground color
	if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
		if code, ok := ansiMap[parts[0]]; ok {
			codes.WriteString(code)
		}
	}

	// Part 1: Background color
	if len(parts) > 1 && parts[1] != "" && parts[1] != "-" {
		// Try with "bg" suffix
		if code, ok := ansiMap[parts[1]+"bg"]; ok {
			codes.WriteString(code)
		}
	}

	// Part 2: Flags (each character is a flag: b=bold, u=underline, etc.)
	if len(parts) > 2 && parts[2] != "" {
		for _, flag := range parts[2] {
			flagStr := string(flag)
			if code, ok := ansiMap[flagStr]; ok {
				codes.WriteString(code)
			}
		}
	}

	return codes.String()
}

// Sprintf formats according to a format specifier and returns the string with ANSI codes
func Sprintf(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return ToANSI(msg)
}

// Println prints a line with ANSI color codes parsed
func Println(a ...interface{}) {
	msg := fmt.Sprint(a...)
	fmt.Println(ToANSI(msg))
}

// Parse is a convenience alias for ToANSI (backwards compatibility)
func Parse(text string) string {
	return ToANSI(text)
}

// Translate is a convenience alias for ToTview (backwards compatibility)
func Translate(text string) string {
	return ToTview(text)
}

// PrepareForTUI is a convenience alias for ToTview (backwards compatibility)
func PrepareForTUI(text string) string {
	return ToTview(text)
}

// ExpandSemanticTags is a convenience alias for ToTview (backwards compatibility)
func ExpandSemanticTags(text string) string {
	return ToTview(text)
}

// TranslateToTview is a convenience alias for ToTview (backwards compatibility)
func TranslateToTview(text string) string {
	return ToTview(text)
}

// ForTUI prepares text for cview/tview display with proper bracket escaping.
// Order of operations:
// 1. Escape literal brackets [text] -> [[text]] (cview.Escape)
// 2. Convert our tags {{_Tag_}} and {{|code|}} to [cview] format
// This ensures user content like [NOTICE] or [v2.0] doesn't get interpreted as color codes.
func ForTUI(text string) string {
	// 1. Escape any literal brackets in the text
	text = cview.Escape(text)
	// 2. Convert our semantic and direct tags to cview format
	return ToTview(text)
}

// Legacy compatibility functions

// RegisterColor is a legacy alias for RegisterSemanticTag
func RegisterColor(name, value string) {
	// Strip underscore wrapper if present (legacy format)
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	RegisterSemanticTag(name, value)
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

// ToCviewTag is a no-op for compatibility (tags are already in proper format)
func ToCviewTag(tag string) string {
	return tag
}
