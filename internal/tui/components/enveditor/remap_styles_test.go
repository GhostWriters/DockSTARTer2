package enveditor

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRemapStylesKeepsTextReadable(t *testing.T) {
	white, black, blue := lipgloss.Color("#ffffff"), lipgloss.Color("#000000"), lipgloss.Color("#0000ee")
	fieldFg, fieldBg := lipgloss.Color("#000000"), lipgloss.Color("#ffffff")
	dialogFg, dialogBg := lipgloss.Color("#ffffff"), lipgloss.Color("#0000aa")

	var s StyleState
	s.Text = lipgloss.NewStyle()                                              // unset: takes the field's colors
	s.CommentText = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0f0f0")) // near-white, not the dialog's: lands on the white field
	s.ModifiedText = lipgloss.NewStyle().Foreground(blue)                     // readable on white, kept
	s.BuiltinText = lipgloss.NewStyle().Foreground(dialogFg)                  // the dialog's text: follows the field

	got := RemapStyles(s, dialogFg, dialogBg, fieldFg, fieldBg)
	tests := []struct {
		name   string
		st     lipgloss.Style
		fg, bg color.Color
	}{
		{"unset", got.Text, fieldFg, fieldBg},
		{"white on white", got.CommentText, fieldFg, fieldBg},
		{"readable accent", got.ModifiedText, blue, fieldBg},
		{"dialog text", got.BuiltinText, fieldFg, fieldBg},
	}
	for _, tt := range tests {
		if !sameColor(tt.st.GetForeground(), tt.fg) || !sameColor(tt.st.GetBackground(), tt.bg) {
			t.Errorf("%s: fg %v bg %v, want fg %v bg %v", tt.name, tt.st.GetForeground(), tt.st.GetBackground(), tt.fg, tt.bg)
		}
	}

	// A field foreground that doesn't read on its own background either
	// falls back to black or white.
	got = RemapStyles(s, dialogFg, dialogBg, white, white)
	if fg := got.CommentText.GetForeground(); !sameColor(fg, black) {
		t.Errorf("unreadable field fg: got %v, want black", fg)
	}
}
