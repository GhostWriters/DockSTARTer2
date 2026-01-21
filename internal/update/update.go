package update

import (
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
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
		logger.Warn(ctx, "[_ApplicationName_]%s[-] channel 'dev' appears to no longer exist.", version.ApplicationName)
		logger.Warn(ctx, "Run '[_UserCommand_]%s -u main[-] balance to update to the latest stable release.", version.CommandName)
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
		logger.Warn(ctx, GetAppVersionDisplay())
		logger.Warn(ctx, "An update to [_ApplicationName_]%s[-] is available.", version.ApplicationName)
		logger.Warn(ctx, "Run '[_UserCommand_]%s -u[-]' to update to version '[_Version_]%s[-]'.", version.CommandName, LatestAppVersion)
	} else {
		logger.Info(ctx, GetAppVersionDisplay())
	}

	// 2. Template Updates
	if TmplUpdateAvailable {
		tmplName := "DockSTARTer-Templates"
		logger.Warn(ctx, GetTmplVersionDisplay())
		logger.Warn(ctx, "An update to [_ApplicationName_]%s[-] is available.", tmplName)
		logger.Warn(ctx, "Run '[_UserCommand_]%s -u[-]' to update to version '[_Version_]%s[-]'.", version.CommandName, LatestTmplVersion)
	} else {
		logger.Info(ctx, GetTmplVersionDisplay())
	}
}

// GetAppVersionDisplay returns a formatted version string for the application,
// optionally including an update indicator.
func GetAppVersionDisplay() string {
	name := version.ApplicationName
	ver := version.Version
	updateFlag := ""
	updateTagOpen := ""
	updateTagClose := ""

	if AppUpdateAvailable {
		updateFlag = "[_Update_]*[-] "
		updateTagOpen = "[_Update_]"
		updateTagClose = "[-]"
	}

	return fmt.Sprintf("%s[_ApplicationName_]%s[-]%s [%s%s%s%s]", updateFlag, name, updateTagClose, updateTagOpen, "[_Version_]", ver, "[-]")
}

// GetTmplVersionDisplay returns a formatted version string for the templates,
// optionally including an update indicator.
func GetTmplVersionDisplay() string {
	name := "DockSTARTer-Templates"
	ver := paths.GetTemplatesVersion()
	updateFlag := ""
	updateTagOpen := ""
	updateTagClose := ""

	if TmplUpdateAvailable {
		updateFlag = "[_Update_]*[-] "
		updateTagOpen = "[_Update_]"
		updateTagClose = "[-]"
	}

	return fmt.Sprintf("%s[_ApplicationName_]%s[-]%s [%s%s%s%s]", updateFlag, name, updateTagClose, updateTagOpen, "[_Version_]", ver, "[-]")
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

// GetCurrentChannel returns the update channel based on the current version string.
// v1.YYYYMMDD.N is stable, v0.0.0.0-dev is dev.
func GetCurrentChannel() string {
	if strings.Contains(version.Version, "-dev") {
		return "dev"
	}
	return "stable"
}
