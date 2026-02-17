package tui

import (
	"fmt"
	"image/color"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

var semanticStyleCache = make(map[string]lipgloss.Style)

// SemanticStyle translates a semantic color tag (e.g., {{|Theme_Title|}}) or color code
// (e.g., {{[black:white:-B]}}) into a lipgloss.Style.
func SemanticStyle(tag string) lipgloss.Style {
	if s, ok := semanticStyleCache[tag]; ok {
		return s
	}

	// If it's a semantic tag, we can try resolving it raw first for efficiency
	if strings.HasPrefix(tag, console.SemanticPrefix) && strings.HasSuffix(tag, console.SemanticSuffix) {
		name := tag[len(console.SemanticPrefix) : len(tag)-len(console.SemanticSuffix)]
		return SemanticRawStyle(name)
	}

	s := ApplyTagsToStyle(tag, lipgloss.NewStyle(), lipgloss.NewStyle())
	semanticStyleCache[tag] = s
	return s
}

// SemanticRawStyle translates a raw semantic name (e.g., "Theme_Title") into a lipgloss.Style.
func SemanticRawStyle(name string) lipgloss.Style {
	cacheKey := "raw:" + name
	if s, ok := semanticStyleCache[cacheKey]; ok {
		return s
	}

	def := console.GetColorDefinition(name)
	s := ApplyTagsToStyle(def, lipgloss.NewStyle(), lipgloss.NewStyle())
	semanticStyleCache[cacheKey] = s
	return s
}

// Color parsing now uses tcell/v3/colors for RGB conversion via console.GetHexForColor().
// This ensures all colors are resolved to RGB/hex values, allowing proper color profile
// downgrading for terminals with limited color support.

// themeTagRegex matches any tag using current delimiters
var themeTagRegex = console.GetDelimitedRegex()

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

	// Ensure the starting text has the correct background/foreground
	getCodes := func(s lipgloss.Style) string {
		rendered := s.Render("_")
		return strings.Split(rendered, "_")[0]
	}

	// Resolve tags to ANSI
	rendered := console.ToANSI(text)

	// Combine components and ensure reset at end
	result := getCodes(resetStyle) + rendered + console.CodeReset

	// Prevent embedded resets from clearing container background
	return MaintainBackground(result, resetStyle)
}

// ApplyStyleCode applies tview-style color codes (fg:bg:flags) to a lipgloss style
func ApplyStyleCode(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	// Full reset to base style
	if styleCode == console.CodeReset || styleCode == "-" {
		return resetStyle
	}

	parts := strings.Split(styleCode, ":")
	if len(parts) == 0 {
		return style
	}

	// Pre-emptive reset of flags ONLY if they start with '-'
	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		style = theme.ResetFlags(style)
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
			style = theme.ResetFlags(style)
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
				// High intensity OFF (colors remain at base level)
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
		semantic := subMatch[1]
		direct := subMatch[2]

		if semantic != "" {
			// This branch is rare after Translate, but good for robustness
			tagName := strings.Trim(semantic, "_")
			def := console.GetColorDefinition(tagName)
			style = ApplyTagsToStyle(def, style, resetStyle)
		} else if direct != "" {
			if direct == "|" || direct == "-" {
				style = resetStyle
			} else {
				code := strings.Trim(direct, "|")
				style = ApplyStyleCode(style, resetStyle, code)
			}
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
	if len(match) > 2 {
		semantic := match[1]
		direct := match[2]

		if semantic != "" {
			tagContent := semantic
			translated := console.Translate(console.WrapSemantic(tagContent))
			// Recurse into translated (recursive check)
			return GetInitialStyle(translated, base)
		} else if direct != "" {
			colorCode := strings.Trim(direct, "|")
			return ApplyStyleCode(base, base, colorCode)
		}
	}
	return base
}

// MaintainBackground replaces ANSI resets (\x1b[0m) with a reset followed by the parent style's background.
// This prevents content-level resets from "bleeding" to the terminal default background.
func MaintainBackground(text string, style lipgloss.Style) string {
	// Extract foreground and background codes from dummy renders
	getANSI := func(s lipgloss.Style) (fgCode, bgCode string) {
		rendered := s.Render("_")
		// The rendered string is: [fg][bg][attr]_ [reset]
		// We want to find the codes before the "_"
		parts := strings.Split(rendered, "_")
		if len(parts) > 0 {
			// This is an oversimplification but usually works for simple styles
			// In lipgloss, FG and BG are separate sequences
			if fg := s.GetForeground(); fg != nil {
				fgDummy := lipgloss.NewStyle().Foreground(fg).Render("_")
				fgCode = strings.Split(fgDummy, "_")[0]
			}
			if bg := s.GetBackground(); bg != nil {
				bgDummy := lipgloss.NewStyle().Background(bg).Render("_")
				bgCode = strings.Split(bgDummy, "_")[0]
			}
		}
		return
	}

	fgCode, bgCode := getANSI(style)

	// Replace \x1b[0m with \x1b[0m + all codes
	if bgCode != "" || fgCode != "" {
		text = strings.ReplaceAll(text, console.CodeReset, console.CodeReset+fgCode+bgCode)
	}

	// Also replace explicit FG/BG resets
	if fgCode != "" {
		text = strings.ReplaceAll(text, console.CodeFGReset, console.CodeFGReset+fgCode)
	}
	if bgCode != "" {
		text = strings.ReplaceAll(text, console.CodeBGReset, console.CodeBGReset+bgCode)
	}

	return text
}

// GetPlainText strips all {{...}} theme tags from text
func GetPlainText(text string) string {
	return themeTagRegex.ReplaceAllString(text, "")
}
