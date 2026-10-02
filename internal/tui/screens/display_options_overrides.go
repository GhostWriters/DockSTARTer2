package screens

import (
	"context"
	"image/color"
	"reflect"
	"regexp"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/x/ansi"
)

// overrideSlot is one color an element's overrides can set.
type overrideSlot struct {
	base  string // base16/base24 slot name, e.g. "base08"
	label string // what it colors
}

// overrideGroups lists the override slots by group: the 16 standard ANSI
// colors, the grayscale ramp from darkest to lightest, then the other
// accents. A slot in more than one group edits the same color from each
// row.
var overrideGroups = []struct {
	label string
	slots []overrideSlot
}{
	{"Standard colors", []overrideSlot{
		{"base00", "black"}, {"base08", "red"}, {"base0b", "green"}, {"base0a", "yellow"},
		{"base0d", "blue"}, {"base0e", "magenta"}, {"base0c", "cyan"}, {"base05", "white"},
		{"base03", "bright black"}, {"base12", "bright red"}, {"base14", "bright green"}, {"base13", "bright yellow"},
		{"base16", "bright blue"}, {"base17", "bright magenta"}, {"base15", "bright cyan"}, {"base07", "bright white"},
	}},
	{"Grayscale, darkest to lightest", []overrideSlot{
		{"base11", "gray 0"}, {"base10", "gray 1"}, {"base00", "gray 2"}, {"base01", "gray 3"}, {"base02", "gray 4"},
		{"base03", "gray 5"}, {"base04", "gray 6"}, {"base05", "gray 7"}, {"base06", "gray 8"}, {"base07", "gray 9"},
	}},
	{"Other colors", []overrideSlot{
		{"base09", "orange"}, {"base0f", "brown"},
	}},
}

// overrideUsage is each slot's suggested usage, from the base16 and
// base24 styling guidelines.
var overrideUsage = map[string]string{
	"base11": "darkest background",
	"base10": "darker background",
	"base00": "default background",
	"base01": "lighter background",
	"base02": "selection background",
	"base03": "comments",
	"base04": "dark foreground",
	"base05": "default foreground",
	"base06": "light foreground",
	"base07": "light background",
}

// overrideNotes returns what follows slot's value in group: its names in the
// other groups that list it, then its suggested usage.
func overrideNotes(group int, slot overrideSlot) (names, usage string) {
	var others []string
	for g, other := range overrideGroups {
		for _, o := range other.slots {
			if g != group && o.base == slot.base {
				others = append(others, o.label)
			}
		}
	}
	return strings.Join(others, ", "), overrideUsage[slot.base]
}

// overrideField returns a pointer to e's field for slot base (e.g. base0b ->
// Base0B).
func overrideField(e *config.AnsiElementColors, base string) *string {
	f := reflect.ValueOf(e).Elem().FieldByName("Base" + strings.ToUpper(strings.TrimPrefix(base, "base")))
	if !f.IsValid() || f.Kind() != reflect.String {
		return nil
	}
	return f.Addr().Interface().(*string)
}

