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
	"github.com/go-git/go-git/v5/plumbing/object"
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
		if head, err := repo.Head(); err == nil {
			if head.Name().IsBranch() {
				requestedBranch = head.Name().Short()
			} else {
				// Detached HEAD: pinned to a specific tag/version rather
				// than tracking a branch. Re-resolve that same tag/version
				// on the next fetch instead of defaulting to main, which
				// this install was never actually tracking.
				requestedBranch = paths.GetTemplatesVersion()
			}
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
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} POST git-upload-pack (186 bytes)")
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} From https://github.com/GhostWriters/DockSTARTer-Templates")
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}}  = [up to date]      %-10s -> origin/%s", requestedBranch, requestedBranch)
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
// For branch "main", targets the latest tag reachable from origin/main
// instead of main's literal tip, since CI commits land between releases and
// the tip is rarely what the user was actually on. Falls back to main's tip
// if no tag exists yet. An explicit tag name or commit hash bypasses this
// policy entirely (resolved separately below).
//
// If staying on the same branch (currentBranch == requestedBranch) and the
// resolved target is an ancestor of current HEAD, returns HEAD itself so
// callers see "no update available" instead of offering to move backward.
// Skipped when switching branches, since a branch forked after the latest
// tag being a descendant of it must never block the switch.
//
// head is the caller-resolved current HEAD, used only for that ancestor check.
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

// latestReachableTag returns the most recently committed tag (by the
// tagged commit's committer date, not by comparing tag name strings) that
// is an ancestor of (or equal to) branchRef's commit. ok is false if no tag
// reaches branchRef at all.
//
// Deliberately does not use compareVersions (name-string comparison): a
// repo that has changed its tag-naming scheme over time (e.g.
// "v2026.01.19-1" -> "v1.20260628.1") can have two tags whose names sort in
// the wrong chronological order. Comparing by commit date is immune to
// naming-scheme changes.
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
	var bestCommit *object.Commit
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
		if bestCommit == nil || tagCommit.Committer.When.After(bestCommit.Committer.When) {
			bestRef = tagRef
			bestCommit = tagCommit
			bestName = tagRef.Name().Short()
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
		if info.requestedBranch != info.CurrentDisplay {
			logger.Notice(ctx, "Current version is '%s'", TmplVersionLink(info.CurrentDisplay))
		}
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
		// A tag (a pinned version, not a tracked branch like main) takes
		// priority over creating a same-named local branch below -- tags
		// are never branches, so checking out a tag must land on that tag
		// (detached HEAD), not on a synthetic local branch that shares its
		// name.
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewTagReferenceName(info.requestedBranch),
			Force:  true,
		})
	}
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
			Hash:  info.remoteRef.Hash(),
			Force: true,
		})
	}
	if err == nil {
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} Already on '%s'", info.requestedBranch)
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} Your branch is up to date with 'origin/%s'.", info.requestedBranch)
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
				logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} HEAD is now at %s %s", hash, subject)
			} else {
				logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} Already up to date.")
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

	// Resync per-app env files against the (possibly changed) templates.
	// NeedsUpdate's template-dependency check (see appTemplateDefaultFile)
	// makes this a no-op for apps whose template didn't actually change --
	// without this call, though, nothing ever re-checks after a template
	// update, so an edited default/variable would sit unnoticed until the
	// user happened to trigger appenv.Update() some other way (or --reset).
	if err := appenv.Update(ctx, false, ""); err != nil {
		logger.Warn(ctx, "Failed to update environment variable files after templates update: %v", err)
	}

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

	logger.Warn(ctx, "Attempting to clone {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} repo to '"+console.FormatFolderPath(templatesDir)+"' location.")

	if _, err := os.Stat(templatesDir); err == nil {
		logger.Notice(ctx, "Running: {{|RunningCommand|}}rm -rf %s{{[-]}}", templatesDir)
		if err := os.RemoveAll(templatesDir); err != nil {
			logger.FatalWithStack(ctx, "Failed to remove %s.", templatesDir)
		}
	}

	url := "https://github.com/GhostWriters/DockSTARTer-Templates"
	branch := "main"

	logger.Notice(ctx, "Running: {{|RunningCommand|}}git clone -b %s %s %s{{[-]}}", branch, url, templatesDir)
	logger.Notice(ctx, "\t{{|RunningCommand|}}git:{{[-]}} Cloning into '%s'.", templatesDir)

	_, err := git.PlainClone(templatesDir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName("refs/heads/" + branch),
	})
	if err != nil {
		return err
	}

	// The main clone above is a valid working state on its own -- from here
	// on, a failure just leaves the install on main's tip instead of the
	// latest release, so each step is logged rather than returned.
	repo, openErr := git.PlainOpen(templatesDir)
	if openErr != nil {
		logger.Warn(ctx, "Failed to reopen {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} after cloning: %v", openErr)
		return nil
	}
	paths.InvalidateTemplatesVersionCache()
	logger.Notice(ctx, "Cloned {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} at '%s'.", TmplVersionLink(paths.GetTemplatesVersion()))

	// Resolves latestReachableTag directly instead of going through
	// CheckTemplatesUpdate: that function's "already at/past the latest
	// tag while tracking main" case exists to avoid offering an active
	// main-tracker what looks like a downgrade, but a freshly cloned main
	// tip is *always* at or ahead of the latest tag, so it would report no
	// update every time and this step would never actually do anything.
	mainRef, refErr := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+branch), true)
	if refErr != nil {
		logger.Warn(ctx, "Failed to resolve {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} origin/%s after cloning: %v", branch, refErr)
		return nil
	}
	tagRef, tagName, ok := latestReachableTag(repo, mainRef)
	if !ok {
		// No tagged release reachable from main yet -- main's tip stands.
		return nil
	}
	if tagRef.Hash() == mainRef.Hash() {
		// main's tip is already the latest release -- nothing to check out.
		return nil
	}

	info := &TemplatesUpdateInfo{
		HasUpdate:       true,
		CurrentDisplay:  paths.GetTemplatesVersion(),
		RemoteDisplay:   tagName,
		repo:            repo,
		remoteRef:       tagRef,
		requestedBranch: tagName,
		force:           false,
	}
	return ApplyTemplatesUpdate(ctx, info, true)
}
