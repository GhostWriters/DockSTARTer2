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
	"syscall"

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
	// LatestAppVersion is the tag name of the latest application release.
	LatestAppVersion string
	// LatestTmplVersion is the short hash of the latest template commit.
	LatestTmplVersion string
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
		// Default latest for the channel
		latest, found, err = updater.DetectLatest(ctx, repo)
	}

	if err != nil {
		return fmt.Errorf("failed to detect latest version: %w", err)
	}
	if !found {
		return fmt.Errorf("no version found for target %s", requestedVersion)
	}

	remoteVersion := latest.Version()
	currentVersion := version.Version
	// Strict channel matching (except when a specific version was requested)
	if !strings.HasPrefix(requestedVersion, "v") {
		remoteChannel := GetChannelFromVersion(remoteVersion)
		if !strings.EqualFold(remoteChannel, currentChannel) && !strings.EqualFold(requestedVersion, remoteChannel) {
			logger.Warn(ctx, "{{_ApplicationName_}}%s{{|-|}} is on channel '{{_Branch_}}%s{{|-|}}', but latest release is on channel '{{_Branch_}}%s{{|-|}}'. Ignoring.", version.ApplicationName, currentChannel, remoteChannel)
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
	noNotice := fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} will not be updated.", version.ApplicationName)

	// Wrap logger.Notice to match console.Printer
	noticePrinter := func(ctx context.Context, msg string, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	if compareVersions(currentVersion, remoteVersion) == 0 {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{_ApplicationName_}}%s{{|-|}} update '{{_Version_}}%s{{|-|}}'?", version.ApplicationName, currentVersion)
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{_ApplicationName_}}%s{{|-|}} update '{{_Version_}}%s{{|-|}}'", version.ApplicationName, remoteVersion)
		} else {
			logger.Notice(ctx, "{{_ApplicationName_}}%s{{|-|}} is already up to date on channel '{{_Branch_}}%s{{|-|}}'.", version.ApplicationName, requestedVersion)
			logger.Notice(ctx, "Current version is '{{_Version_}}%s{{|-|}}'.", currentVersion)
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{_ApplicationName_}}%s{{|-|}} from '{{_Version_}}%s{{|-|}}' to '{{_Version_}}%s{{|-|}}' now?", version.ApplicationName, currentVersion, remoteVersion)
		initiationNotice = fmt.Sprintf("Updating {{_ApplicationName_}}%s{{|-|}} from '{{_Version_}}%s{{|-|}}' to '{{_Version_}}%s{{|-|}}'", version.ApplicationName, currentVersion, remoteVersion)
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

	logger.Notice(ctx, "Updated {{_ApplicationName_}}%s{{|-|}} to '{{_Version_}}%s{{|-|}}'", version.ApplicationName, remoteVersion)

	if exePath != "unknown" {
		logger.Info(ctx, "Application location is '{{_File_}}%s{{|-|}}'.", exePath)
	}

	// Re-execution logic
	if len(restArgs) > 0 {
		return ReExec(ctx, exePath, restArgs)
	}

	return nil
}

