package console

import (
	"DockSTARTer2/internal/testutils"
	"testing"
)

func TestTranslate(t *testing.T) {
	// Setup: Register some test colors
	ResetCustomColors()
	RegisterColor("_TestColor_", "[red]")
	RegisterColor("_TestNested_", "[_TestColor_]")
	RegisterColor("_Complex_", "[blue:yellow:b]")
	defer ResetCustomColors()

	tests := []struct {
		input    string
		expected string
	}{
		// Basic Pass-through
		{"Hello World", "Hello World"},
		{"[red]Red Text[-]", "[red]Red Text[-]"},

		// Semantic Tag Resolution
		{"[_TestColor_]Hello", "[red]Hello"},
		{"Prefix[_TestColor_]Suffix", "Prefix[red]Suffix"},

		// Recursive Resolution
		{"[_TestNested_]", "[red]"},

		// Complex Tags
		{"[_Complex_]Bold", "[blue:yellow:b]Bold"},

		// Undefined Tags (Pass through)
		{"[_Unknown_]", "[_Unknown_]"},

		// Mixed
		{"[_TestColor_]Red and [_Complex_]Complex", "[red]Red and [blue:yellow:b]Complex"},
	}

	var cases []testutils.TestCase

	for _, tt := range tests {
		actual := Translate(tt.input)
		pass := actual == tt.expected
		cases = append(cases, testutils.TestCase{
			Input:    tt.input,
			Expected: tt.expected,
			Actual:   actual,
			Pass:     pass,
		})
	}

	testutils.PrintTestTable(t, cases)
}
