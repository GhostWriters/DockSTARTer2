package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// TemplatesUpdateInfo holds the result of CheckTemplatesUpdate.
// Pass it to ApplyTemplatesUpdate to perform the actual update without re-fetching.
type TemplatesUpdateInfo struct {
	HasUpdate       bool
	CurrentDisplay  string
	RemoteDisplay   string
	repo            *git.Repository
	remoteRef       *plumbing.Reference
	requestedBranch string
	force           bool
}

// CheckTemplatesUpdate fetches remote state and returns whether an update is available.
// If force is true, HasUpdate is true even when already up to date.
func CheckTemplatesUpdate(ctx context.Context, force bool, requestedBranch string) (*TemplatesUpdateInfo, error) {
	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("templates directory not found at %s", templatesDir)
	}

	repo, err := git.PlainOpen(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open templates repo: %w", err)
	}

	if requestedBranch == "" {
		if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
			requestedBranch = head.Name().Short()
		} else {
			requestedBranch = "main"
		}
	}

	logger.Info(ctx, "Setting file ownership on current repository files")
	system.SetPermissions(ctx, templatesDir)
	logger.Info(ctx, "Fetching recent changes from git.")
	logger.Info(ctx, "Running: {{|RunningCommand|}}git fetch --all --prune -v{{[-]}}")
	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
	})
	if err == nil || err == git.NoErrAlreadyUpToDate {
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} POST git-upload-pack (186 bytes)")
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} From https://github.com/GhostWriters/DockSTARTer-Templates")
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}}  = [up to date]      %-10s -> origin/%s", requestedBranch, requestedBranch)
	}
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch templates: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get templates HEAD: %w", err)
	}
	currentHash := head.Hash().String()[:7]
	currentDisplay := paths.GetTemplatesVersion()
	currentBranch := ""
	if head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	remoteRef, remoteDisplay, err := resolveTemplatesTarget(repo, head, currentBranch, requestedBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve templates target %s: %w", requestedBranch, err)
	}
	remoteHash := remoteRef.Hash().String()
	if len(remoteHash) > 7 {
		remoteHash = remoteHash[:7]
	}

	hasUpdate := currentHash != remoteHash || force
	return &TemplatesUpdateInfo{
		HasUpdate:       hasUpdate,
		CurrentDisplay:  currentDisplay,
		RemoteDisplay:   remoteDisplay,
		repo:            repo,
		remoteRef:       remoteRef,
		requestedBranch: requestedBranch,
		force:           force,
	}, nil
}

// resolveTemplatesTarget determines the remote ref to update the templates
// repo to, and its display string (tag name, or "<branch> commit <hash>").
//
// When the resolved branch is "main" (whether auto-detected from HEAD, or
// explicitly named -- naming the branch is "pick which branch to track",
// not "opt out of the release policy"), this applies a release policy
// instead of main's literal tip: renovate and other CI commits land on main
// between releases, so main's tip is frequently NOT the commit the user was
// actually on -- restrict to the latest tag reachable from origin/main's
// history instead. If no such tag exists yet (e.g. a fresh repo before any
// release), falls back to main's tip.
//
// If currentBranch already equals requestedBranch (i.e. this is an update
// check/apply while staying on the same branch, not a switch onto main from
// elsewhere) and the resolved target is an ancestor of (or equal to)
// current HEAD -- e.g. the user is already ahead of the latest tag -- this
// returns current HEAD itself as the target, so callers see "no update
// available" rather than incorrectly offering to move backward to an older
// tag. This check is skipped when switching branches: a different branch
// (e.g. one that forked from main after the latest tag) being a descendant
// of that tag must never block the switch itself.
//
// Only an explicit literal tag name or commit hash bypasses this policy --
// those resolve via the refs/tags/ or raw-hash fallback below and are
// never equal to the branch name "main", so they naturally skip it.
//
// head is the repo's current HEAD reference (already resolved by the
// caller), used only for the ancestor-of-latest-tag check above.
func resolveTemplatesTarget(repo *git.Repository, head *plumbing.Reference, currentBranch, requestedBranch string) (*plumbing.Reference, string, error) {
	remoteRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+requestedBranch), true)
	if err != nil {
		remoteRef, err = repo.Reference(plumbing.ReferenceName("refs/tags/"+requestedBranch), true)
	}
	if err != nil {
		return nil, "", err
	}

	if requestedBranch == "main" {
		if tagRef, tagName, ok := latestReachableTag(repo, remoteRef); ok {
			if currentBranch == requestedBranch {
				if ahead, err := isAncestorOrEqual(repo, tagRef, head); err == nil && ahead {
					// Current HEAD is already at or ahead of the latest tag --
					// report current HEAD as the target so callers see no update.
					return head, paths.GetTemplatesVersion(), nil
				}
			}
			return tagRef, tagName, nil
		}
		// No reachable tag yet -- fall back to main's tip (old behavior).
	}

	remoteDisplay := templatesRefDisplay(repo, requestedBranch, remoteRef)
	return remoteRef, remoteDisplay, nil
}

