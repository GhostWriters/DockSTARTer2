package docker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var (
	dockerClient *client.Client
	clientOnce   sync.Once
	clientErr    error
)

// GetClient returns a shared Docker SDK client instance.
func GetClient() (*client.Client, error) {
	clientOnce.Do(func() {
		dockerClient, clientErr = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	})
	return dockerClient, clientErr
}

// StopContainer stops a container by ID.
func StopContainer(ctx context.Context, containerID string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}

	timeout := 10
	return cli.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	})
}

// RemoveContainer removes a container by ID.
func RemoveContainer(ctx context.Context, containerID string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}

	return cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

// StartContainer starts a container by name or ID.
func StartContainer(ctx context.Context, containerName string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	return cli.ContainerStart(ctx, containerName, container.StartOptions{})
}

// RestartContainer restarts a container by name or ID.
func RestartContainer(ctx context.Context, containerName string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	return cli.ContainerRestart(ctx, containerName, container.StopOptions{})
}

// ContainerIsTTY reports whether a container was created with Config.Tty
// set. Docker's log stream format depends on this: a TTY container's stream
// is raw (no multiplexing headers), while a non-TTY container's stream
// interleaves stdout/stderr with 8-byte framing headers -- callers of
// ContainerLogs must inspect this before deciding whether to demultiplex
// with stdcopy.StdCopy (TTY streams must be copied directly instead).
func ContainerIsTTY(ctx context.Context, containerName string) (bool, error) {
	cli, err := GetClient()
	if err != nil {
		return false, err
	}
	inspect, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return false, err
	}
	return inspect.Config != nil && inspect.Config.Tty, nil
}

// ContainerLogs returns the raw log stream for a container by name or ID.
// Follow keeps the stream open for new log lines as they're written; the
// caller is responsible for closing the returned ReadCloser. Whether the
// stream needs demultiplexing (see github.com/docker/docker/pkg/stdcopy.StdCopy)
// depends on the container's TTY setting -- see ContainerIsTTY.
func ContainerLogs(ctx context.Context, containerName string, follow bool) (io.ReadCloser, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	return cli.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
	})
}

// ListAllContainerNames returns the names of every container Docker knows
// about, regardless of state or which project (if any) created it.
func ListAllContainerNames(ctx context.Context) ([]string, error) {
	return listContainerNames(ctx, true)
}

// ListRunningContainerNames returns the names of every currently-running
// ("started") container Docker knows about.
func ListRunningContainerNames(ctx context.Context) ([]string, error) {
	return listContainerNames(ctx, false)
}

// ListStoppedContainerNames returns the names of every container Docker
// knows about that isn't currently running.
func ListStoppedContainerNames(ctx context.Context) ([]string, error) {
	all, err := ListAllContainerNames(ctx)
	if err != nil {
		return nil, err
	}
	running, err := ListRunningContainerNames(ctx)
	if err != nil {
		return nil, err
	}
	runningSet := make(map[string]bool, len(running))
	for _, name := range running {
		runningSet[name] = true
	}
	stopped := make([]string, 0, len(all)-len(running))
	for _, name := range all {
		if !runningSet[name] {
			stopped = append(stopped, name)
		}
	}
	return stopped, nil
}

func listContainerNames(ctx context.Context, all bool) ([]string, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		if len(c.Names) > 0 {
			names = append(names, strings.TrimPrefix(c.Names[0], "/"))
		}
	}
	return names, nil
}

// GetContainerStatus returns the status of a container by ID.
func GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "not found", nil
		}
		return "", err
	}

	return inspect.State.Status, nil
}
