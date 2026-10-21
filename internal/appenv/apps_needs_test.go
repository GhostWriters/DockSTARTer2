package appenv

import (
	"os"
	"path/filepath"
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
