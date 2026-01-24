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
	colorMap    map[string]string
	aliasMap    map[string]string
	aliasDefs   map[string]string
	semanticMap map[string]string // Raw definitions for base semantic colors
	tagRegex    = regexp.MustCompile(`\[([a-zA-Z0-9_:\-+]+)\]`)
	isTTYGlobal bool
)

func init() {
	// Initialize maps
	aliasMap = make(map[string]string)
	aliasDefs = make(map[string]string)
	semanticMap = make(map[string]string)

	// Check TTY once
	stat, _ := os.Stdout.Stat()
	isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0

	// Do NOT call BuildColorMap here - it will be called lazily via ensureMaps()
	// This avoids init() ordering issues between parser.go and colors.go
}

// ensureMaps ensures color maps are built if they were missed by init
func ensureMaps() {
	if len(semanticMap) == 0 {
		BuildColorMap()
	}
}

// BuildColorMap inspects the Colors struct to build a comprehensive map
func BuildColorMap() {
	colorMap = make(map[string]string)

	// Whitelist of standard keys that do NOT get underscores
	standardKeys := map[string]bool{
		"reset": true, "bold": true, "underline": true, "reverse": true,
		"black": true, "red": true, "green": true, "yellow": true, "blue": true, "magenta": true, "cyan": true, "white": true,
		"blackbg": true, "redbg": true, "greenbg": true, "yellowbg": true, "bluebg": true, "magentabg": true, "cyanbg": true, "whitebg": true,
	}

	val := reflect.ValueOf(Colors)
	typ := val.Type()

	// ANSI lookup for standard keys
	ansiMap := map[string]string{
		"reset": CodeReset, "bold": CodeBold, "dim": CodeDim, "underline": CodeUnderline, "blink": CodeBlink, "reverse": CodeReverse,
		"black": CodeBlack, "red": CodeRed, "green": CodeGreen, "yellow": CodeYellow, "blue": CodeBlue, "magenta": CodeMagenta, "cyan": CodeCyan, "white": CodeWhite,
		"blackbg": CodeBlackBg, "redbg": CodeRedBg, "greenbg": CodeGreenBg, "yellowbg": CodeYellowBg, "bluebg": CodeBlueBg, "magentabg": CodeMagentaBg, "cyanbg": CodeCyanBg, "whitebg": CodeWhiteBg,
	}

	// Pass 1: Standard Keys and Modifiers (The building blocks)
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := strings.ToLower(field.Name)
		if standardKeys[key] {
			if code, ok := ansiMap[key]; ok {
				colorMap[key] = code
			}
		}
	}

	// Add generic Reset aliases and tview modifiers
	if isTTYGlobal {
		colorMap["-"] = CodeReset
		// tview style modifiers
		colorMap["::b"] = CodeBold
		colorMap["::u"] = CodeUnderline
		colorMap["::b"] = CodeBold
		colorMap["::d"] = CodeDim
		colorMap["::l"] = CodeBlink
		colorMap["::r"] = CodeReverse
	} else {
		colorMap["-"] = ""
		colorMap["::u"] = ""
		colorMap["::b"] = ""
		colorMap["::d"] = ""
		colorMap["::l"] = ""
		colorMap["::r"] = ""
	}

	// Pass 2: Semantic colors (which may use standard keys inside tags)
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := strings.ToLower(field.Name)
		if !standardKeys[key] {
			valStr := val.Field(i).String()
			tagKey := "_" + key + "_"
			// 1. Store raw template definition for TUI translation
			// This is now always a graphical tag [tag] from colors.go
			semanticMap[tagKey] = valStr
			// 2. Resolve to ANSI for standard terminal output
			// We MUST use a clean parse here that doesn't trigger recursive colorMap lookups
			// since we are currently building the colorMap.
			colorMap[tagKey] = parseToANSI(valStr)
		}
	}
	// Semantic generic reset alias
	if isTTYGlobal {
		colorMap["_-_"] = CodeReset
	} else {
		colorMap["_-_"] = ""
	}
}

