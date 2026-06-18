package theme

// Style conversion utilities — implemented in semstyle/theme, re-exported here
// so internal/tui and other DS2 packages that import internal/theme keep working.

import (
	"image/color"

	semtheme "DockSTARTer2/internal/semstyle/theme"
	"DockSTARTer2/internal/semstyle"
	"charm.land/lipgloss/v2"
)

// StyleFlags holds ANSI style modifier state.
type StyleFlags = semtheme.StyleFlags

// ResetFlags clears all text attributes from a lipgloss style.
var ResetFlags = semtheme.ResetFlags

// ToStyle resolves semantic/direct tags in text and applies the result to style.
// Uses the Default semstyle Styler.
func ToStyle(text string, style lipgloss.Style, resetStyle lipgloss.Style) lipgloss.Style {
	return semtheme.ToStyle(semstyle.Default, text, style, resetStyle)
}

// ToStyleCode applies a raw fg:bg:flags code to a lipgloss.Style.
func ToStyleCode(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	return semtheme.ToStyleCode(styleCode, style, resetStyle)
}

// ToStyleFlags parses the flags field of a raw style code into a StyleFlags struct.
func ToStyleFlags(rawCode string) StyleFlags {
	return semtheme.ToStyleFlags(rawCode)
}

// ParseColor converts a color name or hex string to a color.Color.
func ParseColor(name string) color.Color {
	return semstyle.ToColor(name)
}

// BrightenColor brightens a color by 30% of remaining headroom toward white.
func BrightenColor(c color.Color) color.Color {
	return semtheme.BrightenColor(c)
}
