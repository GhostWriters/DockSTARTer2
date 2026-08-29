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

	templateUpdated, varsUpdated := GetAppTemplateTimestamps("audiobookshelf")

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
	templateUpdated2, _ := GetAppTemplateTimestamps("AUDIOBOOKSHELF")
	if !templateUpdated2.Equal(templateUpdated) {
		t.Errorf("case-insensitive lookup mismatch: %v vs %v", templateUpdated2, templateUpdated)
	}

	// An app with no template folder should return zero values, not error.
	templateUpdated3, varsUpdated3 := GetAppTemplateTimestamps("doesnotexist")
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
	if _, err := os.Stat(filepath.Join(cacheDir, "audiobookshelf.vars")); err != nil {
		t.Errorf("expected a cached .vars touch file, got: %v", err)
	}
	cachedTemplate, cachedVars := GetAppTemplateTimestamps("audiobookshelf")
	if !cachedTemplate.Equal(templateUpdated) || !cachedVars.Equal(varsUpdated) {
		t.Errorf("cached read = %v/%v, want %v/%v", cachedTemplate, cachedVars, templateUpdated, varsUpdated)
	}

	// A new commit (moving HEAD) must invalidate the cache: re-run against
	// a fresh commit and confirm the stale cached value isn't returned.
	commitFile(t, repoDir, wt, ".apps/audiobookshelf/docker-compose.yml", "services: {again: true}", base.AddDate(0, 0, 4))
	templateUpdated4, _ := GetAppTemplateTimestamps("audiobookshelf")
	if templateUpdated4.Equal(templateUpdated) {
		t.Errorf("expected cache invalidation after a new commit moved HEAD, still got the stale %v", templateUpdated4)
	}
	if !templateUpdated4.Local().Truncate(time.Second).Equal(base.AddDate(0, 0, 4).Local().Truncate(time.Second)) {
		t.Errorf("templateUpdated after cache invalidation = %v, want %v", templateUpdated4, base.AddDate(0, 0, 4))
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

	templateUpdated, varsUpdated := GetAppTemplateTimestamps("anyapp")
	if !templateUpdated.IsZero() || !varsUpdated.IsZero() {
		t.Errorf("expected zero timestamps when templates dir isn't a git checkout, got %v / %v", templateUpdated, varsUpdated)
	}
}
