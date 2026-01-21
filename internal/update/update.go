package update

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// SelfUpdate handles updating the application binary using GitHub Releases.
// If requestedVersion is "stable" or empty, it looks for the latest release in the current channel.
// If requestedVersion is a custom string like "testing", it looks for the latest release with that suffix.
func SelfUpdate(ctx context.Context, force bool, yes bool, requestedVersion string, restArgs []string) error {
	// Map "main" or "master" to "stable" for application updates
	if strings.EqualFold(requestedVersion, "main") || strings.EqualFold(requestedVersion, "master") {
		requestedVersion = "stable"
	}

	// If no version requested, detect from current version
	wasAutoDetected := false
	if requestedVersion == "" {
		requestedVersion = GetCurrentChannel()
		wasAutoDetected = true
	}

	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	config := selfupdate.Config{
		Source: source,
	}
	updater, err := selfupdate.NewUpdater(config)
	if err != nil {
		return fmt.Errorf("failed to create updater: %w", err)
	}

	// Fetch all releases to find the best match for our custom versioning
	repo := "GhostWriters/DockSTARTer2"
	sourceReleases, err := source.ListReleases(ctx, selfupdate.ParseSlug(repo))
	if err != nil {
		return fmt.Errorf("failed to list releases: %w", err)
	}

	// Helper to find best match
	findBestMatch := func(reqVer string) (selfupdate.Release, string) {
		var bMatch selfupdate.Release
		var bVer string
		for _, rel := range sourceReleases {
			tag := rel.GetTagName()
			if !matchesRequest(tag, reqVer) {
				continue
			}

			if bVer == "" || compareVersions(tag, bVer) > 0 {
				bVer = tag
				// Detecting specifically by version to get the full Release object with asset info
				detected, ok, err := updater.DetectVersion(ctx, selfupdate.ParseSlug(repo), tag)
				if err != nil || !ok {
					continue
				}
				bMatch = *detected
			}
		}
		return bMatch, bVer
	}

	bestMatch, bestVersion := findBestMatch(requestedVersion)

	// If implicit request (auto-detected) and not found, warn and return (don't error)
	if bestVersion == "" && wasAutoDetected {
		logger.Warn(ctx, "No [_ApplicationName_]%s[-] releases found for channel '[_Branch_]%s[-]'.", version.ApplicationName, requestedVersion)
		logger.Warn(ctx, "Run '[_UserCommand_]%s -u main[-]' to update to the latest stable release.", version.CommandName)
		return nil
	}

	if bestVersion == "" {
		if requestedVersion != "" {
			return fmt.Errorf("no release found matching version/channel: %s", requestedVersion)
		}
		logger.Notice(ctx, "[_ApplicationName_]%s[-] is already up to date.", version.ApplicationName)
		return nil
	}

	// Compare with current version
	isUpToDate := compareVersions(bestVersion, version.Version) <= 0

	if !force && isUpToDate {
		logger.Notice(ctx, "[_ApplicationName_]%s[-] is already up to date on branch '[_Branch_]%s[-]'.", version.ApplicationName, requestedVersion)
		logger.Notice(ctx, "Current version is '[_Version_]%s[-]'", version.Version)
		return nil
	}

	// Printer wrapper for logger.Notice
	noticePrinter := func(ctx context.Context, msg string, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	question := ""
	if force && isUpToDate {
		question = fmt.Sprintf("Would you like to forcefully re-apply [_ApplicationName_]%s[-] update '[_Version_]%s[-]'?", version.ApplicationName, version.Version)
	} else {
		question = fmt.Sprintf("Would you like to update [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]' now?", version.ApplicationName, version.Version, bestVersion)
	}

	if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
		logger.Notice(ctx, "[_ApplicationName_]%s[-] will not be updated.", version.ApplicationName)
		return nil
	}

	if !force {
		logger.Notice(ctx, "Updating [_ApplicationName_]%s[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]'", version.ApplicationName, version.Version, bestVersion)
	} else {
		logger.Notice(ctx, "Forcefully re-applying [_ApplicationName_]%s[-] update '[_Version_]%s[-]'", version.ApplicationName, bestVersion)
	}

	err = updater.UpdateTo(ctx, &bestMatch, os.Args[0])
	if err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	// Use the success message from bash script logic if possible, or just standard success
	logger.Notice(ctx, "Updated [_ApplicationName_]%s[-] to '[_Version_]%s[-]'", version.ApplicationName, bestVersion)

	if len(restArgs) > 0 {
		logger.Notice(ctx, "Re-executing with remaining arguments: %v", restArgs)
		cmd := exec.Command(os.Args[0], restArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to re-execute updated binary: %w", err)
		}
		os.Exit(0)
	}

	logger.Notice(ctx, "Please restart the application.")
	os.Exit(0)
	return nil
}

