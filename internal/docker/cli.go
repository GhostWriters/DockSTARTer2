package docker

import (
	"context"
	"io"
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

// ContainerLogs returns the raw multiplexed log stream for a container by
// name or ID. Follow keeps the stream open for new log lines as they're
// written; the caller is responsible for closing the returned ReadCloser and
// demultiplexing it (see github.com/docker/docker/pkg/stdcopy.StdCopy).
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
