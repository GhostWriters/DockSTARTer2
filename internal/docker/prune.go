package docker

import (
	"context"
	"fmt"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/go-units"
)

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

	answer, err := console.QuestionPrompt(ctx, printer, "Docker Prune", question, "Y", assumeYes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, yesNotice)
	logger.Notice(ctx, "Running: {{|RunningCommand|}}docker system prune -af --volumes{{[-]}}")

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

	// 4. Images (--all = include non-dangling)
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