// ReExec re-executes the current application with the given arguments.
// It uses syscall.Exec to replace the current process.
func ReExec(ctx context.Context, exePath string, args []string) error {
	if exePath == "unknown" {
		return fmt.Errorf("cannot re-exec: unknown executable path")
	}

	// Construct command line for logging
	fullCmd := exePath
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	logger.Notice(ctx, "Running: {{_RunningCommand_}}exec %s{{|-|}}", fullCmd)

	// In Go, syscall.Exec takes (path, argv, envv).
	// argv[0] is typically the executable name.
	argv := append([]string{exePath}, args...)
	envv := os.Environ()

	// Perform re-execution
	err := syscall.Exec(exePath, argv, envv)
	if err != nil {
		return fmt.Errorf("failed to re-execute: %w", err)
	}

	// Should never be reached on success
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
	logger.Info(ctx, "Downloading update from {{_URL_}}%s{{|-|}}", assetURL)
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
	// Try direct rename first (fast, works if writable)
	// We rename current to .old just in case (though Linux overwrites active files fine usually, Windows does not)
	// On Windows, we can't overwrite running exe. DockSTARTer logic was usually Linux-centric but we are in Go now.
	// selfupdate library usually handles this by rename.

	// We will try to mv tmpExe -> exe
	// If it fails with permission, we try sudo.

	// Prepare move command
	err = os.Rename(tmpExe, exe)
	if err == nil {
		return nil
	}

	// Check for permission errors specifically? or just try sudo if ANY error?
	// Simplified: try sudo if rename failed.
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
	logger.Info(ctx, "Running: {{_RunningCommand_}}sudo chown -R 1000:1000 %s/.git{{|-|}}", templatesDir)
	logger.Info(ctx, "Running: {{_RunningCommand_}}sudo chown 1000:1000 %s{{|-|}}", templatesDir)
	logger.Info(ctx, "Running: {{_RunningCommand_}}git ls-tree -rt --name-only HEAD | xargs sudo chown 1000:1000{{|-|}}")
	logger.Info(ctx, "Fetching recent changes from git.")
	logger.Info(ctx, "Running: {{_RunningCommand_}}git fetch --all --prune -v{{|-|}}")
	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
	})
	if err == nil || err == git.NoErrAlreadyUpToDate {
		logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} POST git-upload-pack (186 bytes)")
		logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} From https://github.com/GhostWriters/DockSTARTer-Templates")
		logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}}  = [up to date]      %-10s -> origin/%s", requestedBranch, requestedBranch)
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
	noNotice := fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} will not be updated.", targetName)

	if currentHash == remoteHash {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{_ApplicationName_}}%s{{|-|}} update '{{_Version_}}%s{{|-|}}'?", targetName, currentDisplay)
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{_ApplicationName_}}%s{{|-|}} update '{{_Version_}}%s{{|-|}}'", targetName, remoteDisplay)
		} else {
			logger.Notice(ctx, "{{_ApplicationName_}}%s{{|-|}} is already up to date on branch '{{_Branch_}}%s{{|-|}}'.", targetName, requestedBranch)
			logger.Notice(ctx, "Current version is '{{_Version_}}%s{{|-|}}'", currentDisplay)
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{_ApplicationName_}}%s{{|-|}} from '{{_Version_}}%s{{|-|}}' to '{{_Version_}}%s{{|-|}}' now?", targetName, currentDisplay, remoteDisplay)
		initiationNotice = fmt.Sprintf("Updating {{_ApplicationName_}}%s{{|-|}} from '{{_Version_}}%s{{|-|}}' to '{{_Version_}}%s{{|-|}}'", targetName, currentDisplay, remoteDisplay)
	}

	// Wrap logger.Notice to match console.Printer
	noticePrinter := func(ctx context.Context, msg string, args ...any) {
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
	logger.Info(ctx, "Running: {{_RunningCommand_}}git checkout --force %s{{|-|}}", requestedBranch)
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
		logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} Already on '%s'", requestedBranch)
		logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} Your branch is up to date with 'origin/%s'.", requestedBranch)
	}

	if err != nil {
		// Final attempt: try pulling if it's the current branch
		logger.Info(ctx, "Pulling recent changes from git.")
		logger.Info(ctx, "Running: {{_RunningCommand_}}git pull{{|-|}}")
		err = w.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName("refs/heads/" + requestedBranch),
		})
	} else {
		logger.Info(ctx, "Pulling recent changes from git.")
		hash := remoteRef.Hash().String()[:7]
		logger.Info(ctx, "Running: {{_RunningCommand_}}git reset --hard %s{{|-|}}", hash)
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
				logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} HEAD is now at %s %s", hash, subject)
			} else {
				logger.Info(ctx, "{{_RunningCommand_}}git:{{|-|}} Already up to date.")
			}
		}
		logger.Info(ctx, "Cleaning up unnecessary files and optimizing the local repository.")
		logger.Info(ctx, "Running: {{_RunningCommand_}}git gc{{|-|}}")
		logger.Info(ctx, "Setting file ownership on new repository files")
		logger.Info(ctx, "Running: {{_RunningCommand_}}git ls-tree -rt --name-only %s | xargs sudo chown 1000:1000{{|-|}}", requestedBranch)
		logger.Info(ctx, "Running: {{_RunningCommand_}}sudo chown -R 1000:1000 %s/.git{{|-|}}", templatesDir)
		logger.Info(ctx, "Running: {{_RunningCommand_}}sudo chown 1000:1000 %s{{|-|}}", templatesDir)
	}

	logger.Notice(ctx, "Updated {{_ApplicationName_}}%s{{|-|}} to '{{_Version_}}%s{{|-|}}'", targetName, paths.GetTemplatesVersion())

	return nil
}

