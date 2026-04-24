package update

import (
	"context"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// UpdateTemplates handles updating the templates directory.
func UpdateTemplates(ctx context.Context, force bool, yes bool, requestedBranch string) error {
	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return fmt.Errorf("templates directory not found at %s", templatesDir)
	}

	repo, err := git.PlainOpen(templatesDir)
	if err != nil {
		return fmt.Errorf("failed to open templates repo: %w", err)
	}

	if requestedBranch == "" {
		// Default to the branch the templates repo is currently on, not necessarily "main"
		if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
			requestedBranch = head.Name().Short()
		} else {
			requestedBranch = "main"
		}
	}

	// Fetch updates to get remote hash
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
		return fmt.Errorf("failed to fetch templates: %w", err)
	}

	// Compare current HEAD with remote
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get templates HEAD: %w", err)
	}
	currentHash := head.Hash().String()[:7]
	currentDisplay := paths.GetTemplatesVersion()

	remoteRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+requestedBranch), true)
	if err != nil {
		// Fallback to tags if branch not found
		remoteRef, err = repo.Reference(plumbing.ReferenceName("refs/tags/"+requestedBranch), true)
	}
	if err != nil {
		return fmt.Errorf("failed to resolve templates target %s: %w", requestedBranch, err)
	}
	remoteHash := remoteRef.Hash().String()
	if len(remoteHash) > 7 {
		remoteHash = remoteHash[:7]
	}

	// Try to find a tag for the remote commit
	tags, _ := repo.Tags()
	foundTag := ""
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == remoteRef.Hash() {
			foundTag = ref.Name().Short()
			return fmt.Errorf("found")
		}
		return nil
	})

	var remoteDisplay string
	if foundTag != "" {
		remoteDisplay = foundTag
	} else {
		remoteDisplay = fmt.Sprintf("%s commit %s", requestedBranch, remoteHash)
	}

	question := ""
	initiationNotice := ""
	targetName := "DockSTARTer-Templates"
	noNotice := fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} will not be updated.", targetName)

	if currentHash == remoteHash {
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on branch '{{|Branch|}}%s{{[-]}}'.", targetName, requestedBranch)
		logger.Notice(ctx, "Current version is '{{|Version|}}%s{{[-]}}'", currentDisplay)

		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'?", targetName, currentDisplay)
			initiationNotice = fmt.Sprintf("Forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'", targetName, remoteDisplay)
		} else {
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}' now?", targetName, currentDisplay, remoteDisplay)
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}'", targetName, currentDisplay, remoteDisplay)
	}

	// Wrap logger.Notice to match console.Printer
	noticePrinter := func(ctx context.Context, msg any, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	// Prompt user
	answer, err := console.QuestionPrompt(ctx, noticePrinter, "Update", question, "Y", yes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, noNotice)
		return nil
	}

	// Execution
	logger.Notice(ctx, initiationNotice)
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get templates worktree: %w", err)
	}

	// Try checking out as branch first
	logger.Info(ctx, "Running: {{|RunningCommand|}}git checkout --force %s{{[-]}}", requestedBranch)
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(requestedBranch),
		Force:  true,
	})
	if err != nil {
		// If branch doesn't exist locally, create it and check it out tracking the remote hash
		err = w.Checkout(&git.CheckoutOptions{
			Hash:   remoteRef.Hash(),
			Branch: plumbing.NewBranchReferenceName(requestedBranch),
			Create: true,
			Force:  true,
		})
	}
	if err != nil {
		// Fallback to tag
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewTagReferenceName(requestedBranch),
			Force:  true,
		})
	}
	if err != nil {
		// Fallback to specific commit/reference as a last resort (this results in detached HEAD)
		err = w.Checkout(&git.CheckoutOptions{
			Hash:  remoteRef.Hash(),
			Force: true,
		})
	}
	if err == nil {
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Already on '%s'", requestedBranch)
		logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Your branch is up to date with 'origin/%s'.", requestedBranch)
	}

	if err != nil {
		// Final attempt: try pulling if it's the current branch
		logger.Info(ctx, "Pulling recent changes from git.")
		logger.Info(ctx, "Running: {{|RunningCommand|}}git pull{{[-]}}")
		err = w.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName("refs/heads/" + requestedBranch),
		})
	} else {
		logger.Info(ctx, "Pulling recent changes from git.")
		hash := remoteRef.Hash().String()[:7]
		logger.Info(ctx, "Running: {{|RunningCommand|}}git reset --hard %s{{[-]}}", hash)
		err = w.Reset(&git.ResetOptions{
			Mode:   git.HardReset,
			Commit: remoteRef.Hash(),
		})
	}

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to update templates to %s: %w", requestedBranch, err)
	}

	if err == nil {
		newHead, _ := repo.Head()
		if newHead != nil {
			commit, _ := repo.CommitObject(newHead.Hash())
			if commit != nil {
				subject := strings.Split(commit.Message, "\n")[0]
				hash := newHead.Hash().String()[:7]
				logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} HEAD is now at %s %s", hash, subject)
			} else {
				logger.Info(ctx, "{{|RunningCommand|}}git:{{[-]}} Already up to date.")
			}
		}
		logger.Info(ctx, "Cleaning up unnecessary files and optimizing the local repository.")
		logger.Info(ctx, "Running: {{|RunningCommand|}}git gc{{[-]}}")
		logger.Info(ctx, "Setting file ownership on new repository files")
		system.SetPermissions(ctx, templatesDir)
	}

	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '{{|Version|}}%s{{[-]}}'", targetName, paths.GetTemplatesVersion())
	appenv.InvalidateAppMetaCache()

	// Reset all needs markers (DELETED ResetNeeds in favor of granular detection)
	system.SetPermissions(ctx, paths.GetTimestampsDir())

	return nil
}

// EnsureTemplates checks if the templates directory exists and clones it if missing.
func EnsureTemplates(ctx context.Context) error {
	templatesDir := paths.GetTemplatesDir()
	// Check if the directory is a valid git repository
	if _, err := git.PlainOpen(templatesDir); err == nil {
		return nil
	}

	logger.Warn(ctx, "Attempting to clone {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} repo to '{{|Folder|}}%s{{[-]}}' location.", templatesDir)

	// Remove if exists but is invalid (no .git)
	if _, err := os.Stat(templatesDir); err == nil {
		logger.Notice(ctx, "Running: {{|RunningCommand|}}rm -rf %s{{[-]}}", templatesDir)
		if err := os.RemoveAll(templatesDir); err != nil {
			logger.FatalWithStack(ctx, "Failed to remove %s.", templatesDir)
		}
	}

	url := "https://github.com/GhostWriters/DockSTARTer-Templates"
	branch := "main" // Default branch

	logger.Notice(ctx, "Running: {{|RunningCommand|}}git clone -b %s %s %s{{[-]}}", branch, url, templatesDir)
	logger.Notice(ctx, "{{|RunningCommand|}}git:{{[-]}} Cloning into '%s'.", templatesDir)

	_, err := git.PlainClone(templatesDir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName("refs/heads/" + branch),
	})
	if err != nil {
		return err
	}

	return nil
}
