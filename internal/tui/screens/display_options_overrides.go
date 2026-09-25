package screens

import (
	"context"
	"reflect"
	"regexp"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"github.com/GhostWriters/semstyle"
)

// overrideSlot is one color an element's overrides can set.
type overrideSlot struct {
	base  string // base16/base24 slot name, e.g. "base08"
	label string // what it colors
}

// overrideSlots lists every override slot: the 16 standard ANSI colors,
// then base24's extra shades, which have no ANSI slot.
var overrideSlots = []overrideSlot{
	{"base00", "black"}, {"base08", "red"}, {"base0b", "green"}, {"base0a", "yellow"},
	{"base0d", "blue"}, {"base0e", "magenta"}, {"base0c", "cyan"}, {"base05", "white"},
	{"base03", "bright black"}, {"base12", "bright red"}, {"base14", "bright green"}, {"base13", "bright yellow"},
	{"base16", "bright blue"}, {"base17", "bright magenta"}, {"base15", "bright cyan"}, {"base07", "bright white"},
	{"base01", "darker background"}, {"base02", "darkest background"}, {"base04", "dark foreground"},
	{"base06", "light foreground"}, {"base09", "orange"}, {"base0f", "brown"},
	{"base10", "base24 darker background"}, {"base11", "base24 darkest background"},
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

// overrideItems returns one row per override slot with its staged value and
// a swatch; Enter edits it.
func (s *DisplayOptionsScreen) overrideItems() []displayengine.MenuItem {
	el := s.stagedTint()
	base := s.baseTint()
	items := make([]displayengine.MenuItem, 0, len(overrideSlots)+1)
	for i, slot := range overrideSlots {
		if i == 16 {
			items = append(items, displayengine.MenuItem{Tag: "base24 extras", IsSeparator: true})
		}
		value := ""
		if f := overrideField(el, slot.base); f != nil {
			value = *f
		}
		desc := "{{|ItemList|}}" + slot.base + "  "
		if value == "" {
			desc += "(tint's)"
		} else {
			desc += value
			if hex := colorHex(value); hex != "" {
				desc += "  {{[" + hex + "]}}███{{[-]}}"
			}
		}
		changed := false
		if f := overrideField(&base, slot.base); f != nil {
			changed = *f != value
		}
		items = append(items, displayengine.MenuItem{
			Tag:        slot.label,
			Desc:       desc,
			Changed:    changed,
			Help:       "Override the " + slot.label + " color (" + slot.base + "); Enter to edit, empty to use the tint's",
			Selectable: true,
			Action:     s.promptOverride(slot, value),
			Metadata:   map[string]string{"slot": slot.base},
		})
	}
	return items
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
