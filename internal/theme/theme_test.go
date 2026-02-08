package theme

import (
	"DockSTARTer2/internal/testutils"
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestGetColorStr(t *testing.T) {
	tests := []struct {
		input    lipgloss.TerminalColor
		expected string
	}{
		// Standard Colors (Mapped to ANSI Indices)
		{lipgloss.Color("0"), "0"},
		{lipgloss.Color("1"), "1"},
		{lipgloss.Color("2"), "2"},
		{lipgloss.Color("4"), "4"},
		{lipgloss.Color("7"), "7"},

		// Custom RGB
		{lipgloss.Color("#010203"), "#010203"},
	}

	var cases []testutils.TestCase

	for _, tt := range tests {
		actual := GetColorStr(tt.input)
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

func TestParseColor(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard Colors (Now expecting Hex)
		{"black", "#000000"},
		{"red", "#ff0000"},

		// Hex Codes
		{"#123456", "#123456"},

		// Tcell Extended Colors
		{"orange", "#ffa500"}, // CSS Orange
		{"rebeccapurple", "#663399"},
	}

	for _, tt := range tests {
		c := parseColor(tt.input)
		actual := ""
		if c != nil {
			actual = fmt.Sprintf("%v", c)
		}

		if actual != tt.expected {
			t.Errorf("parseColor(%q) = %q, want %q", tt.input, actual, tt.expected)
		}
	}
}
