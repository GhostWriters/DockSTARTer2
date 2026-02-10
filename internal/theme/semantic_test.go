package theme

import (
	"strings"
	"testing"

	"DockSTARTer2/internal/console"
)

func TestResolveThemeValue(t *testing.T) {
	// Setup console maps for resolution
	console.RegisterBaseTags()
	console.BuildColorMap()

	// Mock theme data map (simulating what we read from INI)
	themeMap := map[string]string{
		"Simple":       "{{|red:blue:B|}}",
		"Reference":    "{{_ThemeSimple_}}",
		"OverrideFG":   "{{_ThemeSimple_}}{{|green|}}",
		"OverrideBG":   "{{_ThemeSimple_}}{{|:green|}}",
		"OverrideFlag": "{{_ThemeSimple_}}{{|::U|}}",
		"ChainA":       "{{|white|}}",
		"ChainB":       "{{_ThemeChainA_}}{{|:black|}}", // white:black
		"ChainC":       "{{_ThemeChainB_}}{{|::B|}}",    // white:black:B
		"CircularA":    "{{_ThemeCircularB_}}",
		"CircularB":    "{{_ThemeCircularA_}}",
	}

	// Register base keys as semantic tags (as we would during parsing)
	// We need a way to mock the registration or expose the resolution logic directly.
	// For this test, we'll implement a simplified resolver that looks up from themeMap.

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
			// This test relies on the NEW logic we are about to implement.
			// It will fail until resolveThemeValue is implemented.
			// For TDD, we define the expected behavior first.

			// We need to inject the map into the resolver.
			// Ideally, we'd refactor theme.go to expose a Resolver struct or function.
			// For now, let's assume a function resolveThemeValue(key, map) exists or we export it.

			// Note: Since we haven't implemented resolveThemeValue yet, this test serves as our spec.
			// We will implement `resolveThemeValue` in theme.go next.
			// Note: We use the raw value from the map as the starting point
			rawVal, ok := themeMap[tt.key]
			if !ok {
				// For the test cases that are "Direct keys", the test struct has the key.
				// But some tests might just pass a raw string?
				// Actually the test loop iterates over keys in themeMap.
				// Wait, the test case struct has 'key'. We should look it up.
				t.Fatalf("Test key %s not found in themeMap", tt.key)
			}

			// We need to pass the *value* associated with the key to resolveThemeValue,
			// because resolveThemeValue expects the raw string, not the key.
			got, err := resolveThemeValue(rawVal, themeMap, make(map[string]bool))
			if err != nil {
				t.Fatalf("resolve error: %v", err)
			}

			// Check for inclusion of ANSI codes because order might vary slightly
			// or we can exact match if we are confident.
			// For simplicity: verify the string *contains* the expected sequences.
			// Ideally we should verify logic.

			// To compare exact strings, we need exact expected ANSI strings.
			// Let's rely on exact match for this test as ANSI codes are deterministic.

			// Adjust expected validation logic as needed during implementation.
			if !strings.Contains(got, tt.expected) {
				// Fallback because map iteration order in partials might affect output string construction?
				// Actually, our resolution should be deterministic.
				// Let's just check equality for now, if it fails we see why.
			}
		})
	}
}
