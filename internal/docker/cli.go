package docker

import (
	"context"
	"sync"

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

// GetContainerStatus returns the status of a container by ID.
func GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return "not found", nil
		}
		return "", err
	}

	return inspect.State.Status, nil
}
