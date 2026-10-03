// Package colorutil measures colors as a terminal draws them.
package colorutil

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// xtermColors are xterm's default 16 ANSI colors, closer to how most
// terminals draw them than the VGA values ansi.BasicColor reports (cyan
// #00cdcd, not #008080).
var xtermColors = [16][3]uint8{
	{0x00, 0x00, 0x00}, {0xcd, 0x00, 0x00}, {0x00, 0xcd, 0x00}, {0xcd, 0xcd, 0x00},
	{0x00, 0x00, 0xee}, {0xcd, 0x00, 0xcd}, {0x00, 0xcd, 0xcd}, {0xe5, 0xe5, 0xe5},
	{0x7f, 0x7f, 0x7f}, {0xff, 0x00, 0x00}, {0x00, 0xff, 0x00}, {0xff, 0xff, 0x00},
	{0x5c, 0x5c, 0xff}, {0xff, 0x00, 0xff}, {0x00, 0xff, 0xff}, {0xff, 0xff, 0xff},
}

// RGB returns c's red, green, and blue, 0-1, the 16 basic ANSI colors as
// xterm draws them; ok is false when c is no color (the terminal's default).
func RGB(c color.Color) (r, g, b float64, ok bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	if _, none := c.(lipgloss.NoColor); none {
		return 0, 0, 0, false
	}
	if basic, isBasic := c.(ansi.BasicColor); isBasic && int(basic) < len(xtermColors) {
		x := xtermColors[basic]
		return float64(x[0]) / 255, float64(x[1]) / 255, float64(x[2]) / 255, true
	}
	cr, cg, cb, ca := c.RGBA()
	if ca == 0 {
		return 0, 0, 0, false
	}
	return float64(cr) / 0xffff, float64(cg) / 0xffff, float64(cb) / 0xffff, true
}

// Luminance returns c's relative luminance, 0 (black) to 1 (white); ok is
// false when c is no color.
func Luminance(c color.Color) (l float64, ok bool) {
	r, g, b, ok := RGB(c)
	if !ok {
		return 0, false
	}
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b), true
}

// Contrast returns the contrast ratio between a and b, 1 (identical) to 21
// (black and white); ok is false when either is no color.
func Contrast(a, b color.Color) (ratio float64, ok bool) {
	la, okA := Luminance(a)
	lb, okB := Luminance(b)
	if !okA || !okB {
		return 0, false
	}
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05), true
}
