package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"DockSTARTer2/cmd"
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
)

func main() {
	os.Exit(run())
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
				// This panic was intentional from logger.Fatal/FatalNoTrace
				exitCode = 1
			} else {
				// Re-panic for other errors
				panic(r)
			}
		}
		if exitCode != 0 {
			fmt.Fprintln(os.Stderr, console.Parse(fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} did not finish running successfully.", version.ApplicationName)))
		}
	}()

	// Ensure embedded assets are extracted
	if err := assets.EnsureAssets(ctx); err != nil {
		logger.Error(ctx, "Failed to ensure assets: %v", err)
		// We continue, as the app might still work with hardcoded defaults
	}

	// Ensure templates are cloned
	if err := update.EnsureTemplates(ctx); err != nil {
		logger.Fatal(ctx, "Failed to clone {{_ApplicationName_}}DockSTARTer-Templates{{|-|}} repo.")
	}

	// Check if the current channel still exists on GitHub

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
