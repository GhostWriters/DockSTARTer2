package docker

import (
	"context"
	"fmt"

	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	dockerimage "github.com/docker/docker/api/types/image"
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

	stopSpinner := console.StartSpinner()

	// Pre-flight: list candidate images and their layers so we can detect failures.
	candidates, candidateErr := buildImageCandidates(ctx, cli)

	report := PruneReport{AsciiMode: asciiMode, Candidates: candidates}

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

	// 4. Images (--all = include non-dangling)
	iReport, err := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "false")))
	if err != nil {
		report.ImagesError = err
	}
	if iReport.ImagesDeleted != nil {
		report.ImagesDeleted = iReport.ImagesDeleted
		report.SpaceReclaimed += iReport.SpaceReclaimed
	}

	// Warn if candidate pre-flight failed — display will fall back to deleted-only view.
	if candidateErr != nil {
		logger.Warn(ctx, "Could not list images before pruning: %v", candidateErr)
	}

	stopSpinner()

	if report.SpaceReclaimed > 0 || len(report.ImagesDeleted) > 0 ||
		len(report.NetworksDeleted) > 0 || len(report.VolumesDeleted) > 0 ||
		len(report.ContainersDeleted) > 0 || report.hasErrors() {
		LogPruneReport(ctx, report, imageServices)
	}

	return nil
}

// ImageCandidate holds a pre-flight image entry — the ref and its expected layer IDs.
type ImageCandidate struct {
	Ref    string   // e.g. "ghcr.io/autobrr/autobrr:latest"
	Layers []string // sha256 layer IDs from ImageHistory
}

// buildImageCandidates lists all dangling=false images and fetches their layer history.
func buildImageCandidates(ctx context.Context, cli *client.Client) ([]ImageCandidate, error) {
	imgs, err := cli.ImageList(ctx, dockerimage.ListOptions{
		All:     false,
		Filters: filters.NewArgs(filters.Arg("dangling", "false")),
	})
	if err != nil {
		return nil, fmt.Errorf("image list: %w", err)
	}

	var candidates []ImageCandidate
	for _, img := range imgs {
		refs := img.RepoTags
		if len(refs) == 0 {
			refs = []string{""} // dangling — include with empty ref
		}
		// Fetch layer history for the image ID (same for all tags of the same image).
		history, err := cli.ImageHistory(ctx, img.ID)
		var layers []string
		if err == nil {
			for _, h := range history {
				if h.ID != "" && h.ID != "<missing>" {
					layers = append(layers, h.ID)
				}
			}
		}
		for _, ref := range refs {
			candidates = append(candidates, ImageCandidate{Ref: ref, Layers: layers})
		}
	}
	return candidates, nil
}
