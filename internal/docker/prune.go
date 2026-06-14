package docker

import (
	"context"
	"fmt"

	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/api/types/filters"
)

// Prune removes unused docker resources.
// Mirrors docker_prune.sh from the original Bash implementation.
func Prune(ctx context.Context, assumeYes bool) error {
	question := "Would you like to remove all unused containers, networks, volumes, images and build cache?"
	yesNotice := "Removing unused docker resources."
	noNotice := "Nothing will be removed."

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
	logger.Notice(ctx, "Running: {{|RunningCommand|}}docker system prune --all --force --volumes{{[-]}}")

	cli, err := GetClient()
	if err != nil {
		return fmt.Errorf("failed to get docker client: %w", err)
	}

	asciiMode := !console.LineCharacters
	imageServices := compose.LoadImageServices(ctx)

	stopSpinner := console.StartSpinner()

	report := PruneReport{AsciiMode: asciiMode}

	// 1. Containers
	cReport, err := cli.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		report.ContainersError = err
	}
	if cReport.ContainersDeleted != nil {
		report.ContainersDeleted = cReport.ContainersDeleted
		report.SpaceReclaimed += cReport.SpaceReclaimed
	}

	// 2. Networks
	nReport, err := cli.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		report.NetworksError = err
	}
	if nReport.NetworksDeleted != nil {
		report.NetworksDeleted = nReport.NetworksDeleted
	}

	// 3. Volumes
	vReport, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		report.VolumesError = err
	}
	if vReport.VolumesDeleted != nil {
		report.VolumesDeleted = vReport.VolumesDeleted
		report.SpaceReclaimed += vReport.SpaceReclaimed
	}

	// 4. Images (--all = include non-dangling, equivalent to docker image prune --all)
	iReport, err := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "false")))
	if err != nil {
		report.ImagesError = err
	}
	if iReport.ImagesDeleted != nil {
		report.ImagesDeleted = iReport.ImagesDeleted
		report.SpaceReclaimed += iReport.SpaceReclaimed
	}

	stopSpinner()

	if report.SpaceReclaimed > 0 || len(report.ImagesDeleted) > 0 ||
		len(report.NetworksDeleted) > 0 || len(report.VolumesDeleted) > 0 ||
		len(report.ContainersDeleted) > 0 || report.hasErrors() {
		LogPruneReport(ctx, report, imageServices)
	}

	return nil
}
