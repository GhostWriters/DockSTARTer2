package config

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v4"
	"golang.org/x/text/unicode/norm"
)

// base16Palette mirrors the 16 base0X color fields shared by both base16
// scheme formats -- see base16Scheme (current, spec-0.11) and
// base16LegacyScheme (original spec) below. Also carries base24's 8
// extension fields (base10-base17, https://github.com/tinted-theming/base24/
// blob/main/styling.md) -- empty for a genuinely base16 source, since only
// the current (non-legacy) nested format is documented to ever carry them.
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

	// base24 extension fields -- base10/base11 (darker backgrounds) have no
	// ANSI terminal assignment and aren't mapped onto AnsiColors; the other
	// 6 give red/yellow/green/cyan/blue/magenta a real, distinct bright
	// color instead of base16's fallback of reusing the normal one.
	Base10 string `yaml:"base10"`
	Base11 string `yaml:"base11"`
	Base12 string `yaml:"base12"`
	Base13 string `yaml:"base13"`
	Base14 string `yaml:"base14"`
	Base15 string `yaml:"base15"`
	Base16 string `yaml:"base16"`
	Base17 string `yaml:"base17"`
}

// isZero reports whether every field is empty -- used to detect that a
// document didn't actually match this shape (rather than genuinely setting
// every color to "").
func (p base16Palette) isZero() bool {
	return p == base16Palette{}
}

// base16Scheme is the current tinted-theming (spec-0.11) format: colors
// nested under a top-level "palette:" key, hex values "#rrggbb"
// (https://github.com/tinted-theming/home/blob/main/styling.md).
type base16Scheme struct {
	System  string        `yaml:"system"`
	Name    string        `yaml:"name"`
	Slug    string        `yaml:"slug"`
	Author  string        `yaml:"author"`
	Variant string        `yaml:"variant"`
	Palette base16Palette `yaml:"palette"`
}

// base16LegacyScheme is the original base16 spec's format: base00-base0F
// at the document's root (no "palette:" wrapper), hex values without a "#"
// prefix (e.g. "0f1419"), and a "slug" field instead of "system"/"variant".
// Still in circulation from base16's pre-tinted-theming era.
type base16LegacyScheme struct {
	Name          string `yaml:"name"`
	Slug          string `yaml:"slug"`
	Author        string `yaml:"author"`
	base16Palette `yaml:",inline"`
}

// hexNoHash matches a bare 6-digit hex color with no "#" prefix, as used by
// the legacy base16 format.
var hexNoHash = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// normalizeHex adds a "#" prefix to a bare 6-digit hex value (the legacy
// format's convention) so it resolves the same way as the current format's
// "#rrggbb" values downstream (semstyle.ToColor etc.). Anything else
// (already "#"-prefixed, an ANSI/tcell color name, empty) passes through
// unchanged.
func normalizeHex(v string) string {
	if hexNoHash.MatchString(v) {
		return "#" + v
	}
	return v
}

// parseBase16Palette tries the current nested format first, then falls
// back to the legacy flat format if that comes back with every slot empty
// (i.e. the document didn't actually match that shape) -- so both are
// recognized transparently by every caller. Legacy hex values are
// normalized to "#rrggbb" via normalizeHex. Returns an error if data isn't
// valid YAML, or an empty palette if neither shape matched anything
// (callers treat that as "not a valid base16 scheme").
func parseBase16Palette(data []byte) (base16Palette, error) {
	var current base16Scheme
	if err := yaml.Unmarshal(data, &current); err != nil {
		return base16Palette{}, err
	}
	if !current.Palette.isZero() {
		return current.Palette, nil
	}

	var legacy base16LegacyScheme
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return base16Palette{}, err
	}
	if legacy.isZero() {
		return base16Palette{}, nil
	}
	p := legacy.base16Palette
	return base16Palette{
		Base00: normalizeHex(p.Base00), Base01: normalizeHex(p.Base01),
		Base02: normalizeHex(p.Base02), Base03: normalizeHex(p.Base03),
		Base04: normalizeHex(p.Base04), Base05: normalizeHex(p.Base05),
		Base06: normalizeHex(p.Base06), Base07: normalizeHex(p.Base07),
		Base08: normalizeHex(p.Base08), Base09: normalizeHex(p.Base09),
		Base0A: normalizeHex(p.Base0A), Base0B: normalizeHex(p.Base0B),
		Base0C: normalizeHex(p.Base0C), Base0D: normalizeHex(p.Base0D),
		Base0E: normalizeHex(p.Base0E), Base0F: normalizeHex(p.Base0F),
	}, nil
}

// Base16SchemeMeta holds a base16 scheme's own descriptive metadata (not
// its colors) -- name/author/variant as the scheme's own file declares
// them (variant is empty for a legacy-format file, which has no such
// field), for display purposes (see --tint's status display). Not
// necessarily the same as the name in a "repo:<name>" Tint reference (a
// scheme's own "name" field is free text, e.g. "Default Dark", not always
// the file's slug).
type Base16SchemeMeta struct {
	Name    string
	Slug    string
	Author  string
	Variant string
}

