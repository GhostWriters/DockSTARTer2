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
	"strings"

	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"DockSTARTer2/internal/version"

	selfupdate "github.com/creativeprojects/go-selfupdate"
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

	// Quick check using git ls-remote to see if tags for this channel exist.
	// This avoids hitting the GitHub releases API unnecessarily.
	if !strings.HasPrefix(requestedVersion, "v") {
		tag, err := latestChannelTag(requestedVersion)
		if err != nil {
			logger.Debug(ctx, "Git tag check failed: %v (will fall back to API)", err)
		} else if tag == "" {
			// No tags found for this channel - show warning and return early
			msg := []string{
				fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} channel '{{|Branch|}}%s{{[-]}}' appears to no longer exist (no releases found).", version.ApplicationName, requestedVersion),
				fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} is currently on version '{{|Version|}}%s{{[-]}}'.", version.ApplicationName, version.Version),
				fmt.Sprintf("Run '{{|UserCommand|}}%s -u main{{[-]}}' to update to the latest stable release.", version.CommandName),
			}
			logger.Warn(ctx, msg)
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
		// Find the latest tag for this specific channel, then detect that version.
		// This avoids DetectLatest returning a release from a different channel.
		channelTag, tagErr := latestChannelTag(requestedVersion)
		if tagErr != nil {
			logger.Debug(ctx, "Channel tag lookup failed: %v (falling back to DetectLatest)", tagErr)
			latest, found, err = updater.DetectLatest(ctx, repo)
		} else if channelTag == "" {
			found = false
		} else {
			latest, found, err = updater.DetectVersion(ctx, repo, channelTag)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to detect latest version: %w", err)
	}
	if !found {
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
		logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} is already up to date on channel '{{|Branch|}}%s{{[-]}}'.", version.ApplicationName, requestedVersion)
		logger.Notice(ctx, "Current version is '{{|Version|}}%s{{[-]}}'.", currentVersion)

		if force {
			question = fmt.Sprintf("Would you like to forcefully re-apply {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'?", version.ApplicationName, currentVersion)
			initiationNotice = fmt.Sprintf("Forcefully re-applying {{|ApplicationName|}}%s{{[-]}} update '{{|Version|}}%s{{[-]}}'", version.ApplicationName, remoteVersion)
		} else {
			return nil
		}
	} else {
		question = fmt.Sprintf("Would you like to update {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}' now?", version.ApplicationName, currentVersion, remoteVersion)
		initiationNotice = fmt.Sprintf("Updating {{|ApplicationName|}}%s{{[-]}} from '{{|Version|}}%s{{[-]}}' to '{{|Version|}}%s{{[-]}}'", version.ApplicationName, currentVersion, remoteVersion)
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

	logger.Notice(ctx, "Updated {{|ApplicationName|}}%s{{[-]}} to '{{|Version|}}%s{{[-]}}'", version.ApplicationName, remoteVersion)

	if exePath != "unknown" {
		logger.Info(ctx, "Application location is '{{|File|}}%s{{[-]}}'.", exePath)
	}

	// Reset all needs markers
	system.SetPermissions(ctx, paths.GetTimestampsDir())
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

// installUpdate downloads and installs the binary from the given URL.
func installUpdate(ctx context.Context, assetURL string) error {
	// Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "ds2-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	logger.Info(ctx, "Downloading update from {{|URL|}}%s{{[-]}}", assetURL)
	resp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

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

	// Replace current executable
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

	mvCmd, err := dsexec.SudoCommand(ctx, "mv", tmpExe, exe)
	if err != nil {
		return fmt.Errorf("sudo update failed: %w", err)
	}
	if out, err := mvCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudo update failed: %s: %w", string(out), err)
	}

	// Restore ownership to root:root if sudo was used.
	// Usually /usr/local/bin is root owned.
	if chownCmd, err := dsexec.SudoCommand(ctx, "chown", "root:root", exe); err == nil {
		_ = chownCmd.Run()
	}

	return nil
}