var hexColor = regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{6})$`)

// validOverrideColor reports whether v is a color an override accepts: a
// hex value or a named color.
func validOverrideColor(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return hexColor.MatchString(v) || semstyle.GetHexForColor(v) != ""
}

// overrideEditMsg stages value for slot base on the shown element.
type overrideEditMsg struct{ base, value string }

// overrideItems returns one row per override slot: its color without an
// override, the staged override, its other names, and its suggested usage;
// Enter edits it.
func (s *DisplayOptionsScreen) overrideItems() []displayengine.MenuItem {
	el := s.stagedTint()
	base := s.baseTint()
	scheme := s.overrideSchemeColors(el)
	var items []displayengine.MenuItem
	for g, group := range overrideGroups {
		items = append(items, displayengine.MenuItem{Tag: group.label, IsSeparator: true, IsCategory: true})
		for _, slot := range group.slots {
			names, usage := overrideNotes(g, slot)
			items = append(items, s.overrideItem(el, base, scheme, slot, names, usage))
		}
	}
	return items
}

// overrideSchemeColors returns the colors el's tint sets, or nil when its
// tint is off or won't load (either way, it renders untinted).
func (s *DisplayOptionsScreen) overrideSchemeColors(el *config.AnsiElementColors) *config.AnsiElementColors {
	if !el.TintEnabled || el.Tint == "" {
		return nil
	}
	colors, ok := s.schemeColors[el.Tint]
	if !ok {
		if c, err := tui.TintSchemeColors(context.Background(), el.Tint); err == nil {
			colors = &c
		}
		if s.schemeColors == nil {
			s.schemeColors = map[string]*config.AnsiElementColors{}
		}
		s.schemeColors[el.Tint] = colors
	}
	return colors
}

// overrideValueWidth is a color column's width before its swatch, so the
// columns after it line up: the longest terminal color name, bright-magenta.
const overrideValueWidth = 14

// overrideCell returns text padded to overrideValueWidth, then swatch.
func overrideCell(text, swatch string) string {
	return text + strutil.Repeat(" ", overrideValueWidth-len([]rune(text))) + swatch
}

// colorSwatch returns value's swatch, or blank space if it isn't a color.
func colorSwatch(value string) string {
	if hex := colorHex(value); value != "" && hex != "" {
		return "{{[" + hex + "]}}███{{[-]}}"
	}
	return "   "
}

// tintSwatch returns slot base's swatch as the tint registered under key
// colors it, or the terminal's own palette for key "". The row is drawn
// under the screen's tint, so it's rendered here and embedded as escape
// codes.
func tintSwatch(key, base string) string {
	var sw string
	console.ActivateTintKey(key, func() { sw = semstyle.ToANSI("{{[" + base + "]}}███") })
	return sw + "\x1b[39m"
}

// slotColor returns the color slot base resolves to under the tint
// registered under key ("" for none): "#rrggbb", or the name of the terminal
// color it falls back to (e.g. "bright-black").
func slotColor(key, base string) string {
	var c color.Color
	console.ActivateTintKey(key, func() { c = semstyle.ToColor(base) })
	if b, ok := c.(ansi.BasicColor); ok {
		if b < 8 {
			return semstyle.BasicColors[b]
		}
		return "bright-" + semstyle.BasicColors[b-8]
	}
	return semstyle.ToColorStr(c)
}

// schemeTintKey returns the tint key holding ref's scheme alone, without
// overrides, registering it when ref changes. It gives a row the derived
// color of a slot the scheme doesn't set.
func (s *DisplayOptionsScreen) schemeTintKey(ref string) string {
	key := s.previewTintKey + "-scheme"
	if s.schemeTintRef != ref {
		s.schemeTintRef = ref
		tui.RegisterTintKey(context.Background(), key, config.AnsiElementColors{Tint: ref, TintEnabled: true})
	}
	return key
}

// overrideItem returns slot's row: the color it has without an override
// (the tint's, or the terminal's own without one), then the staged
// override, then its other names and suggested usage; the override is
// marked changed when it differs from base.
func (s *DisplayOptionsScreen) overrideItem(el *config.AnsiElementColors, base config.AnsiElementColors, scheme *config.AnsiElementColors, slot overrideSlot, names, usage string) displayengine.MenuItem {
	value := ""
	if f := overrideField(el, slot.base); f != nil {
		value = *f
	}
	beforeText, beforeSwatch := slotColor("", slot.base), tintSwatch("", slot.base)
	if scheme != nil {
		before := ""
		if f := overrideField(scheme, slot.base); f != nil {
			before = *f
		}
		if before == "" {
			// A scheme without this slot gets a color derived from the others.
			before = slotColor(s.schemeTintKey(el.Tint), slot.base)
		}
		beforeText, beforeSwatch = before, colorSwatch(before)
	}
	// Without an override, the color before it is the one in effect, so it's
	// swatched here too.
	afterText, afterSwatch := value, colorSwatch(value)
	if value == "" {
		afterText, afterSwatch = "(none)", beforeSwatch
	}
	changed := false
	if f := overrideField(&base, slot.base); f != nil {
		changed = *f != value
	}
	// A changed override is marked on its value, in the spaces on either side
	// of it, rather than on the row's label.
	markL, markR := " ", " "
	if changed {
		l, r := displayengine.ChangedIndicatorChars(displayengine.ActiveAppearance().LineCharacters)
		markL = "{{|PanelTitleChangedIndicator|}}" + l + "{{[-]}}{{|ItemList|}}"
		markR = "{{|PanelTitleChangedIndicator|}}" + r + "{{[-]}}{{|ItemList|}}"
	}
	desc := "{{|ItemList|}}" + slot.base + "  " + overrideCell(beforeText, beforeSwatch) + "  → " +
		markL + afterText + markR + strutil.Repeat(" ", overrideValueWidth-len([]rune(afterText))-1) + afterSwatch
	switch {
	case names != "" && usage != "":
		desc += "  " + names + "; " + usage
	case names != "" || usage != "":
		desc += "  " + names + usage
	}
	help := "Override the " + slot.label + " color (" + slot.base + ")"
	if names != "" {
		help += ", also " + names
	}
	if usage != "" {
		help += ", usually the " + usage
	}
	return displayengine.MenuItem{
		Tag:        slot.label,
		Desc:       desc,
		Help:       help + "; Enter to edit, empty for no override",
		Selectable: true,
		Action:     s.promptOverride(slot, value),
		Metadata:   map[string]string{"slot": slot.base},
	}
}

// colorHex returns v as a "#rrggbb" hex value, or "" if it isn't a color.
func colorHex(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if hexColor.MatchString(v) {
		if len(v) == 4 {
			return "#" + string([]byte{v[1], v[1], v[2], v[2], v[3], v[3]})
		}
		return v
	}
	return semstyle.GetHexForColor(v)
}

// promptOverride asks for slot's new value, current by default.
func (s *DisplayOptionsScreen) promptOverride(slot overrideSlot, current string) tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(), func(context.Context, any, ...any) {},
			"Override "+slot.label, "Enter a hex value (#rrggbb) or color name for "+slot.label+" ("+slot.base+"); leave empty for no override", false, current)
		if err != nil {
			return nil
		}
		value := strings.TrimSpace(result)
		if strings.EqualFold(value, "none") {
			value = ""
		}
		if value != "" && !validOverrideColor(value) {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Color",
				Message: "\"" + value + "\" isn't a color: use a hex value like #ff8800 or a color name.",
				Type:    tui.MessageError,
			}
		}
		return overrideEditMsg{base: slot.base, value: value}
	}
}
