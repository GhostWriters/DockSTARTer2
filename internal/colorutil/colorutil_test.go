package colorutil

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestContrast(t *testing.T) {
	tests := []struct {
		a, b     string
		min, max float64
	}{
		{"#ffffff", "#ffffff", 1, 1},
		{"#000000", "#ffffff", 20.9, 21.1},
		{"15", "7", 1, 1.4}, // bright white on white: unreadable
		{"0", "15", 20.9, 21.1},
		{"6", "0", 9, 11}, // cyan on black, xterm's #00cdcd
	}
	for _, tt := range tests {
		got, ok := Contrast(lipgloss.Color(tt.a), lipgloss.Color(tt.b))
		if !ok || got < tt.min || got > tt.max {
			t.Errorf("Contrast(%s, %s) = %.2f, %v; want %.1f-%.1f", tt.a, tt.b, got, ok, tt.min, tt.max)
		}
	}
	if _, ok := Contrast(lipgloss.NoColor{}, lipgloss.Color("0")); ok {
		t.Error("Contrast with no color reported ok")
	}
}
