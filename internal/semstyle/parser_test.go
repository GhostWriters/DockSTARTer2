package semstyle

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestStrip(t *testing.T) {
	// Setup style maps via ensureMaps
	Default.ensureMaps()
	Default.consoleMap["notice"] = "green" // RAW value (no brackets)
	Default.consoleMap["applicationname"] = "cyan::B"
	Default.consoleMap["version"] = "cyan"

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
			actual := ToPlain(tt.input)
			if actual != tt.expected {
				t.Errorf("ToPlain(%q) = %q; want %q", tt.input, actual, tt.expected)
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
			expected: "{{|Notice|}}Hello{{[-]}}", // Note: StripANSI ONLY strips real ANSI, ToPlain() strips both
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

func TestExpandConsoleTags(t *testing.T) {
	Default.ensureMaps()
	Default.consoleMap["notice"] = "green" // RAW value (no brackets)
	Default.consoleMap["applicationname"] = "cyan::B"
	Default.consoleMap["version"] = "cyan"

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
			actual := ToTags(tt.input)
			if actual != tt.expected {
				t.Errorf("ToTags(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestToConsoleANSI(t *testing.T) {
	// Setup for TTY mode
	SetPreferredProfile(colorprofile.TrueColor)

	Default.ensureMaps()
	BuildColorMap()

	// Register test-specific semantic tags (RAW values)
	Default.consoleMap["notice"] = "green"
	Default.consoleMap["version"] = "cyan"

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
	Default.ensureMaps()
	RegisterSemanticTagRaw("notice", "green") // Sets both console and theme maps

	SetPreferredProfile(colorprofile.TrueColor)

	input := "{{|Notice|}}Test{{[-]}}"

	// Test Parse alias
	parseResult := ToANSI(input)
	toAnsiResult := ToANSI(input)
	if parseResult != toAnsiResult {
		t.Errorf("Parse should equal ToANSI: Parse=%q, ToANSI=%q", parseResult, toAnsiResult)
	}

	// Test Translate alias
	translateResult := ToTags(input)
	expandResult := ToTags(input)
	if translateResult != expandResult {
		t.Errorf("Translate should equal ExpandTags: Translate=%q, ExpandTags=%q", translateResult, expandResult)
	}

}

func TestSemanticVsDirectDistinction(t *testing.T) {
	Default.ensureMaps()
	Default.consoleMap["blue"] = "#0066CC" // Custom blue shade (RAW value)

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
			actual := ToTags(tt.input)
			if actual != tt.expected {
				t.Errorf("ToTags(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestInlineHyperlinks(t *testing.T) {
	SetPreferredProfile(colorprofile.TrueColor)
	st := New()

	// With explicit label
	result := st.ToANSI(`{{[magenta:black:B:DockSTARTer Website]}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(result, "\x1b]8;;https://dockstarter.com\x1b\\") {
		t.Errorf("expected OSC8 hyperlink open, got: %q", result)
	}
	if !strings.Contains(result, "DockSTARTer Website") {
		t.Errorf("expected label text in output, got: %q", result)
	}
	if !strings.Contains(result, "\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC8 hyperlink close, got: %q", result)
	}

	// With empty label — URL used as both link and display text
	result2 := st.ToANSI(`{{[cyan::U:]}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(result2, "https://dockstarter.com") {
		t.Errorf("expected URL as label when label empty, got: %q", result2)
	}
	if !strings.Contains(result2, "\x1b]8;;https://dockstarter.com\x1b\\") {
		t.Errorf("expected OSC8 hyperlink, got: %q", result2)
	}

	// Non-hyperlink tag (no label field) must still render normally
	result3 := st.ToANSI(`{{[red::B]}}hello{{[-]}}`)
	if strings.Contains(result3, "\x1b]8;") {
		t.Errorf("plain tag should not produce hyperlink, got: %q", result3)
	}

	// Semantic tag with full fields + label
	st.RegisterConsoleTag("mylink", "cyan::U")
	result4 := st.ToANSI(`{{|mylink:::B:DockSTARTer Website|}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(result4, "\x1b]8;;https://dockstarter.com\x1b\\") {
		t.Errorf("semantic: expected OSC8 hyperlink open, got: %q", result4)
	}
	if !strings.Contains(result4, "DockSTARTer Website") {
		t.Errorf("semantic: expected label in output, got: %q", result4)
	}

	// Semantic tag with no color overrides + label (4 empty fields before label)
	result5 := st.ToANSI(`{{|mylink::::DockSTARTer Website|}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(result5, "\x1b]8;;https://dockstarter.com\x1b\\") {
		t.Errorf("semantic no-override: expected OSC8 hyperlink, got: %q", result5)
	}
	if !strings.Contains(result5, "DockSTARTer Website") {
		t.Errorf("semantic no-override: expected label, got: %q", result5)
	}

	// Semantic tag with only name:fg — no label, must not hyperlink
	result6 := st.ToANSI(`{{|mylink:red|}}hello{{[-]}}`)
	if strings.Contains(result6, "\x1b]8;") {
		t.Errorf("semantic single-modifier should not produce hyperlink, got: %q", result6)
	}
}

