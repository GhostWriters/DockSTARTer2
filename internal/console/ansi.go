package console

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	tcellColor "github.com/gdamore/tcell/v3/color"
)

// colorToFGSequence returns the ANSI opening sequence for a foreground color,
// using the lipgloss renderer (profile-aware via Bubble Tea or auto-detected).
func colorToFGSequence(c color.Color) string {
	rendered := lipgloss.NewStyle().Foreground(c).Render("")
	return strings.TrimSuffix(rendered, CodeReset)
}

// colorToBGSequence returns the ANSI opening sequence for a background color.
func colorToBGSequence(c color.Color) string {
	rendered := lipgloss.NewStyle().Background(c).Render("")
	return strings.TrimSuffix(rendered, CodeReset)
}

// parseStyleCodeToANSI parses fg:bg:flags format and returns ANSI codes.
// Uses the lipgloss global renderer (set from colorprofile in profile.go).
func parseStyleCodeToANSI(content string) string {
	if content == "-" {
		return CodeReset
	}
	if content == "~" {
		return CodeHardReset
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
	if len(parts) > 0 && parts[0] == "~" {
		codes.WriteString(CodeHardFGReset)
	} else if len(parts) > 0 && parts[0] == "-" {
		codes.WriteString(CodeFGReset)
	} else if len(parts) > 0 && parts[0] != "" {
		colorName := strings.ToLower(parts[0])
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		// Check for non-color attributes (bold, etc.) first
		if code, ok := attributeMap[colorName]; ok {
			codes.WriteString(code)
			goto FoundFG
		}

		// Check ansiMap for standard colors (direct ANSI codes, max compatibility)
		if code, ok := ansiMap[colorName]; ok {
			codes.WriteString(code)
			goto FoundFG
		}

		// Extended color: resolve via tcell → hex → lipgloss
		if strings.HasPrefix(colorName, "#") {
			codes.WriteString(colorToFGSequence(lipgloss.Color(colorName)))
		} else {
			tc := ResolveTcellColor(colorName)
			if tc != tcellColor.Default {
				hexVal := tc.Hex()
				if hexVal >= 0 {
					codes.WriteString(colorToFGSequence(lipgloss.Color(fmt.Sprintf("#%06x", hexVal))))
				}
			}
		}
	}
FoundFG:

	// Part 1: Background color
	if len(parts) > 1 && parts[1] == "~" {
		codes.WriteString(CodeHardBGReset)
	} else if len(parts) > 1 && parts[1] == "-" {
		codes.WriteString(CodeBGReset)
	} else if len(parts) > 1 && parts[1] != "" {
		colorName := strings.ToLower(parts[1])
		if highIntensity {
			if brightName, ok := getBrightVariant(colorName); ok {
				colorName = brightName
			}
		}

		if code, ok := attributeMap[colorName]; ok {
			codes.WriteString(code)
			goto FoundBG
		}

		if code, ok := ansiMap[colorName+"bg"]; ok {
			codes.WriteString(code)
			goto FoundBG
		}

		if strings.HasPrefix(colorName, "#") {
			codes.WriteString(colorToBGSequence(lipgloss.Color(colorName)))
		} else {
			tc := ResolveTcellColor(colorName)
			if tc != tcellColor.Default {
				hexVal := tc.Hex()
				if hexVal >= 0 {
					codes.WriteString(colorToBGSequence(lipgloss.Color(fmt.Sprintf("#%06x", hexVal))))
				}
			}
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

var ansiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*((?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?\u0007|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))")

// StripANSI removes all ANSI escape sequences from a string
func StripANSI(text string) string {
	return ansiRegex.ReplaceAllString(text, "")
}

// resolveTaggedStyleToANSI converts a standardized tag like "{{[cyan::B]}}" to ANSI codes
func resolveTaggedStyleToANSI(tag string) string {
	content := tag
	if strings.HasPrefix(tag, DirectPrefix) && strings.HasSuffix(tag, DirectSuffix) {
		content = tag[len(DirectPrefix) : len(tag)-len(DirectSuffix)]
	}
	return parseStyleCodeToANSI(content)
}

// getBrightVariant attempts to get the bright variant of a color name
func getBrightVariant(name string) (string, bool) {
	if strings.HasPrefix(name, "bright-") {
		return name, true
	}
	if _, ok := ansiMap["bright-"+name]; ok {
		return "bright-" + name, true
	}
	return name, false
}
