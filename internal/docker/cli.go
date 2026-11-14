package docker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

var (
	dockerClient *client.Client
	clientOnce   sync.Once
	clientErr    error
)

// GetClient returns a shared Docker SDK client instance.
func GetClient() (*client.Client, error) {
	clientOnce.Do(func() {
		dockerClient, clientErr = client.New(client.FromEnv)
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
	_, err = cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{
		Timeout: &timeout,
	})
	return err
}

// RemoveContainer removes a container by ID.
func RemoveContainer(ctx context.Context, containerID string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}

	_, err = cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	return err
}

// StartContainer starts a container by name or ID.
func StartContainer(ctx context.Context, containerName string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	_, err = cli.ContainerStart(ctx, containerName, client.ContainerStartOptions{})
	return err
}

// RestartContainer restarts a container by name or ID.
func RestartContainer(ctx context.Context, containerName string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	_, err = cli.ContainerRestart(ctx, containerName, client.ContainerRestartOptions{})
	return err
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
	res, err := cli.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err != nil {
		return false, err
	}
	return res.Container.Config != nil && res.Container.Config.Tty, nil
}

// ContainerLogs returns the raw log stream for a container by name or ID.
// Follow keeps the stream open for new log lines as they're written; the
// caller is responsible for closing the returned ReadCloser. Whether the
// stream needs demultiplexing (see github.com/moby/moby/api/pkg/stdcopy.StdCopy)
// depends on the container's TTY setting -- see ContainerIsTTY.
func ContainerLogs(ctx context.Context, containerName string, follow bool) (io.ReadCloser, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	return cli.ContainerLogs(ctx, containerName, client.ContainerLogsOptions{
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
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: all})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Items))
	for _, c := range res.Items {
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

	res, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "not found", nil
		}
		return "", err
	}
	if res.Container.State == nil {
		return "", nil
	}
	return string(res.Container.State.Status), nil
}
