package console

import (
	"testing"
)

func TestStrip(t *testing.T) {
	// 1. Setup mock environment
	semanticMap = make(map[string]string)
	semanticMap["_notice_"] = "[green]"
	semanticMap["_applicationname_"] = "[cyan::b]"
	semanticMap["_version_"] = "[cyan]"

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
			input:    "[_Notice_]Hello[-]",
			expected: "Hello",
		},
		{
			name:     "Escaped brackets with semantic",
			input:    "[[[_Notice_]]]", // [[green]] after expansion -> [[]] after strip (escaped brackets preserved)
			expected: "[[]]",
		},
		{
			name:     "Nested brackets (version info case)",
			input:    "App [[_Version_]v2[-]]",
			expected: "App [v2]",
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
			name:     "ANSI sequence (robust strip)",
			input:    "\x1b[36mCyan\x1b[0m",
			expected: "Cyan",
		},
		{
			name:     "Complex ANSI cursor move",
			input:    "Pos\x1b[16;1HAfter",
			expected: "PosAfter",
		},
		{
			name:     "Corrupted ANSI (no ESC byte) - reset code",
			input:    "Text[0m",
			expected: "Text",
		},
		{
			name:     "Corrupted ANSI (no ESC byte) - color code",
			input:    "[[36mText",
			expected: "[Text",
		},
		{
			name:     "Corrupted ANSI (no ESC byte) - cursor position",
			input:    "Before[16;1HAfter",
			expected: "BeforeAfter",
		},
		{
			name:     "Real-world TUI artifact",
			input:    "DockSTARTer-Templates[0m [[36mv1.20260123",
			expected: "DockSTARTer-Templates [v1.20260123",
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

func TestTranslate(t *testing.T) {
	// 1. Setup mock environment
	semanticMap = make(map[string]string)
	semanticMap["_test_blue_"] = "[blue]"
	semanticMap["_applicationname_"] = "[cyan::b]"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Resolve semantic",
			input:    "[_test_blue_]Text[-]",
			expected: "[blue]Text[-]",
		},
		{
			name:     "Direct tag",
			input:    "[_ApplicationName_]",
			expected: "[cyan::b]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ExpandSemanticTags(tt.input)
			if actual != tt.expected {
				t.Errorf("ExpandSemanticTags(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestPrepareForTUI(t *testing.T) {
	// Setup mock environment
	semanticMap = make(map[string]string)
	semanticMap["_notice_"] = "[green]"
	semanticMap["_version_"] = "[cyan]"
	semanticMap["_applicationname_"] = "[cyan::b]"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Semantic to cview (keeps cview tags)",
			input:    "[_Notice_]Message[-]",
			expected: "[green]Message[-]",
		},
		{
			name:     "Remove complete ANSI but keep cview",
			input:    "\x1b[36m[cyan]Text[0m",
			expected: "[cyan]Text",
		},
		{
			name:     "Remove corrupted ANSI but keep cview",
			input:    "[0m[green]Message[-]",
			expected: "[green]Message[-]",
		},
		{
			name:     "Real-world TUI case",
			input:    "DockSTARTer-Templates[0m [[36mv1.20260123",
			expected: "DockSTARTer-Templates [v1.20260123",
		},
		{
			name:     "Semantic + corrupted ANSI",
			input:    "[_Version_][0mv1.20260123",
			expected: "[cyan]v1.20260123",
		},
		{
			name:     "Preserve literal brackets with colors",
			input:    "[green]Version [v2.0][-]",
			expected: "[green]Version [v2.0][-]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := PrepareForTUI(tt.input)
			if actual != tt.expected {
				t.Errorf("PrepareForTUI(%q) = %q; want %q", tt.input, actual, tt.expected)
			}
		})
	}
}
