package console

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/muesli/termenv"
)

var (
	// semanticMap stores semantic tag -> standardized tag mappings (e.g., "version" -> "{{|cyan|}}")
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

// RegisterSemanticTag registers a semantic tag with its standardized tag value
func RegisterSemanticTag(name, taggedValue string) {
	ensureMaps()
	semanticMap[strings.ToLower(name)] = taggedValue
}

// ExpandTags converts semantic and direct tags to standardized {{|style|}} format
// - {{_Tag_}} : Semantic lookup
// - {{|code|}} : Direct style (no-op, just for consistency)
func ExpandTags(text string) string {
	ensureMaps()

	// 1. Process semantic tags {{_Tag_}}
	text = semanticRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{_" and "_}}"
		content = strings.ToLower(content)

		// Check semantic map
		if tag, ok := semanticMap[content]; ok {
			return tag
		}

		// Unknown semantic tag - strip it
		return ""
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

	// 1. Expand all semantic tags first (Pass 1)
	// This ensures that multi-tag definitions like {{|-|}}{{|blue|}} are fully expanded
	text = ExpandTags(text)

	// 2. Process all direct tags {{|code|}} -> ANSI (Pass 2)
	text = directRegex.ReplaceAllStringFunc(text, func(match string) string {
		content := match[3 : len(match)-3] // Strip "{{|" and "|}}"
		return parseStyleCodeToANSI(content)
	})

	return text
}

// Strip removes all semantic and direct tags from text, leaving plain text
func Strip(text string) string {
	text = semanticRegex.ReplaceAllString(text, "")
	text = directRegex.ReplaceAllString(text, "")
	return text
}

// resolveTaggedStyleToANSI converts a standardized tag like "{{|cyan::B|}}" to ANSI codes
func resolveTaggedStyleToANSI(tag string) string {
	// Support both "{{|content|}}" and plain "content"
	content := tag
	if strings.HasPrefix(tag, "{{|") && strings.HasSuffix(tag, "|}}") {
		content = tag[3 : len(tag)-3]
	}

	return parseStyleCodeToANSI(content)
}

// parseStyleCodeToANSI parses fg:bg:flags format and returns ANSI codes
func parseStyleCodeToANSI(content string) string {
	if content == "-" {
		return CodeReset
	}

	// Split by colons: fg:bg:flags
	parts := strings.Split(content, ":")
	var codes strings.Builder

	// Pre-emptive reset if flags start with '-'
	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		codes.WriteString(CodeReset)
	}

	// Flags (peek for H early to affect colors)
	highIntensity := false
	if len(parts) > 2 {
		f := parts[2]
		if strings.Contains(strings.ToLower(f), "h") {
			highIntensity = true
		}
	}

	// Part 0: Foreground color
	if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
		colorName := strings.ToLower(parts[0])
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		color := preferredProfile.Color(colorName)
		if strings.HasPrefix(colorName, "#") {
			// Hex color
			codes.WriteString(wrapSequence(color.Sequence(false)))
		} else if val, ok := ColorToHexMap[colorName]; ok {
			// Named color resolved to hex or index
			if highIntensity {
				if brightVal, ok := getBrightIndex(val); ok {
					val = brightVal
				}
			}
			color = preferredProfile.Color(val)
			codes.WriteString(wrapSequence(color.Sequence(false)))
		} else if code, ok := ansiMap[colorName]; ok {
			// Direct ANSI code mapping (e.g., "bold" or custom)
			codes.WriteString(code)
		} else {
			// Fallback: Try to use it as a raw color (e.g. "7", "235")
			// This handles the case where theme.go resolves "silver" to "7"
			// and passes "7" here, which isn't in ColorToHexMap/ansiMap.
			color = preferredProfile.Color(colorName)
			seq := color.Sequence(false)
			if seq != "" {
				codes.WriteString(wrapSequence(seq))
			}
		}
	}

	// Part 1: Background color
	if len(parts) > 1 && parts[1] != "" && parts[1] != "-" {
		colorName := strings.ToLower(parts[1])
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		color := preferredProfile.Color(colorName)
		if strings.HasPrefix(colorName, "#") {
			// Hex color
			codes.WriteString(wrapSequence(color.Sequence(true)))
		} else if val, ok := ColorToHexMap[colorName]; ok {
			// Named color resolved to hex or index
			if highIntensity {
				if brightVal, ok := getBrightIndex(val); ok {
					val = brightVal
				}
			}
			color = preferredProfile.Color(val)
			codes.WriteString(wrapSequence(color.Sequence(true)))
		} else if code, ok := ansiMap[colorName+"bg"]; ok {
			// Direct ANSI code mapping
			codes.WriteString(code)
		} else {
			// Fallback: Try to use it as a raw color (e.g. "7", "235")
			color = preferredProfile.Color(colorName)
			seq := color.Sequence(true)
			if seq != "" {
				codes.WriteString(wrapSequence(seq))
			}
		}
	}

	// Part 2: Flags (each character is a flag: b=bold, u=underline, etc.)
	if len(parts) > 2 && parts[2] != "" {
		f := strings.TrimPrefix(parts[2], "-")
		for _, flag := range f {
			flagStr := string(flag)
			if code, ok := ansiMap[flagStr]; ok {
				codes.WriteString(code)
			}
		}
	}

	return codes.String()
}

func getBrightVariant(name string) (string, bool) {
	if strings.HasPrefix(name, "bright-") {
		return name, true
	}
	// Check if bright variant exists in ansiMap
	if _, ok := ansiMap["bright-"+name]; ok {
		return "bright-" + name, true
	}
	return name, false
}

func getBrightIndex(val string) (string, bool) {
	// If val is a single digit 0-7, shift it to 8-15
	if len(val) == 1 && val[0] >= '0' && val[0] <= '7' {
		return string(val[0] + 8), true
	}
	// Handle cases like "13" etc if needed, but usually standard colors are 0-7
	return val, false
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

// Translate is a convenience alias for ExpandTags (backwards compatibility)
func Translate(text string) string {
	return ExpandTags(text)
}

// ExpandSemanticTags is a convenience alias for ExpandTags (backwards compatibility)
func ExpandSemanticTags(text string) string {
	return ExpandTags(text)
}

// TranslateToTagged is a convenience alias for ExpandTags
func TranslateToTagged(text string) string {
	return ExpandTags(text)
}

// ForTUI prepares text for display with standardized tags.
// Literal brackets [text] are now treated as plain text and do NOT need escaping.
func ForTUI(text string) string {
	return ExpandTags(text)
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
