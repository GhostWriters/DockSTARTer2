package update

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

var (
	// AppUpdateAvailable is true if an application update is available.
	AppUpdateAvailable bool
	// TmplUpdateAvailable is true if a template update is available.
	TmplUpdateAvailable bool
	// UpdateCheckError is true if the last update check failed due to network/timeout errors.
	UpdateCheckError bool
	// LatestAppVersion is the tag name of the latest application release.
	LatestAppVersion string
	// LatestTmplVersion is the short hash of the latest template commit.
	LatestTmplVersion string

	// PendingReExec stores the command to run after the TUI shuts down.
	// The actual exec is performed by the main thread after return.
	PendingReExec []string
)

// SelfUpdate handles updating the application binary using GitHub Releases.
func SelfUpdate(ctx context.Context, force bool, yes bool, requestedVersion string, restArgs []string) error {
	// Get current executable path for logging later
	// We do this early because self-update acts on the running binary (renaming it),
	// so os.Executable() might return .ds2.old if called after the update.
	exePath, err := os.Executable()
	if err != nil {
		// Fallback if we can't get it, though unlikely
		exePath = "unknown"
	}

	slug := "GhostWriters/DockSTARTer2"
	repo := selfupdate.ParseSlug(slug)

	currentChannel := GetCurrentChannel()
	if requestedVersion == "" {
		requestedVersion = currentChannel
	}

	// Map "main" to "stable" channel
	if strings.EqualFold(requestedVersion, "main") {
		requestedVersion = "stable"
	}

	var (
		latest *selfupdate.Release
		found  bool
	)

	// Detect latest version first
	updater, err := getUpdater(ctx, requestedVersion)
	if err != nil {
		return fmt.Errorf("failed to create updater: %w", err)
	}

	if strings.HasPrefix(requestedVersion, "v") {
		// Specific version requested
		latest, found, err = updater.DetectVersion(ctx, repo, requestedVersion)
	} else {
		// Default latest for the channel (filtered by channel in getUpdater)
		latest, found, err = updater.DetectLatest(ctx, repo)
	}

	if err != nil {
		return fmt.Errorf("failed to detect latest version: %w", err)
	}
	if !found {
		// Show warning for channels with no releases (e.g., dev)
		msg := []string{
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} channel '{{|Branch|}}%s{{[-]}}' appears to no longer exist (no releases found).", version.ApplicationName, requestedVersion),
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} is currently on version '{{|Version|}}%s{{[-]}}'.", version.ApplicationName, version.Version),
			fmt.Sprintf("Run '{{|UserCommand|}}%s -u main{{[-]}}' to update to the latest stable release.", version.CommandName),
		}
		logger.Warn(ctx, msg)
		return nil
	}

	remoteVersion := latest.Version()
	currentVersion := version.Version
	// Strict channel matching (except when a specific version was requested)
	if !strings.HasPrefix(requestedVersion, "v") {
		remoteChannel := GetChannelFromVersion(remoteVersion)
		if !strings.EqualFold(remoteChannel, currentChannel) && !strings.EqualFold(requestedVersion, remoteChannel) {
			logger.Warn(ctx, "{{|ApplicationName|}}%s{{[-]}} is on channel '{{|Branch|}}%s{{[-]}}', but latest release is on channel '{{|Branch|}}%s{{[-]}}'. Ignoring.", version.ApplicationName, currentChannel, remoteChannel)
			return nil
		}
	}

	// Ensure versions start with 'v' for consistent display
	if !strings.HasPrefix(remoteVersion, "v") {
		remoteVersion = "v" + remoteVersion
	}
	if !strings.HasPrefix(currentVersion, "v") {
		currentVersion = "v" + currentVersion
	}

	question := ""
	initiationNotice := ""
	noNotice := fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} will not be updated.", version.ApplicationName)

	// Wrap logger.Notice to match console.Printer
	noticePrinter := func(ctx context.Context, msg any, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	if compareVersions(currentVersion, remoteVersion) == 0 {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'?", version.ApplicationName, currentVersion)
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'", version.ApplicationName, remoteVersion)
		} else {
			logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on channel '{{|Branch|}}%s{{[-]}}'.", version.ApplicationName, requestedVersion)
			logger.Notice(ctx, "Current version is '{{|Version|}}%s{{[-]}}'.", currentVersion)
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}' now?", version.ApplicationName, currentVersion, remoteVersion)
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}'", version.ApplicationName, currentVersion, remoteVersion)
	}

	// Prompt user
	if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
		logger.Notice(ctx, noNotice)
		return nil
	}

	// Execution
	logger.Notice(ctx, initiationNotice)

	err = installUpdate(ctx, latest.AssetURL)
	if err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}

	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '{{|Version|}}%s{{[-]}}'", version.ApplicationName, remoteVersion)

	if exePath != "unknown" {
		logger.Info(ctx, "Application location is '{{|File|}}%s{{[-]}}'.", exePath)
	}

	// Reset all needs markers
	_ = paths.ResetNeeds()

	// Re-execution logic
	// If no args passed, default to -e flag
	if len(restArgs) == 0 {
		return ReExec(ctx, exePath, []string{"-e"})
	}
	return ReExec(ctx, exePath, restArgs)
}

