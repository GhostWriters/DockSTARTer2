package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"DockSTARTer2/cmd"
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

func main() {
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
	ctx := context.Background()

	// Defer cleanup to ensure it runs even if we return early or panic
	defer cleanup(ctx)

	// Recover from logger.FatalError to ensure cleanup runs
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(logger.FatalError); ok {
				// This panic was intentional from logger.Fatal/FatalWithStack
				exitCode = 1
			} else {
				// Re-panic for other errors
				panic(r)
			}
		}
		if exitCode != 0 {
			fmt.Fprintln(os.Stderr, console.Parse(fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} did not finish running successfully.", version.ApplicationName)))
		}
	}()

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
