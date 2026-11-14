package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/client"

	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockercheck"
	"DockSTARTer2/internal/logger"
)

// Prune removes unused docker resources.
// Mirrors docker_prune.sh from the original Bash implementation.
func Prune(ctx context.Context, assumeYes bool) error {
	// Fail clearly up front (before prompting) if the daemon is missing or
	// too old, instead of a low-level SDK error after the user confirms.
	if err := dockercheck.Require(ctx); err != nil {
		return err
	}

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

	asciiMode := !console.LineCharactersFor(console.ConnTypeFromContext(ctx))
	imageServices := compose.LoadImageServices(ctx)

	// Pre-flight: capture container ID → service/name BEFORE pruning, so deleted
	// containers can be shown under their service. After pruning the containers are gone
	// and can no longer be inspected, so this must happen first.
	containerInfo := containerServiceMap(ctx, cli)

	stopSpinner := console.StartSpinner()

	report := PruneReport{AsciiMode: asciiMode, ContainerInfo: containerInfo}

	// 1. Containers
	cRes, err := cli.ContainerPrune(ctx, client.ContainerPruneOptions{})
	if err != nil {
		report.ContainersError = err
	}
	if cRes.Report.ContainersDeleted != nil {
		report.ContainersDeleted = cRes.Report.ContainersDeleted
		report.SpaceReclaimed += cRes.Report.SpaceReclaimed
	}

	// 2. Networks
	nRes, err := cli.NetworkPrune(ctx, client.NetworkPruneOptions{})
	if err != nil {
		report.NetworksError = err
	}
	if nRes.Report.NetworksDeleted != nil {
		report.NetworksDeleted = nRes.Report.NetworksDeleted
	}

	// 3. Volumes
	vRes, err := cli.VolumePrune(ctx, client.VolumePruneOptions{})
	if err != nil {
		report.VolumesError = err
	}
	if vRes.Report.VolumesDeleted != nil {
		report.VolumesDeleted = vRes.Report.VolumesDeleted
		report.SpaceReclaimed += vRes.Report.SpaceReclaimed
	}

	// 4. Images (--all = include non-dangling, equivalent to docker image prune --all)
	iRes, err := cli.ImagePrune(ctx, client.ImagePruneOptions{Filters: make(client.Filters).Add("dangling", "false")})
	if err != nil {
		report.ImagesError = err
	}
	if iRes.Report.ImagesDeleted != nil {
		report.ImagesDeleted = iRes.Report.ImagesDeleted
		report.SpaceReclaimed += iRes.Report.SpaceReclaimed
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
	f := make(client.Filters)
	if projectName != "" {
		f.Add("label", projectLabel+"="+projectName)
	}
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: f})
	if err != nil {
		return nil
	}
	m := make(map[string]containerMeta, len(res.Items)*2)
	for _, c := range res.Items {
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