// ReExec prepares the application for re-execution with the given arguments.
// It stores the command in PendingReExec and shuts down the TUI.
// The actual exec is performed by the main thread after return.
func ReExec(ctx context.Context, exePath string, args []string) error {
	if exePath == "unknown" {
		return fmt.Errorf("cannot re-exec: unknown executable path")
	}

	// Construct command line for logging
	fullCmd := exePath
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	logger.Notice(ctx, "Running: {{|RunningCommand|}}exec %s{{[-]}}", fullCmd)

	// Store for main thread execution
	PendingReExec = append([]string{exePath}, args...)

	// Cleanly shut down TUI if active before re-execution
	if console.TUIShutdown != nil {
		console.TUIShutdown()
	}

	return nil
}

// installUpdate downloads and installs the binary from the given URL.
func installUpdate(ctx context.Context, assetURL string) error {
	// 1. Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// 2. Create temp dir
	tmpDir, err := os.MkdirTemp("", "ds2-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 3. Download
	logger.Info(ctx, "Downloading update from {{|URL|}}%s{{[-]}}", assetURL)
	resp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// 4. Extract
	tmpExe := filepath.Join(tmpDir, filepath.Base(exe))

	// Handle compressed formats
	if strings.HasSuffix(assetURL, ".tar.gz") || strings.HasSuffix(assetURL, ".tgz") {
		gw, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gw.Close()
		tr := tar.NewReader(gw)

		found := false
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read tar header: %w", err)
			}

			// Simple heuristic: if name matches exe name
			if filepath.Base(header.Name) == filepath.Base(exe) {
				out, err := os.Create(tmpExe)
				if err != nil {
					return fmt.Errorf("failed to create temp file: %w", err)
				}
				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					return fmt.Errorf("failed to extract: %w", err)
				}
				out.Close()
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("executable not found in archive")
		}
	} else {
		return fmt.Errorf("unsupported format: %s", assetURL)
	}

	if err := os.Chmod(tmpExe, 0755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// 5. Replace
	// Try to replace the current executable

	// We will try to mv tmpExe -> exe
	// If it fails with permission, we try sudo.

	// Prepare move command
	err = os.Rename(tmpExe, exe)
	if err == nil {
		return nil
	}

	// If direct rename fails, attempt with sudo
	logger.Warn(ctx, "Direct update failed (%v), trying with sudo...", err)

	mvCmd := exec.Command("sudo", "mv", tmpExe, exe)
	if out, err := mvCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudo update failed: %s: %w", string(out), err)
	}

	// Restore ownership to root:root if sudo was used?
	// Usually /usr/local/bin is root owned.
	exec.Command("sudo", "chown", "root:root", exe).Run()

	return nil
}

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
		requestedBranch = "main"
	}

	// Fetch updates to get remote hash
	logger.Info(ctx, "Setting file ownership on current repository files")
	logger.Info(ctx, "Running: {{|RunningCommand|}}sudo chown -R 1000:1000 %s/.git{{[-]}}", templatesDir)
	logger.Info(ctx, "Running: {{|RunningCommand|}}sudo chown 1000:1000 %s{{[-]}}", templatesDir)
	logger.Info(ctx, "Running: {{|RunningCommand|}}git ls-tree -rt --name-only HEAD | xargs sudo chown 1000:1000{{[-]}}")
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
	remoteHash := remoteRef.Hash().String()[:7]
	remoteDisplay := remoteHash

	// Try to find a tag for the remote commit
	tags, _ := repo.Tags()
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == remoteRef.Hash() {
			remoteDisplay = ref.Name().Short()
			return fmt.Errorf("found")
		}
		return nil
	})

	question := ""
	initiationNotice := ""
	targetName := "DockSTARTer-Templates"
	noNotice := fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} will not be updated.", targetName)

	if currentHash == remoteHash {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'?", targetName, currentDisplay)
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'", targetName, remoteDisplay)
		} else {
			logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on branch '{{|Branch|}}%s{{[-]}}'.", targetName, requestedBranch)
			logger.Notice(ctx, "Current version is '{{|Version|}}%s{{[-]}}'", currentDisplay)
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
	if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
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
	})
	if err != nil {
		// Fallback to tag
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewTagReferenceName(requestedBranch),
		})
	}
	if err != nil {
		// Fallback to specific commit/reference
		err = w.Checkout(&git.CheckoutOptions{
			Hash: remoteRef.Hash(),
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
		logger.Info(ctx, "Running: {{|RunningCommand|}}git ls-tree -rt --name-only %s | xargs sudo chown 1000:1000{{[-]}}", requestedBranch)
		logger.Info(ctx, "Running: {{|RunningCommand|}}sudo chown -R 1000:1000 %s/.git{{[-]}}", templatesDir)
		logger.Info(ctx, "Running: {{|RunningCommand|}}sudo chown 1000:1000 %s{{[-]}}", templatesDir)
	}

	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '{{|Version|}}%s{{[-]}}'", targetName, paths.GetTemplatesVersion())

	// Reset all needs markers
	_ = paths.ResetNeeds()

	return nil
}

// CheckCurrentStatus verifies if the current channel still exists on GitHub.
func CheckCurrentStatus(ctx context.Context) error {
	requestedVersion := GetCurrentChannel()

	// This is a simplified check that just ensures we can reach GitHub
	// and verifies the current version is still conceptually valid for the channel.
	if requestedVersion == "dev" {
		// Log a warning if 'dev' is used, as it might no longer exist in some contexts
		msg := []string{
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} channel '{{|Branch|}}%s{{[-]}}' appears to no longer exist.", version.ApplicationName, requestedVersion),
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} is currently on version '{{|Version|}}%s{{[-]}}'.", version.ApplicationName, version.Version),
			fmt.Sprintf("Run '{{|UserCommand|}}%s -u main{{[-]}}' to update to the latest stable release.", version.CommandName),
		}
		logger.Warn(ctx, msg)
	}

	return nil
}

