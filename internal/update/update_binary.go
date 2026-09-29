//go:build !windows

package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/system"
	"DockSTARTer2/internal/version"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// latestChannelTag returns the most recent tag for the given channel.
func latestChannelTag(channel, repoSlug string) (string, error) {
	tags, err := channelTagsDescending(channel, repoSlug)
	if err != nil || len(tags) == 0 {
		return "", err
	}
	return tags[0], nil
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

// SelfUpdate handles updating the application binary using GitHub Releases.
// requestedVersion is a version/tag/channel name, optionally prefixed with
// "<owner>[/<repo>]@" to install from a fork instead of the canonical
// DockSTARTer2 repo (see ParseRepoAndRef).
func SelfUpdate(ctx context.Context, force bool, yes bool, requestedVersion string, restArgs []string) error {
	// Get current executable path for logging later
	// We do this early because self-update acts on the running binary (renaming it),
	// so os.Executable() might return .ds2.old if called after the update.
	exePath, err := os.Executable()
	if err != nil {
		// Fallback if we can't get it, though unlikely
		exePath = "unknown"
	}

	requestedSpec := requestedVersion
	repoSlug, requestedVersion := ParseRepoAndRef(requestedSpec, appRepoName)

	// A fully bare call (nothing typed at all) means "check whatever repo
	// this binary actually came from" -- version.SourceRepo, baked in at
	// build time (see .goreleaser.yaml), is ground truth for that: unlike a
	// persisted "last explicitly requested repo" record, it can't go stale
	// from a binary swapped in by any means other than this function (a
	// manual download, a distro package, a local build), since it reflects
	// whatever binary is actually running right now. Any explicit ref (even
	// a bare version/channel with no "owner@" prefix) is a deliberate
	// instruction and always means the official repo.
	if requestedSpec == "" && version.SourceRepo != "" && version.SourceRepo != defaultAppRepo {
		repoSlug = version.SourceRepo
	}

	slug := defaultAppRepo
	if repoSlug != "" {
		slug = repoSlug
	}
	repo := selfupdate.ParseSlug(slug)

	currentChannel := GetCurrentChannel()
	switchingChannels := requestedVersion != "" && !strings.EqualFold(requestedVersion, currentChannel) && !strings.EqualFold(requestedVersion, "main")
	if requestedVersion == "" {
		requestedVersion = currentChannel
	}

	// Map "main" to "stable" channel
	if strings.EqualFold(requestedVersion, "main") {
		requestedVersion = "stable"
		switchingChannels = !strings.EqualFold(currentChannel, "stable")
	}

	// Quick check using git ls-remote to see if tags for this channel exist.
	// This avoids hitting the GitHub releases API unnecessarily.
	if !strings.HasPrefix(requestedVersion, "v") {
		tag, err := latestChannelTag(requestedVersion, slug)
		if err != nil {
			logger.Debug(ctx, "Git tag check failed: %v (will fall back to API)", err)
			tag = requestedVersion // treat as non-empty so we fall through to the API
		}
		if err == nil && tag == "" {
			if switchingChannels {
				logger.Error(ctx, "{{|ApplicationName|}}%s{{[-]}} channel '%s' does not exist.", version.ApplicationName, AppBranchLinkForRepo(requestedVersion, slug))
				return fmt.Errorf("channel '%s' does not exist", requestedVersion)
			}
			// No tags at all for this channel — it's genuinely gone.
			logger.Warn(ctx, []string{
				fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} channel '%s' appears to no longer exist.", version.ApplicationName, AppBranchLinkForRepo(requestedVersion, slug)),
				fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} is currently on version '%s'.", version.ApplicationName, AppVersionLink(version.Version)),
				fmt.Sprintf("Run '{{|UserCommand|}}%s -u main{{[-]}}' to update to the latest stable release.", version.CommandName),
			})
			return nil
		}
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
		// Find the latest tag for this specific channel, then detect that
		// version, trying older tags if the newest one has no published
		// release yet (the release workflow pushes the tag before
		// goreleaser finishes building, so there's a window where the
		// newest tag 404s) -- same fallback checkAppUpdate uses.
		tags, tagErr := channelTagsDescending(requestedVersion, slug)
		if tagErr != nil {
			logger.Debug(ctx, "Channel tag lookup failed: %v (falling back to DetectLatest)", tagErr)
			latest, found, err = updater.DetectLatest(ctx, repo)
		} else if len(tags) == 0 {
			found = false
		} else {
			attempts := len(tags)
			if attempts > maxChannelTagFallbacks {
				attempts = maxChannelTagFallbacks
			}
			for _, tag := range tags[:attempts] {
				// Skip the real API call (DetectVersion) entirely for tags
				// that don't even have this platform's asset published yet.
				if !assetExistsForTag(ctx, tag, slug) {
					continue
				}
				latest, found, err = updater.DetectVersion(ctx, repo, tag)
				if err == nil && found {
					break
				}
			}
		}
	}

	if err != nil {
		return fmt.Errorf("failed to detect latest version: %w", err)
	}
	if !found {
		if switchingChannels {
			logger.Error(ctx, "{{|ApplicationName|}}%s{{[-]}} channel '%s' does not exist.", version.ApplicationName, AppBranchLinkForRepo(requestedVersion, slug))
			return fmt.Errorf("channel '%s' does not exist", requestedVersion)
		}
		// None of the attempted tags had a published release.
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on channel '%s'.", version.ApplicationName, AppBranchLinkForRepo(requestedVersion, slug))
		if requestedVersion != version.Version {
			logger.Notice(ctx, "Current version is '%s'.", AppVersionLink(version.Version))
		}
		return nil
	}

	remoteVersion := latest.Version()
	currentVersion := version.Version

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
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on channel '%s'.", version.ApplicationName, AppBranchLinkForRepo(requestedVersion, slug))
		if requestedVersion != currentVersion {
			logger.Notice(ctx, "Current version is '%s'.", AppVersionLink(currentVersion))
		}

		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '%s'?", version.ApplicationName, AppVersionLink(currentVersion))
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{|ApplicationName|}}%s{{[-]}} update '%s'", version.ApplicationName, AppVersionLinkForRepo(remoteVersion, slug))
		} else {
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s' now?", version.ApplicationName, AppVersionLink(currentVersion), AppVersionLinkForRepo(remoteVersion, slug))
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '%s' to '%s'", version.ApplicationName, AppVersionLink(currentVersion), AppVersionLinkForRepo(remoteVersion, slug))
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

	err = installUpdate(ctx, latest.AssetURL)
	if err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}

	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '%s'", version.ApplicationName, AppVersionLinkForRepo(remoteVersion, slug))

	if exePath != "unknown" {
		logger.Info(ctx, "Application location is '"+console.FormatFilePath(exePath)+"'.")
	}

	// Reset all needs markers
	system.SetPermissions(ctx, paths.GetTimestampsDir())
	_ = paths.ResetNeeds()

	// Offer/re-apply the file-capability grant to the just-replaced binary
	// here rather than waiting for the re-exec'd process's own startup check
	// -- installUpdate strips the grant (attached to the binary file) when
	// auto_setcap is enabled, so doing it now means the re-exec below
	// already has it.
	//
	// Ask the one-time offer here too if unanswered: the user just answered
	// "update?" so this is known interactive, a better moment than a later
	// unrelated startup. Gated on being answerable (TUI-connected client or
	// real terminal) so an unattended run (cron, daemon restart) never asks
	// a question nobody can see and silently burns the offer on "no".
	if runtime.GOOS == "linux" {
		conf := config.LoadAppConfig()
		promptable := console.TUIConfirm != nil || (console.IsTTY() && console.IsStdoutTTY() && console.IsStdinTTY())
		switch {
		case !conf.System.SetcapAsked && promptable:
			asked, enabled, _, err := system.RunSetcapCommand(ctx)
			if err != nil {
				logger.Warn(ctx, "Failed to offer file capabilities: %v", err)
			}
			if asked != conf.System.SetcapAsked || enabled != conf.System.AutoSetcap {
				conf.System.SetcapAsked = asked
				conf.System.AutoSetcap = enabled
				if err := config.SaveAppConfig(conf); err != nil {
					logger.Warn(ctx, "Failed to save auto_setcap setting: %v", err)
				}
			}
		case conf.System.AutoSetcap:
			if _, err := system.EnableSetcap(ctx); err != nil {
				logger.Warn(ctx, "Failed to re-apply file capabilities after update: %v", err)
			}
		}
	}

	// Record the installed version so other running instances detect the
	// update -- deliberately after the setcap step above, not before: any
	// watcher (including a daemon-level one with no connected session)
	// treats this as the "safe to restart into" signal, so it must not fire
	// until the binary is actually fully ready.
	if exePath != "unknown" {
		if err := sessionlocks.Sessions.WriteInstalledVersion(exePath, remoteVersion); err != nil {
			logger.Warn(ctx, "Could not write installed version record: %v", err)
		}
	}

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

	logger.Notice(ctx, "Restarting {{|ApplicationName|}}%s{{[-]}}", version.ApplicationName)
	logger.Notice(ctx, "Running: {{|RunningCommand|}}exec %s{{[-]}}", fullCmd)

	// Store for main thread execution
	PendingReExec = append([]string{exePath}, args...)

	// Cleanly shut down TUI if active before re-execution
	if console.TUIShutdown != nil {
		console.TUIShutdown()
	}

	// If running inside a daemon, disconnect active sessions first so they don't
	// block server shutdown, then cancel the server context so StartSSHServer
	// returns and main() can pick up PendingReExec to exec the new binary.
	if console.ServerDisconnect != nil {
		console.ServerDisconnect()
	}
	if console.DaemonShutdown != nil {
		console.DaemonShutdown()
	}

	return nil
}

