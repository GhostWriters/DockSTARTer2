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

// IsRestartSafeLocally reports whether this process is in a safe state to
// restart — i.e. not editing and not on the vars editor page. Exported for
// callers (e.g. Display Options screens) that need to restart the program in
// place to apply a setting requiring re-construction, such as refresh rate.
func IsRestartSafeLocally() bool {
	return isRestartSafeLocally()
}

// RestartForConfigChange re-execs the running process immediately, restoring
// the current page via GetNavArgs. Intended for settings (like refresh rate)
// that can only take effect at program construction time, not via the live
// displayengine.ConfigChangedMsg sync path.
func RestartForConfigChange(ctx context.Context) {
	var reExecArgs []string
	if console.IsDaemon {
		reExecArgs = daemonReExecArgs()
	} else {
		reExecArgs = append(reExecArgs, console.CurrentFlags...)
		reExecArgs = append(reExecArgs, GetNavArgs()...)
		reExecArgs = append(reExecArgs, console.RestArgs...)
	}
	_ = update.ReExec(ctx, registeredExePath, reExecArgs)
}

// daemonReExecArgs returns the args to re-exec a --server-daemon process
// with, preserving whatever it was actually launched with (notably a
// non-default port override, which only exists in these args, not the
// shared config file). Falls back to a bare --server-daemon if somehow
// unset, rather than failing to restart at all.
func daemonReExecArgs() []string {
	if len(console.DaemonArgs) > 0 {
		return console.DaemonArgs
	}
	return []string{"--server-daemon"}
}

// isRestartSafeLocally returns true when this process is in a safe state to
// restart — i.e. not editing and not on the vars editor page.
func isRestartSafeLocally() bool {
	if CurrentPageName == "tabbed_vars" {
		return false
	}
	// Page-scoped rather than a bare bool check so a screen that somehow
	// left without explicitly clearing this (e.g. Back/Exit mid-edit) can
	// never permanently wedge future restarts once the page changes away.
	if CurrentPageName == "app-select" && CurrentScreenHasUnsavedEdit {
		return false
	}
	if sessionlocks.Sessions.HoldEditLockLocal() {
		return false
	}
	return true
}

// updateRestartSafeMarker writes or clears the .restartunsafe marker file
// based on the current local state. Called on every page transition.
func updateRestartSafeMarker() {
	if isRestartSafeLocally() {
		sessionlocks.Sessions.MarkRestartSafe()
	} else {
		sessionlocks.Sessions.MarkRestartUnsafe()
	}
}

// isRestartSafe returns true when ALL registered processes (including this one)
// are in a safe state to restart.
func isRestartSafe() bool {
	if !isRestartSafeLocally() {
		return false
	}
	return !sessionlocks.Sessions.SelfRestartUnsafe()
}

// startRestartWatcher starts this session's restart-safety marker and, for a
// local (non-daemon) process, a background goroutine that polls the
// installed-version file, sets update.RestartPending when it finds a newer
// version, and fires the re-exec once safe (or defers to checkPendingRestart
// until the user navigates to a safe page).
//
// Under --server-daemon, StartDaemonRestartWatcher (started once for the
// whole daemon, not per session) is the sole detector/decision-maker: every
// connected session shares update.RestartPending/PendingRestartVersion as
// process-wide state, so a per-session poller here would just duplicate that
// work and race to be the one that re-execs. A session still needs its own
// marker kept current (updateRestartSafeMarker, also called on every page
// transition) since the daemon watcher's safety check aggregates across all
// connected sessions.
func startRestartWatcher(ctx context.Context) {
	updateRestartSafeMarker()
	if console.IsDaemon {
		return
	}
	go func() {
		defer sessionlocks.Sessions.MarkRestartSafe()
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
				if update.RestartPending && isRestartSafe() {
					triggerPendingRestart(ctx)
					return
				}
			}
		}
	}()
}

// StartDaemonRestartWatcher launches a background poller at the daemon's own
// top level, independent of any connected session, so an update is picked up
// even while nobody is connected -- and, per startRestartWatcher's doc
// comment, is the sole restart detector/decision-maker for every session
// connected to this daemon. Sets update.RestartPending/PendingRestartVersion
// as soon as a newer version is found (visible to every connected session's
// header on its next render), then defers the actual restart until no
// connected session is mid-edit (sessionlocks.Sessions.SelfRestartUnsafe(),
// which aggregates every session sharing this daemon process's PID -- it
// deliberately does not check other, unrelated processes, e.g. a different
// --server-daemon instance on another port, since restarting this one has no
// effect on them; there's no local page state to check here since this isn't
// a session). Stops when ctx is cancelled.
func StartDaemonRestartWatcher(ctx context.Context) {
	exePath := sessionlocks.ResolvedExePath()
	go func() {
		ticker := time.NewTicker(restartWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !update.RestartPending {
					installed := sessionlocks.Sessions.ReadInstalledVersion(exePath)
					if installed != "" && installed != version.Version {
						PendingRestartVersion = installed
						update.RestartPending = true
						Send(UpdateHeaderMsg{})
					}
				}
				if update.RestartPending && !sessionlocks.Sessions.SelfRestartUnsafe() {
					triggerDaemonRestart(ctx, exePath)
					return
				}
			}
		}
	}()
}