// GetUpdateStatus checks for updates in the background without prompting.
func GetUpdateStatus(ctx context.Context) (appUpdate bool, tmplUpdate bool) {
	// 1. Check Application Updates
	appUpdate = checkAppUpdate(ctx)

	// 2. Check Template Updates
	tmplUpdate = checkTmplUpdate(ctx)

	return appUpdate, tmplUpdate
}

func checkAppUpdate(ctx context.Context) bool {
	requestedVersion := GetCurrentChannel()

	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return false
	}

	repo := "GhostWriters/DockSTARTer2"
	sourceReleases, err := source.ListReleases(ctx, selfupdate.ParseSlug(repo))
	if err != nil {
		return false
	}

	var bestVersion string
	for _, rel := range sourceReleases {
		tag := rel.GetTagName()
		if !matchesRequest(tag, requestedVersion) {
			continue
		}

		if bestVersion == "" || compareVersions(tag, bestVersion) > 0 {
			bestVersion = tag
		}
	}

	if bestVersion == "" {
		return false
	}

	return compareVersions(bestVersion, version.Version) > 0
}

func checkTmplUpdate(ctx context.Context) bool {
	templatesDir := paths.GetTemplatesDir()
	if _, err := os.Stat(filepath.Join(templatesDir, ".git")); os.IsNotExist(err) {
		return false
	}

	r, err := git.PlainOpen(templatesDir)
	if err != nil {
		return false
	}

	// Fetch latest
	err = r.FetchContext(ctx, &git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return false
	}

	// Determine Branch
	branch := ""
	head, err := r.Head()
	if err == nil && head.Name().IsBranch() {
		branch = head.Name().Short()
	} else {
		branch = "main"
	}

	// Get Local Hash
	localHash := head.Hash()

	// Get Remote Hash
	remoteRefName := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/origin/%s", branch))
	remoteRef, err := r.Reference(remoteRefName, true)
	if err != nil {
		return false
	}
	remoteHash := remoteRef.Hash()

	return localHash != remoteHash
}

// CheckCurrentStatus checks if the current version's channel still exists on GitHub.
// It is intended to be called at startup.
func CheckCurrentStatus(ctx context.Context) error {
	channel := GetCurrentChannel()
	if channel == "" || strings.EqualFold(channel, "stable") {
		return nil
	}

	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil // Don't block startup on network/config errors
	}

	repo := "GhostWriters/DockSTARTer2"
	sourceReleases, err := source.ListReleases(ctx, selfupdate.ParseSlug(repo))
	if err != nil {
		return nil // Don't block startup
	}

	found := false
	for _, rel := range sourceReleases {
		if matchesRequest(rel.GetTagName(), channel) {
			found = true
			break
		}
	}

	if !found {
		logger.Warn(ctx, "[_ApplicationName_]%s[-] channel '[_Branch_]%s[-]' appears to no longer exist.", version.ApplicationName, channel)
		logger.Warn(ctx, "Run '[_UserCommand_]%s -u main[-]' to update to the latest stable release.", version.CommandName)
	}

	return nil
}

// GetCurrentChannel extracts the channel suffix from the current version.
func GetCurrentChannel() string {
	if idx := strings.Index(version.Version, "-"); idx != -1 {
		return version.Version[idx+1:]
	}
	return "stable"
}