// updateSpaceMargin is room kept free beyond the new binary itself when
// checking for space to stage it.
const updateSpaceMargin = 16 << 20

// installUpdate downloads the binary from the given URL and replaces the
// running one with it. The new binary is written in full beside the old one,
// on the same filesystem, then renamed over it in one step, so an update
// that fails partway -- no space left, a dropped download -- leaves the
// working binary as it was.
func installUpdate(ctx context.Context, assetURL string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir, exeName := filepath.Dir(exe), filepath.Base(exe)

	if !strings.HasSuffix(assetURL, ".tar.gz") && !strings.HasSuffix(assetURL, ".tgz") {
		return fmt.Errorf("unsupported format: %s", assetURL)
	}
	logger.Info(ctx, "Downloading update from {{|URL|}}%s{{[-]}}", assetURL)
	resp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	gw, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gw.Close()
	tr := tar.NewReader(gw)
	var size int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("executable not found in archive")
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		if filepath.Base(header.Name) == exeName {
			size = header.Size
			break
		}
	}

	// Staged beside the binary when this user can write there; otherwise in
	// a temp folder, then copied beside it with sudo.
	staged, err := os.CreateTemp(exeDir, "."+exeName+".update-*")
	if err == nil {
		defer os.Remove(staged.Name())
		if err := checkSpace(exeDir, size); err != nil {
			staged.Close()
			return err
		}
		if err := writeStaged(staged, tr, size); err != nil {
			return err
		}
		if err := os.Rename(staged.Name(), exe); err != nil {
			return fmt.Errorf("failed to replace '%s': %w", exe, err)
		}
		return nil
	}
	if !os.IsPermission(err) {
		return fmt.Errorf("failed to stage update in '%s': %w", exeDir, err)
	}

	tmpDir, err := os.MkdirTemp("", "ds2-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := checkSpace(tmpDir, size); err != nil {
		return err
	}
	if err := checkSpace(exeDir, size); err != nil {
		return err
	}
	tmpFile, err := os.Create(filepath.Join(tmpDir, exeName))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	if err := writeStaged(tmpFile, tr, size); err != nil {
		return err
	}
	stagedPath := filepath.Join(exeDir, "."+exeName+".update")
	if err := sudoRun(ctx, "cp", tmpFile.Name(), stagedPath); err != nil {
		if rmCmd, rmErr := dsexec.SudoCommand(ctx, "rm", "-f", stagedPath); rmErr == nil {
			_ = rmCmd.Run()
		}
		return fmt.Errorf("sudo update failed staging '%s': %w", stagedPath, err)
	}
	// Same filesystem, so this replaces the binary in one step.
	if err := sudoRun(ctx, "mv", stagedPath, exe); err != nil {
		if rmCmd, rmErr := dsexec.SudoCommand(ctx, "rm", "-f", stagedPath); rmErr == nil {
			_ = rmCmd.Run()
		}
		return fmt.Errorf("sudo update failed: %w", err)
	}

	// Restore ownership (to match the parent directory owner) and mode
	// (0755, executable): sudo cp can leave either wrong depending on the
	// OS/umask. Native (via CAP_CHOWN/CAP_FOWNER, if this process already
	// holds them from an earlier auto_setcap grant) wherever possible,
	// sudo chown/chmod only for whichever piece isn't -- never assumed to
	// need sudo just because the copy itself did.
	if dirInfo, err := os.Stat(exeDir); err == nil {
		if dirStat, ok := dirInfo.Sys().(*syscall.Stat_t); ok {
			if err := system.FixOwnerMode(ctx, exe, int(dirStat.Uid), int(dirStat.Gid), 0755); err != nil {
				logger.Warn(ctx, "Failed to restore ownership/mode on '%s': %v", exe, err)
			}
		}
	}
	return nil
}

