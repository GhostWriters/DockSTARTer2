package docker

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"context"
)

// Prune removes unused docker resources.
// Mirrors docker_prune.sh
func Prune(ctx context.Context, assumeYes bool) error {
	question := "Would you like to remove all unused containers, networks, volumes, images and build cache?"
	yesNotice := "Removing unused docker resources."
	noNotice := "Nothing will be removed."

	// Notice printer adapter
	printer := func(ctx context.Context, msg string, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	if !console.QuestionPrompt(ctx, printer, question, "Y", assumeYes) {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, yesNotice)

	// docker system prune --all --force --volumes
	args := []string{"system", "prune", "--all", "--force", "--volumes"}
	if err := RunCommand(ctx, args...); err != nil {
		logger.Error(ctx, "Failed to remove unused docker resources.")
		return err
	}

	return nil
}
