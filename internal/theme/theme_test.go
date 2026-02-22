package theme

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/testutils"
	"fmt"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestGetColorStr(t *testing.T) {
	tests := []struct {
		input    color.Color
		expected string
	}{
		// Standard Colors (converted to hex in lipgloss v2)
		{lipgloss.Color("0"), "#000000"}, // black
		{lipgloss.Color("1"), "#800000"}, // red (ANSI)
		{lipgloss.Color("2"), "#008000"}, // green
		{lipgloss.Color("4"), "#000080"}, // blue
		{lipgloss.Color("7"), "#c0c0c0"}, // white (silver)

		// Custom RGB
		{lipgloss.Color("#010203"), "#010203"},
	}

	var cases []testutils.TestCase

	for _, tt := range tests {
		actual := console.GetColorStr(tt.input)
		pass := actual == tt.expected
		cases = append(cases, testutils.TestCase{
			Input:    fmt.Sprintf("%v", tt.input),
			Expected: tt.expected,
			Actual:   actual,
			Pass:     pass,
		})
	}

	testutils.PrintTestTable(t, cases)
}
