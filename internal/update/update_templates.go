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
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// customRemoteName is the git remote used to fetch from a caller-specified
// "owner/repo" override (see ParseRepoAndRef) instead of origin. Reused
// across calls (deleted and recreated each time) rather than named per-repo,
// since only one override is ever in flight at once.
const customRemoteName = "ds2-update-source"

// TemplatesUpdateInfo holds the result of CheckTemplatesUpdate.
// Pass it to ApplyTemplatesUpdate to perform the actual update without re-fetching.
type TemplatesUpdateInfo struct {
	HasUpdate       bool
	CurrentDisplay  string
	CurrentRepoSlug string // "" for the official repo; see repoDisplayPrefix
	RemoteDisplay   string
	RemoteRepoSlug  string // "" for the official repo; see repoDisplayPrefix
	repo            *git.Repository
	remoteRef       *plumbing.Reference
	remoteName      string
	requestedBranch string
	force           bool
}

// CheckTemplatesUpdate fetches remote state and returns whether an update is
// available. requestedSpec is a branch/tag name, optionally prefixed with
// "<owner>/<repo>@" to fetch from a fork instead of the canonical
// DockSTARTer-Templates repo (see ParseRepoAndRef). If force is true,
// HasUpdate is true even when already up to date.
func CheckTemplatesUpdate(ctx context.Context, force bool, requestedSpec string) (*TemplatesUpdateInfo, error) {
	repoSlug, requestedBranch := ParseRepoAndRef(requestedSpec, templatesRepoName)

	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("templates directory not found at %s", templatesDir)
	}

	repo, err := git.PlainOpen(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open templates repo: %w", err)
	}

	// Snapshot the pre-update state before anything below (adding a remote,
	// pruning tags, fetching) can change what a local tag lookup would find.
	// CurrentDisplay/CurrentRepoSlug must describe what was actually checked
	// out when this call started -- reading them any later would risk
	// picking up tags this same call just fetched for a *new* target remote
	// and misattributing them to the *old*, not-yet-replaced commit.
	currentDisplay := paths.GetTemplatesVersion()
	currentBranch := ""
	currentRepoSlug := ""
	if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
		currentBranch = head.Name().Short()
		if trackedRemote, trackedURL, ok := trackedRemoteFor(repo, currentBranch); ok && trackedRemote != "origin" {
			currentRepoSlug = repoSlugFromURL(trackedURL)
		}
	}

	// A fully bare call (no repo, no branch/tag at all -- requestedSpec
	// itself is "") means "keep doing whatever I was last explicitly told
	// to do": if the current branch's tracked upstream (set by a prior
	// explicit repo override, see ApplyTemplatesUpdate) points at a fork,
	// stay on that fork instead of silently falling back to the official
	// repo. Anything else the caller actually typed is a deliberate
	// instruction and always means the official repo (clearing any prior
	// fork tracking) -- including a bare "@" (ParseRepoAndRef gives it the
	// same empty repoSlug/ref as a truly empty spec, so bareCall must check
	// requestedSpec directly rather than the parsed pieces to tell them
	// apart): explicitly "official repo, default branch," not "inherit."
	bareCall := requestedSpec == ""

	remoteName := "origin"
	remoteURL := "https://github.com/" + defaultTemplatesRepo
	if repoSlug != "" {
		remoteName = customRemoteName
		remoteURL = "https://github.com/" + repoSlug
		_ = repo.DeleteRemote(remoteName)
		if _, err := repo.CreateRemote(&gitConfig.RemoteConfig{Name: remoteName, URLs: []string{remoteURL}}); err != nil {
			return nil, fmt.Errorf("failed to add remote for %s: %w", repoSlug, err)
		}
	}

	if requestedBranch == "" {
		if head, err := repo.Head(); err == nil {
			if head.Name().IsBranch() {
				requestedBranch = head.Name().Short()
			} else {
				// Detached HEAD: pinned to a tag rather than tracking a
				// branch -- the normal state after this function's own
				// "prefer latest tag reachable from main" policy below,
				// not necessarily a deliberate pin. Re-target whichever
				// branch that tag was actually cut from (a release can be
				// triggered from any branch, not just main -- see
				// DockSTARTer-Templates' release.yml) so a newer tag on
				// that branch can still be found on the next check.
				requestedBranch = bestBranchContaining(repo, head)
			}
		} else {
			requestedBranch = "main"
		}
	}

	if bareCall {
		if trackedRemote, trackedURL, ok := trackedRemoteFor(repo, requestedBranch); ok && trackedRemote != "origin" {
			remoteName = trackedRemote
			remoteURL = trackedURL
		}
	}

	// Git's local tag set accumulates forever across every remote ever
	// fetched -- it's not scoped per remote -- so a tag fetched from origin
	// in the past would otherwise stick around and get misattributed to a
	// fork that doesn't actually have it (or vice versa), the moment their
	// tips happen to share a commit. Pruning on a genuine remote switch (not
	// on every call -- a bareCall or an explicit request confirming the
	// same source already has a clean tag set) keeps the local tag set an
	// honest reflection of whichever remote is currently authoritative, so
	// every plain local tag lookup (this file's own, and paths.GetTemplatesVersion's
	// independent one) stays correct without needing a live network probe.
	previousRemoteName := "origin"
	if currentBranch != "" {
		if tracked, _, ok := trackedRemoteFor(repo, currentBranch); ok {
			previousRemoteName = tracked
		}
	}
	if previousRemoteName != remoteName {
		if err := pruneLocalTags(repo); err != nil {
			logger.Debug(ctx, "Failed to prune local tags before switching remotes: %v", err)
		}
	}

	logger.Info(ctx, "Setting file ownership on current repository files")
	system.SetPermissions(ctx, templatesDir)
	logger.Info(ctx, "Fetching recent changes from git.")
	if remoteName == "origin" {
		logger.Info(ctx, "Running: {{|RunningCommand|}}git fetch --all --prune -v{{[-]}}")
	} else {
		logger.Info(ctx, "Running: {{|RunningCommand|}}git fetch %s --prune -v{{[-]}}", remoteName)
	}
	err = repo.Fetch(&git.FetchOptions{RemoteName: remoteName, Tags: git.AllTags})
	if err == nil || err == git.NoErrAlreadyUpToDate {
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} POST git-upload-pack (186 bytes)")
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}} From %s", remoteURL)
		logger.Info(ctx, "\t{{|RunningCommand|}}git:{{[-]}}  = [up to date]      %-10s -> %s/%s", requestedBranch, remoteName, requestedBranch)
	}
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch templates: %w", err)
	}

	// The prune/fetch above can change what a local tag lookup finds for
	// the current commit (e.g. re-fetching origin's tags after switching
	// back from a fork). paths.GetTemplatesVersion's own 60-second cache
	// doesn't know that -- without invalidating it here, resolveTemplatesTarget's
	// "already at the latest tag" shortcut below (which also reads it) could
	// return the stale pre-fetch value from this same function call's
	// earlier CurrentDisplay snapshot instead of re-scanning.
	paths.InvalidateTemplatesVersionCache()

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get templates HEAD: %w", err)
	}
	currentHash := head.Hash().String()[:7]

	remoteRef, remoteDisplay, err := resolveTemplatesTarget(repo, head, currentBranch, requestedBranch, remoteName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve templates target %s: %w", requestedBranch, err)
	}
	remoteRepoSlug := ""
	if remoteName != "origin" {
		remoteRepoSlug = repoSlugFromURL(remoteURL)
	}
	remoteHash := remoteRef.Hash().String()
	if len(remoteHash) > 7 {
		remoteHash = remoteHash[:7]
	}

	// A source switch (fork <-> official, or fork <-> a different fork) is a
	// real change worth reporting even when the two happen to be at the same
	// commit right now -- "already up to date" would wrongly suggest calling
	// with a different owner/repo did nothing. bareCall never triggers this,
	// since remoteRepoSlug is inherited from currentRepoSlug in that case.
	hasUpdate := currentHash != remoteHash || force || remoteRepoSlug != currentRepoSlug
	return &TemplatesUpdateInfo{
		HasUpdate:       hasUpdate,
		CurrentDisplay:  currentDisplay,
		CurrentRepoSlug: currentRepoSlug,
		RemoteDisplay:   remoteDisplay,
		RemoteRepoSlug:  remoteRepoSlug,
		repo:            repo,
		remoteRef:       remoteRef,
		remoteName:      remoteName,
		requestedBranch: requestedBranch,
		force:           force,
	}, nil
}

