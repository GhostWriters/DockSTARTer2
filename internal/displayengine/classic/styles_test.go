package classic

import (
	"strings"
	"testing"
)

func TestStripHyperlinks(t *testing.T) {
	const (
		bel = "\x07"
		st  = "\x1b\\"
	)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no hyperlink, unchanged",
			input: "plain text with no links",
			want:  "plain text with no links",
		},
		{
			name:  "simple hyperlink, BEL terminator",
			input: "\x1b]8;;https://dockstarter.com" + bel + "label" + "\x1b]8;;" + bel,
			want:  "label",
		},
		{
			name:  "simple hyperlink, ST terminator",
			input: "\x1b]8;;https://dockstarter.com" + st + "label" + "\x1b]8;;" + st,
			want:  "label",
		},
		{
			name:  "styled label keeps its SGR codes",
			input: "\x1b]8;;file:///etc/dockstarter/docker-compose.yml" + bel + "\x1b[31mdocker-compose.yml\x1b[0m" + "\x1b]8;;" + bel,
			want:  "\x1b[31mdocker-compose.yml\x1b[0m",
		},
		{
			name:  "surrounding plain text preserved",
			input: "See " + "\x1b]8;;file:///a.yml" + bel + "a.yml" + "\x1b]8;;" + bel + " for details.",
			want:  "See a.yml for details.",
		},
		{
			name: "multiple hyperlinks in one string",
			input: "\x1b]8;;file:///a.yml" + bel + "a.yml" + "\x1b]8;;" + bel +
				" and " +
				"\x1b]8;;file:///b.env" + bel + "b.env" + "\x1b]8;;" + bel,
			want: "a.yml and b.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHyperlinks(tt.input)
			if got != tt.want {
				t.Errorf("StripHyperlinks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHyperlinkPath(t *testing.T) {
	const style = "{{[white::]}}"

	out := HyperlinkPath(style, "/etc/dockstarter/docker-compose.yml")
	if !strings.Contains(out, "file:///etc/dockstarter/docker-compose.yml") {
		t.Errorf("HyperlinkPath output missing expected file:// URL, got %q", out)
	}
	if !strings.Contains(out, "docker-compose.yml") {
		t.Errorf("HyperlinkPath output missing path as label, got %q", out)
	}
	// The label survives stripping the hyperlink wrapper -- it's still
	// wrapped in the style's own SGR color codes, so check containment
	// rather than exact equality.
	if got := StripHyperlinks(out); !strings.Contains(got, "/etc/dockstarter/docker-compose.yml") {
		t.Errorf("StripHyperlinks(HyperlinkPath(...)) = %q, want it to contain the plain path", got)
	}
}

func TestHyperlinkPathEscapesSpecialChars(t *testing.T) {
	const style = "{{[white::]}}"

	// The URL destination must be percent-encoded, but the visible label
	// should stay the literal, unescaped path.
	out := HyperlinkPath(style, "/mnt/my data/file name.txt")
	if !strings.Contains(out, "file:///mnt/my%20data/file%20name.txt") {
		t.Errorf("HyperlinkPath did not percent-encode spaces in the URL, got %q", out)
	}
	if got := StripHyperlinks(out); !strings.Contains(got, "/mnt/my data/file name.txt") {
		t.Errorf("label should stay unescaped, got %q", got)
	}
}

func TestHyperlinkText(t *testing.T) {
	const style = "{{[white::]}}"

	out := HyperlinkText(style, "/etc/dockstarter/docker-compose.yml", "compose file")
	if !strings.Contains(out, "file:///etc/dockstarter/docker-compose.yml") {
		t.Errorf("HyperlinkText output missing expected file:// URL, got %q", out)
	}
	if got := StripHyperlinks(out); !strings.Contains(got, "compose file") {
		t.Errorf("StripHyperlinks(HyperlinkText(...)) = %q, want it to contain the custom label", got)
	}
}

func TestHyperlinkPathMultiSegment(t *testing.T) {
	// Each path component gets its own independent hyperlink pointing at its
	// own cumulative path, so e.g. clicking "dockstarter" opens that folder
	// while clicking "docker-compose.yml" opens that specific file -- built
	// with HyperlinkText per segment (label = segment name, target =
	// cumulative path), the composition HyperlinkPath is meant to support.
	const style = "{{[white::]}}"
	joined := HyperlinkText(style, "/etc", "etc") + "/" +
		HyperlinkText(style, "/etc/dockstarter", "dockstarter") + "/" +
		HyperlinkText(style, "/etc/dockstarter/docker-compose.yml", "docker-compose.yml")

	got := StripHyperlinks(joined)
	for _, want := range []string{"etc", "dockstarter", "docker-compose.yml"} {
		if !strings.Contains(got, want) {
			t.Errorf("StripHyperlinks(joined multi-segment path) = %q, want it to contain %q", got, want)
		}
	}
	if strings.Count(joined, "\x1b]8;;") != 6 { // 1 open + 1 close per segment, 3 segments
		t.Errorf("expected three independent hyperlink spans, got: %q", joined)
	}
}
