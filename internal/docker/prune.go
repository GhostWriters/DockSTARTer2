package docker

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
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

	// Pre-flight: capture container ID → service/name BEFORE pruning, so deleted
	// containers can be shown under their service. After pruning the containers are gone
	// and can no longer be inspected, so this must happen first.
	containerInfo := containerServiceMap(ctx, cli)

	stopSpinner := console.StartSpinner()

	report := PruneReport{AsciiMode: asciiMode, ContainerInfo: containerInfo}

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

// containerServiceMap returns a map of container ID (both full and 12-char short forms)
// to its compose service name and display name, for containers belonging to the current
// project. Used so prune can display deleted containers under their service.
func containerServiceMap(ctx context.Context, cli *client.Client) map[string]containerMeta {
	const projectLabel = "com.docker.compose.project"
	const serviceLabel = "com.docker.compose.service"

	projectName := compose.ProjectName()
	f := filters.NewArgs()
	if projectName != "" {
		f.Add("label", projectLabel+"="+projectName)
	}
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil
	}
	m := make(map[string]containerMeta, len(containers)*2)
	for _, c := range containers {
		svc := c.Labels[serviceLabel]
		if svc == "" {
			continue
		}
		name := svc
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		meta := containerMeta{service: svc, name: name}
		m[c.ID] = meta
		if len(c.ID) >= 12 {
			m[c.ID[:12]] = meta
		}
	}
	return m
}
