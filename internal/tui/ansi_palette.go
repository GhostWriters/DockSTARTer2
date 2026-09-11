package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"

	semstyle "github.com/GhostWriters/semstyle"
	"go.yaml.in/yaml/v4"
)

// buildAnsiPaletteOSC returns an OSC 4 sequence overriding the ANSI palette
// slots resolved from colors (its own explicit fields layered over its
// SchemeFile, if set), or "" if none are set. Each value accepts anything
// semstyle.ToColor understands: hex, an ANSI name, or any broader color
// name tcell resolves.
//
// Sent once at session start (AppModel.Init) so a theme's named ANSI colors
// (which compile to plain ANSI SGR codes, not literal RGB) render as
// intended regardless of the connecting terminal's own default palette --
// most relevant for the web frontend, whose client has no user-configured
// palette the way a real local/SSH terminal does.
func buildAnsiPaletteOSC(ctx context.Context, colors config.AnsiColors) string {
	colors = resolveAnsiColors(ctx, colors)
	var b strings.Builder
	for i, v := range colors.Slots() {
		if v == "" {
			continue
		}
		r, g, bl, _ := semstyle.ToColor(v).RGBA()
		fmt.Fprintf(&b, ";%d;rgb:%02x/%02x/%02x", i, r>>8, g>>8, bl>>8)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\x1b]4" + b.String() + "\x07"
}

// resolveAnsiColors layers colors' own explicit fields over its SchemeFile
// (if set), so an individually-set field always wins over the scheme.
func resolveAnsiColors(ctx context.Context, colors config.AnsiColors) config.AnsiColors {
	if colors.SchemeFile == "" {
		return colors
	}
	scheme, err := loadBase16Scheme(colors.SchemeFile)
	if err != nil {
		logger.Warn(ctx, "ansi_palette: could not load scheme_file %q: %v", colors.SchemeFile, err)
		return colors
	}
	return colors.WithDefaults(scheme)
}

// base16Palette mirrors the palette fields of a tinted-theming base16
// scheme YAML file (https://github.com/tinted-theming/schemes, spec-0.11).
type base16Palette struct {
	Base00 string `yaml:"base00"`
	Base01 string `yaml:"base01"`
	Base02 string `yaml:"base02"`
	Base03 string `yaml:"base03"`
	Base04 string `yaml:"base04"`
	Base05 string `yaml:"base05"`
	Base06 string `yaml:"base06"`
	Base07 string `yaml:"base07"`
	Base08 string `yaml:"base08"`
	Base09 string `yaml:"base09"`
	Base0A string `yaml:"base0A"`
	Base0B string `yaml:"base0B"`
	Base0C string `yaml:"base0C"`
	Base0D string `yaml:"base0D"`
	Base0E string `yaml:"base0E"`
	Base0F string `yaml:"base0F"`
}

type base16Scheme struct {
	Palette base16Palette `yaml:"palette"`
}

// loadBase16Scheme reads and parses a tinted-theming base16 scheme YAML
// file, mapping it onto the standard 16 ANSI terminal slots per
// tinted-theming/home's documented terminal mapping
// (https://github.com/tinted-theming/home/blob/main/styling.md):
// normal 0-7 = base00/08/0B/0A/0D/0E/0C/05, bright 8-15 =
// base03/08/0B/0A/0D/0E/0C/07.
func loadBase16Scheme(path string) (config.AnsiColors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.AnsiColors{}, err
	}
	var scheme base16Scheme
	if err := yaml.Unmarshal(data, &scheme); err != nil {
		return config.AnsiColors{}, err
	}
	p := scheme.Palette
	return config.AnsiColors{
		Black:         p.Base00,
		Red:           p.Base08,
		Green:         p.Base0B,
		Yellow:        p.Base0A,
		Blue:          p.Base0D,
		Magenta:       p.Base0E,
		Cyan:          p.Base0C,
		White:         p.Base05,
		BrightBlack:   p.Base03,
		BrightRed:     p.Base08,
		BrightGreen:   p.Base0B,
		BrightYellow:  p.Base0A,
		BrightBlue:    p.Base0D,
		BrightMagenta: p.Base0E,
		BrightCyan:    p.Base0C,
		BrightWhite:   p.Base07,
	}, nil
}
