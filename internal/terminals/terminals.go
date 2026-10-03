// Package terminals identifies a terminal from its XTVERSION or DA1 reply,
// using the embedded terminals.toml table.
package terminals

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/colorprofile"
	"github.com/pelletier/go-toml/v2"

	"DockSTARTer2/internal/assets"
)

// Entry is one terminals.toml [[terminal]] entry.
type Entry struct {
	Name       string `toml:"name"`
	Match      string `toml:"match"`
	DA1        string `toml:"da1"`
	Color      string `toml:"color"`
	MinVersion string `toml:"min_version"`

	re    *regexp.Regexp
	da1re *regexp.Regexp
}

// Info is a matched terminal.
type Info struct {
	Name    string
	Version string // capture group 1 of the entry's match, if any

	// Profile is the color profile the terminal supports, or
	// colorprofile.Unknown when the entry says "auto" or the version is
	// below min_version.
	Profile colorprofile.Profile
}

var (
	loadOnce sync.Once
	entries  []Entry
	loadErr  error
)

// Entries returns the parsed table. Entries whose pattern fails to compile
// are dropped and reported in the returned error.
func Entries() ([]Entry, error) {
	loadOnce.Do(func() {
		data, err := assets.GetTerminals()
		if err != nil {
			loadErr = err
			return
		}
		entries, loadErr = parse(data)
	})
	return entries, loadErr
}

func parse(data []byte) ([]Entry, error) {
	var f struct {
		Terminal []Entry `toml:"terminal"`
	}
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	var out []Entry
	var errs []string
	for _, e := range f.Terminal {
		if e.Match == "" && e.DA1 == "" {
			errs = append(errs, fmt.Sprintf("%s: needs match or da1", e.Name))
			continue
		}
		var err error
		if e.Match != "" {
			if e.re, err = regexp.Compile(e.Match); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", e.Name, err))
				continue
			}
		}
		if e.DA1 != "" {
			if e.da1re, err = regexp.Compile(e.DA1); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", e.Name, err))
				continue
			}
		}
		if _, ok := parseColor(e.Color); !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown color %q", e.Name, e.Color))
			continue
		}
		out = append(out, e)
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("terminals.toml: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// Lookup matches an XTVERSION reply against the table.
func Lookup(xtversion string) (Info, bool) {
	list, _ := Entries()
	return lookup(list, xtversion)
}

func lookup(list []Entry, xtversion string) (Info, bool) {
	xtversion = strings.TrimSpace(xtversion)
	if xtversion == "" {
		return Info{}, false
	}
	for _, e := range list {
		if e.re == nil {
			continue
		}
		m := e.re.FindStringSubmatch(xtversion)
		if m == nil {
			continue
		}
		info := Info{Name: e.Name}
		if len(m) > 1 {
			info.Version = m[1]
		}
		info.Profile, _ = parseColor(e.Color)
		if e.MinVersion != "" && compareVersions(info.Version, e.MinVersion) < 0 {
			info.Profile = colorprofile.Unknown
		}
		return info, true
	}
	return Info{}, false
}

// LookupDA1 matches a DA1 reply's parameters, joined with ";", against the
// table's da1 patterns. For terminals that don't answer XTVERSION.
func LookupDA1(params string) (Info, bool) {
	list, _ := Entries()
	return lookupDA1(list, params)
}

func lookupDA1(list []Entry, params string) (Info, bool) {
	if params == "" {
		return Info{}, false
	}
	for _, e := range list {
		if e.da1re == nil || !e.da1re.MatchString(params) {
			continue
		}
		p, _ := parseColor(e.Color)
		return Info{Name: e.Name, Profile: p}, true
	}
	return Info{}, false
}

func parseColor(s string) (colorprofile.Profile, bool) {
	switch strings.ToLower(s) {
	case "truecolor":
		return colorprofile.TrueColor, true
	case "256":
		return colorprofile.ANSI256, true
	case "16":
		return colorprofile.ANSI, true
	case "auto", "":
		return colorprofile.Unknown, true
	}
	return colorprofile.Unknown, false
}

// compareVersions compares dot-separated numeric versions part by part. A
// missing or non-numeric part counts as 0.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
