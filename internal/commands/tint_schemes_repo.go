package commands

import (
	"context"
	"fmt"
	"os"

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
