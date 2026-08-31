package console

import (
	"path/filepath"
	"strings"
	"testing"
)

func withViaOwnServer(t *testing.T, v bool, fn func()) {
	t.Helper()
	orig := IsViaOwnServer()
	SetViaOwnServer(v)
	defer SetViaOwnServer(orig)
	fn()
}

// nativePath joins segments using the OS-native separator, mirroring how a
// real path would look on whichever OS the test happens to run on.
func nativePath(segments ...string) string {
	return string(filepath.Separator) + filepath.Join(segments...)
}

// The detailed per-segment/per-mode formatting logic now lives in semstyle
// (see semstyle.FormatFilePath and friends) -- these tests only check that
// DS2's wrappers delegate correctly and that blocksHyperlink is actually
// wired into semstyle.HyperlinkEligibleFunc (see profile.go's init).

func TestFormatFilePathDelegatesAndRespectsViaOwnServer(t *testing.T) {
	path := nativePath("home", "clhatch", ".config", "compose", ".env")

	withViaOwnServer(t, false, func() {
		got := FormatFilePath(path)
		if !strings.Contains(got, "file:///home/clhatch/.config/compose/.env") {
			t.Errorf("FormatFilePath(%q) should include a file:// URL when not blocked, got %q", path, got)
		}
	})

	withViaOwnServer(t, true, func() {
		got := FormatFilePath(path)
		if strings.Contains(got, "file://") {
			t.Errorf("FormatFilePath(%q) should omit the URL when blocksHyperlink is true, got %q", path, got)
		}
		if !strings.Contains(got, ".env") {
			t.Errorf("FormatFilePath(%q) should still show the plain path text, got %q", path, got)
		}
	})
}

func TestFormatFileNameDelegates(t *testing.T) {
	path := nativePath("tmp", "ds2.global.abc123.tmp")

	withViaOwnServer(t, false, func() {
		got := FormatFileName(".env", path)
		if !strings.Contains(got, ".env") || !strings.Contains(got, "file://") {
			t.Errorf("FormatFileName(...) = %q, want label + file:// URL", got)
		}
	})

	got := FormatFileName(".env", "")
	want := "{{|File|}}.env{{[-]}}"
	if got != want {
		t.Errorf("FormatFileName(name, \"\") = %q, want %q", got, want)
	}
}

func TestFormatLinkDelegates(t *testing.T) {
	got := FormatLink("Var", "v1.0.0", "https://example.com/releases/v1.0.0")
	if !strings.Contains(got, "v1.0.0") || !strings.Contains(got, "https://example.com/releases/v1.0.0") {
		t.Errorf("FormatLink(...) = %q, want label + url in the raw tag markup", got)
	}
	if got != "{{|Var::::https://example.com/releases/v1.0.0|}}v1.0.0{{[-]}}" {
		t.Errorf("FormatLink(...) = %q, want the standard explicit-url tag form", got)
	}
}