// writeStaged writes size bytes from r to f, makes it executable, and closes
// it, returning an error when the copy comes up short or any step fails, as
// when the disk fills.
func writeStaged(f *os.File, r io.Reader, size int64) error {
	n, err := io.Copy(f, r)
	if err == nil && n != size {
		err = fmt.Errorf("wrote %d of %d bytes", n, size)
	}
	if err == nil {
		err = f.Chmod(0755)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("failed to write update to '%s': %w", f.Name(), err)
	}
	return nil
}

// checkSpace returns an error when dir's filesystem hasn't room for size
// bytes plus updateSpaceMargin; nil when it can't tell.
func checkSpace(dir string, size int64) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return nil
	}
	free := uint64(st.Bavail) * uint64(st.Bsize)
	need := uint64(size) + updateSpaceMargin
	if free < need {
		return fmt.Errorf("not enough free space in '%s' to update: %s needed, %s free -- the current binary is unchanged", dir, formatBytes(need), formatBytes(free))
	}
	return nil
}

// formatBytes returns n in MiB, e.g. "42.0 MiB".
func formatBytes(n uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
}

// sudoRun runs name with args under sudo; a failure's error includes the
// command's output.
func sudoRun(ctx context.Context, name string, args ...string) error {
	cmd, err := dsexec.SudoCommand(ctx, name, args...)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
