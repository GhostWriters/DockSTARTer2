package config

import (
	"os"

	"go.yaml.in/yaml/v4"
)

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

// ParseBase16Scheme parses a tinted-theming base16 scheme YAML file's
// contents, mapping it onto the standard 16 ANSI terminal slots per
// tinted-theming/home's documented terminal mapping
// (https://github.com/tinted-theming/home/blob/main/styling.md):
// normal 0-7 = base00/08/0B/0A/0D/0E/0C/05, bright 8-15 =
// base03/08/0B/0A/0D/0E/0C/07.
func ParseBase16Scheme(data []byte) (AnsiColors, error) {
	var scheme base16Scheme
	if err := yaml.Unmarshal(data, &scheme); err != nil {
		return AnsiColors{}, err
	}
	p := scheme.Palette
	return AnsiColors{
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

// LoadBase16Scheme reads and parses a tinted-theming base16 scheme YAML
// file from path. See ParseBase16Scheme for the mapping used.
func LoadBase16Scheme(path string) (AnsiColors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AnsiColors{}, err
	}
	return ParseBase16Scheme(data)
}