// UpdateTemplates handles updating the DockSTARTer-Templates repository using go-git.
func UpdateTemplates(ctx context.Context, force bool, yes bool, branch string) error {
	templatesDir := paths.GetTemplatesDir()

	// Printer wrapper for logger.Notice
	noticePrinter := func(ctx context.Context, msg string, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	// Determine progress writer based on log level
	var progress io.Writer
	if logger.LevelVar.Level() <= logger.LevelInfo {
		progress = os.Stdout
	}

	// 1. Clone if missing
	if _, err := os.Stat(filepath.Join(templatesDir, ".git")); os.IsNotExist(err) {
		question := fmt.Sprintf("Would you like to clone [_ApplicationName_]DockSTARTer-Templates[-] to '[_Folder_]%s[-]' location?", templatesDir)
		if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
			logger.Notice(ctx, "[_ApplicationName_]DockSTARTer-Templates[-] will not be cloned.")
			return nil
		}

		logger.Notice(ctx, "[_ApplicationName_]DockSTARTer-Templates[-] not found. Cloning...")

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(templatesDir), 0755); err != nil {
			return fmt.Errorf("failed to create templates parent directory: %w", err)
		}

		repoURL := "https://github.com/GhostWriters/DockSTARTer-Templates"
		_, err := git.PlainCloneContext(ctx, templatesDir, false, &git.CloneOptions{
			URL:      repoURL,
			Progress: progress,
		})
		if err != nil {
			return fmt.Errorf("failed to clone templates: %w", err)
		}
	}

	// Open repository
	r, err := git.PlainOpen(templatesDir)
	if err != nil {
		return fmt.Errorf("failed to open templates repo: %w", err)
	}

	// 2. Fetch latest
	logger.Info(ctx, "Fetching recent changes from git.")
	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		Tags:       git.AllTags,
		Progress:   progress,
	}
	err = r.FetchContext(ctx, fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch templates: %w", err)
	}

	// 3. Determine Branch
	if branch == "" {
		head, err := r.Head()
		if err == nil && head.Name().IsBranch() {
			branch = head.Name().Short()
		} else {
			branch = "main"
		}
	}

	// 4. Check for updates
	// Get Local Hash (HEAD)
	headRef, err := r.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	localHash := headRef.Hash()

	// Get Remote Hash (origin/branch)
	remoteRefName := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/origin/%s", branch))
	remoteRef, err := r.Reference(remoteRefName, true)
	if err != nil {
		return fmt.Errorf("failed to find remote branch 'origin/%s': %w", branch, err)
	}
	remoteHash := remoteRef.Hash()

	isUpToDate := localHash == remoteHash
	remoteShort := remoteHash.String()[:7]

	// Determine current version display string
	currentVerStr := paths.GetTemplatesVersion()

	if !force && isUpToDate {
		logger.Notice(ctx, "[_ApplicationName_]DockSTARTer-Templates[-] is already up to date on branch '[_Branch_]%s[-]'.", branch)
		logger.Notice(ctx, "Current version is '[_Version_]%s[-]'", currentVerStr)
		return nil
	}

	question := ""
	if force && isUpToDate {
		question = fmt.Sprintf("Would you like to forcefully re-apply [_ApplicationName_]DockSTARTer-Templates[-] update '[_Version_]%s[-]'?", currentVerStr)
	} else {
		question = fmt.Sprintf("Would you like to update [_ApplicationName_]DockSTARTer-Templates[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]' now?", currentVerStr, remoteShort)
	}

	if !console.QuestionPrompt(ctx, noticePrinter, question, "Y", yes) {
		logger.Notice(ctx, "[_ApplicationName_]DockSTARTer-Templates[-] will not be updated.")
		return nil
	}

	if force && isUpToDate {
		logger.Notice(ctx, "Forcefully re-applying [_ApplicationName_]DockSTARTer-Templates[-] update '[_Version_]%s[-]'", currentVerStr)
	} else {
		logger.Notice(ctx, "Updating [_ApplicationName_]DockSTARTer-Templates[-] from '[_Version_]%s[-]' to '[_Version_]%s[-]'", currentVerStr, remoteShort)
	}

	// 5. Checkout and Reset (Update)
	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Checkout branch (create if doesn't exist? No, we rely on origin)
	// Actually, just Reset --hard to origin/branch is usually enough if we are "updating".
	// But it's good practice to be on the branch.
	// Check if local branch exists.
	branchRefName := plumbing.NewBranchReferenceName(branch)
	err = w.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Force:  true, // Discard local changes
	})
	if err != nil {
		// If checkout failed (maybe branch doesn't exist locally yet?), create it starting from remote
		// But usually we just want to match remote.
		// Let's rely on Reset.
		// If we are DETACHED, checkout might be needed.
		// For simplicity/robustness: Just Reset --hard to the remote hash.
		// But paths.GetTemplatesVersion relies on HEAD sticking to a branch name to report "main commit ...".
		// If we are detached, it reports "HEAD commit ...".
		// So we SHOULD checkout the branch.

		// Create local branch tracking remote if not exists
		// go-git checkout can create.
		err = w.Checkout(&git.CheckoutOptions{
			Branch: branchRefName,
			Create: true,
			Force:  true,
			Keep:   false,
		})
		if err != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
		}
	}

	logger.Info(ctx, "Pulling recent changes from git.")
	// Reset --hard to remote hash
	err = w.Reset(&git.ResetOptions{
		Commit: remoteHash,
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("failed to reset to %s: %w", remoteShort, err)
	}

	// Mimic "git reset --hard" output in verbose mode
	if c, err := r.CommitObject(remoteHash); err == nil {
		// Get first line of commit message
		msg := strings.SplitN(c.Message, "\n", 2)[0]
		logger.Info(ctx, "HEAD is now at %s %s", remoteShort, msg)
	}

	// Calculate new version string
	newVerStr := paths.GetTemplatesVersion()

	logger.Notice(ctx, "Updated [_ApplicationName_]DockSTARTer-Templates[-] to '[_Version_]%s[-]'", newVerStr)
	return nil
}

