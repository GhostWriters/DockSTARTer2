package theme

import (
	"image/color"
	"strings"

	"github.com/GhostWriters/semstyle"
	semtheme "github.com/GhostWriters/semstyle/theme"
	"github.com/charmbracelet/x/ansi"
)

// Theme variants (see Variant).
const (
	VariantDark   = semtheme.VariantDark
	VariantLight  = semtheme.VariantLight
	VariantTinted = semtheme.VariantTinted
)

// Kinds of theme colors (see semtheme.ThemeFile.ColorInfo).
const (
	ColorsMonochrome     = semtheme.ColorsMonochrome
	ColorsSemiMonochrome = semtheme.ColorsSemiMonochrome
	ColorsMultiColor     = semtheme.ColorsMultiColor
	ColorsTinted         = semtheme.ColorsTinted
)

// Variant returns tf's variant: the one it declares, or VariantTinted when
// it names a tint slot (see semtheme.ThemeFile.Variant), otherwise
// VariantDark or VariantLight by how bright its Dialog background is.
// detected is true for anything it doesn't declare; "" when its Dialog
// background is the terminal's own.
func Variant(tf ThemeFile) (variant string, detected bool) {
	if v, d := tf.Variant(); v != "" {
		return v, d
	}
	resolved, err := semtheme.ResolveColors(tf)
	if err != nil {
		return "", false
	}
	// fg:bg:flags; Reverse draws the foreground as the background.
	parts := append(strings.SplitN(resolved["Dialog"], ":", 3), "", "", "")
	bg := parts[1]
	if strings.ContainsRune(parts[2], 'R') {
		bg = parts[0]
	}
	if bg == "" || bg == "-" {
		return "", false
	}
	c := semstyle.ToColor(bg)
	if basic, ok := c.(ansi.BasicColor); ok && int(basic) < len(xtermColors) {
		c = xtermColors[basic]
	}
	if c == nil {
		return "", false
	}
	r, g, b, _ := c.RGBA()
	// Perceived brightness, 0 to 0xffff.
	if 299*r+587*g+114*b < 1000*0x8000 {
		return VariantDark, true
	}
	return VariantLight, true
}

// xtermColors are xterm's default 16 ANSI colors, closer to how most
// terminals draw them than the VGA values ansi.BasicColor reports (cyan
// #00cdcd, not #008080).
var xtermColors = [16]color.Color{
	color.RGBA{0x00, 0x00, 0x00, 0xff}, color.RGBA{0xcd, 0x00, 0x00, 0xff},
	color.RGBA{0x00, 0xcd, 0x00, 0xff}, color.RGBA{0xcd, 0xcd, 0x00, 0xff},
	color.RGBA{0x00, 0x00, 0xee, 0xff}, color.RGBA{0xcd, 0x00, 0xcd, 0xff},
	color.RGBA{0x00, 0xcd, 0xcd, 0xff}, color.RGBA{0xe5, 0xe5, 0xe5, 0xff},
	color.RGBA{0x7f, 0x7f, 0x7f, 0xff}, color.RGBA{0xff, 0x00, 0x00, 0xff},
	color.RGBA{0x00, 0xff, 0x00, 0xff}, color.RGBA{0xff, 0xff, 0x00, 0xff},
	color.RGBA{0x5c, 0x5c, 0xff, 0xff}, color.RGBA{0xff, 0x00, 0xff, 0xff},
	color.RGBA{0x00, 0xff, 0xff, 0xff}, color.RGBA{0xff, 0xff, 0xff, 0xff},
}
