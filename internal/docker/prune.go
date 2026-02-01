package docker

import (
	"DockSTARTer2/internal/console"
	execpkg "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"context"
)

// TODO: Future enhancement - use Docker SDK (github.com/docker/docker/client) instead of CLI commands
// Benefits: Better cross-platform compatibility, no dependency on docker CLI being installed
// Note: Will need to simulate command output format differently (can't use RunAndLog pattern)

// Prune removes unused docker resources.
// Mirrors docker_prune.sh from the original Bash implementation.
func Prune(ctx context.Context, assumeYes bool) error {
	question := "Would you like to remove all unused containers, networks, volumes, images and build cache?"
	yesNotice := "Removing unused docker resources."
	noNotice := "Nothing will be removed."

	// Notice printer adapter
	printer := func(ctx context.Context, msg any, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	// Ask for confirmation
	if !console.QuestionPrompt(ctx, printer, question, "Y", assumeYes) {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, yesNotice)

	// Run docker system prune using RunAndLog pattern
	// RunAndLog notice "docker:notice" error "Failed to remove unused docker resources." "${Command[@]}"
	return execpkg.RunAndLog(ctx,
		"notice",        // runningNoticeType
		"docker:notice", // outputNoticeType
		"error",         // errorNoticeType
		"Failed to remove unused docker resources.", // errorMessage
		"docker",                                           // command
		"system", "prune", "--all", "--force", "--volumes", // args
	)
}
