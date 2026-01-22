package update

import (
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
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
	if requestedVersion == "" || requestedVersion == "stable" {
		requestedVersion = currentChannel
	}

	logger.Info(ctx, "Checking for updates to [_ApplicationName_]%s[-] (%s channel)...", version.ApplicationName, requestedVersion)

	latest, err := selfupdate.UpdateSelf(ctx, version.Version, repo)
	if err != nil {
		return fmt.Errorf("failed to update application: %w", err)
	}

	latestVer, err := semver.NewVersion(latest.Version())
	if err != nil {
		return fmt.Errorf("failed to parse latest version: %w", err)
	}
	currentVer := semver.MustParse(version.Version)

	if latestVer.Equal(currentVer) {
		logger.Notice(ctx, "[_ApplicationName_]%s[-] is already up to date.", version.ApplicationName)
	} else {
		logger.Notice(ctx, "Successfully updated [_ApplicationName_]%s[-] to version [_Version_]%s[-].", version.ApplicationName, latest.Version())
		logger.Notice(ctx, "Please restart the application.")
	}

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

	logger.Info(ctx, "Updating templates at %s (branch %s)...", templatesDir, requestedBranch)

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = w.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.ReferenceName("refs/heads/" + requestedBranch),
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			logger.Notice(ctx, "Templates are already up to date.")
			return nil
		}
		return fmt.Errorf("failed to pull templates: %w", err)
	}

	head, _ := repo.Head()
	logger.Notice(ctx, "Successfully updated templates to commit [_Version_]%s[-].", head.Hash().String()[:7])

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

	latest, err := selfupdate.UpdateSelf(ctx, version.Version, repo)
	if err != nil {
		return false, ""
	}

	latestVer, err := semver.NewVersion(latest.Version())
	if err != nil {
		return false, ""
	}
	currentVer := semver.MustParse(version.Version)

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
	if len(p1) < len(p2) {
		if strings.Contains(p2[len(p1)], "-") {
			return 1
		}
		return -1
	}

	return 0
}

// GetCurrentChannel returns the update channel based on the current version string.
// v1.YYYYMMDD.N is stable, v0.0.0.0-dev is dev.
func GetCurrentChannel() string {
	if strings.Contains(version.Version, "-dev") {
		return "dev"
	}
	return "stable"
}
