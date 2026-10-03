package commands

import (
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"

	"github.com/pelletier/go-toml/v2"
)

// TintHueNames are the hue tags a scheme can carry (see TintEntry.Hues), in
// the order a picker lists them.
var TintHueNames = []string{"gray", "red", "orange", "yellow", "green", "cyan", "blue", "purple", "magenta", "pink"}

// Kinds of scheme colors (see TintEntry.Kind).
const (
	TintKindMonochrome = "monochrome"
	TintKindMultiColor = "multi-color"
)

// tintHueBuckets are the upper hue bounds, in degrees, of each non-gray hue.
var tintHueBuckets = []struct {
	top  float64
	name string
}{{15, "red"}, {40, "orange"}, {65, "yellow"}, {160, "green"}, {195, "cyan"},
	{235, "blue"}, {285, "purple"}, {320, "magenta"}, {345, "pink"}, {360, "red"}}

const (
	// tintGrayChroma and tintTintedChroma bound a background's chroma (0-1):
	// below the first it's gray only, below the second gray with a lean.
	tintGrayChroma   = 0.03
	tintTintedChroma = 0.08
	// tintHueOverlap is how near, in degrees, a lean counts for the next hue
	// too.
	tintHueOverlap = 12
	// tintAccentChroma is the chroma an accent needs to count as a hue.
	tintAccentChroma = 0.12
)

// tintHues returns a scheme's hue tags and kind: its .HUES.toml entry's
// hues when it has one, else the ones its background leans to (see
// backgroundHues); monochrome when its accents share one hue.
func tintHues(slug string, data []byte) (hues []string, kind string) {
	colors, err := config.ParseBase16Scheme(data)
	if err != nil {
		return nil, ""
	}
	kind = TintKindMultiColor
	var accentHues []string
	for _, hex := range []string{colors.Base08, colors.Base09, colors.Base0A, colors.Base0B, colors.Base0C, colors.Base0D, colors.Base0E} {
		if r, g, b, ok := parseTintHex(hex); ok && chroma(r, g, b) >= tintAccentChroma {
			if h := hueBucket(hue(r, g, b)); !slices.Contains(accentHues, h) {
				accentHues = append(accentHues, h)
			}
		}
	}
	if len(accentHues) <= 1 {
		kind = TintKindMonochrome
	}
	if listed, ok := listedTintHues(slug); ok {
		return listed, kind
	}
	return backgroundHues(colors.Base00, colors.Base01), kind
}

// backgroundHues returns the hues a background leans to, base00 weighted
// three to one with base01: gray when it's muted, the hue it leans to
// unless it's nearly gray, and the next hue too when that's near.
func backgroundHues(base00, base01 string) []string {
	var r, g, b, weight float64
	for _, c := range []struct {
		hex    string
		weight float64
	}{{base00, 3}, {base01, 1}} {
		if cr, cg, cb, ok := parseTintHex(c.hex); ok {
			r, g, b, weight = r+cr*c.weight, g+cg*c.weight, b+cb*c.weight, weight+c.weight
		}
	}
	if weight == 0 {
		return nil
	}
	r, g, b = r/weight, g/weight, b/weight
	c := chroma(r, g, b)
	var hues []string
	if c < tintTintedChroma {
		hues = append(hues, "gray")
	}
	if c < tintGrayChroma {
		return hues
	}
	h := hue(r, g, b)
	hues = append(hues, hueBucket(h))
	for _, near := range []float64{h - tintHueOverlap, h + tintHueOverlap} {
		if n := hueBucket(math.Mod(near+360, 360)); !slices.Contains(hues, n) {
			hues = append(hues, n)
		}
	}
	return hues
}

func hueBucket(h float64) string {
	for _, b := range tintHueBuckets {
		if h < b.top {
			return b.name
		}
	}
	return "red"
}

func chroma(r, g, b float64) float64 { return max(r, g, b) - min(r, g, b) }

// hue returns r, g, b's hue in degrees.
func hue(r, g, b float64) float64 {
	hi, d := max(r, g, b), chroma(r, g, b)
	if d == 0 {
		return 0
	}
	var h float64
	switch hi {
	case r:
		h = 60 * math.Mod((g-b)/d, 6)
	case g:
		h = 60 * ((b-r)/d + 2)
	default:
		h = 60 * ((r-g)/d + 4)
	}
	return math.Mod(h+360, 360)
}

// parseTintHex parses "#rrggbb" or "rrggbb" into 0-1 channels.
func parseTintHex(s string) (r, g, b float64, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, true
}

var (
	tintHueListOnce sync.Once
	tintHueList     map[string][]string
)

// listedTintHues returns slug's hues from the embedded .HUES.toml: an
// exact key's, else the longest matching pattern's.
func listedTintHues(slug string) ([]string, bool) {
	tintHueListOnce.Do(func() {
		var f struct {
			Hues map[string][]string `toml:"hues"`
		}
		if data, err := assets.GetTintHues(); err == nil && toml.Unmarshal(data, &f) == nil {
			tintHueList = f.Hues
		}
	})
	if hues, ok := tintHueList[slug]; ok {
		return hues, true
	}
	best := ""
	for pattern := range tintHueList {
		if ok, _ := path.Match(pattern, slug); ok && len(pattern) > len(best) {
			best = pattern
		}
	}
	if best == "" {
		return nil, false
	}
	return tintHueList[best], true
}
