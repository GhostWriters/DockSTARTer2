package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sync"
	"syscall"

	"DockSTARTer2/cmd"
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/boot"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockercheck"
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/system"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
)

func main() {
	// Create a background context for the recovery handler
	ctx := context.Background()
	defer logger.Recover(ctx)
	exitCode := run()
	if update.PendingReExec != nil {
		// Perform re-execution if triggered by the TUI update
		// This uses the simplest approach: the main thread executes the replacement
		// after the TUI has cleanly shut down and returned from run().
		exePath := update.PendingReExec[0]
		// Args for the new process (excluding the executable name for exec.Command)
		// update.PendingReExec contains [exePath, arg1, arg2...]
		var args []string
		if len(update.PendingReExec) > 1 {
			args = update.PendingReExec[1:]
		}

		logger.Debug(context.Background(), "Re-executing: %s %v", exePath, args)

		envv := os.Environ()

		// Try syscall.Exec first (non-Windows)
		err := syscall.Exec(exePath, update.PendingReExec, envv)
		if err != nil {
			// Fallback for Windows or other failures
			logger.Debug(context.Background(), "syscall.Exec failed: %v. Attempting exec.Command.", err)

			cmd := exec.Command(exePath, args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = envv

			if err := cmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to re-execute: %v\n", err)
			} else {
				// Wait for the child to correct exit code propagation
				if state, err := cmd.Process.Wait(); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to wait for re-execution: %v\n", err)
				} else {
					exitCode = state.ExitCode()
				}
			}
		}
	}
	os.Exit(exitCode)
}

