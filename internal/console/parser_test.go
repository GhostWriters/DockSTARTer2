package console

import (
	"os"
	"testing"

	"github.com/muesli/termenv"
)

func TestStrip(t *testing.T) {
	// Setup style maps via ensureMaps
	ensureMaps()
	semanticMap["notice"] = "green"        // RAW value (no brackets)
	semanticMap["applicationname"] = "cyan::B"
	semanticMap["version"] = "cyan"

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
			input:    "{{|Notice|}}Hello{{[-]}}",
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
			input:    "{{|ApplicationName|}}App{{[-]}} {{|Version|}}v1.0{{[-]}}",
			expected: "App v1.0",
		},
		{
			name:     "Mixed literal and semantic",
			input:    "{{|Notice|}}Update [v2.0] available{{[-]}}",
			expected: "Update [v2.0] available",
		},
		{
			name:     "Direct color tag",
			input:    "{{[red]}}Error{{[-]}}",
			expected: "Error",
		},
		{
			name:     "Direct style tag",
			input:    "{{[cyan::B]}}Bold cyan{{[-]}}",
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

func TestStripANSI(t *testing.T) {
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
			name:     "Simple ANSI color",
			input:    "\x1b[31mRed Text\x1b[0m",
			expected: "Red Text",
		},
		{
			name:     "Multiple ANSI codes",
			input:    "\x1b[31;1;4mBold Underline Red\x1b[0m",
			expected: "Bold Underline Red",
		},
		{
			name:     "Mixed ANSI and tags",
			input:    "\x1b[31m{{|Notice|}}Hello{{[-]}}\x1b[0m",
			expected: "{{|Notice|}}Hello{{[-]}}", // Note: StripANSI ONLY strips real ANSI, Strip() strips both
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := StripANSI(tt.input)
			if actual != tt.expected {
				t.Errorf("StripANSI(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestExpandTags(t *testing.T) {
	ensureMaps()
	semanticMap["notice"] = "green"        // RAW value (no brackets)
	semanticMap["applicationname"] = "cyan::B"
	semanticMap["version"] = "cyan"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Resolve semantic tag",
			input:    "{{|Notice|}}Text{{[-]}}",
			expected: "{{[green]}}Text{{[-]}}",
		},
		{
			name:     "ApplicationName semantic",
			input:    "{{|ApplicationName|}}",
			expected: "{{[cyan::B]}}",
		},
		{
			name:     "Direct color stays intact",
			input:    "{{[red]}}Error{{[-]}}",
			expected: "{{[red]}}Error{{[-]}}",
		},
		{
			name:     "Preserve literal brackets",
			input:    "{{|Notice|}}Version [v2.0]{{[-]}}",
			expected: "{{[green]}}Version [v2.0]{{[-]}}",
		},
		{
			name:     "Unknown semantic tag - strip it",
			input:    "{{|UnknownTag|}}",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ExpandTags(tt.input)
			if actual != tt.expected {
				t.Errorf("ExpandTags(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestToANSI(t *testing.T) {
	// Setup for TTY mode
	isTTYGlobal = true
	SetPreferredProfile(termenv.TrueColor)

	ensureMaps()
	BuildColorMap()

	// Register test-specific semantic tags (RAW values)
	semanticMap["notice"] = "green"
	semanticMap["version"] = "cyan"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Resolve semantic to ANSI",
			input:    "{{|Notice|}}Hello{{[-]}}",
			expected: "\x1b[32m" + "Hello" + CodeReset,
		},
		{
			name:     "Resolve direct tag (color)",
			input:    "{{[red]}}Error{{[-]}}",
			expected: "\x1b[31m" + "Error" + CodeReset,
		},
		{
			name:     "Resolve direct tag (color:bg)",
			input:    "{{[white:red]}}Alert{{[-]}}",
			expected: "\x1b[37m\x1b[41m" + "Alert" + CodeReset,
		},
		{
			name:     "Resolve direct tag (color::flags)",
			input:    "{{[cyan::B]}}Bold{{[-]}}",
			expected: "\x1b[36m" + CodeBold + "Bold" + CodeReset,
		},
		{
			name:     "Resolve direct tag (color:bg:flags)",
			input:    "{{[red:white:U]}}Underline{{[-]}}",
			expected: "\x1b[31m\x1b[47m" + CodeUnderline + "Underline" + CodeReset,
		},
		{
			name:     "Direct style with High Intensity (H) to ANSI",
			input:    "{{[red::H]}}Vibrant{{[-]}}",
			expected: "\x1b[91m" + "Vibrant" + CodeReset,
		},
		{
			name:     "Direct style with mix High Intensity and Dim (HD) to ANSI",
			input:    "{{[red::HD]}}MutedVibrant{{[-]}}",
			expected: "\x1b[91m" + CodeDim + "MutedVibrant" + CodeReset,
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
	ensureMaps()
	semanticMap["notice"] = "green"  // RAW value

	isTTYGlobal = true
	SetPreferredProfile(termenv.TrueColor)

	input := "{{|Notice|}}Test{{[-]}}"

	// Test Parse alias
	parseResult := Parse(input)
	toAnsiResult := ToANSI(input)
	if parseResult != toAnsiResult {
		t.Errorf("Parse should equal ToANSI: Parse=%q, ToANSI=%q", parseResult, toAnsiResult)
	}

	// Test Translate alias
	translateResult := Translate(input)
	expandResult := ExpandTags(input)
	if translateResult != expandResult {
		t.Errorf("Translate should equal ExpandTags: Translate=%q, ExpandTags=%q", translateResult, expandResult)
	}

	// Test ForTUI alias
	forTUIResult := ForTUI(input)
	if forTUIResult != expandResult {
		t.Errorf("ForTUI should equal ExpandTags: ForTUI=%q, ExpandTags=%q", forTUIResult, expandResult)
	}
}

func TestSemanticVsDirectDistinction(t *testing.T) {
	ensureMaps()
	semanticMap["blue"] = "#0066CC" // Custom blue shade (RAW value)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Semantic blue uses custom color",
			input:    "{{|blue|}}",
			expected: "{{[#0066CC]}}",
		},
		{
			name:     "Direct blue uses default blue",
			input:    "{{[blue]}}",
			expected: "{{[blue]}}",
		},
		{
			name:     "Mixed semantic and direct",
			input:    "{{|blue|}}custom{{[-]}} vs {{[blue]}}standard{{[-]}}",
			expected: "{{[#0066CC]}}custom{{[-]}} vs {{[blue]}}standard{{[-]}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ExpandTags(tt.input)
			if actual != tt.expected {
				t.Errorf("ExpandTags(%q) = %q; want %q", tt.input, actual, tt.expected)
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
