package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/docker"
	"DockSTARTer2/internal/dockercheck"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/pkg/stdcopy"
)

// HandleContainerStart runs the standalone --start command.
func HandleContainerStart(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, nil, "start", "Start", "Starting", docker.StartContainer)
}

// HandleContainerStop runs the standalone --stop command.
func HandleContainerStop(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, nil, "stop", "Stop", "Stopping", docker.StopContainer)
}

// HandleContainerRestart runs the standalone --restart command.
func HandleContainerRestart(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, nil, "restart", "Restart", "Restarting", docker.RestartContainer)
}

// HandleContainerStartAll runs the standalone --start-all command.
func HandleContainerStartAll(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListAllContainerNames, "start", "Start", "Starting", docker.StartContainer)
}

// HandleContainerStopAll runs the standalone --stop-all command.
func HandleContainerStopAll(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListAllContainerNames, "stop", "Stop", "Stopping", docker.StopContainer)
}

// HandleContainerRestartAll runs the standalone --restart-all command.
func HandleContainerRestartAll(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListAllContainerNames, "restart", "Restart", "Restarting", docker.RestartContainer)
}

// HandleContainerStartStopped runs the standalone --start-stopped command.
func HandleContainerStartStopped(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListStoppedContainerNames, "start", "Start", "Starting", docker.StartContainer)
}

// HandleContainerStopStarted runs the standalone --stop-started command.
func HandleContainerStopStarted(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListRunningContainerNames, "stop", "Stop", "Stopping", docker.StopContainer)
}

// HandleContainerRestartStarted runs the standalone --restart-started command.
func HandleContainerRestartStarted(ctx context.Context, group *CommandGroup, state *CmdState) error {
	return handleContainerVerb(ctx, group, state, docker.ListRunningContainerNames, "restart", "Restart", "Restarting", docker.RestartContainer)
}

// handleContainerVerb runs `docker <verb> <container>` per named container,
// via the SDK directly (not docker compose) -- container names are used as
// typed, with no .env or compose-project lookup involved. Shared by
// --start/--stop/--restart and their -all/-started/-stopped variants, which
// differ only in the verb, prompt wording, underlying SDK call, and (when
// nameSource is set) where the container list comes from instead of
// group.Args. Runs all containers' SDK calls concurrently (one goroutine
// each) rather than one at a time -- the Engine API has no batch endpoint,
// so this mirrors how docker/cli itself fans out a multi-name `docker stop
// c1 c2 c3` invocation.
func handleContainerVerb(ctx context.Context, group *CommandGroup, state *CmdState, nameSource func(context.Context) ([]string, error), verb, imperative, presentParticiple string, action func(context.Context, string) error) error {
	if err := dockercheck.Require(ctx); err != nil {
		return err
	}

	names := group.Args
	if nameSource != nil {
		var err error
		names, err = nameSource(ctx)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			logger.Notice(ctx, "No containers found.")
			return nil
		}
	} else if len(names) == 0 {
		return fmt.Errorf("--%s requires at least one container name", verb)
	}

	namesJoined := strings.Join(names, ", ")
	question := fmt.Sprintf("%s container: {{|App|}}%s{{[-]}}?", imperative, namesJoined)
	answer, err := console.QuestionPrompt(ctx, logger.Notice, "Docker "+imperative, question, "Y", state.Yes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, "Not %s: {{|App|}}%s{{[-]}}.", strings.ToLower(presentParticiple), namesJoined)
		return nil
	}

	logger.Notice(ctx, "%s container: {{|App|}}%s{{[-]}}.", presentParticiple, namesJoined)
	// One combined "docker <verb> <names...>" line, matching how `docker
	// stop c1 c2 c3` reads as a single command on the real CLI -- even
	// though each container's SDK call below runs concurrently, not as one
	// request (the Engine API has no batch endpoint; docker/cli's own
	// multi-name support is the same per-container-goroutine shape).
	logger.Notice(ctx, "Running: {{|RunningCommand|}}docker %s %s{{[-]}}", verb, strings.Join(names, " "))

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := action(ctx, name); err != nil {
				logger.Error(ctx, "Failed to %s '{{|App|}}%s{{[-]}}': %v", verb, name, err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			// Same tab-indented, tool-prefixed Notice convention used for git
			// output in update_templates.go.
			logger.Notice(ctx, "\t{{|RunningCommand|}}docker:{{[-]}} %s", name)
		}(name)
	}
	wg.Wait()
	return firstErr
}

// HandleContainerLogs runs the standalone --logs command: `docker logs
// [-f] <container>` for one named container, via the SDK directly. Writes
// straight to stdout/stderr, bypassing DS2's own logger -- this is the
// target container's own log content, not DS2 diagnostic output, and with
// -F it can stream indefinitely.
func HandleContainerLogs(ctx context.Context, group *CommandGroup, state *CmdState) error {
	if len(group.Args) == 0 {
		return fmt.Errorf("--logs requires a container name")
	}
	name := group.Args[0]

	if err := dockercheck.Require(ctx); err != nil {
		return err
	}

	var stdout, stderr io.Writer
	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		// -g routes command output through the ProgramBox dialog, not the
		// real stdout/stderr -- same io.Writer for both streams, since the
		// dialog is one combined viewport, not separate stdout/stderr areas.
		stdout, stderr = w, w
	} else {
		stopSpinner := console.StartSpinner()
		defer stopSpinner()
		stdout = console.SpinnerSafeWriter(os.Stdout)
		stderr = console.SpinnerSafeWriter(os.Stderr)
	}

	followFlag := ""
	if state.Follow {
		followFlag = "-f "
	}
	logger.Notice(ctx, "Running: {{|RunningCommand|}}docker logs %s%s{{[-]}}", followFlag, name)

	logCtx := ctx
	if state.Follow {
		// Scope Ctrl+C to just this stream -- canceling logCtx breaks the
		// ContainerLogs read, and HandleScopedInterrupt (called from main's
		// SIGINT handler) intercepts the signal before it hits the default
		// "abort the whole process" behavior.
		var cancel context.CancelFunc
		logCtx, cancel = context.WithCancel(ctx)
		console.SetInterruptScope(cancel)
		defer console.SetInterruptScope(nil)
	}

	isTTY, err := docker.ContainerIsTTY(ctx, name)
	if err != nil {
		logger.Error(ctx, "Failed to inspect '{{|App|}}%s{{[-]}}': %v", name, err)
		return err
	}

	rc, err := docker.ContainerLogs(logCtx, name, state.Follow)
	if err != nil {
		logger.Error(ctx, "Failed to get logs for '{{|App|}}%s{{[-]}}': %v", name, err)
		return err
	}
	var copyErr error
	if isTTY {
		_, copyErr = io.Copy(stdout, rc)
	} else {
		_, copyErr = stdcopy.StdCopy(stdout, stderr, rc)
	}
	_ = rc.Close()
	// A canceled context (Ctrl+C stopping the follow) isn't a failure --
	// nothing the command was doing actually failed, it just stopped
	// watching.
	if copyErr != nil && !errors.Is(copyErr, context.Canceled) {
		return copyErr
	}
	return nil
}
