package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
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

func init() {
	config.TintRefMigrationHook = CanonicalTintRef
}

// CanonicalTintRef returns ref as it's saved, naming its scheme's system
// the way tinty does: a repo scheme named without one ("repo:dracula", or a
// bare "dracula" from before refs had sources) as the file it loads,
// "repo:base24-dracula" or "repo:base16-dracula", and a user or bundled one
// ("embedded:ansi") with the system its file declares
// ("embedded:base24-ansi"). Only local files are checked, never the
// network; a ref whose file isn't found, or a "file:" ref, is returned
// unchanged.
func CanonicalTintRef(ref string) string {
	if strings.HasPrefix(ref, "file:") {
		return ref
	}
	for _, src := range []struct {
		prefix string
		read   func(string) ([]byte, error)
	}{{"user:", readUserTint}, {"embedded:", assets.GetTintTheme}} {
		name, ok := strings.CutPrefix(ref, src.prefix)
		if !ok {
			continue
		}
		if _, subs := repoSchemeSubfolders(name); len(subs) == 1 {
			return ref
		}
		data, err := src.read(name)
		if err != nil {
			return ref
		}
		meta, err := config.ParseBase16SchemeMeta(data)
		if err != nil || meta.System != "base16" && meta.System != "base24" {
			return ref
		}
		return src.prefix + meta.System + "-" + name
	}
	name := strings.TrimPrefix(ref, "repo:")
	slug, subs := repoSchemeSubfolders(name)
	if len(subs) == 1 {
		return "repo:" + name
	}
	dir := paths.GetTintedThemingSchemesDir()
	for _, sub := range subs {
		if _, err := os.Stat(filepath.Join(dir, sub, slug+".yaml")); err == nil {
			return "repo:" + sub + "-" + slug
		}
	}
	return ref
}

// tintSchemeURL returns the GitHub blob URL of the scheme file sub/slug.yaml.
func tintSchemeURL(sub, slug string) string {
	return fmt.Sprintf("https://github.com/%s/blob/%s/%s/%s.yaml",
		tintedThemingSchemesRepo, tintedThemingSchemesBranch, sub, slug)
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
			return tintSchemeURL(sub, slug), true
		}
	}
	return "", false
}
