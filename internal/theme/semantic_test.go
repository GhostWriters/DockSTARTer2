package theme

import (
	"strings"
	"testing"

	"DockSTARTer2/internal/console"
)

func TestResolveThemeValue(t *testing.T) {
	// Force TTY mode to ensure ANSI codes are generated
	oldTTY := console.SetTTY(true)
	defer console.SetTTY(oldTTY)

	// Setup console maps for resolution
	console.RegisterBaseTags()
	console.BuildColorMap()

	// Mock theme data map (simulating what we read from INI)
	// Using new delimiter format: {{[direct]}} and {{|semantic|}}
	themeMap := map[string]string{
		"Simple":       "{{[red:blue:B]}}",
		"Reference":    "{{|Theme_Simple|}}",
		"OverrideFG":   "{{|Theme_Simple|}}{{[green]}}",
		"OverrideBG":   "{{|Theme_Simple|}}{{[:green]}}",
		"OverrideFlag": "{{|Theme_Simple|}}{{[::U]}}",
		"ChainA":       "{{[white]}}",
		"ChainB":       "{{|Theme_ChainA|}}{{[:black]}}", // white:black
		"ChainC":       "{{|Theme_ChainB|}}{{[::B]}}",    // white:black:B
		"CircularA":    "{{|Theme_CircularB|}}",
		"CircularB":    "{{|Theme_CircularA|}}",
	}

	tests := []struct {
		name     string
		key      string
		expected string // We'll check if output contains specific ANSI codes
	}{
		{
			name:     "Simple Value",
			key:      "Simple",
			expected: "\x1b[31m\x1b[44m" + console.CodeBold, // red fg, blue bg, bold
		},
		{
			name:     "Direct Reference",
			key:      "Reference",
			expected: "\x1b[31m\x1b[44m" + console.CodeBold, // same as Simple
		},
		{
			name:     "Override Foreground",
			key:      "OverrideFG",
			expected: "\x1b[32m\x1b[44m" + console.CodeBold, // green fg, blue bg, bold
		},
		{
			name:     "Override Background",
			key:      "OverrideBG",
			expected: "\x1b[31m\x1b[42m" + console.CodeBold, // red fg, green bg, bold
		},
		{
			name:     "Override Flag (Additive)",
			key:      "OverrideFlag",
			expected: "\x1b[31m\x1b[44m" + console.CodeBold + console.CodeUnderline, // red fg, blue bg, bold + underline
		},
		{
			name:     "Chained Resolution",
			key:      "ChainC",
			expected: "\x1b[37m\x1b[40m" + console.CodeBold, // white fg, black bg, bold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Since we haven't implemented resolveThemeValue yet, this test serves as our spec.
			// We will implement `resolveThemeValue` in theme.go next.
			// Note: We use the raw value from the map as the starting point
			rawVal, ok := themeMap[tt.key]
			if !ok {
				t.Fatalf("Test key %s not found in themeMap", tt.key)
			}

			// We need to pass the *value* associated with the key to resolveThemeValue,
			// because resolveThemeValue expects the raw string, not the key.
			// Pass the delimiters used in the test data
			got, err := resolveThemeValue(rawVal, themeMap, make(map[string]bool),
				console.SemanticPrefix, console.SemanticSuffix, console.DirectPrefix, console.DirectSuffix)
			if err != nil {
				t.Fatalf("resolve error: %v", err)
			}

			// Since resolveThemeValue returns a tag string (e.g. {{[red:blue:B]}}),
			// we must expand it to ANSI to compare with expected ANSI values.
			gotExpanded := console.ToANSI(console.WrapDirect(got))

			if !strings.Contains(gotExpanded, tt.expected) {
				t.Errorf("resolveThemeValue() ANSI = %q, want %q (raw: %v)", gotExpanded, tt.expected, got)
			}
		})
	}
}
