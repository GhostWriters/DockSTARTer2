package console

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"codeberg.org/tslocum/cview"
	"github.com/muesli/termenv"
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

	// preferredProfile stores the detected or forced color profile
	preferredProfile termenv.Profile
)

func init() {
	// Initialize maps
	semanticMap = make(map[string]string)
	ansiMap = make(map[string]string)

	// Check TTY once
	stat, _ := os.Stdout.Stat()
	isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0

	// Detect color profile
	preferredProfile = detectProfile()

	// Build color maps lazily via ensureMaps()
}

// GetPreferredProfile returns the detected or forced color profile
func GetPreferredProfile() termenv.Profile {
	return preferredProfile
}

// SetPreferredProfile explicitly sets the color profile (useful for testing)
func SetPreferredProfile(p termenv.Profile) {
	preferredProfile = p
}

func detectProfile() termenv.Profile {
	// 1. Check COLORTERM for explicit overrides
	colorTerm := strings.ToLower(os.Getenv("COLORTERM"))
	switch colorTerm {
	case "truecolor", "24bit":
		return termenv.TrueColor
	case "8bit", "256color":
		return termenv.ANSI256
	case "4bit", "16color", "8color", "3bit":
		return termenv.ANSI
	case "1bit", "2color", "mono", "false", "0":
		return termenv.Ascii
	}

	// 2. Check TERM for well-known color-capable terms
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "direct") {
		return termenv.TrueColor
	}
	if strings.Contains(term, "256color") {
		return termenv.ANSI256
	}
	if strings.Contains(term, "16color") {
		return termenv.ANSI
	}
	if term == "dumb" {
		return termenv.Ascii
	}

	// 3. Fallback to automatic detection
	return termenv.ColorProfile()
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
	ansiMap["b"] = CodeBoldOff
	ansiMap["B"] = CodeBold
	ansiMap["d"] = CodeDimOff
	ansiMap["D"] = CodeDim
	ansiMap["u"] = CodeUnderlineOff
	ansiMap["U"] = CodeUnderline
	ansiMap["l"] = CodeBlinkOff
	ansiMap["L"] = CodeBlink
	ansiMap["r"] = CodeReverseOff
	ansiMap["R"] = CodeReverse
	ansiMap["s"] = CodeStrikethroughOff
	ansiMap["S"] = CodeStrikethrough

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

		// Unknown semantic tag - strip it
		return ""
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
		color := preferredProfile.Color(parts[0])
		if strings.HasPrefix(parts[0], "#") {
			// Hex color
			codes.WriteString(wrapSequence(color.Sequence(false)))
		} else if val, ok := ColorToHexMap[strings.ToLower(parts[0])]; ok {
			// Named color resolved to hex or index
			color = preferredProfile.Color(val)
			codes.WriteString(wrapSequence(color.Sequence(false)))
		} else if code, ok := ansiMap[parts[0]]; ok {
			// Direct ANSI code mapping
			codes.WriteString(code)
		}
	}

	// Part 1: Background color
	if len(parts) > 1 && parts[1] != "" && parts[1] != "-" {
		color := preferredProfile.Color(parts[1])
		if strings.HasPrefix(parts[1], "#") {
			// Hex color
			codes.WriteString(wrapSequence(color.Sequence(true)))
		} else if val, ok := ColorToHexMap[strings.ToLower(parts[1])]; ok {
			// Named color resolved to hex or index
			color = preferredProfile.Color(val)
			codes.WriteString(wrapSequence(color.Sequence(true)))
		} else if code, ok := ansiMap[parts[1]+"bg"]; ok {
			// Direct ANSI code mapping
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

// wrapSequence ensures a color sequence part is wrapped in CSI delimiters
func wrapSequence(seq string) string {
	if seq == "" {
		return ""
	}
	if strings.HasPrefix(seq, "\x1b[") {
		return seq
	}
	return "\033[" + seq + "m"
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
