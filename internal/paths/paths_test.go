package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitFile writes content to a file (creating parent dirs) inside repoDir,
// stages it, and commits it at the given time.
func commitFile(t *testing.T, repoDir string, wt *git.Worktree, relPath, content string, when time.Time) {
	t.Helper()
	full := filepath.Join(repoDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(relPath); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: when}
	if _, err := wt.Commit("update "+relPath, &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}
}

func TestGetAppTemplateTimestamps(t *testing.T) {
	repoDir := t.TempDir()
	r, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Oldest: create the compose file for audiobookshelf.
	commitFile(t, repoDir, wt, ".apps/audiobookshelf/docker-compose.yml", "services: {}", base)
	// Middle: create its var file.
	commitFile(t, repoDir, wt, ".apps/audiobookshelf/.env", "AUDIOBOOKSHELF__ENABLED=false", base.AddDate(0, 0, 1))
	// Newest: a non-var template file changes (compose.yml touched again).
	commitFile(t, repoDir, wt, ".apps/audiobookshelf/docker-compose.yml", "services: {updated: true}", base.AddDate(0, 0, 2))
	// Unrelated app, should not affect audiobookshelf's timestamps.
	commitFile(t, repoDir, wt, ".apps/otherapp/.env", "OTHERAPP__ENABLED=false", base.AddDate(0, 0, 3))

	origOverride := TemplatesDirOverride
	TemplatesDirOverride = repoDir
	origState := StateHomeOverride
	StateHomeOverride = t.TempDir()
	defer func() {
		TemplatesDirOverride = origOverride
		StateHomeOverride = origState
	}()

	templateUpdated, varsUpdated := GetAppTemplateTimestamps("audiobookshelf", ".env")

	if templateUpdated.IsZero() {
		t.Fatal("expected non-zero templateUpdated")
	}
	if !templateUpdated.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 2).Local().Truncate(time.Second)) {
		t.Errorf("templateUpdated = %v, want %v (the newest commit, a compose.yml change)", templateUpdated, base.AddDate(0, 0, 2))
	}

	if varsUpdated.IsZero() {
		t.Fatal("expected non-zero varsUpdated")
	}
	if !varsUpdated.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 1).Local().Truncate(time.Second)) {
		t.Errorf("varsUpdated = %v, want %v (only the .env commit, not the later compose.yml-only commit)", varsUpdated, base.AddDate(0, 0, 1))
	}

	// A case-different app name should still match (folder names are lowercase).
	templateUpdated2, _ := GetAppTemplateTimestamps("AUDIOBOOKSHELF", ".env")
	if !templateUpdated2.Equal(templateUpdated) {
		t.Errorf("case-insensitive lookup mismatch: %v vs %v", templateUpdated2, templateUpdated)
	}

	// An app with no template folder should return zero values, not error.
	templateUpdated3, varsUpdated3 := GetAppTemplateTimestamps("doesnotexist", ".env")
	if !templateUpdated3.IsZero() || !varsUpdated3.IsZero() {
		t.Errorf("expected zero timestamps for a nonexistent app, got %v / %v", templateUpdated3, varsUpdated3)
	}

	// Cache round-trip: the on-disk touch files should now exist and, on
	// a fresh call, be read back byte-for-byte the same as what the walk
	// computed (mtime encoding survives a write/read cycle).
	cacheDir := appTimestampCacheDir()
	if _, err := os.Stat(filepath.Join(cacheDir, "audiobookshelf.template")); err != nil {
		t.Errorf("expected a cached .template touch file, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, varsCacheFileName("audiobookshelf", ".env"))); err != nil {
		t.Errorf("expected a cached vars touch file, got: %v", err)
	}
	cachedTemplate, cachedVars := GetAppTemplateTimestamps("audiobookshelf", ".env")
	if !cachedTemplate.Equal(templateUpdated) || !cachedVars.Equal(varsUpdated) {
		t.Errorf("cached read = %v/%v, want %v/%v", cachedTemplate, cachedVars, templateUpdated, varsUpdated)
	}

	// A new commit (moving HEAD) must invalidate the cache: re-run against
	// a fresh commit and confirm the stale cached value isn't returned.
	commitFile(t, repoDir, wt, ".apps/audiobookshelf/docker-compose.yml", "services: {again: true}", base.AddDate(0, 0, 4))
	templateUpdated4, _ := GetAppTemplateTimestamps("audiobookshelf", ".env")
	if templateUpdated4.Equal(templateUpdated) {
		t.Errorf("expected cache invalidation after a new commit moved HEAD, still got the stale %v", templateUpdated4)
	}
	if !templateUpdated4.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 4).Local().Truncate(time.Second)) {
		t.Errorf("templateUpdated after cache invalidation = %v, want %v", templateUpdated4, base.AddDate(0, 0, 4))
	}
}

// TestGetAppTemplateTimestamps_PerFileScoping ensures "Vars updated" is
// scoped to the one specific var file requested, not "any var file
// anywhere under the app's folder" -- a multi-service app has several
// (immich's .env, .env.app.immich-database, .env.app.immich___postgres),
// and a change to one must not make an unrelated one look changed too.
func TestGetAppTemplateTimestamps_PerFileScoping(t *testing.T) {
	repoDir := t.TempDir()
	r, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	commitFile(t, repoDir, wt, ".apps/immich/.env", "IMMICH__ENABLED=false", base)
	commitFile(t, repoDir, wt, ".apps/immich/.env.app.immich-database", "DB_HOST=''", base.AddDate(0, 0, 1))
	// Only the .env file changes again -- the -database file's own content is untouched.
	commitFile(t, repoDir, wt, ".apps/immich/.env", "IMMICH__ENABLED=true", base.AddDate(0, 0, 2))

	origOverride := TemplatesDirOverride
	TemplatesDirOverride = repoDir
	origState := StateHomeOverride
	StateHomeOverride = t.TempDir()
	defer func() {
		TemplatesDirOverride = origOverride
		StateHomeOverride = origState
	}()

	_, envVars := GetAppTemplateTimestamps("immich", ".env")
	_, dbVars := GetAppTemplateTimestamps("immich", ".env.app.immich-database")

	if !envVars.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 2).Local().Truncate(time.Second)) {
		t.Errorf(".env vars timestamp = %v, want %v (its own latest change)", envVars, base.AddDate(0, 0, 2))
	}
	if !dbVars.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 1).Local().Truncate(time.Second)) {
		t.Errorf(".env.app.immich-database vars timestamp = %v, want %v (unaffected by the later .env-only change)", dbVars, base.AddDate(0, 0, 1))
	}
	if envVars.Equal(dbVars) {
		t.Errorf("the two files' vars timestamps should differ, both got %v", envVars)
	}
}

func TestGetAppTemplateTimestamps_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	origOverride := TemplatesDirOverride
	TemplatesDirOverride = dir
	origState := StateHomeOverride
	StateHomeOverride = t.TempDir()
	defer func() {
		TemplatesDirOverride = origOverride
		StateHomeOverride = origState
	}()

	templateUpdated, varsUpdated := GetAppTemplateTimestamps("anyapp", ".env")
	if !templateUpdated.IsZero() || !varsUpdated.IsZero() {
		t.Errorf("expected zero timestamps when templates dir isn't a git checkout, got %v / %v", templateUpdated, varsUpdated)
	}
}
