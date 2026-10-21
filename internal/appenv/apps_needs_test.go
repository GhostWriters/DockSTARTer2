package appenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
)

// writeTemplate creates dir/<app>.yml with the given modification time.
func writeTemplate(t *testing.T, dir, app string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, app+".yml")
	if err := os.WriteFile(file, []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// TestAppTemplateChangedSince checks the change check looks at the template
// DS2 actually uses: the repo copy, a user-only template, or a user override
// of a repo app, whose repo copy is then ignored.
func TestAppTemplateChangedSince(t *testing.T) {
	origConfig, origTemplates := paths.ConfigHomeOverride, paths.TemplatesDirOverride
	paths.ConfigHomeOverride = t.TempDir()
	paths.TemplatesDirOverride = t.TempDir()
	defer func() { paths.ConfigHomeOverride, paths.TemplatesDirOverride = origConfig, origTemplates }()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	edited := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	repoApps := filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName)
	userApps := paths.GetUserAppsDir()

	writeTemplate(t, filepath.Join(repoApps, "repoapp"), "repoapp", edited)
	writeTemplate(t, filepath.Join(userApps, "userapp"), "userapp", edited)
	writeTemplate(t, filepath.Join(userApps, "olduserapp"), "olduserapp", old)
	// A user override that's older than its repo copy: only the override counts.
	writeTemplate(t, filepath.Join(repoApps, "overridden"), "overridden", edited)
	writeTemplate(t, filepath.Join(userApps, "overridden"), "overridden", old)

	for _, tc := range []struct {
		app  string
		want bool
	}{
		{"repoapp", true},
		{"userapp", true},
		{"USERAPP__INSTANCE", true},
		{"olduserapp", false},
		{"overridden", false},
		{"missingapp", false},
	} {
		if got := AppTemplateChangedSince(tc.app, synced); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.app, got, tc.want)
		}
	}
}

// TestTemplateSourceChanged checks adding and removing a user override counts
// as a change even when the newly used files are older than the last sync.
func TestTemplateSourceChanged(t *testing.T) {
	origConfig, origTemplates := paths.ConfigHomeOverride, paths.TemplatesDirOverride
	paths.ConfigHomeOverride = t.TempDir()
	paths.TemplatesDirOverride = t.TempDir()
	defer func() { paths.ConfigHomeOverride, paths.TemplatesDirOverride = origConfig, origTemplates }()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	markers := t.TempDir()
	repoDir := filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, "someapp")
	userDir := filepath.Join(paths.GetUserAppsDir(), "someapp")
	writeTemplate(t, repoDir, "someapp", old)

	if !TemplateSourceChanged(markers, "someapp") {
		t.Error("no record yet: want changed")
	}
	RecordTemplateSource(markers, "someapp")
	if TemplateSourceChanged(markers, "someapp") {
		t.Error("just recorded: want unchanged")
	}

	writeTemplate(t, userDir, "someapp", old)
	if !TemplateSourceChanged(markers, "someapp") {
		t.Error("override added: want changed")
	}
	RecordTemplateSource(markers, "someapp")

	if err := os.RemoveAll(userDir); err != nil {
		t.Fatal(err)
	}
	if !TemplateSourceChanged(markers, "someapp") {
		t.Error("override removed: want changed")
	}
}

// TestIsAnyFileNewerUnreadable checks a missing root has nothing newer, and
// a folder that can't be read counts as newer.
func TestIsAnyFileNewerUnreadable(t *testing.T) {
	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if IsAnyFileNewer(filepath.Join(t.TempDir(), "missing"), since) {
		t.Error("missing root: want not newer")
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("can't make a folder unreadable here")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	writeTemplate(t, locked, "app", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	if !IsAnyFileNewer(root, since) {
		t.Error("unreadable folder: want newer")
	}
}