func run() (exitCode int) {
	// NOTE: if DS2 was launched via "sudo ds2 ...", privileges were already
	// dropped back to the invoking user before main() even started --
	// internal/boot's init() (pulled in below every filesystem-touching
	// package via internal/paths) handles it, so not even package-level
	// initializers can compute paths from or write into root's home. True
	// root falls through untouched and is rejected by CheckNotRoot below.

	// Hidden elevated helper mode, invoked by DS2 on itself via sudo when it
	// lacks the privileges to fix file ownership/permissions natively (see
	// system.RunInternalFixPermissions). This child deliberately runs as
	// root: boot's demotion skips it by recognizing the same argument, and
	// it's dispatched here before CheckNotRoot, config loading, logging
	// setup, or the other-instances detection (deadlock risk if the parent
	// holds an edit lock).
	if len(os.Args) >= 2 && os.Args[1] == system.InternalFixPermissionsArg {
		return system.RunInternalFixPermissions(os.Args[2:])
	}

	// Hidden flag, must be given first: skip anything at startup that might
	// prompt for input (e.g. the one-time setcap offer, which can shell out
	// to sudo). Used by the systemd unit's ExecStart/ExecStop
	// (--server-daemon, --server stop) -- both run unattended with no
	// controlling terminal to answer a prompt. Deliberately left in
	// os.Args rather than stripped in place: anything downstream that
	// wants to know "how was this process actually launched" (e.g.
	// RegisterProc's "other instances running" listing, or a self-update
	// re-exec preserving the original daemon's args) should still see it.
	// Only cmd.Parse below needs it hidden, via its own local args slice.
	nonInteractive := len(os.Args) >= 2 && os.Args[1] == "--non-interactive"

	// Handle internal tool commands immediately before any startup work.
	// These are invoked by ds2 itself (e.g. restart watcher) and must be fast and silent.
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "--print-version":
			fmt.Println(version.Version)
			return 0
		case "--print-templates-version":
			fmt.Println(paths.GetTemplatesVersion())
			return 0
		}
	}

	// Initialize logger level styles to avoid import cycle (logger -> theme -> config -> logger)
	logger.LevelStyleFunc = func(tag string, label string) lipgloss.Style {
		s := theme.ConsoleSemanticStyle(tag)
		if label != "" {
			return s.SetString(label)
		}
		return s
	}

	slog.SetDefault(logger.NewLogger())

	// Must happen before any real work (including config loading, which can
	// create files) -- see CheckNotRoot's doc comment for why. Normal sudo
	// invocations never reach this as root anymore (demoted above); this
	// rejects true root sessions, which have no user to drop back to.
	system.CheckNotRoot(context.Background())

	// Always visible (not gated behind -v): silently continuing under a
	// different identity than the user typed would be surprising.
	if note := boot.DemotionNotice(); note != "" {
		logger.Notice(context.Background(), note)
	}

	// Wire up embedded theme callbacks (breaks theme→assets→logger→theme cycle)
	// before the first LoadAppConfig() call below -- a fresh-install/DS1
	// migration needs these to load the chosen theme's suggested defaults.
	theme.EmbeddedThemeLister = assets.ListThemes
	theme.EmbeddedThemeReader = assets.GetTheme

	// Fix ownership/permissions of the top-level XDG config dir itself
	// (non-recursive -- other apps' subfolders in here are none of DS2's
	// business) before the first LoadAppConfig() call below tries to create
	// its own subfolder in it. Otherwise a ~/.config owned by root (e.g.
	// left behind by an earlier root-run process) blocks that mkdir outright.
	system.TakeOwnership(context.Background(), xdg.ConfigHome)

	// Apply spinner/line-char config early so spinner works during startup log messages.
	var earlyConf config.AppConfig
	{
		earlyConf = config.LoadAppConfig()
		console.LineCharacters = earlyConf.UI.LineCharacters
		console.SpinnerEnabled = earlyConf.UI.Spinner
		console.SpinnerSpeed = earlyConf.UI.SpinnerSpeed
	}

	// Re-tighten permissions on DS2's own config/state/log files every
	// startup, not just at creation time, so they stay correct regardless
	// of what created or last touched them.
	hostKeyPath := earlyConf.Server.HostKey
	if hostKeyPath == "" {
		hostKeyPath = filepath.Join(paths.GetStateDir(), "server_host_key")
	}
	system.HardenOwnPath(context.Background(), paths.GetStateDir(), 0700)
	system.HardenOwnPath(context.Background(), paths.GetConfigDir(), 0700)
	system.HardenOwnPath(context.Background(), paths.GetConfigFilePath(), 0600)
	system.HardenOwnPath(context.Background(), logger.GetLogFilePath(), 0600)
	system.HardenOwnPath(context.Background(), logger.GetFatalLogFilePath(), 0600)
	system.HardenOwnPath(context.Background(), hostKeyPath, 0600)

	// Start the CLI viewport — a fixed-height scrolling region that all console
	// output flows through. Only active in TTY CLI mode (not TUI, not piped).
	stopViewport := console.StartViewport()
	defer stopViewport()
	if console.GlobalViewport != nil {
		logger.SetConsoleOutput(console.ViewportWriter())
	}

	// Create a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a sync.Once to ensure we only cancel and log once
	var exitOnce sync.Once

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		interruptCount := 0
		for {
			sig, ok := <-sigChan
			if !ok {
				return
			}
			if sig == os.Interrupt {
				interruptCount++
				if interruptCount > 1 {
					console.RestoreCursor()
					fmt.Fprintln(os.Stderr, "\nForced exit.")
					os.Exit(1)
				}
				exitOnce.Do(func() {
					// Stop viewport first so we exit the alternate screen before
					// any further output, then restore cursor.
					if console.GlobalViewport != nil {
						console.GlobalViewport.ForceStop()
					}
					console.RestoreCursor()
					logger.TUIMode = false
					logger.Error(ctx, "User aborted via CTRL-C")
					exitCode = 1
					cancel()
				})
			}
		}
	}()

	// Defer cleanup to ensure it runs even if we return early or panic
	defer cleanup(ctx)

	// Migrate the pre-user-folder themes location (.config/dockstarter2/themes)
	// to its new home under the user-content folder
	// (.config/dockstarter2/user/themes) -- only when the new location
	// doesn't exist yet, so this never overwrites themes a user has already
	// placed at the new path.
	if _, err := os.Stat(paths.GetThemesDir()); os.IsNotExist(err) {
		legacyThemesDir := paths.GetLegacyThemesDir()
		if _, err := os.Stat(legacyThemesDir); err == nil {
			userDir := filepath.Dir(paths.GetThemesDir())
			if err := os.MkdirAll(userDir, 0700); err != nil {
				logger.FatalWithStack(ctx, []string{
					"Failed to create folder.",
					"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
				}, userDir)
			}
			logger.Notice(ctx, "Moving user themes from '"+console.FormatFolderPath(legacyThemesDir)+"' to '"+console.FormatUserFolderPath(paths.GetThemesDir(), paths.GetThemesDir())+"'.")
			logger.Info(ctx, "Running: {{|RunningCommand|}}mv \"%s\" \"%s\"{{[-]}}", legacyThemesDir, paths.GetThemesDir())
			// Try a plain rename first; fall back to sudo mv on permission
			// failure, same pattern as installUpdate's binary replace.
			if err := os.Rename(legacyThemesDir, paths.GetThemesDir()); err != nil {
				mvCmd, sudoErr := dsexec.SudoCommand(ctx, "mv", legacyThemesDir, paths.GetThemesDir())
				if sudoErr != nil {
					logger.Warn(ctx, "Failed to move themes folder to its new location: %v", sudoErr)
				} else if out, runErr := mvCmd.CombinedOutput(); runErr != nil {
					logger.Warn(ctx, "Failed to move themes folder to its new location: %s: %v", string(out), runErr)
				}
			}
		}
	}

	// Ensure user themes directory exists
	themesDir := paths.GetThemesDir()
	if _, err := os.Stat(themesDir); os.IsNotExist(err) {
		logger.Info(ctx, "Creating folder '"+console.FormatUserFolderPath(paths.GetThemesDir(), themesDir)+"'.")
		if err := os.MkdirAll(themesDir, 0700); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to create folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, themesDir)
		}
	}
	// Fix ownership/permissions regardless of which path got us here (fresh
	// mkdir above, or the migration above it) -- a folder moved via the
	// sudo-mv fallback keeps its old ownership (mv doesn't chown), and
	// --theme-extract/--theme-extract-all later write into this folder
	// directly with no sudo fallback of their own, so it must already be
	// owned by the current user by the time they run.
	system.SetPermissions(ctx, themesDir)

	// Keep the user themes folder's .TEMPLATE.ds2theme reference copy in
	// sync with the embedded one -- write it if missing, or if it's stale
	// (e.g. after a DS2 update changed the embedded template). Dot-prefixed,
	// so theme.List skips it in --theme-list/the Appearance menu (see
	// internal/theme/list.go) unless it's somehow the active selection.
	if embeddedTemplate, err := assets.GetTheme(".TEMPLATE"); err == nil {
		templatePath := filepath.Join(themesDir, ".TEMPLATE.ds2theme")
		existing, readErr := os.ReadFile(templatePath)
		if readErr != nil || !bytes.Equal(existing, embeddedTemplate) {
			if err := os.WriteFile(templatePath, embeddedTemplate, 0600); err != nil {
				logger.Warn(ctx, "Failed to write theme template reference: %v", err)
			} else {
				logger.Notice(ctx, "Theme template reference updated: "+console.FormatUserFilePath(paths.GetThemesDir(), templatePath))
			}
		}
	}

	// Ensure user app templates directory exists, alongside the themes
	// folder -- no legacy location to migrate from, this is a new folder.
	userAppsDir := paths.GetUserAppsDir()
	if _, err := os.Stat(userAppsDir); os.IsNotExist(err) {
		logger.Info(ctx, "Creating folder '"+console.FormatUserFolderPath(paths.GetUserAppsDir(), userAppsDir)+"'.")
		if err := os.MkdirAll(userAppsDir, 0700); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to create folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, userAppsDir)
		}
	}
	system.SetPermissions(ctx, userAppsDir)
	appenv.SyncUserAppTemplateReference(ctx)

	// Ensure lock subdirectories exist (created lazily but do it here too
	// so they are present from first startup regardless of code path).
	procsDir := filepath.Join(paths.GetLocksDir(), "procs")
	versionsDir := filepath.Join(paths.GetLocksDir(), "versions")
	sessionsDir := filepath.Join(paths.GetLocksDir(), "sessions")
	_ = os.MkdirAll(procsDir, 0700)
	_ = os.MkdirAll(versionsDir, 0700)
	_ = os.MkdirAll(sessionsDir, 0700)

	// Register this process so other instances can see it in startup warnings.
	exePath := sessionlocks.ResolvedExePath()
	sessionlocks.Sessions.RegisterProc(exePath, version.Version, os.Args[1:])
	defer sessionlocks.Sessions.UnregisterProc()

	// Seed the installed-version file so the restart watcher always has a
	// baseline to compare against, even after a manual binary replacement.
	sessionlocks.Sessions.SeedInstalledVersion(exePath, version.Version)

	stopStartupSpinner := console.StartSpinner()

	// Ensure templates are cloned
	if err := update.EnsureTemplates(ctx); err != nil {
		// Only fatal if we are NOT running a status/help command that doesn't need templates
		// But practically, most commands need templates.
		stopStartupSpinner()
		logger.FatalWithStack(ctx, "Failed to clone {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} repo.")
	}

	_ = update.CheckCurrentStatus(ctx)
	// Check for application and template updates
	update.CheckUpdates(ctx)
	// Warn if an SSH server or active session is running
	serve.CheckStartupStatus(ctx)

	stopStartupSpinner()

	// A setcap command on the command line decides the capability setting
	// itself and may re-exec with the remaining arguments -- so both the
	// startup setcap offer AND the startup Docker probe are skipped for it
	// (the re-exec'd child repeats startup, and would duplicate any
	// warnings the parent already printed).
	setcapCmdPresent := slices.ContainsFunc(os.Args[1:], func(a string) bool {
		return a == "--setcap" || a == "--config-setcap" || a == "--config-no-setcap"
	})
	interactive := !nonInteractive && console.IsTTY() && console.IsStdoutTTY() && console.IsStdinTTY()

	// Offer/maintain the optional CAP_CHOWN/CAP_FOWNER grant on the binary
	// (lets permission fixes run without sudo; Linux + interactive startups
	// only). The user's answer is persisted so they're only ever asked once.
	if !setcapCmdPresent {
		conf := config.LoadAppConfig()
		asked, enabled, applied := system.AutoSetcapStartup(ctx, conf.System.SetcapAsked, conf.System.AutoSetcap, interactive)
		if asked != conf.System.SetcapAsked || enabled != conf.System.AutoSetcap {
			conf.System.SetcapAsked = asked
			conf.System.AutoSetcap = enabled
			if err := config.SaveAppConfig(conf); err != nil {
				logger.Warn(ctx, "Failed to save auto_setcap setting: %v", err)
			}
		}
		// File capabilities only bind at exec time -- re-exec with the
		// original command line (nothing has been consumed yet; cmd.Parse
		// hasn't run) so this invocation actually benefits from the grant
		// just applied, instead of finishing this run without it and
		// telling the user their command needs to be run again.
		if applied {
			exePath, err := os.Executable()
			if err != nil {
				logger.Warn(ctx, "Cannot re-exec to pick up the new capabilities: %v", err)
			} else if err := update.ReExec(ctx, exePath, os.Args[1:]); err == nil {
				return 0
			}
		}
	}

	// Probe the Docker daemon once at startup: warn -- never block -- if
	// it's missing, unreachable, or too old, since everything that doesn't
	// touch Docker (adding apps, editing env files, themes, ...) must keep
	// working regardless. Operations that DO need the daemon re-check and
	// hard-error at their own start (dockercheck.Require). When the
	// permission-denied cause is specifically a missing docker-group
	// membership, offer to fix it right here (interactive only).
	if !setcapCmdPresent {
		if st := dockercheck.StartupCheck(ctx); !st.OK() {
			handled := false
			if st.PermissionDenied && interactive {
				handled = system.OfferDockerGroupFix(ctx)
			}
			if !handled {
				dockercheck.LogProblem(ctx, st, false)
			}
		}
	}

	// Parse command line arguments. --non-interactive (if present) is
	// dropped only for this call -- os.Args itself keeps it, see the
	// nonInteractive comment above.
	parseArgs := os.Args[1:]
	if nonInteractive {
		parseArgs = parseArgs[1:]
	}
	groups, err := cmd.Parse(parseArgs)
	if err != nil {
		logger.Error(ctx, err.Error())
		exitCode = 1
	} else {
		// Hand off execution to the cmd package
		exitCode = cmd.Execute(ctx, groups)
	}

	if exitCode != 0 {
		logger.Display(ctx, "{{|ApplicationName|}}%s{{[-]}} did not finish running successfully.", version.ApplicationName)
		logger.Display(ctx, "Check logs in '"+console.FormatFilePath(logger.GetLogFilePath())+"'.")
	}

	return exitCode
}

func cleanup(_ context.Context) {
	console.RestoreCursor()
	logger.Cleanup()
}
