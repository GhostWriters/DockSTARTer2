package tui

import (
	"regexp"
	"strings"

	"DockSTARTer2/internal/console"

	"github.com/charmbracelet/lipgloss"
)

// colorMap is deprecated in favor of ParseColor helper

// themeTagRegex matches {{_SymanticColor_}} or {{|codes|}} or {{|-|}}
var themeTagRegex = regexp.MustCompile(`\{\{(_[^}]+_|\|[^}]*\|)\}\}`)

// cviewColorRegex matches cview color tags like [white:blue:b] or [-]
var cviewColorRegex = regexp.MustCompile(`\[([^\]]+)\]`)

// RenderThemeText takes text with {{...}} theme tags and returns lipgloss-styled text
// defaultStyle is used for reset state and unstyled text
func RenderThemeText(text string, defaultStyle ...lipgloss.Style) string {
	var result strings.Builder
	var currentStyle lipgloss.Style
	var resetStyle lipgloss.Style

	// Use provided default style or blank style
	if len(defaultStyle) > 0 {
		resetStyle = defaultStyle[0]
	} else {
		resetStyle = lipgloss.NewStyle()
	}

	currentStyle = resetStyle
	lastEnd := 0

	// Helper to get ANSI codes from a style without the trailing reset
	getCodes := func(s lipgloss.Style) string {
		// Use Zero Width No-Break Space as marker to force code generation
		// without visible artifacts or layout impact
		marker := "\uFEFF"
		rendered := s.Render(marker)
		parts := strings.Split(rendered, marker)
		if len(parts) > 0 {
			return parts[0]
		}
		return ""
	}

	matches := themeTagRegex.FindAllStringSubmatchIndex(text, -1)
	for _, match := range matches {
		// Add text before this match with current style
		if match[0] > lastEnd {
			textBefore := text[lastEnd:match[0]]
			result.WriteString(getCodes(currentStyle) + textBefore)
		}

		// Parse the tag content
		tagContent := text[match[2]:match[3]]

		if tagContent == "|-|" || tagContent == "-" {
			// Reset style to default
			currentStyle = resetStyle
		} else if strings.HasPrefix(tagContent, "|") && strings.HasSuffix(tagContent, "|") {
			// Direct color code: {{|white:blue:b|}}
			colorCode := strings.Trim(tagContent, "|")
			currentStyle = ApplyTviewStyle(currentStyle, resetStyle, colorCode)
		} else if strings.HasPrefix(tagContent, "_") && strings.HasSuffix(tagContent, "_") {
			// Semantic tag: {{_ThemeHostname_}}
			translated := console.Translate("{{" + tagContent + "}}")
			// Match ALL tview tags in the definition (e.g. [-][black:green])
			if matches := cviewColorRegex.FindAllStringSubmatch(translated, -1); len(matches) > 0 {
				for j, m := range matches {
					// Special Handling: If the LAST tag in the definition is a REset [-],
					// and we have prior tags, we likely want to ignore this reset.
					if j == len(matches)-1 && len(matches) > 1 {
						if m[1] == "-" {
							continue
						}
					}
					currentStyle = ApplyTviewStyle(currentStyle, resetStyle, m[1])
				}
			}
		}

		lastEnd = match[1]
	}

	// Always ensure we have trailing text covered if no tags followed
	if lastEnd < len(text) {
		result.WriteString(getCodes(currentStyle) + text[lastEnd:])
	}

	// Terminate with a hard reset to prevent style bleeding (e.g. underline)
	// into subsequent UI elements
	result.WriteString("\x1b[0m")

	return result.String()
}