// triggerDaemonRestart is StartDaemonRestartWatcher's equivalent of
// triggerPendingRestart, using an explicit exePath since there's no
// per-session registeredExePath here.
func triggerDaemonRestart(ctx context.Context, exePath string) {
	update.RestartPending = false

	onDiskVer := binaryVersionAt(exePath)
	if onDiskVer != "" {
		if onDiskVer == version.Version {
			_ = sessionlocks.Sessions.WriteInstalledVersion(exePath, onDiskVer)
			Send(UpdateHeaderMsg{})
			return
		}
		_ = sessionlocks.Sessions.WriteInstalledVersion(exePath, onDiskVer)
	}

	_ = update.ReExec(ctx, exePath, daemonReExecArgs())
}

// checkPendingRestart is called whenever the active screen changes. Under
// --server-daemon, StartDaemonRestartWatcher is the sole restart trigger (see
// startRestartWatcher's doc comment), so this only matters for a local
// (non-daemon) process: if a restart is pending and the new page is safe, it
// fires the re-exec immediately rather than waiting for the next poll cycle.
//
// triggerPendingRestart is dispatched on its own goroutine, not called
// directly: this runs inside Bubble Tea's Update() call chain, but
// triggerPendingRestart ends in Shutdown(), which calls p.Quit() and blocks
// waiting for p.Run() to return -- which can't happen while still waiting on
// this very Update() call, so calling it inline would deadlock. A separate
// goroutine lets Update() return immediately so Run()'s loop can process
// the pending Quit().
func checkPendingRestart(ctx context.Context) {
	if console.IsDaemon {
		return
	}
	if update.RestartPending && isRestartSafe() {
		go triggerPendingRestart(ctx)
	}
}

// binaryVersion runs registeredExePath --print-version and returns the trimmed
// output, or "" on failure.
func binaryVersion() string {
	return binaryVersionAt(registeredExePath)
}

// binaryVersionAt runs exePath --print-version and returns the trimmed
// output, or "" on failure.
func binaryVersionAt(exePath string) string {
	var buf bytes.Buffer
	cmd := exec.Command(exePath, "--print-version")
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
		reExecArgs = daemonReExecArgs()
	} else {
		reExecArgs = append(reExecArgs, console.CurrentFlags...)
		reExecArgs = append(reExecArgs, GetNavArgs()...)
		reExecArgs = append(reExecArgs, console.RestArgs...)
	}

	_ = update.ReExec(ctx, registeredExePath, reExecArgs)
}

// captureExePath captures the resolved exe path at TUI start before any
// binary replacement can occur. Registration is handled by main().
func captureExePath() {
	registeredExePath = sessionlocks.ResolvedExePath()
}

// showPendingRestartDialog returns a tea.Cmd that shows the appropriate dialog
// when the user clicks the app version block while a restart is pending.
//   - This session unsafe (mid-edit locally): warn about unsaved changes.
//   - This session safe, but another session sharing this process (e.g.
//     another connection to the same --server-daemon) is unsafe: identify
//     that session (same detail used for edit-lock warnings elsewhere) and
//     offer to disconnect it before restarting.
//   - Otherwise: plain confirm.
func showPendingRestartDialog(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ver := PendingRestartVersion
		verSuffix := ""
		if ver != "" {
			verSuffix = " to " + ver
		}

		localUnsafe := !isRestartSafeLocally()
		otherUnsafe := !localUnsafe && sessionlocks.Sessions.SelfRestartUnsafe()

		var question string
		var onConfirm func()
		switch {
		case localUnsafe:
			question = "The application was updated" + verSuffix + ". You have unsaved changes — restart anyway and lose them?"
			onConfirm = func() { triggerPendingRestart(ctx) }
		case otherUnsafe:
			detail := "Another connection"
			if lines, _ := sessionlocks.EditLockDetail(sessionlocks.Sessions.ReadEditInfo()); len(lines) > 0 {
				detail = strings.Join(lines, "\n")
			}
			question = "The application was updated" + verSuffix + ". " + detail +
				"\n\nDisconnecting it will lose its unsaved changes. Disconnect and restart?"
			onConfirm = func() {
				_ = sessionlocks.Sessions.Disconnect(ctx, true)
				if !console.IsDaemon {
					// No separate daemon watcher to fall back to locally --
					// finish the restart ourselves.
					triggerPendingRestart(ctx)
				}
				// Under --server-daemon, StartDaemonRestartWatcher notices
				// the now-cleared unsafe state on its next poll and restarts
				// on its own (the sole restart trigger for daemon sessions).
			}
		default:
			question = "The application was updated" + verSuffix + ". Restart now to use the new version?"
			onConfirm = func() { triggerPendingRestart(ctx) }
		}

		resultChan := make(chan bool, 1)
		go func() {
			confirmed := <-resultChan
			if confirmed {
				onConfirm()
			}
		}()

		return ShowConfirmDialogMsg{
			Title:      "Restart Pending",
			Question:   question,
			DefaultYes: !localUnsafe && !otherUnsafe,
			ResultChan: resultChan,
		}
	}
}
