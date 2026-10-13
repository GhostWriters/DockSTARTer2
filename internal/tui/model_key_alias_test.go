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
		"<": displayengine.Keys.InnerTabPrev,
		">": displayengine.Keys.InnerTabNext,
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

// drawnCursorInput is a focused text field that draws its own cursor.
type drawnCursorInput struct{}

func (drawnCursorInput) TextInputFocused() bool { return true }
func (drawnCursorInput) GetInputCursor() (int, int, tea.CursorShape, bool) {
	return 0, 0, tea.CursorBar, false
}

// hardwareCursorInput is a focused text field that shows the hardware cursor.
type hardwareCursorInput struct{}

func (hardwareCursorInput) GetInputCursor() (int, int, tea.CursorShape, bool) {
	return 0, 0, tea.CursorBar, true
}

// TestTextInputFocused checks a field drawing its own cursor still counts as
// focused, so plain alias keys type into it.
func TestTextInputFocused(t *testing.T) {
	if !textInputFocused(drawnCursorInput{}) {
		t.Error("a field drawing its own cursor isn't focused")
	}
	if !textInputFocused(hardwareCursorInput{}) {
		t.Error("a field showing the hardware cursor isn't focused")
	}
	if textInputFocused(struct{}{}) {
		t.Error("a model with no text field is focused")
	}
}