// trackedRemoteFor reports the remote name and URL that branchName's git
// config says to track (branch.<name>.remote), and that remote's configured
// URL, so a bare update call can keep following a fork set up by a prior
// explicit "owner/repo@branch" call instead of defaulting back to origin.
// ok is false if the branch has no tracking config or the remote it names
// no longer exists.
func trackedRemoteFor(repo *git.Repository, branchName string) (remoteName, remoteURL string, ok bool) {
	branchCfg, err := repo.Branch(branchName)
	if err != nil || branchCfg.Remote == "" {
		return "", "", false
	}
	remoteCfg, err := repo.Remote(branchCfg.Remote)
	if err != nil || len(remoteCfg.Config().URLs) == 0 {
		return "", "", false
	}
	return branchCfg.Remote, remoteCfg.Config().URLs[0], true
}

// pruneLocalTags deletes every local tag ref. Used when switching the
// templates repo to a different remote (see CheckTemplatesUpdate), so the
// upcoming fetch repopulates the local tag set from only that remote --
// git tags aren't namespaced per remote, so without this, tags fetched from
// a previous remote would otherwise linger and could get misattributed to
// whichever remote is now authoritative.
func pruneLocalTags(repo *git.Repository) error {
	tags, err := repo.Tags()
	if err != nil {
		return err
	}
	var names []string
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		names = append(names, ref.Name().Short())
		return nil
	})
	for _, name := range names {
		if err := repo.DeleteTag(name); err != nil {
			return err
		}
	}
	return nil
}

