package commands

import (
	"context"
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

	logger.Notice(ctx, "Restarting container: %s.", namesJoined)
	var firstErr error
	for _, name := range names {
		logger.Notice(ctx, "Running: {{|RunningCommand|}}docker restart %s{{[-]}}", name)
		if err := docker.RestartContainer(ctx, name); err != nil {
			logger.Error(ctx, "Failed to restart '{{|App|}}%s{{[-]}}': %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
		}
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

	var firstErr error
	for _, name := range names {
		followFlag := ""
		if state.Follow {
			followFlag = "-f "
		}
		logger.Notice(ctx, "Running: {{|RunningCommand|}}docker logs %s%s{{[-]}}", followFlag, name)

		rc, err := docker.ContainerLogs(ctx, name, state.Follow)
		if err != nil {
			logger.Error(ctx, "Failed to get logs for '{{|App|}}%s{{[-]}}': %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, copyErr := stdcopy.StdCopy(os.Stdout, os.Stderr, rc)
		_ = rc.Close()
		if copyErr != nil && firstErr == nil {
			firstErr = copyErr
		}
	}
	return firstErr
}
