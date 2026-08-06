package console

import "testing"

func TestFormatLink(t *testing.T) {
	got := FormatLink("Version", "v1.2.3", "https://example.com/v1.2.3")
	want := "{{|Version::::https://example.com/v1.2.3|}}v1.2.3{{[-]}}"
	if got != want {
		t.Errorf("FormatLink(...) = %q, want %q", got, want)
	}
}

func TestFormatLinkNoURL(t *testing.T) {
	got := FormatLink("Version", "unknown", "")
	want := "{{|Version|}}unknown{{[-]}}"
	if got != want {
		t.Errorf("FormatLink(..., \"\") = %q, want %q", got, want)
	}
}