// parseToANSI is a restricted version of Parse used during initialization
// to prevent infinite recursion and ensure we map tags directly to pure ANSI codes.
func parseToANSI(text string) string {
	// Standard ANSI lookups
	ansiMap := map[string]string{
		"reset": CodeReset, "bold": CodeBold, "dim": CodeDim, "underline": CodeUnderline, "blink": CodeBlink, "reverse": CodeReverse,
		"black": CodeBlack, "red": CodeRed, "green": CodeGreen, "yellow": CodeYellow, "blue": CodeBlue, "magenta": CodeMagenta, "cyan": CodeCyan, "white": CodeWhite,
		"blackbg": CodeBlackBg, "redbg": CodeRedBg, "greenbg": CodeGreenBg, "yellowbg": CodeYellowBg, "bluebg": CodeBlueBg, "magentabg": CodeMagentaBg, "cyanbg": CodeCyanBg, "whitebg": CodeWhiteBg,
		"-": CodeReset,
		// Modifiers
		"::b": CodeBold, "::d": CodeDim, "::u": CodeUnderline, "::l": CodeBlink, "::r": CodeReverse,
	}

	return tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		tag := strings.ToLower(match[1 : len(match)-1])

		// Fast path: exact match
		if code, ok := ansiMap[tag]; ok {
			return code
		}

		// Handle complex [fg:bg:flags] or [fg::flags]
		if strings.Contains(tag, ":") {
			parts := strings.Split(tag, ":")
			var codes strings.Builder

			for i, part := range parts {
				if part == "" {
					continue
				}

				// Check if this is a modifier (previous part was empty, indicating ::)
				if i > 0 && parts[i-1] == "" {
					// This is a modifier like ::b, ::u, etc.
					modKey := "::" + part
					if code, ok := ansiMap[modKey]; ok {
						codes.WriteString(code)
					}
				} else {
					// This is a regular color
					if code, ok := ansiMap[part]; ok {
						codes.WriteString(code)
					}
				}
			}

			if codes.Len() > 0 {
				return codes.String()
			}
		}

		return ""
	})
}

// Sprintf formats according to a format specifier and returns the resulting string.
// It parses cview-style color tags (e.g. [red]text[-]).
func Sprintf(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return Parse(msg)
}

// Println is a convenience wrapper for fmt.Println with parsing
func Println(a ...interface{}) {
	msg := fmt.Sprint(a...)
	fmt.Println(Parse(msg))
}

// ToCviewTag standardizes a color tag string for use in cview.
// Currently it returns the tag as-is, but serves as a hook for normalization.
func ToCviewTag(tag string) string {
	// Future: validation or normalization logic
	return tag
}

// RegisterColor allows external registration of new color aliases.
// The name is case-insensitive. The value should be a valid tag string (e.g. "[white:red]").
func RegisterColor(name, value string) {
	if aliasMap == nil {
		aliasMap = make(map[string]string)
		aliasDefs = make(map[string]string)
	}
	name = strings.ToLower(name)

	// 1. Store raw definition for TUI/Graphical path
	// This ensures semantic translation never returns ANSI
	aliasDefs[name] = value

	// 2. Parse and store ANSI code for Console/Terminal path
	aliasMap[name] = parseToANSI(value)
}

// GetColorDefinition returns the original (cview-compatible) tag string for an alias.
func GetColorDefinition(name string) string {
	if aliasDefs == nil {
		return ""
	}
	return aliasDefs[strings.ToLower(name)]
}

// UnregisterColor removes a custom alias from the map.
func UnregisterColor(name string) {
	if aliasMap == nil {
		return
	}
	key := strings.ToLower(name)
	delete(aliasMap, key)
	delete(aliasDefs, key)
}

// ResetCustomColors clears all user-defined aliases, leaving standard/semantic colors intact.
func ResetCustomColors() {
	aliasMap = make(map[string]string)
	aliasDefs = make(map[string]string)
}

// Parse replaces color tags in the string with actual ANSI codes (or removes them if not TTY).
func Parse(text string) string {
	ensureMaps()
	return tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		// match is "[tag]"
		tagContent := match[1 : len(match)-1] // strip brackets
		tagContent = strings.ToLower(tagContent)

		// Fast path: Exact match (standard, semantic, or ::mods)
		// 1. Check internal map first
		if code, ok := colorMap[tagContent]; ok {
			return code
		}
		// 2. Check external alias map
		if code, ok := aliasMap[tagContent]; ok {
			return code
		}

		// Complex path: [fg:bg:flags]
		if strings.Contains(tagContent, ":") {
			parts := strings.Split(tagContent, ":")
			var codes strings.Builder
			foundAny := false

			// Helper to lookup code in both maps
			lookup := func(key string) (string, bool) {
				if code, ok := colorMap[key]; ok {
					return code, true
				}
				if code, ok := aliasMap[key]; ok {
					return code, true
				}
				return "", false
			}

			// Part 1: Foreground
			if len(parts) > 0 && parts[0] != "" {
				if code, ok := lookup(parts[0]); ok {
					codes.WriteString(code)
					foundAny = true
				}
			}

			// Part 2: Background
			if len(parts) > 1 && parts[1] != "" {
				// Try suffix "bg" first (e.g. "red" -> "redbg")
				if code, ok := lookup(parts[1] + "bg"); ok {
					codes.WriteString(code)
					foundAny = true
				}
			}

			// Part 3: Flags
			if len(parts) > 2 && parts[2] != "" {
				for _, flag := range parts[2] {
					switch flag {
					case 'b':
						if code, ok := colorMap["bold"]; ok {
							codes.WriteString(code)
							foundAny = true
						}
					case 'u':
						if code, ok := colorMap["underline"]; ok {
							codes.WriteString(code)
							foundAny = true
						}
					case 'r':
						if code, ok := colorMap["reverse"]; ok {
							codes.WriteString(code)
							foundAny = true
						}
					case 'l':
						if code, ok := colorMap["blink"]; ok {
							codes.WriteString(code)
							foundAny = true
						}
					case 'd':
						if code, ok := colorMap["dim"]; ok {
							codes.WriteString(code)
							foundAny = true
						}
					}
				}
			}

			if foundAny {
				return codes.String()
			}
		}

		// If tag not found, leave it as is (escape mechanism)
		return match
	})
}

