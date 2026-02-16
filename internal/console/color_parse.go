package console

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ParseColor converts a color name or hex string to a color.Color.
// This uses tcell to resolve color names to hex values, ensuring proper color profile
// downgrading for terminals with limited color support.
//
// Standard ANSI Color Reference (for tcell/lipgloss mapping):
// black   = 0 (#000000)
// red     = 1 (#800000 / #ff0000)
// green   = 2 (#008000)
// yellow  = 3 (#808000 / #ffff00)
// blue    = 4 (#000080)
// magenta = 5 (#800080 / #ff00ff) (Aliased to Fuchsia)
// cyan    = 6 (#008080 / #00ffff) (Aliased to Aqua)
// white   = 7 (#c0c0c0)
func ParseColor(c string) color.Color {
	c = strings.ToLower(strings.TrimSpace(c))

	// 1. Hex codes - pass directly
	if strings.HasPrefix(c, "#") {
		return lipgloss.Color(c)
	}

	// 2. Try resolving with tcell (supports extended names and aliases)
	if hexVal := GetHexForColor(c); hexVal != "" {
		return lipgloss.Color(hexVal)
	}

	// 3. Fallback for numeric color codes or unknown colors
	return lipgloss.Color(c)
}

// GetColorStr extracts the string representation (hex or ANSI index) from a color.Color.
// Used for converting lipgloss colors back to tag format for console output.
func GetColorStr(c color.Color) string {
	if c == nil {
		return ""
	}

	// For lipgloss ANSI colors (0-255), return the index
	// lipgloss.ANSIColor implements String() which returns the index
	if s, ok := c.(fmt.Stringer); ok {
		str := s.String()
		// Check if it's a simple ANSI index (0-255)
		if len(str) > 0 && str[0] >= '0' && str[0] <= '9' {
			return str
		}
		// If it's a hex string, return as-is
		if strings.HasPrefix(str, "#") {
			return strings.ToLower(str)
		}
	}

	// For other color.Color types, convert to hex
	r, g, b, _ := c.RGBA()
	// RGBA returns 16-bit values, scale down to 8-bit
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
