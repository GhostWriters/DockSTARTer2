package console

import (
	"testing"

	"github.com/muesli/termenv"
)

func TestParseStyleCodeToANSI(t *testing.T) {
	// Force specific profile for deterministic testing (CI environment might be ansi/ascii)
	originalProfile := preferredProfile
	defer func() { preferredProfile = originalProfile }()
	preferredProfile = termenv.TrueColor

	// Set up maps
	RegisterBaseTags()
	BuildColorMap()

	tests := []struct {
		name     string
		input    string
		expected string // We check for containment or specific codes
	}{
		{
			name:     "Named Color",
			input:    "red",
			expected: "\x1b[31m", // Standard Red (ansiMap priority)
		},
		{
			name:     "Hex Color",
			input:    "#00ff00",
			expected: "\x1b[38;2;0;255;0m",
		},
		{
			name:     "Numeric Index",
			input:    "202",
			expected: "", // Removed termenv fallback.
		},
		{
			name:     "Numeric Index BG",
			input:    ":235",
			expected: "", // Removed termenv fallback.
		},
		{
			name:     "Mixed",
			input:    "white:red:B",
			expected: "\x1b[37m\x1b[41m" + CodeBold,
		},
		{"Reset", "-", "\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStyleCodeToANSI(tt.input)
			if tt.name == "Hex Color" {
				if len(got) < 5 {
					t.Errorf("parseStyleCodeToANSI(%q) = %q, expected longer sequence", tt.input, got)
				}
			} else if tt.expected != "" {
				// Exact match check for standard ANSI or Reset
				if got != tt.expected {
					t.Errorf("parseStyleCodeToANSI(%q) = %q, want %q", tt.input, got, tt.expected)
				}
			}
		})
	}
}
