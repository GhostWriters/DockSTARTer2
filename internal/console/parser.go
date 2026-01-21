package console

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
)

var (
	colorMap    map[string]string
	aliasMap    map[string]string
	aliasDefs   map[string]string
	tagRegex    = regexp.MustCompile(`\[([a-zA-Z0-9_:\-+]+)\]`)
	isTTYGlobal bool
)

func init() {
	// Check TTY once for the parser
	stat, _ := os.Stdout.Stat()
	isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0

	BuildColorMap()

	// Initialize separate maps for custom colors
	aliasMap = make(map[string]string)  // ANSI codes
	aliasDefs = make(map[string]string) // Raw definitions
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

	// Pass 1: Standard Keys and Modifiers (The building blocks)
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := strings.ToLower(field.Name)
		if standardKeys[key] {
			colorMap[key] = val.Field(i).String()
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
			// Resolve the semantic tag value (e.g. "[cyan:b]") to ANSI using Parse
			// Parse will use the standard keys we just loaded in Pass 1.
			colorMap["_"+key+"_"] = Parse(valStr)
		}
	}
	// Semantic generic reset alias
	if isTTYGlobal {
		colorMap["_-_"] = CodeReset
	} else {
		colorMap["_-_"] = ""
	}
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

	// 1. Normalize/Wrap
	cviewTag := ToCviewTag(value)

	// 2. Store raw definition for TUI
	aliasDefs[name] = cviewTag

	// 3. Parse and store ANSI code for Console
	aliasMap[name] = Parse(cviewTag)
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

// Translate resolves semantic tags (e.g. [_Version_]) into cview-compatible tags ([cyan]).
// This is used for TUI output where ANSI codes are not appropriate.
// It is recursive to handle nested definitions.
func Translate(text string) string {
	prev := ""
	// Maximum depth to prevent infinite loops (though shouldn't happen with themes)
	for i := 0; i < 5 && text != prev; i++ {
		prev = text
		text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
			// match is "[tag]"
			tagContent := match[1 : len(match)-1] // strip brackets
			tagContent = strings.ToLower(tagContent)

			// 1. Check if it's an alias definition (which is already a cview tag or another semantic tag)
			if def := GetColorDefinition(tagContent); def != "" {
				return def
			}

			// 2. If it's a standard color/style like [red] or [::b], keep it as is
			return match
		})
	}
	return text
}