// setBranchTracking sets (or overwrites) branchName's tracked remote in the
// repo's git config -- equivalent to "git branch --set-upstream-to". Writes
// the config map directly rather than going through repo.CreateBranch,
// which errors with ErrBranchExists whenever a config entry for the branch
// is already present (true for every branch this code touches after its
// first checkout), silently preventing any update after the first.
func setBranchTracking(repo *git.Repository, branchName, remoteName string) error {
	cfg, err := repo.Config()
	if err != nil {
		return err
	}
	cfg.Branches[branchName] = &gitConfig.Branch{
		Name:   branchName,
		Remote: remoteName,
		Merge:  plumbing.NewBranchReferenceName(branchName),
	}
	return repo.SetConfig(cfg)
}

// CurrentTemplatesRepoSlug returns the "owner/repo" slug the currently
// checked-out templates branch tracks (set by a prior explicit
// "owner/repo@branch" update, see ApplyTemplatesUpdate), or "" if it tracks
// origin (the official repo), tracking can't be determined, or HEAD is
// detached (pinned to a tag). Used by version displays outside an active
// update check -- -V, sysinfo, the TUI header -- so a fork stays visibly
// distinguishable from the official repo everywhere, not just mid-update.
func CurrentTemplatesRepoSlug() string {
	repo, err := git.PlainOpen(paths.GetTemplatesDir())
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil || !head.Name().IsBranch() {
		return ""
	}
	trackedRemote, trackedURL, ok := trackedRemoteFor(repo, head.Name().Short())
	if !ok || trackedRemote == "origin" {
		return ""
	}
	return repoSlugFromURL(trackedURL)
}

