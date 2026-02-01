package exec

import (
	"DockSTARTer2/internal/logger"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunAndLog mirrors the Bash RunAndLog function.
// It executes a command, captures output, prefixes each line, and logs appropriately.
//
// Parameters:
//   - ctx: Context for the command execution
//   - runningNoticeType: Notice type for logging the "Running: ..." message ("notice", "info", etc.). Empty string to skip.
//   - outputNoticeType: Notice type for logging output. Can include prefix like "git:info" or "docker:notice". Empty string to skip.
//   - errorNoticeType: Notice type for logging errors ("error", "warn", etc.). Empty string to skip.
//   - errorMessage: Message to log on error
//   - command: Command name (e.g., "docker", "git")
//   - args: Command arguments
//
// Returns error if command fails.
func RunAndLog(ctx context.Context, runningNoticeType, outputNoticeType, errorNoticeType, errorMessage, command string, args ...string) error {
	cmdText := command
	if len(args) > 0 {
		cmdText = fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	}

	// Log the running command if runningNoticeType is set
	if runningNoticeType != "" {
		logByType(ctx, runningNoticeType, "Running: {{_RunningCommand_}}%s{{|-|}}", cmdText)
	}

	// Execute the command
	cmd := exec.CommandContext(ctx, command, args...)
	var outputBuf bytes.Buffer

	// If outputNoticeType is set, capture output to process it.
	// Otherwise, stream directly to stdout/stderr.
	if outputNoticeType != "" {
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err := cmd.Run()

	// Process output if we have any and outputNoticeType is set
	if outputNoticeType != "" && outputBuf.Len() > 0 {
		// Parse prefix and notice type (e.g., "docker:notice" -> prefix="docker:", type="notice")
		prefix := ""
		noticeType := outputNoticeType
		if strings.Contains(outputNoticeType, ":") {
			parts := strings.SplitN(outputNoticeType, ":", 2)
			prefix = parts[0] + ":"
			noticeType = parts[1]
		}

		// Prefix each line and log
		scanner := bufio.NewScanner(&outputBuf)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" { // Skip empty lines
				if prefix != "" {
					prefixedLine := fmt.Sprintf("{{_RunningCommand_}}%s{{|-|}} %s", prefix, line)
					logByType(ctx, noticeType, prefixedLine)
				} else {
					logByType(ctx, noticeType, line)
				}
			}
		}
	}

	// Handle error
	if err != nil {
		if errorNoticeType != "" && errorMessage != "" {
			// Log error message and failing command
			logByType(ctx, errorNoticeType, errorMessage)
			logByType(ctx, errorNoticeType, "Failing command: {{_FailingCommand_}}%s{{|-|}}", cmdText)
		}
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// logByType logs a message with the appropriate logger function based on type
func logByType(ctx context.Context, noticeType string, format string, args ...any) {
	switch strings.ToLower(noticeType) {
	case "notice":
		logger.Notice(ctx, format, args...)
	case "info":
		logger.Info(ctx, format, args...)
	case "warn", "warning":
		logger.Warn(ctx, format, args...)
	case "error":
		logger.Error(ctx, format, args...)
	case "debug":
		logger.Debug(ctx, format, args...)
	default:
		logger.Notice(ctx, format, args...)
	}
}

// RunCommand executes a command without logging. Use this for simple command execution.
func RunCommand(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd.Run()
}

// RunCommandOutput executes a command and returns its output.
func RunCommandOutput(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
