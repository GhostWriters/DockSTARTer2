package theme

import (
	"DockSTARTer2/internal/testutils"
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestGetColorStr(t *testing.T) {
	tests := []struct {
		input    tcell.Color
		expected string
	}{
		// Standard Colors (Mapped to ANSI Indices)
		{tcell.ColorBlack, "0"},
		{tcell.ColorRed, "1"},
		{tcell.ColorGreen, "2"},
		{tcell.ColorBlue, "4"},
		{tcell.ColorWhite, "7"},

		// Custom RGB (Not in map, returns Name/Hex from tcell)
		// tcell.NewRGBColor returns a color where .Name() might be the hex string if not standard.
		// GetColorStr falls back to c.Name().ToLower()
		{tcell.NewRGBColor(1, 2, 3), "#010203"},
	}

	var cases []testutils.TestCase

	for _, tt := range tests {
		actual := GetColorStr(tt.input)
		pass := actual == tt.expected
		cases = append(cases, testutils.TestCase{
			Input:    tt.input.String(), // tcell.Color.String() typically returns Name or Hex
			Expected: tt.expected,
			Actual:   actual,
			Pass:     pass,
		})
	}

	testutils.PrintTestTable(t, cases)
}