// ApplyTviewStyle updates an existing lipgloss.Style with tview-style color codes like "white:blue:b"
// currentStyle is the style to update, resetStyle is used for field resets (indicated by '-')
func ApplyTviewStyle(currentStyle lipgloss.Style, resetStyle lipgloss.Style, def string) lipgloss.Style {
	// Full reset to base style
	if def == "[-]" || def == "-" {
		return resetStyle
	}

	style := currentStyle

	// Remove brackets if present (from semantic definitions)
	if strings.HasPrefix(def, "[") && strings.HasSuffix(def, "]") {
		def = def[1 : len(def)-1]
	}

	parts := strings.Split(def, ":")
	if len(parts) == 0 {
		return style
	}

	// Foreground color
	if len(parts) > 0 && parts[0] != "" {
		if parts[0] == "-" {
			// Reset to default foreground
			style = style.Foreground(resetStyle.GetForeground())
		} else {
			c := ParseColor(parts[0])
			if c != nil {
				style = style.Foreground(c)
			}
		}
	}

	// Background color
	if len(parts) > 1 && parts[1] != "" {
		if parts[1] == "-" {
			// Reset to default background
			style = style.Background(resetStyle.GetBackground())
		} else {
			c := ParseColor(parts[1])
			if c != nil {
				style = style.Background(c)
			}
		}
	}

	// Styles (bold, underline, etc.)
	if len(parts) > 2 {
		for _, s := range parts[2:] {
			for _, char := range s {
				switch char {
				case 'B':
					style = style.Bold(true)
				case 'b':
					style = style.Bold(false)
				case 'U':
					style = style.Underline(true)
				case 'u':
					style = style.Underline(false)
				case 'I':
					style = style.Italic(true)
				case 'i':
					style = style.Italic(false)
				case 'D':
					style = style.Faint(true)
				case 'd':
					style = style.Faint(false)
				case 'L':
					style = style.Blink(true)
				case 'l':
					style = style.Blink(false)
				case 'R':
					style = style.Reverse(true)
				case 'r':
					style = style.Reverse(false)
				case 'S':
					style = style.Strikethrough(true)
				case 's':
					style = style.Strikethrough(false)
				}
			}
		}
	}

	return style
}

// ParseColor converts a color name or hex to lipgloss.TerminalColor
func ParseColor(name string) lipgloss.TerminalColor {
	if strings.HasPrefix(name, "#") {
		return lipgloss.Color(name)
	}

	// Map standard names to ANSI indices (0-15) for terminal theme consistency
	switch strings.ToLower(name) {
	case "black":
		return lipgloss.Color("0")
	case "red":
		return lipgloss.Color("1")
	case "green":
		return lipgloss.Color("2")
	case "yellow":
		return lipgloss.Color("3")
	case "blue":
		return lipgloss.Color("4")
	case "magenta", "purple":
		return lipgloss.Color("5")
	case "cyan":
		return lipgloss.Color("6")
	case "white", "gray", "grey", "silver":
		return lipgloss.Color("7")
	// Bright variants
	case "bright-black", "dark-gray":
		return lipgloss.Color("8")
	case "bright-red":
		return lipgloss.Color("9")
	case "bright-green":
		return lipgloss.Color("10")
	case "bright-yellow":
		return lipgloss.Color("11")
	case "bright-blue":
		return lipgloss.Color("12")
	case "bright-magenta":
		return lipgloss.Color("13")
	case "bright-cyan":
		return lipgloss.Color("14")
	case "bright-white":
		return lipgloss.Color("15")
	}

	// Fallback
	return lipgloss.Color(name)
}

// GetInitialStyle peeks at the first theme tag in text and returns a style derived from it.
// Useful for setting container backgrounds to match themed content.
func GetInitialStyle(text string, base lipgloss.Style) lipgloss.Style {
	match := themeTagRegex.FindStringSubmatch(text)
	if len(match) > 1 {
		tagContent := match[1]
		if strings.HasPrefix(tagContent, "_") && strings.HasSuffix(tagContent, "_") {
			translated := console.Translate("{{" + tagContent + "}}")
			// Match ALL tview tags in the definition (e.g. [-][black:green])
			if matches := cviewColorRegex.FindAllStringSubmatch(translated, -1); len(matches) > 0 {
				style := base
				for j, m := range matches {
					// Special Handling: If the LAST tag in the definition is a REset [-],
					// and we have prior tags, we likely want to ignore this reset.
					if j == len(matches)-1 && len(matches) > 1 {
						if m[1] == "-" {
							continue
						}
					}
					style = ApplyTviewStyle(style, base, m[1])
				}
				return style
			}
		} else if strings.HasPrefix(tagContent, "|") && strings.HasSuffix(tagContent, "|") {
			colorCode := strings.Trim(tagContent, "|")
			return ApplyTviewStyle(base, base, colorCode)
		}
	}
	return base
}

// MaintainBackground replaces ANSI resets (\x1b[0m) with a reset followed by the parent style's background.
// This prevents content-level resets from "bleeding" to the terminal default background.
func MaintainBackground(text string, style lipgloss.Style) string {
	bg := style.GetBackground()
	if bg == nil {
		return text
	}

	// Extract background code from a dummy render
	// We want the code that sets the background
	dummy := lipgloss.NewStyle().Background(bg).Render("T")
	parts := strings.Split(dummy, "T")
	if len(parts) == 0 {
		return text
	}
	bgCode := parts[0]

	// Replace \x1b[0m with \x1b[0m + bgCode
	return strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+bgCode)
}

// GetPlainText strips all {{...}} theme tags from text
func GetPlainText(text string) string {
	return themeTagRegex.ReplaceAllString(text, "")
}