// templatesRefDisplay returns a tag name if one points at remoteRef's
// commit, otherwise "<branch> commit <hash>".
func templatesRefDisplay(repo *git.Repository, requestedBranch string, remoteRef *plumbing.Reference) string {
	tags, _ := repo.Tags()
	foundTag := ""
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == remoteRef.Hash() {
			foundTag = ref.Name().Short()
			return fmt.Errorf("found")
		}
		return nil
	})
	if foundTag != "" {
		return foundTag
	}
	remoteHash := remoteRef.Hash().String()
	if len(remoteHash) > 7 {
		remoteHash = remoteHash[:7]
	}
	return fmt.Sprintf("%s commit %s", requestedBranch, remoteHash)
}

// latestReachableTag returns the highest-versioned tag (by compareVersions,
// the same semver-aware comparison used for app-update channel selection)
// that is an ancestor of (or equal to) branchRef's commit. ok is false if no
// tag reaches branchRef at all.
func latestReachableTag(repo *git.Repository, branchRef *plumbing.Reference) (ref *plumbing.Reference, name string, ok bool) {
	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return nil, "", false
	}

	tags, err := repo.Tags()
	if err != nil {
		return nil, "", false
	}

	var bestRef *plumbing.Reference
	bestName := ""
	_ = tags.ForEach(func(tagRef *plumbing.Reference) error {
		tagCommit, err := repo.CommitObject(tagRef.Hash())
		if err != nil {
			return nil
		}
		reachable, err := tagCommit.IsAncestor(branchCommit)
		if err != nil || (!reachable && tagCommit.Hash != branchCommit.Hash) {
			return nil
		}
		tagName := tagRef.Name().Short()
		if bestRef == nil || compareVersions(tagName, bestName) > 0 {
			bestRef = tagRef
			bestName = tagName
		}
		return nil
	})

	if bestRef == nil {
		return nil, "", false
	}
	return bestRef, bestName, true
}

// isAncestorOrEqual reports whether ancestorRef's commit is the same as, or
// a git-history ancestor of, descendantRef's commit.
func isAncestorOrEqual(repo *git.Repository, ancestorRef, descendantRef *plumbing.Reference) (bool, error) {
	if ancestorRef.Hash() == descendantRef.Hash() {
		return true, nil
	}
	ancestorCommit, err := repo.CommitObject(ancestorRef.Hash())
	if err != nil {
		return false, err
	}
	descendantCommit, err := repo.CommitObject(descendantRef.Hash())
	if err != nil {
		return false, err
	}
	return ancestorCommit.IsAncestor(descendantCommit)
}

