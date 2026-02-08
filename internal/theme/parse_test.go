package theme

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestParseTagWithStyles(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected StyleFlags
		fg       tcell.Color
		bg       tcell.Color
	}{
		{
			name: "All flags lowercase",
			tag:  "white:blue:buldrsi",
			expected: StyleFlags{
				Bold:          true,
				Underline:     true,
				Dim:           true,
				Reverse:       true,
				Strikethrough: true,
				Italic:        true,
				Blink:         true,
			},
		},
		{
			name: "All flags uppercase",
			tag:  "white:blue:BULDRSI",
			expected: StyleFlags{
				Bold:          true,
				Underline:     true,
				Dim:           true,
				Reverse:       true,
				Strikethrough: true,
				Italic:        true,
				Blink:         true,
			},
		},
		{
			name: "Mixed flags",
			tag:  "white:blue:BsU",
			expected: StyleFlags{
				Bold:          true,
				Strikethrough: true,
				Underline:     true,
			},
		},
		{
			name:     "No flags",
			tag:      "white:blue",
			expected: StyleFlags{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, actual := parseTagWithStyles(tt.tag)
			if actual != tt.expected {
				t.Errorf("parseTagWithStyles(%q) = %+v; want %+v", tt.tag, actual, tt.expected)
			}
		})
	}
}
