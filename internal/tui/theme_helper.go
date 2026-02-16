package tui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"

	"DockSTARTer2/internal/console"

	"charm.land/lipgloss/v2"
)

// Color parsing now uses tcell/v3/colors for RGB conversion via console.GetHexForColor().
// This ensures all colors are resolved to RGB/hex values, allowing proper color profile
// downgrading for terminals with limited color support.

// themeTagRegex matches {{_SymanticColor_}} or {{|codes|}} or {{|-|}}
var themeTagRegex = regexp.MustCompile(`\{\{(_[^}]+_|\|[^}]*\|)\}\}`)

// RenderThemeText takes text with {{...}} theme tags and returns lipgloss-styled text
// defaultStyle is used for reset state and unstyled text
func RenderThemeText(text string, defaultStyle ...lipgloss.Style) string {
	var resetStyle lipgloss.Style

	// Use provided default style or blank style
	if len(defaultStyle) > 0 {
		resetStyle = defaultStyle[0]
	} else {
		resetStyle = lipgloss.NewStyle()
	}

	// 1. Get initial codes for the default style
	// This ensures the starting text has the correct background/foreground
	getCodes := func(s lipgloss.Style) string {
		rendered := s.Render("_")
		return strings.Split(rendered, "_")[0]
	}

	// 2. Convert all tags to ANSI using standard console logic
	// This ensures {{_Tag_}} and {{|code|}} behave exactly as in terminal logs
	rendered := console.ToANSI(text)

	// 3. Prepend default style codes and apply background maintenance
	// We wrap the text in a reset at the end too.
	result := getCodes(resetStyle) + rendered + "\x1b[0m"

	// 4. Ensure resets within the text don't kill the container's background
	return MaintainBackground(result, resetStyle)
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

	// Pre-emptive reset of flags ONLY if they start with '-'
	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		style = style.
			Bold(false).
			Underline(false).
			Italic(false).
			Faint(false).
			Blink(false).
			Reverse(false).
			Strikethrough(false)
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
		if strings.HasPrefix(parts[2], "-") {
			// Reset all supported flags before applying new ones
			style = style.Bold(false).
				Underline(false).
				Italic(false).
				Faint(false).
				Blink(false).
				Reverse(false).
				Strikethrough(false)
		}
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
				// High intensity ON: brighten the color
				if fg := style.GetForeground(); fg != nil {
					style = style.Foreground(brightenColor(fg))
				}
				if bg := style.GetBackground(); bg != nil {
					style = style.Background(brightenColor(bg))
				}
			case 'h':
				// High intensity OFF: do nothing (colors remain at base level)
				// Note: Cannot undo previous 'H' in sequential processing
				// If dimming is needed, use 'D' flag for ANSI dim attribute
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

// ParseColor is a wrapper around console.ParseColor for TUI use
func ParseColor(name string) color.Color {
	return console.ParseColor(name)
}

// brightenColor attempts to brighten a color by adding 30% of remaining headroom.
// Used by 'H' flag for high intensity ON.
// Works by extracting RGBA values and brightening them mathematically.
func brightenColor(c color.Color) color.Color {
	if c == nil {
		return c
	}

	// Extract RGBA values (returns 0-65535 range)
	rr, gg, bb, _ := c.RGBA()
	// Convert to 0-255 range
	r := int(rr >> 8)
	g := int(gg >> 8)
	b := int(bb >> 8)

	// Brighten by 30% of remaining headroom (capped at 255)
	r = min(255, r+int(float64(255-r)*0.3))
	g = min(255, g+int(float64(255-g)*0.3))
	b = min(255, b+int(float64(255-b)*0.3))

	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
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
