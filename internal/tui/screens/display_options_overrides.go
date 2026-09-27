package screens

import (
	"context"
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

// overrideItems returns one row per override slot with its staged value, a
// swatch, its other names, and its suggested usage; Enter edits it.
func (s *DisplayOptionsScreen) overrideItems() []displayengine.MenuItem {
	el := s.stagedTint()
	base := s.baseTint()
	var items []displayengine.MenuItem
	for g, group := range overrideGroups {
		items = append(items, displayengine.MenuItem{Tag: group.label, IsSeparator: true})
		for _, slot := range group.slots {
			names, usage := overrideNotes(g, slot)
			items = append(items, s.overrideItem(el, base, slot, names, usage))
		}
	}
	return items
}

// overrideValueWidth is the value column's width, so the notes after it
// line up.
const overrideValueWidth = 9

// overrideItem returns slot's row: its staged value and a swatch, then its
// other names and suggested usage; changed when the value differs from
// base.
func (s *DisplayOptionsScreen) overrideItem(el *config.AnsiElementColors, base config.AnsiElementColors, slot overrideSlot, names, usage string) displayengine.MenuItem {
	value := ""
	if f := overrideField(el, slot.base); f != nil {
		value = *f
	}
	shown := value
	if shown == "" {
		shown = "(tint's)"
	}
	swatch := "   "
	if hex := colorHex(value); value != "" && hex != "" {
		swatch = "{{[" + hex + "]}}███{{[-]}}"
	}
	desc := "{{|ItemList|}}" + slot.base + "  " + shown + strutil.Repeat(" ", overrideValueWidth-len([]rune(shown))) + swatch
	switch {
	case names != "" && usage != "":
		desc += "  " + names + "; " + usage
	case names != "" || usage != "":
		desc += "  " + names + usage
	}
	changed := false
	if f := overrideField(&base, slot.base); f != nil {
		changed = *f != value
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
		Changed:    changed,
		Help:       help + "; Enter to edit, empty to use the tint's",
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
			"Override "+slot.label, "Enter a hex value (#rrggbb) or color name for "+slot.label+" ("+slot.base+"); leave empty to use the tint's", false, current)
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
