package update

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"

	"github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

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
	// Check Application Updates
	appUpdate, appVer, appErr := checkAppUpdate(ctx)

	// Check Template Updates
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
		// Check Application Updates
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

		// Check Template Updates
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

		// Docker Compose SDK
		logger.Info(ctx, GetComposeSdkVersionDisplay())
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

// GetComposeSdkVersionDisplay returns a formatted version string for the Docker Compose SDK.
func GetComposeSdkVersionDisplay() string {
	name := "Docker Compose SDK"
	ver := version.GetComposeSdkVersion()

	return fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", name, ver)
}

func checkAppUpdate(ctx context.Context) (updateAvailable bool, ver string, hadError bool) {
	slug := "GhostWriters/DockSTARTer2"
	repo := selfupdate.ParseSlug(slug)

	channel := GetCurrentChannel()

	// Quick check using git ls-remote to see if tags for this channel exist.
	// This avoids hitting the GitHub releases API unnecessarily.
	channelTag, err := latestChannelTag(channel)
	if err != nil {
		return false, "", true
	}
	if channelTag == "" {
		return false, "", false
	}

	updater, err := getUpdater(ctx, channel)
	if err != nil {
		return false, "", true
	}

	latest, found, err := updater.DetectVersion(ctx, repo, channelTag)
	if err != nil || !found {
		return false, "", true
	}

	if compareVersions(latest.Version(), version.Version) > 0 {
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

	// Compare current HEAD with the remote tracking branch for the current branch
	head, err := repo.Head()
	if err != nil {
		return false, "", false // Repo issue, not network error
	}

	// Determine which remote branch to compare against (stay on current branch)
	currentBranch := "main"
	if head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	remoteHead, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+currentBranch), true)
	if err != nil {
		return false, "", false // Remote branch not found — not an error
	}

	if head.Hash() != remoteHead.Hash() {
		remoteHash := remoteHead.Hash().String()
		if len(remoteHash) > 7 {
			remoteHash = remoteHash[:7]
		}

		// Try to find a tag for the remote commit
		tags, _ := repo.Tags()
		foundTag := ""
		_ = tags.ForEach(func(ref *plumbing.Reference) error {
			if ref.Hash() == remoteHead.Hash() {
				foundTag = ref.Name().Short()
				return fmt.Errorf("found")
			}
			return nil
		})

		var remoteDisplay string
		if foundTag != "" {
			remoteDisplay = foundTag
		} else {
			remoteDisplay = fmt.Sprintf("%s commit %s", currentBranch, remoteHash)
		}

		return true, remoteDisplay, false
	}

	return false, paths.GetTemplatesVersion(), false
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

		// Handle suffixes (e.g. "9-Prerelease" vs "13-Prerelease")
		// Split off the suffix and compare the numeric part first.
		h1 := strings.Contains(s1, "-")
		h2 := strings.Contains(s2, "-")
		if h1 || h2 {
			n1str, sfx1, _ := strings.Cut(s1, "-")
			n2str, sfx2, _ := strings.Cut(s2, "-")
			n1, e1 := strconv.Atoi(n1str)
			n2, e2 := strconv.Atoi(n2str)
			if e1 == nil && e2 == nil && n1 != n2 {
				if n1 > n2 {
					return 1
				}
				return -1
			}
			// Numeric parts equal (or non-numeric): compare suffixes.
			// No suffix > has suffix (stable > pre-release).
			if sfx1 == "" && sfx2 != "" {
				return 1
			}
			if sfx1 != "" && sfx2 == "" {
				return -1
			}
			if sfx1 > sfx2 {
				return 1
			}
			if sfx1 < sfx2 {
				return -1
			}
			continue
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

// latestChannelTag returns the most recent tag for the given channel by
// listing remote tags and picking the lexicographically greatest match.
// Version tags sort correctly lexicographically (v2.YYYYMMDD.N[-suffix]).
func latestChannelTag(channel string) (string, error) {
	remote := git.NewRemote(nil, &gitConfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/GhostWriters/DockSTARTer2.git"},
	})

	refs, err := remote.List(&git.ListOptions{})
	if err != nil {
		return "", err
	}

	latest := ""
	for _, ref := range refs {
		if !ref.Name().IsTag() {
			continue
		}
		tagName := ref.Name().Short()
		if !strings.EqualFold(GetChannelFromVersion(tagName), channel) {
			continue
		}
		if compareVersions(tagName, latest) > 0 {
			latest = tagName
		}
	}
	return latest, nil
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
