package appenv

import "testing"

func TestStripFrontMatter(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"front matter", "---\nstatus: deprecated\n---\n# Airsonic\n\nText\n", "# Airsonic\n\nText\n"},
		{"blank lines after", "---\na: b\n---\n\n\n# Title\n", "# Title\n"},
		{"dots close", "---\na: b\n...\n# Title\n", "# Title\n"},
		{"CRLF", "---\r\na: b\r\n---\r\n# Title\r\n", "# Title\r\n"},
		{"none", "# Title\n\n---\n\nMore\n", "# Title\n\n---\n\nMore\n"},
		{"unclosed", "---\na: b\n# Title\n", "---\na: b\n# Title\n"},
		{"rule later only", "Intro\n---\nx\n---\n", "Intro\n---\nx\n---\n"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		if got := stripFrontMatter(tt.in); got != tt.want {
			t.Errorf("%s: stripFrontMatter(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
		}
	}
}
