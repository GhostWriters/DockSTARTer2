package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockercheck"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"

	"github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
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
			fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} is currently on version '%s'.", version.ApplicationName, AppVersionLink(version.Version)),
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

const updateCheckCacheFile = "update_check"
const updateCheckCacheDuration = 24 * time.Hour

func updateCheckTimestampPath() string {
	return filepath.Join(paths.GetTimestampsDir(), updateCheckCacheFile)
}

// updateCheckFresh returns true if the update check timestamp is less than
// 24 hours old, meaning we can skip the network check.
func updateCheckFresh() bool {
	info, err := os.Stat(updateCheckTimestampPath())
	return err == nil && time.Since(info.ModTime()) < updateCheckCacheDuration
}

// touchUpdateCheckTimestamp updates the mtime of the update check timestamp
// file to now, creating it if it doesn't exist.
func touchUpdateCheckTimestamp() {
	_ = os.MkdirAll(paths.GetTimestampsDir(), 0755)
	path := updateCheckTimestampPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte{}, 0644)
	} else {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
}

// CheckUpdates performs a startup update check and notifies the user if updates
// are available. Skipped if the check was performed within the last 24 hours,
// unless --force is set.
func CheckUpdates(ctx context.Context) {
	if !console.Force() && updateCheckFresh() {
		return
	}

	// Create a timeout context for the update check (3 seconds max)
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Run update check synchronously (before user command executes)
	GetUpdateStatus(checkCtx)

	if UpdateCheckError {
		logger.Warn(ctx, "Failed to check for updates (network timeout or error).")
		return
	}

	// Touch the timestamp so we skip the check for the next 24 hours.
	touchUpdateCheckTimestamp()

	if AppUpdateAvailable {
		logger.Warn(ctx, []string{
			GetAppVersionDisplay(),
			fmt.Sprintf("An update to {{|ApplicationName|}}%s{{[-]}} is available.", version.ApplicationName),
			fmt.Sprintf("Run '{{|UserCommand|}}%s -u{{[-]}}' to update to version '%s'.", version.CommandName, AppVersionLink(LatestAppVersion)),
		})
	} else {
		logger.Info(ctx, GetAppVersionDisplay())
	}
	if TmplUpdateAvailable {
		logger.Warn(ctx, []string{
			GetTmplVersionDisplay(),
			fmt.Sprintf("An update to {{|ApplicationName|}}%s{{[-]}} is available.", "DockSTARTer-Templates"),
			fmt.Sprintf("Run '{{|UserCommand|}}%s -u{{[-]}}' to update to version '%s'.", version.CommandName, TmplVersionLink(LatestTmplVersion)),
		})
	} else {
		logger.Info(ctx, GetTmplVersionDisplay())
	}
	logger.Info(ctx, GetComposeSdkVersionDisplay())
}

// versionTag wraps a version string in a semstyle Version hyperlink tag pointing at the
// given URL. Used so version numbers in CLI/log output are clickable links to their source.
// An empty url renders the version as plain styled text with no link.
func versionTag(ver, url string) string {
	return console.FormatLink("Version", ver, url)
}

// AppVersionLink wraps a DockSTARTer2 version string as a link to its GitHub release tag.
func AppVersionLink(ver string) string {
	return versionTag(ver, "https://github.com/GhostWriters/DockSTARTer2/releases/tag/"+ver)
}

// TmplVersionLink wraps a DockSTARTer-Templates version string as a link to its source on
// GitHub. Tagged versions link to the release tag; the "branch commit hash" fallback form
// links to that commit.
func TmplVersionLink(ver string) string {
	if _, hash, ok := strings.Cut(ver, " commit "); ok {
		return versionTag(ver, "https://github.com/GhostWriters/DockSTARTer-Templates/commit/"+hash)
	}
	if ver == "" || ver == "Unknown Version" {
		return versionTag(ver, "")
	}
	return versionTag(ver, "https://github.com/GhostWriters/DockSTARTer-Templates/releases/tag/"+ver)
}

// ComposeSdkVersionLink wraps a Docker Compose SDK version string as a link to its GitHub tag.
func ComposeSdkVersionLink(ver string) string {
	if ver == "" || ver == "unknown" {
		return versionTag(ver, "")
	}
	return versionTag(ver, "https://github.com/docker/compose/releases/tag/"+ver)
}

// branchTag wraps a branch/channel name in a semstyle Branch hyperlink tag pointing at the
// given URL. An empty url renders the name as plain styled text with no link.
func branchTag(name, url string) string {
	return console.FormatLink("Branch", name, url)
}

// AppBranchLink wraps a DockSTARTer2 channel name as a link to the matching branch on GitHub.
// The release workflow names channel branches to match the channel (e.g. "Prerelease"),
// except "stable", which is released from "main".
func AppBranchLink(name string) string {
	branch := name
	if branch == "stable" {
		branch = "main"
	}
	return branchTag(name, "https://github.com/GhostWriters/DockSTARTer2/tree/"+branch)
}

// TmplBranchLink wraps a DockSTARTer-Templates branch name as a link to that branch on GitHub.
func TmplBranchLink(name string) string {
	return branchTag(name, "https://github.com/GhostWriters/DockSTARTer-Templates/tree/"+name)
}

// GetAppVersionDisplay returns a formatted version string for the application,
// optionally including an update indicator.
func GetAppVersionDisplay() string {
	return fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [%s]", version.ApplicationName, AppVersionLink(version.Version))
}

