package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"DockSTARTer2/cmd"
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"charm.land/lipgloss/v2"
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

	// Apply spinner/line-char config early so spinner works during startup log messages.
	{
		earlyConf := config.LoadAppConfig()
		console.LineCharacters = earlyConf.UI.LineCharacters
		console.SpinnerEnabled = earlyConf.UI.Spinner
		console.SpinnerSpeed = earlyConf.UI.SpinnerSpeed
	}

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

	// Wire up embedded theme callbacks (breaks theme→assets→logger→theme cycle)
	theme.EmbeddedThemeLister = assets.ListThemes
	theme.EmbeddedThemeReader = assets.GetTheme

	// Ensure user themes directory exists
	themesDir := paths.GetThemesDir()
	if _, err := os.Stat(themesDir); os.IsNotExist(err) {
		logger.Info(ctx, "Creating folder '{{|Folder|}}%s{{[-]}}'.", themesDir)
		if err := os.MkdirAll(themesDir, 0755); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to create folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, themesDir)
		}
	}

	// Ensure lock subdirectories exist (created lazily but do it here too
	// so they are present from first startup regardless of code path).
	procsDir := filepath.Join(paths.GetLocksDir(), "procs")
	versionsDir := filepath.Join(paths.GetLocksDir(), "versions")
	sessionsDir := filepath.Join(paths.GetLocksDir(), "sessions")
	_ = os.MkdirAll(procsDir, 0755)
	_ = os.MkdirAll(versionsDir, 0755)
	_ = os.MkdirAll(sessionsDir, 0755)

	// Register this process so other instances can see it in startup warnings.
	exePath := sessionlocks.ResolvedExePath()
	sessionlocks.Sessions.RegisterProc(exePath, version.Version)
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

	// Parse command line arguments
	groups, err := cmd.Parse(os.Args[1:])
	if err != nil {
		logger.Error(ctx, err.Error())
		return 1
	}

	// Hand off execution to the cmd package
	exitCode = cmd.Execute(ctx, groups)

	if exitCode != 0 {
		logger.Display(ctx, "{{|ApplicationName|}}%s{{[-]}} did not finish running successfully.", version.ApplicationName)
		logger.Display(ctx, "Check logs in '{{|File|}}%s{{[-]}}'.", logger.GetLogFilePath())
	}

	return exitCode
}

func cleanup(_ context.Context) {
	console.RestoreCursor()
	logger.Cleanup()
}
