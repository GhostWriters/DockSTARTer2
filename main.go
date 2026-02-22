package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"DockSTARTer2/cmd"
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/update"
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
			logger.Debug(context.Background(), "syscall.Exec failed: %v. Attempting exec.Command...", err)

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
	slog.SetDefault(logger.NewLogger())

	// Create a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a sync.Once to ensure we only cancel and log once
	var exitOnce sync.Once

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		sig, ok := <-sigChan
		if ok && sig == os.Interrupt {
			exitOnce.Do(func() {
				logger.TUIMode = false
				logger.Error(ctx, "User aborted via CTRL-C")
				exitCode = 1
				cancel()
			})
		}
	}()

	// Defer cleanup to ensure it runs even if we return early or panic
	defer cleanup(ctx)

	// Ensure embedded assets are extracted
	if err := assets.EnsureAssets(ctx); err != nil {
		logger.Error(ctx, "Failed to ensure assets: %v", err)
		// We continue, as the app might still work with hardcoded defaults
	}

	// Ensure templates are cloned
	if err := update.EnsureTemplates(ctx); err != nil {
		// Only fatal if we are NOT running a status/help command that doesn't need templates
		// But practically, most commands need templates.
		logger.FatalWithStack(ctx, "Failed to clone {{|ApplicationName|}}DockSTARTer-Templates{{[-]}} repo.")
	}

	_ = update.CheckCurrentStatus(ctx)
	// Check for application and template updates
	update.CheckUpdates(ctx)
	// Parse command line arguments
	groups, err := cmd.Parse(os.Args[1:])
	if err != nil {
		logger.Error(ctx, err.Error())
		return 1
	}

	// Hand off execution to the cmd package
	exitCode = cmd.Execute(ctx, groups)

	return exitCode
}

func cleanup(ctx context.Context) {
	logger.Info(ctx, "Cleaning up...")
	logger.Cleanup()
}
