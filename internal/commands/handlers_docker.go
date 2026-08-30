package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/docker"
	"DockSTARTer2/internal/dockercheck"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/pkg/stdcopy"
)

// HandleContainerRestart runs the standalone --restart command: `docker
// restart <container>` per named container, via the SDK directly (not
// docker compose) -- container names are used as typed, with no .env or
// compose-project lookup involved.
func HandleContainerRestart(ctx context.Context, group *CommandGroup, state *CmdState) error {
	names := group.Args
	if len(names) == 0 {
		return fmt.Errorf("--restart requires at least one container name")
	}

	if err := dockercheck.Require(ctx); err != nil {
		return err
	}

	namesJoined := strings.Join(names, ", ")
	question := fmt.Sprintf("Restart container: {{|App|}}%s{{[-]}}?", namesJoined)
	answer, err := console.QuestionPrompt(ctx, logger.Notice, "Docker Restart", question, "Y", state.Yes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, "Not restarting: {{|App|}}%s{{[-]}}.", namesJoined)
		return nil
	}

	logger.Notice(ctx, "Restarting container: {{|App|}}%s{{[-]}}.", namesJoined)
	var firstErr error
	for _, name := range names {
		logger.Notice(ctx, "Running: {{|RunningCommand|}}docker restart %s{{[-]}}", name)
		if err := docker.RestartContainer(ctx, name); err != nil {
			logger.Error(ctx, "Failed to restart '{{|App|}}%s{{[-]}}': %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Same tab-indented, tool-prefixed Notice convention used for git
		// output in update_templates.go.
		logger.Notice(ctx, "\t{{|RunningCommand|}}docker:{{[-]}} %s", name)
	}
	return firstErr
}

// HandleContainerLogs runs the standalone --logs command: `docker logs
// [-f] <container>` per named container, via the SDK directly. Writes
// straight to stdout/stderr, bypassing DS2's own logger -- this is the
// target container's own log content, not DS2 diagnostic output, and with
// -F it can stream indefinitely.
func HandleContainerLogs(ctx context.Context, group *CommandGroup, state *CmdState) error {
	names := group.Args
	if len(names) == 0 {
		return fmt.Errorf("--logs requires at least one container name")
	}

	if err := dockercheck.Require(ctx); err != nil {
		return err
	}

	stopSpinner := console.StartSpinner()
	defer stopSpinner()
	stdout := console.SpinnerSafeWriter(os.Stdout)
	stderr := console.SpinnerSafeWriter(os.Stderr)

	var firstErr error
	for _, name := range names {
		followFlag := ""
		if state.Follow {
			followFlag = "-f "
		}
		logger.Notice(ctx, "Running: {{|RunningCommand|}}docker logs %s%s{{[-]}}", followFlag, name)

		logCtx := ctx
		if state.Follow {
			// Scope Ctrl+C to just this stream -- canceling logCtx breaks
			// the ContainerLogs read, and HandleScopedInterrupt (called from
			// main's SIGINT handler) intercepts the signal before it hits
			// the default "abort the whole process" behavior.
			var cancel context.CancelFunc
			logCtx, cancel = context.WithCancel(ctx)
			console.SetInterruptScope(cancel)
		}

		rc, err := docker.ContainerLogs(logCtx, name, state.Follow)
		if err != nil {
			if state.Follow {
				console.SetInterruptScope(nil)
			}
			logger.Error(ctx, "Failed to get logs for '{{|App|}}%s{{[-]}}': %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, copyErr := stdcopy.StdCopy(stdout, stderr, rc)
		_ = rc.Close()
		if state.Follow {
			console.SetInterruptScope(nil)
		}
		// A canceled context (Ctrl+C stopping the follow) isn't a failure --
		// nothing the command was doing actually failed, it just stopped
		// watching.
		if copyErr != nil && !errors.Is(copyErr, context.Canceled) && firstErr == nil {
			firstErr = copyErr
		}
	}
	return firstErr
}
