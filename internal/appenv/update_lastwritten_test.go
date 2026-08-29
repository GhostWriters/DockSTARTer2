package appenv

import (
	"strings"
	"testing"
	"time"
)

func TestStampLastWritten(t *testing.T) {
	original := []string{fileHeaderLines("/home/user/.config/compose/.env", time.Time{})[0], "", "ALLOW_CORS=1"}
	lines := stampLastWritten(original, time.Date(2026, 8, 29, 1, 23, 45, 0, time.Local))

	if len(lines) != 4 {
		t.Fatalf("expected one line inserted, got: %v", lines)
	}
	if lines[0] != original[0] {
		t.Errorf("lines[0] (File:) must be unchanged, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "### Last written: ") {
		t.Errorf("lines[1] should be the new Last written line, got %q", lines[1])
	}
	if lines[2] != "" || lines[3] != "ALLOW_CORS=1" {
		t.Errorf("everything after the inserted line should shift down unchanged, got: %v", lines)
	}
	// original must not be mutated.
	if len(original) != 3 {
		t.Errorf("stampLastWritten must not mutate its input, got: %v", original)
	}
}

func TestStampLastWritten_EmptyLines(t *testing.T) {
	var lines []string
	result := stampLastWritten(lines, time.Now()) // must not panic
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got: %v", result)
	}
}

func TestStampLastWritten_AlignsWithFileLine(t *testing.T) {
	lines := fileHeaderLines("/home/user/.config/compose/.env", time.Time{})
	lines = stampLastWritten(lines, time.Date(2026, 8, 29, 1, 23, 45, 0, time.Local))

	fileIdx := strings.Index(lines[0], "/home")
	writtenIdx := strings.Index(lines[1], "20") // year prefix of the timestamp
	if fileIdx != writtenIdx {
		t.Errorf("values should start at the same column: File value at %d, Last written value at %d\nline0=%q\nline1=%q", fileIdx, writtenIdx, lines[0], lines[1])
	}
}

func TestStripLastWrittenLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "strips a present Last written line",
			content: "### File:          /path/.env\n### Last written:   2026-08-29 01:23:45\n\nALLOW_CORS=1\n",
			want:    "### File:          /path/.env\n\nALLOW_CORS=1\n",
		},
		{
			name:    "no Last written line present -- unchanged",
			content: "### File:          /path/.env\n\nALLOW_CORS=1\n",
			want:    "### File:          /path/.env\n\nALLOW_CORS=1\n",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "single line content -- unchanged (no line[1] to check)",
			content: "### File:          /path/.env",
			want:    "### File:          /path/.env",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripLastWrittenLine(tt.content); got != tt.want {
				t.Errorf("stripLastWrittenLine(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// TestStampThenStrip_RoundTrips ensures the pair used by the write-comparison
// logic in Update() actually round-trips: stamping and then stripping must
// reproduce the original unstamped content, so the "did content actually
// change" comparison isn't fooled by the inserted line's own ever-changing
// timestamp.
func TestStampThenStrip_RoundTrips(t *testing.T) {
	original := []string{fileHeaderLines("/path/.env", time.Time{})[0], "", "ALLOW_CORS=1"}
	stamped := stampLastWritten(original, time.Date(2026, 8, 29, 1, 23, 45, 0, time.Local))

	joined := strings.Join(stamped, "\n")
	stripped := stripLastWrittenLine(joined)

	wantJoined := strings.Join(original, "\n")
	if stripped != wantJoined {
		t.Errorf("round-trip mismatch: got %q, want %q", stripped, wantJoined)
	}
}