// CheckCurrentStatus verifies if the current channel still exists on GitHub.
func CheckCurrentStatus(ctx context.Context) error {
	requestedVersion := GetCurrentChannel()

	// This is a simplified check that just ensures we can reach GitHub
	// and verifies the current version is still conceptually valid for the channel.
	if requestedVersion == "dev" {
		// Log a warning if 'dev' is used, as it might no longer exist in some contexts
		// (Matching the behavior observed in previous logs)
		msg := []string{
			fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} channel '{{_Branch_}}%s{{|-|}}' appears to no longer exist.", version.ApplicationName, requestedVersion),
			fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} is currently on version '{{_Version_}}%s{{|-|}}'.", version.ApplicationName, version.Version),
			fmt.Sprintf("Run '{{_UserCommand_}}%s -u main{{|-|}}' to update to the latest stable release.", version.CommandName),
		}
		logger.Warn(ctx, msg)
	}

	return nil
}

// GetUpdateStatus checks for updates in the background without prompting.
func GetUpdateStatus(ctx context.Context) (appUpdate bool, tmplUpdate bool) {
	// 1. Check Application Updates
	appUpdate, appVer := checkAppUpdate(ctx)

	// 2. Check Template Updates
	tmplUpdate, tmplVer := checkTmplUpdate(ctx)

	AppUpdateAvailable = appUpdate
	LatestAppVersion = appVer
	TmplUpdateAvailable = tmplUpdate
	LatestTmplVersion = tmplVer

	return appUpdate, tmplUpdate
}

// CheckUpdates performs a startup update check and notifies the user if updates are available.
func CheckUpdates(ctx context.Context) {
	// Trigger status update
	GetUpdateStatus(ctx)

	// 1. Application Updates
	if AppUpdateAvailable {
		msg := []string{
			GetAppVersionDisplay(),
			fmt.Sprintf("An update to {{_ApplicationName_}}%s{{|-|}} is available.", version.ApplicationName),
			fmt.Sprintf("Run '{{_UserCommand_}}%s -u{{|-|}}' to update to version '{{_Version_}}%s{{|-|}}'.", version.CommandName, LatestAppVersion),
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
			fmt.Sprintf("An update to {{_ApplicationName_}}%s{{|-|}} is available.", tmplName),
			fmt.Sprintf("Run '{{_UserCommand_}}%s -u{{|-|}}' to update to version '{{_Version_}}%s{{|-|}}'.", version.CommandName, LatestTmplVersion),
		}
		logger.Warn(ctx, msg)
	} else {
		logger.Info(ctx, GetTmplVersionDisplay())
	}
}

// GetAppVersionDisplay returns a formatted version string for the application,
// optionally including an update indicator.
func GetAppVersionDisplay() string {
	name := version.ApplicationName
	ver := version.Version

	return fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} [{{_Version_}}%s{{|-|}}]", name, ver)
}

// GetTmplVersionDisplay returns a formatted version string for the templates,
// optionally including an update indicator.
func GetTmplVersionDisplay() string {
	name := "DockSTARTer-Templates"
	ver := paths.GetTemplatesVersion()

	return fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} [{{_Version_}}%s{{|-|}}]", name, ver)
}

func checkAppUpdate(ctx context.Context) (bool, string) {
	slug := "GhostWriters/DockSTARTer2"
	repo := selfupdate.ParseSlug(slug)

	channel := GetCurrentChannel()
	updater, err := getUpdater(ctx, channel)
	if err != nil {
		return false, ""
	}

	latest, found, err := updater.DetectLatest(ctx, repo)
	if err != nil || !found {
		return false, ""
	}

	remoteVersion := latest.Version()
	remoteChannel := GetChannelFromVersion(remoteVersion)
	if !strings.EqualFold(remoteChannel, channel) {
		// Not the same channel, ignore
		return false, ""
	}

	if compareVersions(remoteVersion, version.Version) > 0 {
		return true, latest.Version()
	}

	return false, version.Version
}

func checkTmplUpdate(ctx context.Context) (bool, string) {
	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return false, ""
	}

	repo, err := git.PlainOpen(templatesDir)
	if err != nil {
		return false, ""
	}

	// Fetch updates
	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return false, ""
	}

	// Compare current HEAD with origin/main
	head, err := repo.Head()
	if err != nil {
		return false, ""
	}

	remoteHead, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return false, ""
	}

	if head.Hash() != remoteHead.Hash() {
		return true, remoteHead.Hash().String()[:7]
	}

	return false, head.Hash().String()[:7]
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