// GetUpdateStatus checks for updates in the background without prompting.
func GetUpdateStatus(ctx context.Context) (appUpdate bool, tmplUpdate bool) {
	// 1. Check Application Updates
	appUpdate, appVer, appErr := checkAppUpdate(ctx)

	// 2. Check Template Updates
	tmplUpdate, tmplVer, tmplErr := checkTmplUpdate(ctx)

	// Set global state
	AppUpdateAvailable = appUpdate
	LatestAppVersion = appVer
	TmplUpdateAvailable = tmplUpdate
	LatestTmplVersion = tmplVer
	UpdateCheckError = appErr || tmplErr

	return appUpdate, tmplUpdate
}

// CheckUpdates performs a startup update check and notifies the user if updates are available.
func CheckUpdates(ctx context.Context) {
	// Create a timeout context for the update check (3 seconds max)
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Run update check synchronously (before user command executes)
	GetUpdateStatus(checkCtx)

	// Log update check results (either success with updates, or failure)
	if UpdateCheckError {
		// Check failed - warn user
		logger.Warn(ctx, "Failed to check for updates (network timeout or error).")
	} else {
		// Check succeeded - log update availability
		// 1. Application Updates
		if AppUpdateAvailable {
			msg := []string{
				GetAppVersionDisplay(),
				fmt.Sprintf("An update to {{|ApplicationName|}}%s{{[-]}} is available.", version.ApplicationName),
				fmt.Sprintf("Run '{{|UserCommand|}}%s -u{{[-]}}' to update to version '{{|Version|}}%s{{[-]}}'.", version.CommandName, LatestAppVersion),
			}
			logger.Warn(ctx, msg)
		} else {
			// Info level is hidden by default (-v shows it), matching main.sh use of VERBOSE
			logger.Info(ctx, GetAppVersionDisplay())
		}

		// 2. Template Updates
		if TmplUpdateAvailable {
			tmplName := "DockSTARTer-Templates"
			msg := []string{
				GetTmplVersionDisplay(),
				fmt.Sprintf("An update to {{|ApplicationName|}}%s{{[-]}} is available.", tmplName),
				fmt.Sprintf("Run '{{|UserCommand|}}%s -u{{[-]}}' to update to version '{{|Version|}}%s{{[-]}}'.", version.CommandName, LatestTmplVersion),
			}
			logger.Warn(ctx, msg)
		} else {
			logger.Info(ctx, GetTmplVersionDisplay())
		}
	}
}

// GetAppVersionDisplay returns a formatted version string for the application,
// optionally including an update indicator.
func GetAppVersionDisplay() string {
	name := version.ApplicationName
	ver := version.Version

	return fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", name, ver)
}

// GetTmplVersionDisplay returns a formatted version string for the templates,
// optionally including an update indicator.
func GetTmplVersionDisplay() string {
	name := "DockSTARTer-Templates"
	ver := paths.GetTemplatesVersion()

	return fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", name, ver)
}

