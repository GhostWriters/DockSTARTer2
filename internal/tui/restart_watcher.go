package tui

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"

	tea "charm.land/bubbletea/v2"
)

const restartWatchInterval = 3 * time.Second

// registeredExePath is the resolved executable path captured at TUI start,
// before any binary replacement. Used for version-file polling and re-exec.
var registeredExePath string

// PendingRestartVersion is the version that was installed, displayed in the
// header indicator and confirmation prompt.
var PendingRestartVersion string

// isRestartSafePage returns true when the current page is safe to re-exec on —
// i.e. no unsaved state or in-progress operation that would be interrupted.
func isRestartSafePage() bool {
	if CurrentPageName == "tabbed_vars" {
		return false
	}
	if sessionlocks.Sessions.HoldEditLockLocal() {
		return false
	}
	return true
}

// startRestartWatcher launches a background goroutine that polls the installed-
// version file for the current executable. When a newer version is recorded it
// sets update.RestartPending and triggers a header refresh. If the current page
// is already safe it fires the re-exec immediately; otherwise re-exec is
// deferred until the user navigates to a safe page (checkPendingRestart).
func startRestartWatcher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(restartWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !update.RestartPending {
					installed := sessionlocks.Sessions.ReadInstalledVersion(registeredExePath)
					if installed != "" && installed != version.Version {
						PendingRestartVersion = installed
						update.RestartPending = true
						Send(UpdateHeaderMsg{})
					}
				}
				if update.RestartPending && isRestartSafePage() {
					triggerPendingRestart(ctx)
					return
				}
			}
		}
	}()
}

// checkPendingRestart is called whenever the active screen changes. If a restart
// is pending and the new page is safe, it fires the re-exec immediately rather
// than waiting for the next poll cycle.
func checkPendingRestart(ctx context.Context) {
	if update.RestartPending && isRestartSafePage() {
		triggerPendingRestart(ctx)
	}
}

// binaryVersion runs registeredExePath --print-version and returns the trimmed
// output, or "" on failure.
func binaryVersion() string {
	var buf bytes.Buffer
	cmd := exec.Command(registeredExePath, "--print-version")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// triggerPendingRestart verifies the on-disk binary version before re-execing.
// If the binary reports the same version we are already running, the version
// file was stale — clear the pending flag and update the file instead of
// restarting.
func triggerPendingRestart(ctx context.Context) {
	update.RestartPending = false

	// Double-check what the binary on disk actually reports.
	onDiskVer := binaryVersion()
	if onDiskVer != "" {
		if onDiskVer == version.Version {
			// Binary is actually the same version — stale version file, no restart needed.
			_ = sessionlocks.Sessions.WriteInstalledVersion(registeredExePath, onDiskVer)
			Send(UpdateHeaderMsg{})
			return
		}
		// Update the file to what the binary actually reports in case it drifted.
		_ = sessionlocks.Sessions.WriteInstalledVersion(registeredExePath, onDiskVer)
		PendingRestartVersion = onDiskVer
	}

	var reExecArgs []string
	if console.IsDaemon {
		reExecArgs = append(reExecArgs, "--server-daemon")
	} else {
		reExecArgs = append(reExecArgs, console.CurrentFlags...)
		reExecArgs = append(reExecArgs, GetNavArgs()...)
		reExecArgs = append(reExecArgs, console.RestArgs...)
	}

	_ = update.ReExec(ctx, registeredExePath, reExecArgs)
}

// captureAndRegister captures the resolved exe path and registers the process.
// Called at TUI/daemon start before any binary replacement can occur.
// Pair with sessionlocks.Sessions.UnregisterProc() on exit.
func captureAndRegister(currentVersion string) {
	registeredExePath = sessionlocks.ResolvedExePath()
	sessionlocks.Sessions.RegisterProc(registeredExePath, currentVersion)
}

// showPendingRestartDialog returns a tea.Cmd that shows the appropriate dialog
// when the user clicks the app version block while a restart is pending.
// On safe pages: confirm restart with version info.
// On unsafe pages: warn about unsaved changes and confirm restart.
func showPendingRestartDialog(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ver := PendingRestartVersion
		verSuffix := ""
		if ver != "" {
			verSuffix = " to " + ver
		}

		var question string
		if isRestartSafePage() {
			question = "The application was updated" + verSuffix + ". Restart now to use the new version?"
		} else {
			question = "The application was updated" + verSuffix + ". You have unsaved changes — restart anyway and lose them?"
		}

		resultChan := make(chan bool, 1)
		go func() {
			confirmed := <-resultChan
			if confirmed {
				triggerPendingRestart(ctx)
			}
		}()

		return ShowConfirmDialogMsg{
			Title:      "Restart Pending",
			Question:   question,
			DefaultYes: isRestartSafePage(),
			ResultChan: resultChan,
		}
	}
}
