package update

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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
	slug := "GhostWriters/DockSTARTer2"
	repo := selfupdate.ParseSlug(slug)

	currentChannel := GetCurrentChannel()
	if requestedVersion == "" {
		requestedVersion = currentChannel
	}

	var (
		latest *selfupdate.Release
		found  bool
		err    error
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
			logger.Warn(ctx, "[_ApplicationName_]%s[-] is on channel '[_Branch_]%s[-]', but latest release is on channel '[_Branch_]%s[-]'. Ignoring.", version.ApplicationName, currentChannel, remoteChannel)
			return nil
		}
	}

	question := ""
	initiationNotice := ""
	noNotice := fmt.Sprintf("[_ApplicationName_]%s[-] will not be updated.", version.ApplicationName)

	// Wrap logger.Notice to match console.Printer
	noticePrinter := func(ctx context.Context, msg string, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	if currentVersion == remoteVersion {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply [_ApplicationName_]%s[-] update '[_Version_]%s[-]'?", version.ApplicationName, currentVersion)
			initiationNotice = fmt.Sprintf("Forcefully re-applying [_ApplicationName_]%s[-] update '[_Version_]%s[-]'", version.ApplicationName, remoteVersion)
		} else {
			logger.Notice(ctx, "[_ApplicationName_]%s[-] is already up to date on channel '%s'.", version.ApplicationName, requestedVersion)
			logger.Notice(ctx, "Current version is '[_Version_]%s[-]'", currentVersion)
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]' now?", version.ApplicationName, currentVersion, remoteVersion)
		initiationNotice = fmt.Sprintf("Updating [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]'", version.ApplicationName, currentVersion, remoteVersion)
	}

	// Prompt user
	if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
		logger.Notice(ctx, noNotice)
		return nil
	}

	// Execution
	logger.Notice(ctx, initiationNotice)
	if strings.HasPrefix(requestedVersion, "v") {
		err = selfupdate.UpdateTo(ctx, version.Version, requestedVersion, slug)
	} else {
		_, err = updater.UpdateSelf(ctx, version.Version, repo)
	}

	if err != nil {
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "Access is denied") {
			logger.Warn(ctx, "Permission denied. Attempting to run with sudo...")
			exe, _ := os.Executable()
			args := os.Args[1:]
			cmd := exec.Command("sudo", append([]string{exe}, args...)...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if runErr := cmd.Run(); runErr != nil {
				return fmt.Errorf("failed to update with sudo: %w", runErr)
			}
			return nil
		}
		return fmt.Errorf("failed to update application: %w", err)
	}

	logger.Notice(ctx, "Updated [_ApplicationName_]%s[-] to '[_Version_]%s[-]'", version.ApplicationName, remoteVersion)

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
	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
	})
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
	noNotice := fmt.Sprintf("[_ApplicationName_]%s[-] will not be updated.", targetName)

	if currentHash == remoteHash {
		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply [_ApplicationName_]%s[-] update '[_Version_]%s[-]'?", targetName, currentDisplay)
			initiationNotice = fmt.Sprintf("Forcefully re-applying [_ApplicationName_]%s[-] update '[_Version_]%s[-]'", targetName, remoteDisplay)
		} else {
			logger.Notice(ctx, "[_ApplicationName_]%s[-] is already up to date on branch '[_Branch_]%s[-]'.", targetName, requestedBranch)
			logger.Notice(ctx, "Current version is '[_Version_]%s[-]'", currentDisplay)
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]' now?", targetName, currentDisplay, remoteDisplay)
		initiationNotice = fmt.Sprintf("Updating [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]'", targetName, currentDisplay, remoteDisplay)
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

	if err != nil {
		// Final attempt: try pulling if it's the current branch
		err = w.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName("refs/heads/" + requestedBranch),
		})
	}

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to update templates to %s: %w", requestedBranch, err)
	}

	logger.Notice(ctx, "Updated [_ApplicationName_]%s[-] to '[_Version_]%s[-]'", targetName, paths.GetTemplatesVersion())

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
			fmt.Sprintf("[_ApplicationName_]%s[-] channel '[_Branch_]%s[-]' appears to no longer exist.", version.ApplicationName, requestedVersion),
			fmt.Sprintf("[_ApplicationName_]%s[-] is currently on version '[_Version_]%s[-]'.", version.ApplicationName, version.Version),
			fmt.Sprintf("Run '[_UserCommand_]%s -u main[-] to update to the latest stable release.", version.CommandName),
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
			fmt.Sprintf("An update to [_ApplicationName_]%s[-] is available.", version.ApplicationName),
			fmt.Sprintf("Run '[_UserCommand_]%s -u[-]' to update to version '[_Version_]%s[-]'.", version.CommandName, LatestAppVersion),
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
			fmt.Sprintf("An update to [_ApplicationName_]%s[-] is available.", tmplName),
			fmt.Sprintf("Run '[_UserCommand_]%s -u[-]' to update to version '[_Version_]%s[-]'.", version.CommandName, LatestTmplVersion),
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

	return fmt.Sprintf("[_ApplicationName_]%s[-] [[_Version_]%s[-]]", name, ver)
}

// GetTmplVersionDisplay returns a formatted version string for the templates,
// optionally including an update indicator.
func GetTmplVersionDisplay() string {
	name := "DockSTARTer-Templates"
	ver := paths.GetTemplatesVersion()

	return fmt.Sprintf("[_ApplicationName_]%s[-] [[_Version_]%s[-]]", name, ver)
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

	latestVer, err := semver.NewVersion(remoteVersion)
	if err != nil {
		return false, ""
	}
	currentVer, err := semver.NewVersion(version.Version)
	if err != nil {
		return false, ""
	}

	if latestVer.GreaterThan(currentVer) {
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
	return -1
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