// matchesRequest checks if a tag matches the requested version or channel suffix.
func matchesRequest(tag, request string) bool {
	if request == "" || strings.EqualFold(request, "stable") {
		// Only allow stable versions (no dash indicating pre-release/suffix)
		return !strings.Contains(tag, "-")
	}
	// Check if the tag ends with -suffix or contains -suffix.
	// We use strings.Contains to be flexible with .N build numbers after the suffix if any,
	// but the user requested suffix filtering specifically.
	return strings.Contains(strings.ToLower(tag), "-"+strings.ToLower(request))
}

// compareVersions handles numeric comparison for vYYYY.MM.DD.N format.
func compareVersions(v1, v2 string) int {
	s1 := strings.TrimPrefix(v1, "v")
	s2 := strings.TrimPrefix(v2, "v")

	// Split by . or - to handle both numeric parts and suffixes
	split := func(s string) ([]string, string) {
		suffix := ""
		if idx := strings.Index(s, "-"); idx != -1 {
			suffix = s[idx+1:]
			s = s[:idx]
		}
		return strings.Split(s, "."), suffix
	}

	parts1, suffix1 := split(s1)
	parts2, suffix2 := split(s2)

	// Compare numeric parts
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			p1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			p2, _ = strconv.Atoi(parts2[i])
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}

	// If numeric parts are equal, compare suffixes
	if suffix1 == "" && suffix2 != "" {
		return 1 // Stable is greater than pre-release
	}
	if suffix1 != "" && suffix2 == "" {
		return -1 // Pre-release is less than stable
	}
	return strings.Compare(suffix1, suffix2)
}

// selfUpdateLogger adapts the selfupdate.Logger interface to our internal logger.
type selfUpdateLogger struct{}

func (l *selfUpdateLogger) Print(v ...interface{}) {
	msg := fmt.Sprint(v...)
	if strings.Contains(msg, "Repository or release not found") {
		return
	}
	logger.Info(context.Background(), msg)
}

func (l *selfUpdateLogger) Printf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if strings.Contains(msg, "Repository or release not found") {
		return
	}
	logger.Info(context.Background(), msg) // msg is already formatted
}

func init() {
	// Enable logging for selfupdate library, piping to our Info level (verbose only)
	selfupdate.SetLogger(&selfUpdateLogger{})
}
