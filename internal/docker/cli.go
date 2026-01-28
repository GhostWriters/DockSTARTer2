package docker

import (
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RunCommand executes a docker command with arguments.
// It logs the command before execution and any error output.
func RunCommand(ctx context.Context, args ...string) error {
	cmdText := fmt.Sprintf("docker %s", strings.Join(args, " "))
	logger.Info(ctx, "Running: {{_RunningCommand_}}%s{{|-|}}", cmdText)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Capture output for logging on error
	// Bash RunAndLog captures stderr/stdout and logs it on error.
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(ctx, "Command failed: %s", string(output))
		return fmt.Errorf("failed to run '%s': %w", cmdText, err)
	}

	// Maybe log output at debug level?
	if len(output) > 0 {
		logger.Debug(ctx, "%s", string(output))
	}

	return nil
}
