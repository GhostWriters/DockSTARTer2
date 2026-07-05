package console

import (
	"path/filepath"
	"strings"
	"testing"

	"DockSTARTer2/internal/strutil"
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

func TestFormatFilePath(t *testing.T) {
	sep := string(filepath.Separator)
	path := nativePath("home", "clhatch", ".config", "compose", ".env")

	withViaOwnServer(t, false, func() {
		got := FormatFilePath(path)
		for _, want := range []string{
			"{{|Folder::::file:///home|}}home{{[-]}}",
			"{{|Folder::::file:///home/clhatch|}}clhatch{{[-]}}",
			"{{|Folder::::file:///home/clhatch/.config|}}.config{{[-]}}",
			"{{|Folder::::file:///home/clhatch/.config/compose|}}compose{{[-]}}",
			"{{|File::::file:///home/clhatch/.config/compose/.env|}}.env{{[-]}}",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatFilePath(%q) missing segment %q, got %q", path, want, got)
			}
		}
		// Separators must be styled (matching the segment they precede) but
		// never wrapped in a hyperlink of their own.
		if !strings.Contains(got, "{{|Folder|}}"+sep+"{{[-]}}") {
			t.Errorf("FormatFilePath(%q) should style '%s' separators as plain Folder tags, got %q", path, sep, got)
		}
		if !strings.Contains(got, "{{|File|}}"+sep+"{{[-]}}") {
			t.Errorf("FormatFilePath(%q) should style the separator before the filename as a plain File tag, got %q", path, got)
		}
	})

	withViaOwnServer(t, true, func() {
		got := FormatFilePath(path)
		want := "{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}home{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}clhatch{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}.config{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}compose{{[-]}}" +
			"{{|File|}}" + sep + "{{[-]}}{{|File|}}.env{{[-]}}"
		if got != want {
			t.Errorf("viaOwnServer FormatFilePath(%q) = %q, want %q (no URL param)", path, got, want)
		}
	})
}

func TestFormatFolderPath(t *testing.T) {
	sep := string(filepath.Separator)
	path := nativePath("home", "clhatch", ".config", "appdata")

	withViaOwnServer(t, false, func() {
		got := FormatFolderPath(path)
		for _, want := range []string{
			"{{|Folder::::file:///home|}}home{{[-]}}",
			"{{|Folder::::file:///home/clhatch|}}clhatch{{[-]}}",
			"{{|Folder::::file:///home/clhatch/.config|}}.config{{[-]}}",
			"{{|Folder::::file:///home/clhatch/.config/appdata|}}appdata{{[-]}}",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatFolderPath(%q) missing segment %q, got %q", path, want, got)
			}
		}
		if strings.Contains(got, "{{|File") {
			t.Errorf("FormatFolderPath(%q) should never emit a File tag, got %q", path, got)
		}
	})

	withViaOwnServer(t, true, func() {
		got := FormatFolderPath(path)
		want := "{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}home{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}clhatch{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}.config{{[-]}}" +
			"{{|Folder|}}" + sep + "{{[-]}}{{|Folder|}}appdata{{[-]}}"
		if got != want {
			t.Errorf("viaOwnServer FormatFolderPath(%q) = %q, want %q (no URL param)", path, got, want)
		}
	})
}

func TestFormatFileName(t *testing.T) {
	path := nativePath("tmp", "ds2.global.abc123.tmp")

	withViaOwnServer(t, false, func() {
		got := FormatFileName(".env", path)
		want := "{{|File::::" + strutil.FileURL(path) + "|}}.env{{[-]}}"
		if got != want {
			t.Errorf("FormatFileName(...) = %q, want %q", got, want)
		}
	})

	withViaOwnServer(t, true, func() {
		got := FormatFileName(".env", path)
		want := "{{|File|}}.env{{[-]}}"
		if got != want {
			t.Errorf("viaOwnServer FormatFileName(...) = %q, want %q", got, want)
		}
	})

	// An empty path means no real location is known -- style only, no link.
	got := FormatFileName(".env", "")
	want := "{{|File|}}.env{{[-]}}"
	if got != want {
		t.Errorf("FormatFileName(name, \"\") = %q, want %q", got, want)
	}
}

func TestFormatFolderName(t *testing.T) {
	path := nativePath("home", "clhatch", "appdata")

	withViaOwnServer(t, false, func() {
		got := FormatFolderName("appdata", path)
		want := "{{|Folder::::" + strutil.FileURL(path) + "|}}appdata{{[-]}}"
		if got != want {
			t.Errorf("FormatFolderName(...) = %q, want %q", got, want)
		}
	})

	got := FormatFolderName("appdata", "")
	want := "{{|Folder|}}appdata{{[-]}}"
	if got != want {
		t.Errorf("FormatFolderName(name, \"\") = %q, want %q", got, want)
	}
}
