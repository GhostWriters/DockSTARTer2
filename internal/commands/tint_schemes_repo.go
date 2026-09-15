package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// tintedThemingSchemesRepo/Branch is the source ResolveRepoTintData clones
// from (--tint's "repo:" reference, and its default).
// Cloned once into DS2's state dir rather than fetched per call, mirroring
// how DockSTARTer-Templates is cloned locally instead of re-fetched on
// every use (see internal/update/update_templates.go).
const (
	tintedThemingSchemesRepo   = "tinted-theming/schemes"
	tintedThemingSchemesBranch = "spec-0.11"
)

// ensureTintedThemingSchemesRepo returns the local clone directory for
// tinted-theming/schemes, cloning it on first use. An existing clone is
// reused as-is -- this never re-fetches on its own; delete the directory
// (paths.GetTintedThemingSchemesDir()) to force a fresh clone.
func ensureTintedThemingSchemesRepo(ctx context.Context) (string, error) {
	dir := paths.GetTintedThemingSchemesDir()
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	url := "https://github.com/" + tintedThemingSchemesRepo
	logger.Notice(ctx, "Running: {{|RunningCommand|}}git clone -b %s %s %s{{[-]}}", tintedThemingSchemesBranch, url, dir)
	logger.Notice(ctx, "\t{{|RunningCommand|}}git:{{[-]}} Cloning into '%s'.", dir)

	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName("refs/heads/" + tintedThemingSchemesBranch),
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("cloning %s: %w", tintedThemingSchemesRepo, err)
	}
	return dir, nil
}

// repoSchemeSubfolders splits a repo scheme name into its slug and, if the
// name carries an explicit "base16-"/"base24-" prefix (the identifier
// format tinted-theming's own reference tool, tinty, uses -- e.g. "tinty
// apply base16-mocha"), the one subfolder to look in. subs is that single
// subfolder, or both in preference order (base24 first, for its real
// distinct bright colors -- see ParseBase16Scheme's doc comment) when name
// has no such prefix.
func repoSchemeSubfolders(name string) (slug string, subs []string) {
	if s, ok := strings.CutPrefix(name, "base24-"); ok {
		return s, []string{"base24"}
	}
	if s, ok := strings.CutPrefix(name, "base16-"); ok {
		return s, []string{"base16"}
	}
	return name, []string{"base24", "base16"}
}

// RepoTintSourceURL returns the GitHub blob URL for name's scheme file --
// for --tint's status display, hyperlinking a "repo:<name>" source to the
// canonical upstream file rather than DS2's own local clone (an
// implementation-detail cache, not somewhere a user has reason to look).
// Checks the same subfolder(s) ResolveRepoTintData resolves the scheme's
// actual data from (see repoSchemeSubfolders), so the link always points at
// whichever file that data really came from. ok is false if name isn't
// found in any of them.
func RepoTintSourceURL(ctx context.Context, name string) (url string, ok bool) {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		return "", false
	}
	slug, subs := repoSchemeSubfolders(name)
	for _, sub := range subs {
		if _, err := os.Stat(filepath.Join(repoDir, sub, slug+".yaml")); err == nil {
			return fmt.Sprintf("https://github.com/%s/blob/%s/%s/%s.yaml",
				tintedThemingSchemesRepo, tintedThemingSchemesBranch, sub, slug), true
		}
	}
	return "", false
}
