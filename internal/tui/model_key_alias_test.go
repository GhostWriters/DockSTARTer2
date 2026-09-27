package tui

import (
	"testing"

	"DockSTARTer2/internal/displayengine"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// TestKeyAliasesMatchTheirShortcuts checks each plain-key alias becomes the
// key its shortcut's binding matches.
func TestKeyAliasesMatchTheirShortcuts(t *testing.T) {
	for plain, binding := range map[string]key.Binding{
		"[": displayengine.Keys.ShiftTab,
		"]": displayengine.Keys.Tab,
		",": displayengine.Keys.TabStripPrev,
		".": displayengine.Keys.TabStripNext,
	} {
		alias, ok := keyAliases[plain]
		if !ok {
			t.Errorf("%q has no alias", plain)
			continue
		}
		if !key.Matches(alias, binding) {
			t.Errorf("%q becomes %q, which %v doesn't match", plain, alias.String(), binding.Keys())
		}
	}
	if typed := (tea.KeyPressMsg{Code: '.', Text: "."}); typed.String() != "." {
		t.Errorf("a typed '.' is %q, not \".\"", typed.String())
	}
}
