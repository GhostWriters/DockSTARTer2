package exec

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunAndLog mirrors the Bash RunAndLog function: executes a command,
// captures output, prefixes each line, and logs appropriately.
//
// runningNoticeType/outputNoticeType/errorNoticeType are notice types
// ("notice", "info", "error", "warn", etc.; outputNoticeType can include a
// prefix like "git:info") -- pass "" to skip logging that part.
func RunAndLog(ctx context.Context, runningNoticeType, outputNoticeType, errorNoticeType, errorMessage, command string, args ...string) error {
	cmdText := command
	if len(args) > 0 {
		cmdText = fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	}

	// Log the running command if runningNoticeType is set
	if runningNoticeType != "" {
		logByType(ctx, runningNoticeType, "Running: {{|RunningCommand|}}%s{{[-]}}", cmdText)
	}

	// Prepare the command (handling sudo password prompting if needed)
	cmd, err := prepareCommand(ctx, command, args)
	if err != nil {
		if errorNoticeType != "" {
			logByType(ctx, errorNoticeType, "Failed to prepare command: %v", err)
		}
		return err
	}
	var outputBuf bytes.Buffer

	// If outputNoticeType is set, capture output to process it.
	// Otherwise, stream directly to stdout/stderr.
	if outputNoticeType != "" {
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf
	} else {
		if w := console.GetTUIWriter(ctx); w != nil {
			cmd.Stdout = w
			cmd.Stderr = w
		} else {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
	}

	err = cmd.Run()

	// Process output if we have any and outputNoticeType is set
	if outputNoticeType != "" && outputBuf.Len() > 0 {
		// Parse prefix and notice type (e.g., "docker:notice" -> prefix="docker:", type="notice")
		prefix := ""
		parsedNoticeType := outputNoticeType
		if strings.Contains(outputNoticeType, ":") {
			parts := strings.SplitN(outputNoticeType, ":", 2)
			prefix = parts[0] + ":"
			parsedNoticeType = parts[1]
		}

		// Prefix each line and log
		scanner := bufio.NewScanner(&outputBuf)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" { // Skip empty lines
				if prefix != "" {
					prefixedLine := fmt.Sprintf("\t{{|RunningCommand|}}%s{{[-]}} %s", prefix, line)
					logByType(ctx, parsedNoticeType, prefixedLine)
				} else {
					logByType(ctx, parsedNoticeType, line)
				}
			}
		}
	}

	// Handle error
	if err != nil {
		if errorNoticeType != "" && errorMessage != "" {
			// Log error message and failing command
			logByType(ctx, errorNoticeType, errorMessage)
			logByType(ctx, errorNoticeType, "Failing command: {{|FailingCommand|}}%s{{[-]}}", cmdText)
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
	cmd, err := prepareCommand(ctx, command, args)
	if err != nil {
		return err
	}
	if w := console.GetTUIWriter(ctx); w != nil {
		cmd.Stdout = w
		cmd.Stderr = w
	}
	return cmd.Run()
}

// RunCommandOutput executes a command and returns its output.
func RunCommandOutput(ctx context.Context, command string, args ...string) (string, error) {
	cmd, err := prepareCommand(ctx, command, args)
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// prepareCommand handles command instantiation, intercepting sudo calls to use the helper.
func prepareCommand(ctx context.Context, command string, args []string) (*exec.Cmd, error) {
	if command != "sudo" {
		return exec.CommandContext(ctx, command, args...), nil
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("sudo called without arguments")
	}

	return SudoCommand(ctx, args[0], args[1:]...)
}

// SudoCommand prepares an exec.Cmd that runs the given command with elevated privileges using sudo.
// It checks if sudo requires a password, prompts the user via TUI/CLI if necessary,
// and securely passes the password to sudo via standard input.
func SudoCommand(ctx context.Context, command string, args ...string) (*exec.Cmd, error) {
	// Always pre-check with "sudo -n true" (non-interactive: succeeds only if
	// a cached credential covers this, never prompts) before ever considering
	// a real sudo invocation. If that fails, a password is genuinely needed:
	// prompt via TUI if one is available, let sudo prompt natively on a real
	// terminal, or fail fast rather than risk blocking on a password prompt
	// nobody can answer (e.g. an unattended --server-daemon run).
	checkCmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	if err := checkCmd.Run(); err != nil {
		hasTerminal := console.IsTTY() && console.IsStdoutTTY() && console.IsStdinTTY()
		if console.TUIPrompt == nil && !hasTerminal {
			return nil, fmt.Errorf("sudo requires a password and no prompt is available (unattended context)")
		}
		if console.TUIPrompt == nil {
			// Real terminal, no TUI dialog wiring -- let sudo prompt natively.
			sudoArgs := append([]string{command}, args...)
			return exec.CommandContext(ctx, "sudo", sudoArgs...), nil
		}

		// Password required — prompt via TUI dialog with retry on wrong password.
		fullCmd := command
		if len(args) > 0 {
			fullCmd += " " + strings.Join(args, " ")
		}
		const maxAttempts = 3
		for attempt := 0; attempt < maxAttempts; attempt++ {
			title := "{{|TitleQuestion|}}Sudo Password Required{{[-]}}"
			if attempt > 0 {
				title = "{{|TitleError|}}Incorrect Password — Try Again{{[-]}}"
			}
			password, err := console.TextPrompt(ctx, func(context.Context, any, ...any) {}, title, fullCmd, true)
			if err != nil {
				return nil, fmt.Errorf("sudo prompt cancelled: %w", err)
			}
			// Validate the password before building the real command.
			validateCmd := exec.CommandContext(ctx, "sudo", "-S", "-v")
			validateCmd.Stdin = strings.NewReader(password + "\n")
			var discard bytes.Buffer
			validateCmd.Stderr = &discard
			if validateCmd.Run() == nil {
				sudoArgs := append([]string{"-S", command}, args...)
				cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
				cmd.Stdin = strings.NewReader(password + "\n")
				return cmd, nil
			}
		}
		return nil, fmt.Errorf("sudo: authentication failed after %d attempts", maxAttempts)
	}

	// No password required (cached credential covers it) — safe to run natively.
	sudoArgs := append([]string{command}, args...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...), nil
}
