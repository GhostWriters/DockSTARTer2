package tui

import (
	"fmt"
	"regexp"
	"strings"

	"DockSTARTer2/internal/console"

	"github.com/charmbracelet/lipgloss"
)

// colorMap is deprecated in favor of ParseColor helper

// themeTagRegex matches {{_SymanticColor_}} or {{|codes|}} or {{|-|}}
var themeTagRegex = regexp.MustCompile(`\{\{(_[^}]+_|\|[^}]*\|)\}\}`)

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
			currentStyle = ApplyStyleCode(currentStyle, resetStyle, colorCode)
		} else if strings.HasPrefix(tagContent, "_") && strings.HasSuffix(tagContent, "_") {
			// Semantic tag: {{_ThemeHostname_}}
			translated := console.Translate("{{" + tagContent + "}}")
			// The translated value might contain multiple tags like {{|-|}}{{|blue|}}
			subMatches := themeTagRegex.FindAllStringSubmatch(translated, -1)
			for _, subMatch := range subMatches {
				subContent := subMatch[1]
				if subContent == "|-|" || subContent == "-" {
					currentStyle = resetStyle
				} else if strings.HasPrefix(subContent, "|") && strings.HasSuffix(subContent, "|") {
					code := strings.Trim(subContent, "|")
					currentStyle = ApplyStyleCode(currentStyle, resetStyle, code)
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

// ApplyStyleCode applies tview-style color codes (fg:bg:flags) to a lipgloss style
func ApplyStyleCode(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	// Full reset to base style
	if styleCode == "[-]" || styleCode == "-" {
		return resetStyle
	}

	parts := strings.Split(styleCode, ":")
	if len(parts) == 0 {
		return style
	}

	// Pre-emptive reset if flags start with '-'
	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		style = resetStyle
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
		s := strings.TrimPrefix(parts[2], "-")
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
			case 'H':
				// High intensity: if foreground/background are standard, shift them
				// (Shift logic will be handled by a dedicated helper or inline if simple)
				if fg := style.GetForeground(); fg != nil {
					style = style.Foreground(brightenColor(fg))
				}
				if bg := style.GetBackground(); bg != nil {
					style = style.Background(brightenColor(bg))
				}
			case 'h':
				// Normal intensity: if foreground/background are bright, shift them back
				if fg := style.GetForeground(); fg != nil {
					style = style.Foreground(dimColor(fg))
				}
				if bg := style.GetBackground(); bg != nil {
					style = style.Background(dimColor(bg))
				}
			}
		}
	}

	return style
}

// ApplyTagsToStyle translates any {{...}} tags and applies them to the given style
func ApplyTagsToStyle(text string, style lipgloss.Style, resetStyle lipgloss.Style) lipgloss.Style {
	translated := console.Translate(text)
	subMatches := themeTagRegex.FindAllStringSubmatch(translated, -1)
	for _, subMatch := range subMatches {
		subContent := subMatch[1]
		if subContent == "|-|" || subContent == "-" {
			style = resetStyle
		} else if strings.HasPrefix(subContent, "|") && strings.HasSuffix(subContent, "|") {
			code := strings.Trim(subContent, "|")
			style = ApplyStyleCode(style, resetStyle, code)
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

// brightenColor shifts standard ANSI colors (0-7) to bright variants (8-15)
func brightenColor(c lipgloss.TerminalColor) lipgloss.TerminalColor {
	if tc, ok := c.(lipgloss.Color); ok {
		s := string(tc)
		switch s {
		case "0", "1", "2", "3", "4", "5", "6", "7":
			idx := 0
			fmt.Sscanf(s, "%d", &idx)
			return lipgloss.Color(fmt.Sprintf("%d", idx+8))
		}
	}
	return c
}

// dimColor shifts bright ANSI colors (8-15) back to standard variants (0-7)
func dimColor(c lipgloss.TerminalColor) lipgloss.TerminalColor {
	if tc, ok := c.(lipgloss.Color); ok {
		s := string(tc)
		switch s {
		case "8", "9", "10", "11", "12", "13", "14", "15":
			idx := 0
			fmt.Sscanf(s, "%d", &idx)
			return lipgloss.Color(fmt.Sprintf("%d", idx-8))
		}
	}
	return c
}

// GetInitialStyle peeks at the first theme tag in text and returns a style derived from it.
// Useful for setting container backgrounds to match themed content.
func GetInitialStyle(text string, base lipgloss.Style) lipgloss.Style {
	match := themeTagRegex.FindStringSubmatch(text)
	if len(match) > 1 {
		tagContent := match[1]
		if strings.HasPrefix(tagContent, "_") && strings.HasSuffix(tagContent, "_") {
			translated := console.Translate("{{" + tagContent + "}}")
			// The translated value is now in {{|code|}} format
			if strings.HasPrefix(translated, "{{|") && strings.HasSuffix(translated, "|}}") {
				code := translated[3 : len(translated)-3]
				return ApplyStyleCode(base, base, code)
			}
		} else if strings.HasPrefix(tagContent, "|") && strings.HasSuffix(tagContent, "|") {
			colorCode := strings.Trim(tagContent, "|")
			return ApplyStyleCode(base, base, colorCode)
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