// ExpandSemanticTags recursively resolves semantic tags (e.g. [_Version_])
// into standard cview graphical tags (e.g. [cyan]).
func ExpandSemanticTags(text string) string {
	ensureMaps()
	prev := ""
	for i := 0; i < 5 && text != prev; i++ {
		prev = text
		text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
			tagContent := strings.ToLower(match[1 : len(match)-1])
			if def, ok := semanticMap[tagContent]; ok {
				return def
			}
			if def, ok := aliasDefs[tagContent]; ok {
				return def
			}
			return match
		})
	}
	return text
}

// Translate is a legacy alias for ExpandSemanticTags.
func Translate(text string) string {
	return ExpandSemanticTags(text)
}

// Strip removes all semantic tags, graphical tags, and ANSI escape sequences
// from the text to provide a clean, colorless string, while preserving literal brackets.
func Strip(text string) string {
	// 1. Resolve semantic tags to graphical markup ONLY (e.g. [_Notice_] -> [green])
	text = ExpandSemanticTags(text)

	// 2. Remove standard ANSI escape sequences (leak protection)
	// Match both complete sequences (with ESC byte) and corrupted sequences (without ESC byte)
	reAnsiComplete := regexp.MustCompile("\x1b" + `[[\]()#;?]*([0-9]{1,4}(;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]`)
	text = reAnsiComplete.ReplaceAllString(text, "")

	// Match CSI sequences where the ESC byte has been lost, leaving just the control sequence
	// This handles corrupted codes like [0m, [36m, [1m, [16;1H (without the \x1b prefix)
	reAnsiCorrupted := regexp.MustCompile(`\[([0-9]{1,4}(;[0-9]{0,4})*)?[mHABCDEFGJKSTfnsu\?hlpr]`)
	text = reAnsiCorrupted.ReplaceAllString(text, "")

	// 3. surgically remove cview-style tags while preserving literal brackets.
	// We target only brackets containing standard tview characters (lowercase letters, colons).
	// This preserves content like [v2.0] or [10.1.1.1] or [NOTICE] (uppercase).
	reSurgical := regexp.MustCompile(`\[([a-z:]+)\]`)

	prev := ""
	for i := 0; i < 5 && text != prev; i++ {
		prev = text
		text = reSurgical.ReplaceAllString(text, "")
		text = strings.ReplaceAll(text, "[-]", "")
	}

	return text
}

// PrepareForTUI prepares text for TUI display by:
// 1. Converting semantic tags to cview graphical tags
// 2. Removing any ANSI escape sequences
// 3. Keeping cview tags for proper color rendering
// 4. Preserving literal brackets
// Use this instead of Strip() when you want colors in the TUI.
func PrepareForTUI(text string) string {
	// 1. Resolve semantic tags to cview graphical tags (e.g. [_Notice_] -> [green])
	text = ExpandSemanticTags(text)

	// 2. Remove ANSI escape sequences (corruption protection)
	// Match both complete sequences (with ESC byte) and corrupted sequences (without ESC byte)
	reAnsiComplete := regexp.MustCompile("\x1b" + `[[\]()#;?]*([0-9]{1,4}(;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]`)
	text = reAnsiComplete.ReplaceAllString(text, "")

	// Match CSI sequences where the ESC byte has been lost
	reAnsiCorrupted := regexp.MustCompile(`\[([0-9]{1,4}(;[0-9]{0,4})*)?[mHABCDEFGJKSTfnsu\?hlpr]`)
	text = reAnsiCorrupted.ReplaceAllString(text, "")

	// 3. DO NOT remove cview tags - they're needed for color rendering!
	// cview.Escape() will handle literal bracket protection when this is written to the TUI

	return text
}

// TranslateToTview resolves semantic tags into cview-compatible tags
// and then handles any remaining ANSI escape sequences using cview's
// native translation routine.
func TranslateToTview(text string) string {
	// 1. Resolve semantic tags (recursive)
	// This turns [_Notice_] into things like [green:b] or raw ANSI (\x1b[32m)
	text = Translate(text)

	// 2. Handle ANSI escape codes using cview
	text = cview.TranslateANSI(text)

	return text
}