// resolveTemplatesTarget determines the remote ref to update the templates
// repo to, and its display string (tag name, or "<branch> commit <hash>").
//
// For branch "main", targets the latest tag reachable from main's tip
// instead of main's literal tip, since CI commits land between releases and
// the tip is rarely what the user was actually on. Falls back to main's tip
// if no tag exists yet. An explicit tag name or commit hash bypasses this
// policy entirely (resolved separately below).
//
// The tag search trusts local git's tag set, which CheckTemplatesUpdate
// keeps honest for whichever remote is currently authoritative by pruning
// it on every genuine remote switch (see pruneLocalTags) -- tags aren't
// namespaced per remote, so without that pruning a tag fetched from a
// previous remote could otherwise get reused for a different remote's
// identically-hashed commit.
//
// If staying on the same branch (currentBranch == requestedBranch) and the
// resolved target is an ancestor of current HEAD, returns HEAD itself so
// callers see "no update available" instead of offering to move backward.
// Skipped when switching branches, since a branch forked after the latest
// tag being a descendant of it must never block the switch.
//
// head is the caller-resolved current HEAD, used only for that ancestor check.
// remoteName selects which remote's branch namespace to search (normally
// "origin"; a caller-specified repo override uses customRemoteName instead).
func resolveTemplatesTarget(repo *git.Repository, head *plumbing.Reference, currentBranch, requestedBranch, remoteName string) (*plumbing.Reference, string, error) {
	remoteRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/"+remoteName+"/"+requestedBranch), true)
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
// reaches branchRef at all. Trusts local git's tag set -- see
// resolveTemplatesTarget for why that's kept accurate per remote.
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

// bestBranchContaining finds which remote branch ref belongs to: prioritizes
// "main" when ref is reachable from multiple branches, otherwise returns the
// sole containing branch. Falls back to "main" if no remote branch contains
// ref at all (e.g. a genuinely orphaned pin), rather than getting stuck on
// the pinned tag forever.
func bestBranchContaining(repo *git.Repository, ref *plumbing.Reference) string {
	refs, err := repo.References()
	if err != nil {
		return "main"
	}

	const remotePrefix = "refs/remotes/origin/"
	var containing []string
	_ = refs.ForEach(func(r *plumbing.Reference) error {
		name := r.Name().String()
		if !strings.HasPrefix(name, remotePrefix) {
			return nil
		}
		branch := strings.TrimPrefix(name, remotePrefix)
		if branch == "HEAD" {
			return nil
		}
		if ok, err := isAncestorOrEqual(repo, ref, r); err == nil && ok {
			containing = append(containing, branch)
		}
		return nil
	})

	for _, b := range containing {
		if b == "main" {
			return "main"
		}
	}
	if len(containing) == 1 {
		return containing[0]
	}
	return "main"
}

// ApplyTemplatesUpdate prompts and applies the update described by info.
// Call CheckTemplatesUpdate first to obtain info.
func ApplyTemplatesUpdate(ctx context.Context, info *TemplatesUpdateInfo, yes bool) error {
	targetName := "DockSTARTer-Templates"
	noNotice := fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} will not be updated.", targetName)

	var question, initiationNotice string
	if !info.HasUpdate {
		// This is the only code path that runs when content already
		// matches, so tracking must be updated here too, not just in the
		// real-content-change path below -- otherwise explicitly requesting
		// the official "main" while already on it would leave a previously
		// tracked fork in place. A bareCall harmlessly rewrites the same
		// value it inherited.
		if head, err := info.repo.Head(); err == nil && head.Name().IsBranch() && head.Name().Short() == info.requestedBranch {
			if err := setBranchTracking(info.repo, info.requestedBranch, info.remoteName); err != nil {
				logger.Debug(ctx, "Failed to record branch tracking for %s: %v", info.requestedBranch, err)
			}
		}
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on branch '%s'.", targetName, TmplBranchLinkForRepo(info.requestedBranch, info.RemoteRepoSlug))
		if info.requestedBranch != info.CurrentDisplay {
			logger.Notice(ctx, "Current version is '%s'", TmplVersionLinkForRepo(info.CurrentDisplay, info.RemoteRepoSlug))
		}
		return nil
	}

	if info.force && info.CurrentDisplay == info.RemoteDisplay {
		question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '%s'?", targetName, TmplVersionLinkForRepo(info.CurrentDisplay, info.CurrentRepoSlug))
		initiationNotice = fmt.Sprintf("Forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '%s'", targetName, TmplVersionLinkForRepo(info.RemoteDisplay, info.RemoteRepoSlug))
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s' now?", targetName, TmplVersionLinkForRepo(info.CurrentDisplay, info.CurrentRepoSlug), TmplVersionLinkForRepo(info.RemoteDisplay, info.RemoteRepoSlug))
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s'", targetName, TmplVersionLinkForRepo(info.CurrentDisplay, info.CurrentRepoSlug), TmplVersionLinkForRepo(info.RemoteDisplay, info.RemoteRepoSlug))
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
			RemoteName:    info.remoteName,
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
		if newHead != nil && newHead.Name().IsBranch() {
			// Record which remote this branch now tracks, so a later fully
			// bare update call (see CheckTemplatesUpdate's bareCall logic)
			// knows to keep following a fork instead of defaulting back to
			// origin. repo.CreateBranch refuses to overwrite an existing
			// branch config entry (returns ErrBranchExists) -- every branch
			// this code ever touches already has one after its first
			// checkout, so write the config directly instead.
			branchName := newHead.Name().Short()
			if err := setBranchTracking(info.repo, branchName, info.remoteName); err != nil {
				logger.Debug(ctx, "Failed to record branch tracking for %s: %v", branchName, err)
			}
		}
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
	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '%s'", targetName, TmplVersionLinkForRepo(paths.GetTemplatesVersion(), info.RemoteRepoSlug))
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

	url := "https://github.com/" + defaultTemplatesRepo
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
