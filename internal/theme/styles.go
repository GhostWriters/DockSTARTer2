package theme

import (
	"DockSTARTer2/internal/console"
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ApplyTagsToStyle translates any {{...}} tags and applies them to the given style.
func ApplyTagsToStyle(text string, style lipgloss.Style, resetStyle lipgloss.Style) lipgloss.Style {
	translated := console.Translate(text)
	re := console.GetDelimitedRegex()
	subMatches := re.FindAllStringSubmatch(translated, -1)
	for _, subMatch := range subMatches {
		semantic := subMatch[1]
		direct := subMatch[2]

		if semantic != "" {
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

// ApplyStyleCode applies a fg:bg:flags style code to a lipgloss.Style.
func ApplyStyleCode(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	if styleCode == "~" {
		return lipgloss.NewStyle()
	}
	if styleCode == console.CodeReset || styleCode == "-" {
		return resetStyle
	}

	parts := strings.Split(styleCode, ":")
	if len(parts) == 0 {
		return style
	}

	if len(parts) > 2 && strings.HasPrefix(parts[2], "-") {
		style = ResetFlags(style)
	}

	if len(parts) > 0 && parts[0] != "" {
		switch parts[0] {
		case "~":
			style = style.Foreground(lipgloss.Color(""))
		case "-":
			style = style.Foreground(resetStyle.GetForeground())
		default:
			if c := ParseColor(parts[0]); c != nil {
				style = style.Foreground(c)
			}
		}
	}

	if len(parts) > 1 && parts[1] != "" {
		switch parts[1] {
		case "~":
			style = style.Background(lipgloss.Color(""))
		case "-":
			style = style.Background(resetStyle.GetBackground())
		default:
			if c := ParseColor(parts[1]); c != nil {
				style = style.Background(c)
			}
		}
	}

	if len(parts) > 2 {
		if strings.HasPrefix(parts[2], "-") {
			style = ResetFlags(style)
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
				if fg := style.GetForeground(); fg != nil {
					style = style.Foreground(BrightenColor(fg))
				}
				if bg := style.GetBackground(); bg != nil {
					style = style.Background(BrightenColor(bg))
				}
			}
		}
	}

	return style
}

// ParseColor converts a color name or hex string to a lipgloss-compatible color.Color.
// Named colors and hex strings are passed directly to lipgloss, which handles
// profile-aware downconversion. Only falls back to tcell for truly extended color names.
func ParseColor(name string) color.Color {
	return console.ParseColor(name)
}

// BrightenColor brightens a color by 30% of remaining headroom.
func BrightenColor(c color.Color) color.Color {
	if c == nil {
		return c
	}
	rr, gg, bb, _ := c.RGBA()
	r := int(rr >> 8)
	g := int(gg >> 8)
	b := int(bb >> 8)
	r = min(255, r+int(float64(255-r)*0.3))
	g = min(255, g+int(float64(255-g)*0.3))
	b = min(255, b+int(float64(255-b)*0.3))
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}
