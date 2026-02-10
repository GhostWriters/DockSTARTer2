package console

import (
	"fmt"
	"strings"

	tcellColor "github.com/gdamore/tcell/v3/color"
)

// parseStyleCodeToANSI parses fg:bg:flags format and returns ANSI codes
func parseStyleCodeToANSI(content string) string {
	if content == "-" {
		return CodeReset
	}

	// Split by colons: fg:bg:flags
	parts := strings.Split(content, ":")
	var codes strings.Builder

	// Pre-emptive reset of flags ONLY if they start with '-'
	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		// 22:Bold/Dim off, 23:Italic off, 24:Underline off, 25:Blink off, 27:Reverse off, 29:Strikethrough off
		codes.WriteString("\x1b[22m\x1b[23m\x1b[24m\x1b[25m\x1b[27m\x1b[29m")
	}

	// Flags (peek for H early to affect colors)
	highIntensity := false
	if len(parts) > 2 {
		f := parts[2]
		if strings.Contains(f, "H") {
			highIntensity = true
		}
	}

	// Part 0: Foreground color
	if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
		colorName := strings.ToLower(parts[0])
		// Handle high intensity by pretending it's the "bright" variant name if standard
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		if strings.HasPrefix(colorName, "#") {
			// Already Hex: Pass directly to termenv
			codes.WriteString(wrapSequence(preferredProfile.Color(colorName).Sequence(false)))
		} else {
			// Check for non-color attributes (bold, etc.) first
			if code, ok := attributeMap[colorName]; ok {
				codes.WriteString(code)
				goto FoundFG
			}

			// NEW: Check ansiMap for standard colors (max compatibility)
			if code, ok := ansiMap[colorName]; ok {
				codes.WriteString(code)
				goto FoundFG
			}

			// Color Name Resolution Strategy:
			// 1. Ask tcell for the color (handles standard "red", extended "orange", and aliases)
			// 2. Get the Hex value from tcell
			// 3. Pass Hex to termenv/lipgloss profile to generate correct sequence (or empty if mono)

			tc := ResolveTcellColor(colorName)
			// tcell.GetColor returns ColorDefault if unknown, or a valid color
			// It handles "red", "black", "orange", etc.

			if tc != tcellColor.Default {
				// We have a valid tcell color. Use its Hex value.
				// For mapped colors (like ColorRed), .Hex() returns the standard hex (e.g. 0xFF0000)
				hexVal := tc.Hex()
				if hexVal >= 0 {
					hexStr := fmt.Sprintf("#%06x", hexVal)
					if c := preferredProfile.Color(hexStr); c != nil {
						codes.WriteString(wrapSequence(c.Sequence(false)))
					}
					goto FoundFG
				}
			}

			// Fallback: Drop unsafe termenv name lookup.
			// Only hex or tcell-resolved colors are supported for names.
			// If it's a raw number string (e.g. "235"), termenv might handle it if we passed it?
			// But for now, strict tcell usage is safer to avoid panics.

		}
	}
FoundFG:

	// Part 1: Background color
	if len(parts) > 1 && parts[1] != "" && parts[1] != "-" {
		colorName := strings.ToLower(parts[1])
		// Handle high intensity background if needed (though usually flags handle this)
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		if strings.HasPrefix(colorName, "#") {
			// Hex color
			if c := preferredProfile.Color(colorName); c != nil {
				codes.WriteString(wrapSequence(c.Sequence(true)))
			}
		} else {
			if code, ok := attributeMap[colorName]; ok {
				// Attributes acting as background? Rare but possible for some maps
				codes.WriteString(code)
				goto FoundBG
			}

			// NEW: Check ansiMap for standard background colors (max compatibility)
			if code, ok := ansiMap[colorName+"bg"]; ok {
				codes.WriteString(code)
				goto FoundBG
			}

			// Standard/Extended Color Resolution via tcell
			tc := ResolveTcellColor(colorName)
			if tc != tcellColor.Default {
				hexVal := tc.Hex()
				if hexVal >= 0 {
					hexStr := fmt.Sprintf("#%06x", hexVal)
					if c := preferredProfile.Color(hexStr); c != nil {
						codes.WriteString(wrapSequence(c.Sequence(true)))
					}
					goto FoundBG
				}
			}
			// Or safer: just drop the naive fallback.
		}
	}
FoundBG:

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

// resolveTaggedStyleToANSI converts a standardized tag like "{{|cyan::B|}}" to ANSI codes
func resolveTaggedStyleToANSI(tag string) string {
	// Support both "{{|content|}}" and plain "content"
	content := tag
	if strings.HasPrefix(tag, "{{|") && strings.HasSuffix(tag, "|}}") {
		content = tag[3 : len(tag)-3]
	}

	return parseStyleCodeToANSI(content)
}

// getBrightVariant attempts to get the bright variant of a color name
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
