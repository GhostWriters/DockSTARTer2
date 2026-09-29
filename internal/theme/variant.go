package theme

import (
	"strings"

	"DockSTARTer2/internal/colorutil"

	"github.com/GhostWriters/semstyle"
	semtheme "github.com/GhostWriters/semstyle/theme"
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
	r, g, b, ok := colorutil.RGB(semstyle.ToColor(bg))
	if !ok {
		return "", false
	}
	// Perceived brightness.
	if 0.299*r+0.587*g+0.114*b < 0.5 {
		return VariantDark, true
	}
	return VariantLight, true
}