func checkAppUpdate(ctx context.Context) (updateAvailable bool, ver string, hadError bool) {
	slug := "GhostWriters/DockSTARTer2"
	repo := selfupdate.ParseSlug(slug)

	channel := GetCurrentChannel()
	updater, err := getUpdater(ctx, channel)
	if err != nil {
		return false, "", true
	}

	latest, found, err := updater.DetectLatest(ctx, repo)
	if err != nil || !found {
		return false, "", true
	}

	remoteVersion := latest.Version()
	remoteChannel := GetChannelFromVersion(remoteVersion)
	if !strings.EqualFold(remoteChannel, channel) {
		// Not the same channel, ignore
		return false, "", false
	}

	if compareVersions(remoteVersion, version.Version) > 0 {
		return true, latest.Version(), false
	}

	return false, version.Version, false
}

func checkTmplUpdate(ctx context.Context) (updateAvailable bool, ver string, hadError bool) {
	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return false, "", false // Not an error - just no templates dir yet
	}

	repo, err := git.PlainOpen(templatesDir)
	if err != nil {
		return false, "", false // Not an error - just no git repo yet
	}

	// Fetch updates with timeout
	fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = repo.FetchContext(fetchCtx, &git.FetchOptions{
		RemoteName: "origin",
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		// Timeout or network error - this IS an error
		return false, "", true
	}

	// Compare current HEAD with origin/main
	head, err := repo.Head()
	if err != nil {
		return false, "", false // Repo issue, not network error
	}

	remoteHead, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return false, "", false // Repo issue, not network error
	}

	if head.Hash() != remoteHead.Hash() {
		return true, remoteHead.Hash().String()[:7], false
	}

	return false, head.Hash().String()[:7], false
}

// compareVersions compares two version strings and returns:
// -1 if v1 < v2
//
//	0 if v1 == v2
//	1 if v1 > v2
func compareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// First try strictly semantic versioning
	sv1, err1 := semver.NewVersion(v1)
	sv2, err2 := semver.NewVersion(v2)
	if err1 == nil && err2 == nil {
		return sv1.Compare(sv2)
	}

	// Fallback for custom versioning (e.g. 2024.01.01.1)
	// Split by dots and compare parts
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")

	for i := 0; i < len(p1) && i < len(p2); i++ {
		s1 := p1[i]
		s2 := p2[i]

		if s1 == s2 {
			continue
		}

		// Handle suffixes (e.g. "1.0.0-beta" vs "1.0.0")
		// If one has a suffix and the other doesn't, and they are otherwise equal:
		// the one without a suffix is GREATER (stable > pre-release)
		h1 := strings.Contains(s1, "-")
		h2 := strings.Contains(s2, "-")
		if h1 || h2 {
			if h1 != h2 {
				if h1 {
					return -1 // s1 has suffix, s2 doesn't -> s1 < s2
				}
				return 1 // s2 has suffix, s1 doesn't -> s1 > s2
			}
			// Both have suffixes, just string compare
			if s1 > s2 {
				return 1
			}
			return -1
		}

		// Try numeric comparison
		n1, e1 := strconv.Atoi(s1)
		n2, e2 := strconv.Atoi(s2)

		if e1 == nil && e2 == nil {
			if n1 > n2 {
				return 1
			}
			return -1
		}

		// Fallback to string comparison
		if s1 > s2 {
			return 1
		}
		return -1
	}

	if len(p1) > len(p2) {
		// 1.0.0.1 > 1.0.0
		// But check if the extra part is a suffix
		if strings.Contains(p1[len(p2)], "-") {
			return -1
		}
		return 1
	}

	// Lengths are different, but no dash in the longer part
	// 1.0.1 > 1.0
	if len(p1) > len(p2) {
		return 1
	}
	if len(p2) > len(p1) {
		return -1
	}
	return 0
}

// getUpdater returns a configured selfupdate.Updater for the given channel.
func getUpdater(ctx context.Context, channel string) (*selfupdate.Updater, error) {
	cfg := selfupdate.Config{}
	// Only allow prereleases if the user is on a prerelease/dev channel
	if !strings.EqualFold(channel, "stable") {
		cfg.Prerelease = true
	} else {
		cfg.Prerelease = false
	}
	return selfupdate.NewUpdater(cfg)
}

// GetCurrentChannel returns the update channel based on the current version string.
// v1.YYYYMMDD.N is stable, v0.0.0.0-dev is dev, -Prerelease is prerelease, -rc1 is rc1, etc.
func GetCurrentChannel() string {
	return GetChannelFromVersion(version.Version)
}

// GetChannelFromVersion extracts the channel (suffix) from a version string.
func GetChannelFromVersion(v string) string {
	parts := strings.SplitN(v, "-", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return "stable"
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
	logger.Notice(ctx, "{{|RunningCommand|}}git:{{[-]}} Cloning into '%s'...", templatesDir)

	_, err := git.PlainClone(templatesDir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: plumbing.ReferenceName("refs/heads/" + branch),
	})
	if err != nil {
		return err
	}

	return nil
}