// GetTmplVersionDisplay returns a formatted version string for the templates,
// optionally including an update indicator.
func GetTmplVersionDisplay() string {
	return fmt.Sprintf("{{|ApplicationName|}}DockSTARTer-Templates{{[-]}} [%s]", TmplVersionLink(paths.GetTemplatesVersion()))
}

// GetComposeSdkVersionDisplay returns a formatted version string for the Docker Compose SDK.
func GetComposeSdkVersionDisplay() string {
	return fmt.Sprintf("{{|ApplicationName|}}Docker Compose SDK{{[-]}} [%s]", ComposeSdkVersionLink(version.GetComposeSdkVersion()))
}

// GetDockerDaemonVersionDisplay returns a formatted version string for the
// Docker daemon DS2 talks to. Uses the startup probe's cached result when
// available, probing fresh otherwise (e.g. when the startup check was
// skipped this invocation). The daemon is the one piece DS2 doesn't ship,
// hence the "external dependency" label.
func GetDockerDaemonVersionDisplay(ctx context.Context) string {
	return formatDockerDaemonVersion(dockerStatus(ctx))
}

// GetDockerAPIVersionDisplay returns a formatted version string for the
// Docker daemon's API version, shown alongside GetDockerDaemonVersionDisplay.
// Normally this is just the daemon's max API version. If the client actually
// negotiated down to something lower (e.g. DOCKER_API_VERSION pinning an
// older version), both are shown as [max/negotiated negotiated], max first
// (the daemon's installed capability) then the negotiated version explicitly
// labeled and in the Error style to flag that it's not using the daemon's
// full capability.
func GetDockerAPIVersionDisplay(ctx context.Context) string {
	return formatDockerAPIVersion(dockerStatus(ctx))
}

func formatDockerDaemonVersion(st dockercheck.Status) string {
	if !st.Reachable || st.ServerVersion == "" {
		return "{{|ApplicationName|}}Docker Engine{{[-]}} [{{|Error|}}not detected{{[-]}}] (external dependency)"
	}
	ver := versionTag("v"+st.ServerVersion, "https://github.com/moby/moby/releases/tag/docker-v"+st.ServerVersion)
	return fmt.Sprintf("{{|ApplicationName|}}Docker Engine{{[-]}} [%s] (external dependency)", ver)
}

func formatDockerAPIVersion(st dockercheck.Status) string {
	if !st.Reachable || st.APIVersion == "" {
		return "{{|ApplicationName|}}Docker API{{[-]}} [{{|Error|}}not detected{{[-]}}] (external dependency)"
	}
	maxVer := versionTag("v"+st.APIVersion, "https://docs.docker.com/reference/api/engine/version/v"+st.APIVersion+"/")
	if st.NegotiatedAPIVersion == "" || st.NegotiatedAPIVersion == st.APIVersion {
		return fmt.Sprintf("{{|ApplicationName|}}Docker API{{[-]}} [%s] (external dependency)", maxVer)
	}
	negotiated := console.FormatLink("Error", "v"+st.NegotiatedAPIVersion, "https://docs.docker.com/reference/api/engine/version/v"+st.NegotiatedAPIVersion+"/")
	return fmt.Sprintf("{{|ApplicationName|}}Docker API{{[-]}} [%s/%s {{|Error|}}negotiated{{[-]}}] (external dependency)", maxVer, negotiated)
}

// fatalSystemInfo builds the extra diagnostic lines (Compose SDK, Docker
// Engine, Docker API) appended to logger's fatal-crash system info. The
// Docker lines use only the startup probe's cached status (dockercheck.Last)
// -- never a fresh probe -- since a fatal handler must not block on a
// network/socket call while the process is already crashing; if no check
// has run yet, those lines are simply omitted rather than shown as
// misleadingly "not detected".
func fatalSystemInfo() []string {
	lines := []string{GetComposeSdkVersionDisplay()}
	if st := dockercheck.Last(); st != nil {
		lines = append(lines, formatDockerDaemonVersion(*st), formatDockerAPIVersion(*st))
	}
	return lines
}

// fatalPathsInfo builds the extra path lines (the user's compose/config app
// folders) appended to logger's fatal-crash Paths section. Loading the
// config here is just a local TOML read, safe for a fatal handler.
func fatalPathsInfo() []string {
	conf := config.LoadAppConfig()
	return []string{
		fmt.Sprintf("%-19s %s", "App Config Folder:", console.FormatFolderPath(conf.ConfigDir)),
		fmt.Sprintf("%-19s %s", "Compose Folder:", console.FormatFolderPath(conf.ComposeDir)),
	}
}

func init() {
	logger.ExtraSystemInfo = fatalSystemInfo
	logger.ExtraPathsInfo = fatalPathsInfo
}

func dockerStatus(ctx context.Context) dockercheck.Status {
	st := dockercheck.Last()
	if st == nil {
		fresh := dockercheck.Check(ctx)
		st = &fresh
	}
	return *st
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

func checkTmplUpdate(_ context.Context) (updateAvailable bool, ver string, hadError bool) {
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
		Tags:       git.AllTags,
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

	// resolveTemplatesTarget applies the same main-means-latest-reachable-tag
	// policy as the real update flow in update_templates.go, so this
	// indicator never disagrees with it.
	remoteHead, _, err := resolveTemplatesTarget(repo, head, currentBranch, currentBranch)
	if err != nil {
		return false, "", false // Remote branch not found — not an error
	}

	if head.Hash() != remoteHead.Hash() {
		return true, templatesRefDisplay(repo, currentBranch, remoteHead), false
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
func getUpdater(_ context.Context, channel string) (*selfupdate.Updater, error) {
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
		if GetChannelFromVersion(tagName) != channel {
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
