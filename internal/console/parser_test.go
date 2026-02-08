package console

import (
	"os"
	"testing"

	"github.com/muesli/termenv"
)

func TestStrip(t *testing.T) {
	// Setup semantic map for tests
	semanticMap = make(map[string]string)
	semanticMap["notice"] = "[green]"
	semanticMap["applicationname"] = "[cyan::b]"
	semanticMap["version"] = "[cyan]"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Base text",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "Semantic tag",
			input:    "{{_Notice_}}Hello{{|-|}}",
			expected: "Hello",
		},
		{
			name:     "Preserve literal brackets",
			input:    "Update available [v2.0]",
			expected: "Update available [v2.0]",
		},
		{
			name:     "Preserve brackets with text",
			input:    "Log [NOTICE] Message",
			expected: "Log [NOTICE] Message",
		},
		{
			name:     "Multiple semantic tags",
			input:    "{{_ApplicationName_}}App{{|-|}} {{_Version_}}v1.0{{|-|}}",
			expected: "App v1.0",
		},
		{
			name:     "Mixed literal and semantic",
			input:    "{{_Notice_}}Update [v2.0] available{{|-|}}",
			expected: "Update [v2.0] available",
		},
		{
			name:     "Direct color tag",
			input:    "{{|red|}}Error{{|-|}}",
			expected: "Error",
		},
		{
			name:     "Direct tview-style tag",
			input:    "{{|cyan::b|}}Bold cyan{{|-|}}",
			expected: "Bold cyan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Strip(tt.input)
			if actual != tt.expected {
				t.Errorf("Strip(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestToTview(t *testing.T) {
	// Setup semantic map for tests
	semanticMap = make(map[string]string)
	semanticMap["notice"] = "[green]"
	semanticMap["applicationname"] = "[cyan::b]"
	semanticMap["version"] = "[cyan]"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Resolve semantic tag",
			input:    "{{_Notice_}}Text{{|-|}}",
			expected: "[green]Text[-]",
		},
		{
			name:     "ApplicationName semantic",
			input:    "{{_ApplicationName_}}",
			expected: "[cyan::b]",
		},
		{
			name:     "Direct color",
			input:    "{{|red|}}Error{{|-|}}",
			expected: "[red]Error[-]",
		},
		{
			name:     "Direct tview-style fg:bg",
			input:    "{{|white:red|}}Alert{{|-|}}",
			expected: "[white:red]Alert[-]",
		},
		{
			name:     "Direct tview-style with flags",
			input:    "{{|cyan::b|}}Bold{{|-|}}",
			expected: "[cyan::b]Bold[-]",
		},
		{
			name:     "Preserve literal brackets",
			input:    "{{_Notice_}}Version [v2.0]{{|-|}}",
			expected: "[green]Version [v2.0][-]",
		},
		{
			name:     "Unknown semantic stays intact",
			input:    "{{_UnknownTag_}}",
			expected: "{{_UnknownTag_}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ToTview(tt.input)
			if actual != tt.expected {
				t.Errorf("ToTview(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestToANSI(t *testing.T) {
	// Setup for TTY mode
	isTTYGlobal = true
	SetPreferredProfile(termenv.TrueColor)

	// Setup semantic map for tests
	semanticMap = make(map[string]string)
	ansiMap = make(map[string]string)

	// Register standard ANSI codes
	ansiMap["-"] = CodeReset
	ansiMap["red"] = CodeRed
	ansiMap["green"] = CodeGreen
	ansiMap["cyan"] = CodeCyan
	ansiMap["white"] = CodeWhite
	ansiMap["redbg"] = CodeRedBg
	ansiMap["b"] = CodeBold
	ansiMap["u"] = CodeUnderline

	// Register semantic tags
	semanticMap["notice"] = "[green]"
	semanticMap["version"] = "[cyan]"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Resolve semantic to ANSI",
			input:    "{{_Notice_}}Text{{|-|}}",
			expected: CodeGreen + "Text" + CodeReset,
		},
		{
			name:     "Direct color to ANSI",
			input:    "{{|red|}}Error{{|-|}}",
			expected: CodeRed + "Error" + CodeReset,
		},
		{
			name:     "Direct tview-style fg:bg to ANSI",
			input:    "{{|white:red|}}Alert{{|-|}}",
			expected: CodeWhite + CodeRedBg + "Alert" + CodeReset,
		},
		{
			name:     "Direct tview-style with flags to ANSI",
			input:    "{{|cyan::b|}}Bold{{|-|}}",
			expected: CodeCyan + CodeBold + "Bold" + CodeReset,
		},
		{
			name:     "Preserve literal brackets",
			input:    "{{_Notice_}}Version [v2.0]{{|-|}}",
			expected: CodeGreen + "Version [v2.0]" + CodeReset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ToANSI(tt.input)
			if actual != tt.expected {
				t.Errorf("ToANSI(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	// Verify legacy function aliases work
	semanticMap = make(map[string]string)
	semanticMap["notice"] = "[green]"

	// Parse should be alias for ToANSI
	isTTYGlobal = true
	SetPreferredProfile(termenv.TrueColor)
	ansiMap = make(map[string]string)
	ansiMap["-"] = CodeReset
	ansiMap["green"] = CodeGreen

	input := "{{_Notice_}}Test{{|-|}}"

	// Test Parse alias
	parseResult := Parse(input)
	toAnsiResult := ToANSI(input)
	if parseResult != toAnsiResult {
		t.Errorf("Parse should equal ToANSI: Parse=%q, ToANSI=%q", parseResult, toAnsiResult)
	}

	// Test Translate alias
	translateResult := Translate(input)
	toTviewResult := ToTview(input)
	if translateResult != toTviewResult {
		t.Errorf("Translate should equal ToTview: Translate=%q, ToTview=%q", translateResult, toTviewResult)
	}

	// Test PrepareForTUI alias
	prepareResult := PrepareForTUI(input)
	if prepareResult != toTviewResult {
		t.Errorf("PrepareForTUI should equal ToTview: PrepareForTUI=%q, ToTview=%q", prepareResult, toTviewResult)
	}
}

func TestSemanticVsDirectDistinction(t *testing.T) {
	// This test verifies that semantic and direct tags are properly distinguished
	semanticMap = make(map[string]string)
	semanticMap["blue"] = "[#0066CC]" // Custom blue shade

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Semantic blue uses custom color",
			input:    "{{_blue_}}",
			expected: "[#0066CC]",
		},
		{
			name:     "Direct blue uses tview's blue",
			input:    "{{|blue|}}",
			expected: "[blue]",
		},
		{
			name:     "Mixed semantic and direct",
			input:    "{{_blue_}}custom{{|-|}} vs {{|blue|}}standard{{|-|}}",
			expected: "[#0066CC]custom[-] vs [blue]standard[-]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ToTview(tt.input)
			if actual != tt.expected {
				t.Errorf("ToTview(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestDetectProfile(t *testing.T) {
	tests := []struct {
		name      string
		colorterm string
		term      string
		expected  termenv.Profile
	}{
		{"TrueColor override", "truecolor", "", termenv.TrueColor},
		{"24bit override", "24bit", "", termenv.TrueColor},
		{"8bit override", "8bit", "", termenv.ANSI256},
		{"256color override", "256color", "", termenv.ANSI256},
		{"4bit override", "4bit", "", termenv.ANSI},
		{"16color override", "16color", "", termenv.ANSI},
		{"1bit override", "1bit", "", termenv.Ascii},
		{"2color override", "2color", "", termenv.Ascii},
		{"Mono override", "mono", "", termenv.Ascii},
		{"False override", "false", "", termenv.Ascii},
		{"Zero override", "0", "", termenv.Ascii},
		{"TERM direct override", "", "xterm-direct", termenv.TrueColor},
		{"TERM 256color override", "", "xterm-256color", termenv.ANSI256},
		{"TERM 16color override", "", "xterm-16color", termenv.ANSI},
		{"TERM dumb override", "", "dumb", termenv.Ascii},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env
			os.Unsetenv("COLORTERM")
			os.Unsetenv("TERM")

			if tt.colorterm != "" {
				os.Setenv("COLORTERM", tt.colorterm)
			}
			if tt.term != "" {
				os.Setenv("TERM", tt.term)
			}

			// We call detectProfile directly to test its logic
			actual := detectProfile()
			if actual != tt.expected {
				t.Errorf("detectProfile() with COLORTERM=%q, TERM=%q = %v; want %v", tt.colorterm, tt.term, actual, tt.expected)
			}
		})
	}

	// Cleanup
	os.Unsetenv("COLORTERM")
	os.Unsetenv("TERM")
}
