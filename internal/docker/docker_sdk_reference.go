//go:build ignore

package docker

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-units"
)

var (
	dockerClient *client.Client
	clientOnce   sync.Once
)

// GetClient returns a shared Docker client instance.
func GetClient() (*client.Client, error) {
	var err error
	clientOnce.Do(func() {
		dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	})
	return dockerClient, err
}

// StopContainer stops a container by ID.
func StopContainer(ctx context.Context, containerID string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}

	timeout := 10
	stopOptions := container.StopOptions{
		Timeout: &timeout,
	}

	return cli.ContainerStop(ctx, containerID, stopOptions)
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

// Prune removes unused docker resources.
// Mirrors docker_prune.sh from the original Bash implementation.
func Prune(ctx context.Context, assumeYes bool) error {
	question := "Would you like to remove all unused containers, networks, volumes, images and build cache?"
	yesNotice := "Removing unused docker resources."
	noNotice := "Nothing will be removed."

	// Notice printer adapter
	printer := func(ctx context.Context, msg any, args ...any) {
		logger.Notice(ctx, msg, args...)
	}

	// Ask for confirmation
	answer, err := console.QuestionPrompt(ctx, printer, question, "Y", assumeYes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, yesNotice)

	cli, err := GetClient()
	if err != nil {
		return fmt.Errorf("failed to get docker client: %w", err)
	}

	// Logging prefix parity with RunAndLog
	logPrune := func(msg string, args ...any) {
		line := fmt.Sprintf(msg, args...)
		if line != "" {
			logger.Notice(ctx, "{{|RunningCommand|}}docker:{{[-]}} %s", line)
		} else {
			logger.Notice(ctx, "{{|RunningCommand|}}docker:{{[-]}}")
		}
	}

	var totalSpace uint64

	// 1. Containers
	cReport, err := cli.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		logger.Error(ctx, "Failed to prune containers: %v", err)
	} else if len(cReport.ContainersDeleted) > 0 {
		logPrune("Deleted Containers:")
		for _, id := range cReport.ContainersDeleted {
			logPrune("%s", id)
		}
		totalSpace += cReport.SpaceReclaimed
		logPrune("")
	}

	// 2. Networks
	nReport, err := cli.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		logger.Error(ctx, "Failed to prune networks: %v", err)
	} else if len(nReport.NetworksDeleted) > 0 {
		logPrune("Deleted Networks:")
		for _, id := range nReport.NetworksDeleted {
			logPrune("%s", id)
		}
		logPrune("")
	}

	// 3. Volumes
	vReport, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		logger.Error(ctx, "Failed to prune volumes: %v", err)
	} else if len(vReport.VolumesDeleted) > 0 {
		logPrune("Deleted Volumes:")
		for _, id := range vReport.VolumesDeleted {
			logPrune("%s", id)
		}
		totalSpace += vReport.SpaceReclaimed
		logPrune("")
	}

	// 4. Images
	// Bash uses --all, which means dangling=false in SDK filters (to include all unused)
	iReport, err := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "false")))
	if err != nil {
		logger.Error(ctx, "Failed to prune images: %v", err)
	} else if len(iReport.ImagesDeleted) > 0 {
		logPrune("Deleted Images:")
		for _, img := range iReport.ImagesDeleted {
			if img.Untagged != "" {
				logPrune("untagged: %s", img.Untagged)
			}
			if img.Deleted != "" {
				logPrune("deleted: %s", img.Deleted)
			}
		}
		totalSpace += iReport.SpaceReclaimed
		logPrune("")
	}

	if totalSpace > 0 {
		logPrune("Total reclaimed space: %s", units.HumanSize(float64(totalSpace)))
	}

	return nil
}