// ParseBase16SchemeMeta parses just a base16 scheme YAML file's descriptive
// metadata, ignoring its palette. Recognizes both the current and legacy
// formats (see parseBase16Palette). Slug is the file's own "slug" field if
// it has one, else Slugify(Name) -- per tinted-theming/home's builder.md:
// "If it is not provided, a builder MUST infer it by slugifying the
// scheme's name."
func ParseBase16SchemeMeta(data []byte) (Base16SchemeMeta, error) {
	var current base16Scheme
	if err := yaml.Unmarshal(data, &current); err != nil {
		return Base16SchemeMeta{}, err
	}
	if current.Name != "" || current.Author != "" || current.Variant != "" {
		slug := current.Slug
		if slug == "" {
			slug = Slugify(current.Name)
		}
		return Base16SchemeMeta{Name: current.Name, Slug: slug, Author: current.Author, Variant: current.Variant}, nil
	}

	var legacy base16LegacyScheme
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return Base16SchemeMeta{}, err
	}
	name := legacy.Name
	slug := legacy.Slug
	if name == "" {
		name = slug
	}
	if slug == "" {
		slug = Slugify(name)
	}
	return Base16SchemeMeta{Name: name, Slug: slug, Author: legacy.Author}, nil
}

// Slugify implements tinted-theming/home's documented slugify algorithm
// (https://github.com/tinted-theming/home/blob/main/builder.md#slugify),
// used to derive a scheme's slug from its display name when the file
// doesn't declare one explicitly:
//  1. Unicode-normalize to NFD and drop combining marks (e.g. "é" -> "e").
//  2. Lowercase.
//  3. Replace spaces with "-".
//  4. Drop every character that's neither alphanumeric nor "-".
//
// e.g. Slugify("Default (Dark)") == "default-dark".
func Slugify(name string) string {
	decomposed := norm.NFD.String(name)
	var noMarks strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		noMarks.WriteRune(r)
	}

	lower := strings.ToLower(noMarks.String())
	dashed := strings.ReplaceAll(lower, " ", "-")

	var out strings.Builder
	for _, r := range dashed {
		if r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// firstNonEmpty returns a, or b if a is empty.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ParseBase16Scheme parses a tinted-theming base16 or base24 scheme YAML
// file's contents -- current (spec-0.11, nested under "palette:") or legacy
// (original spec, flat at the document root, unprefixed hex) format,
// whichever matches (see parseBase16Palette) -- mapping it onto the standard
// 16 ANSI terminal slots. Normal colors and the black/white bright pair
// follow tinted-theming/home's documented terminal mapping (base00/08/0B/0A
// /0D/0E/0C/05 normal, base03/07 bright black/white); the other 6 bright
// colors use base24's own dedicated slots (base12/13/14/15/16/17) when the
// source scheme sets them, falling back to base24's own documented
// base16-compatibility table (https://github.com/tinted-theming/base24/blob
// /main/styling.md#base24-fallbacks) -- reusing the matching normal
// color -- for a base16-only source, exactly as before this fallback
// existed.
func ParseBase16Scheme(data []byte) (AnsiColors, error) {
	p, err := parseBase16Palette(data)
	if err != nil {
		return AnsiColors{}, err
	}
	if p.isZero() {
		return AnsiColors{}, fmt.Errorf("no base16 palette found (neither the current \"palette:\"-nested format nor the legacy flat format matched)")
	}
	return AnsiColors{
		Base00: p.Base00,
		Base08: p.Base08,
		Base0B: p.Base0B,
		Base0A: p.Base0A,
		Base0D: p.Base0D,
		Base0E: p.Base0E,
		Base0C: p.Base0C,
		Base05: p.Base05,
		Base03: p.Base03,
		Base12: firstNonEmpty(p.Base12, p.Base08),
		Base14: firstNonEmpty(p.Base14, p.Base0B),
		Base13: firstNonEmpty(p.Base13, p.Base0A),
		Base16: firstNonEmpty(p.Base16, p.Base0D),
		Base17: firstNonEmpty(p.Base17, p.Base0E),
		Base15: firstNonEmpty(p.Base15, p.Base0C),
		Base07: p.Base07,

		// No ANSI terminal assignment -- stored as-is (possibly empty) so an
		// explicit value survives; a tint derives a stand-in when empty (see
		// semstyle.Palette.slot's fallback logic).
		Base01: p.Base01,
		Base02: p.Base02,
		Base04: p.Base04,
		Base06: p.Base06,
		Base09: p.Base09,
		Base0F: p.Base0F,
		Base10: p.Base10,
		Base11: p.Base11,
	}, nil
}
