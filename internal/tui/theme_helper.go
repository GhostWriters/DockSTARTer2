package tui

import (
	"regexp"
	"strings"

	"DockSTARTer2/internal/console"

	"github.com/charmbracelet/lipgloss"
)

// colorMap maps cview color names to hex codes
var colorMap = map[string]string{
	"black":   "#000000",
	"maroon":  "#800000",
	"red":     "#ff0000",
	"green":   "#008000",
	"lime":    "#00ff00",
	"yellow":  "#ffff00",
	"olive":   "#808000",
	"navy":    "#000080",
	"blue":    "#0000ff",
	"purple":  "#800080",
	"magenta": "#ff00ff",
	"fuchsia": "#ff00ff",
	"teal":    "#008080",
	"cyan":    "#00ffff",
	"aqua":    "#00ffff",
	"silver":  "#c0c0c0",
	"gray":    "#808080",
	"grey":    "#808080",
	"white":   "#ffffff",
}

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

	matches := themeTagRegex.FindAllStringSubmatchIndex(text, -1)
	for _, match := range matches {
		// Add text before this match with current style
		if match[0] > lastEnd {
			textBefore := text[lastEnd:match[0]]
			result.WriteString(currentStyle.Render(textBefore))
		}

		// Parse the tag content
		tagContent := text[match[2]:match[3]]

		if tagContent == "|-|" || tagContent == "-" {
			// Reset style to default
			currentStyle = resetStyle
		} else if strings.HasPrefix(tagContent, "|") && strings.HasSuffix(tagContent, "|") {
			// Direct color code: {{|white:blue:b|}}
			colorCode := strings.Trim(tagContent, "|")
			currentStyle = parseColorCode(colorCode)
		} else if strings.HasPrefix(tagContent, "_") && strings.HasSuffix(tagContent, "_") {
			// Semantic tag: {{_ThemeHostname_}}
			// Translate to get the cview format, then extract color code
			translated := console.Translate("{{" + tagContent + "}}")
			// Extract color code from [code] format
			if cviewMatch := cviewColorRegex.FindStringSubmatch(translated); len(cviewMatch) > 1 {
				currentStyle = parseColorCode(cviewMatch[1])
			}
		}

		lastEnd = match[1]
	}

	// Add remaining text
	if lastEnd < len(text) {
		result.WriteString(currentStyle.Render(text[lastEnd:]))
	}

	return result.String()
}

// parseColorCode converts cview color codes like "white:blue:b" to lipgloss style
func parseColorCode(code string) lipgloss.Style {
	style := lipgloss.NewStyle()

	parts := strings.Split(code, ":")
	if len(parts) == 0 {
		return style
	}

	// Foreground color
	if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
		if hex, ok := colorMap[strings.ToLower(parts[0])]; ok {
			style = style.Foreground(lipgloss.Color(hex))
		} else if strings.HasPrefix(parts[0], "#") {
			style = style.Foreground(lipgloss.Color(parts[0]))
		}
	}

	// Background color
	if len(parts) > 1 && parts[1] != "" && parts[1] != "-" {
		if hex, ok := colorMap[strings.ToLower(parts[1])]; ok {
			style = style.Background(lipgloss.Color(hex))
		} else if strings.HasPrefix(parts[1], "#") {
			style = style.Background(lipgloss.Color(parts[1]))
		}
	}

	// Styles (bold, underline, etc.)
	if len(parts) > 2 {
		for _, s := range parts[2:] {
			switch strings.ToLower(s) {
			case "b", "bold":
				style = style.Bold(true)
			case "u", "underline":
				style = style.Underline(true)
			case "i", "italic":
				style = style.Italic(true)
			case "r", "reverse":
				// Swap foreground and background
				fg := style.GetForeground()
				bg := style.GetBackground()
				style = style.Foreground(bg).Background(fg)
			}
		}
	}

	return style
}

// GetPlainText strips all {{...}} theme tags from text
func GetPlainText(text string) string {
	return themeTagRegex.ReplaceAllString(text, "")
}