// ApplyTemplatesUpdate prompts and applies the update described by info.
// Call CheckTemplatesUpdate first to obtain info.
func ApplyTemplatesUpdate(ctx context.Context, info *TemplatesUpdateInfo, yes bool) error {
	targetName := "DockSTARTer-Templates"
	noNotice := fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} will not be updated.", targetName)

	var question, initiationNotice string
	if !info.HasUpdate {
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on branch '%s'.", targetName, TmplBranchLink(info.requestedBranch))
		logger.Notice(ctx, "Current version is '%s'", TmplVersionLink(info.CurrentDisplay))
		return nil
	}

	if info.force && info.CurrentDisplay == info.RemoteDisplay {
		question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '%s'?", targetName, TmplVersionLink(info.CurrentDisplay))
		initiationNotice = fmt.Sprintf("Forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '%s'", targetName, TmplVersionLink(info.RemoteDisplay))
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s' now?", targetName, TmplVersionLink(info.CurrentDisplay), TmplVersionLink(info.RemoteDisplay))
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s'", targetName, TmplVersionLink(info.CurrentDisplay), TmplVersionLink(info.RemoteDisplay))
	}

	noticePrinter := func(ctx context.Context, msg any, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	answer, err := console.QuestionPrompt(ctx, noticePrinter, "Update", question, "Y", yes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, initiationNotice)
	w, err := info.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get templates worktree: %w", err)
	}

	logger.Info(ctx, "Running: {{|RunningCommand|}}git checkout --force %s{{[-]}}", info.requestedBranch)
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(info.requestedBranch),
		Force:  true,
	})
	if err != nil {
		err = w.Checkout(&git.CheckoutOptions{
			Hash:   info.remoteRef.Hash(),
			Branch: plumbing.NewBranchReferenceName(info.requestedBranch),
			Create: true,
			Force:  true,
		})
	}
	if err != nil {
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewTagReferenceName(info.requestedBranch),
			Force:  true,
		})
	}
	if err != nil {
		err = w.Checkout(&git.CheckoutOptions{
			Hash:  info.remoteRef.Hash(),
			Force: true,
		})
	}
	if err == nil {
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Already on '%s'", info.requestedBranch)
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Your branch is up to date with 'origin/%s'.", info.requestedBranch)
	}

	if err != nil {
		logger.Info(ctx, "Pulling recent changes from git.")
		logger.Info(ctx, "Running: {{|RunningCommand|}}git pull{{[-]}}")
		err = w.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName("refs/heads/" + info.requestedBranch),
		})
	} else {
		logger.Info(ctx, "Pulling recent changes from git.")
		hash := info.remoteRef.Hash().String()[:7]
		logger.Info(ctx, "Running: {{|RunningCommand|}}git reset --hard %s{{[-]}}", hash)
		err = w.Reset(&git.ResetOptions{
			Mode:   git.HardReset,
			Commit: info.remoteRef.Hash(),
		})
	}

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to update templates to %s: %w", info.requestedBranch, err)
	}

	if err == nil {
		newHead, _ := info.repo.Head()
		if newHead != nil {
			commit, _ := info.repo.CommitObject(newHead.Hash())
			if commit != nil {
				subject := strings.Split(commit.Message, "\n")[0]
				hash := newHead.Hash().String()[:7]
				logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} HEAD is now at %s %s", hash, subject)
			} else {
				logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Already up to date.")
			}
		}
		logger.Info(ctx, "Cleaning up unnecessary files and optimizing the local repository.")
		if gitPath, err := exec.LookPath("git"); err == nil {
			logger.Info(ctx, "Running: {{|RunningCommand|}}git maintenance run{{[-]}}")
			_ = exec.CommandContext(ctx, gitPath, "-C", paths.GetTemplatesDir(), "maintenance", "run").Run()
		}
		logger.Info(ctx, "Setting file ownership on new repository files")
		system.SetPermissions(ctx, paths.GetTemplatesDir())
	}

	paths.InvalidateTemplatesVersionCache()
	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '%s'", targetName, TmplVersionLink(paths.GetTemplatesVersion()))
	appenv.InvalidateAppMetaCache()
	system.SetPermissions(ctx, paths.GetTimestampsDir())

	return nil
}

// UpdateTemplates handles updating the templates directory.
func UpdateTemplates(ctx context.Context, force bool, yes bool, requestedBranch string) error {
	info, err := CheckTemplatesUpdate(ctx, force, requestedBranch)
	if err != nil {
		return err
	}
	return ApplyTemplatesUpdate(ctx, info, yes)
}

// EnsureTemplates checks if the templates directory exists and clones it if missing.
func EnsureTemplates(ctx context.Context) error {
	templatesDir := paths.GetTemplatesDir()
	if _, err := git.PlainOpen(templatesDir); err == nil {
		return nil
	}

	logger.Warn(ctx, "Attempting to clone {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} repo to '{{|Folder|}}%s{{[-]}}' location.", templatesDir)

	if _, err := os.Stat(templatesDir); err == nil {
		logger.Notice(ctx, "Running: {{|RunningCommand|}}rm -rf %s{{[-]}}", templatesDir)
		if err := os.RemoveAll(templatesDir); err != nil {
			logger.FatalWithStack(ctx, "Failed to remove %s.", templatesDir)
		}
	}

	url := "https://github.com/GhostWriters/DockSTARTer-Templates"
	branch := "main"

	logger.Notice(ctx, "Running: {{|RunningCommand|}}git clone -b %s %s %s{{[-]}}", branch, url, templatesDir)
	logger.Notice(ctx, "{{|RunningCommand|}}git:{{[-]}} Cloning into '%s'.", templatesDir)

	_, err := git.PlainClone(templatesDir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName("refs/heads/" + branch),
	})
	return err
}
