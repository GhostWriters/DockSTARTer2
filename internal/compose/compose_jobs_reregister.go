package compose

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/version"
)

// jobChangedPattern matches compose's error for a job the engine already has with a
// different spec, which only removing the job resolves.
var jobChangedPattern = regexp.MustCompile(`job "([^"]+)" has changed: run ` + "`docker compose down`")

// changedJobName returns the job err says is registered with a different spec.
func changedJobName(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	m := jobChangedPattern.FindStringSubmatch(err.Error())
	if m == nil {
		return "", false
	}
	return m[1], true
}

// silentEvents discards compose's events, for the passes that remove changed jobs.
type silentEvents struct{}

func (silentEvents) Start(context.Context, string) {}
func (silentEvents) On(...api.Resource)            {}
func (silentEvents) Done(string, bool)             {}

// jobsReregisteredKey marks a run that follows removing changed jobs, so a job still
// refused fails it instead of starting another round.
type jobsReregisteredKey struct{}

// reregisterChangedJobs handles compose refusing job first, which the engine has with
// a different spec: it asks (yes answers for it), removes every job of the project the
// engine has so, and reports whether to run cmd again to register them. A run that
// already followed a removal doesn't try again.
func reregisterChangedJobs(ctx context.Context, yes bool, dockerCLI command.Cli, project *types.Project, first, cmd string) (bool, error) {
	if ctx.Value(jobsReregisteredKey{}) != nil {
		return false, nil
	}
	logger.Warn(ctx, "Job {{|App|}}%s{{[-]}} is registered with different settings, by an older %s or the docker compose CLI.", first, version.ApplicationName)
	answer, err := console.QuestionPrompt(ctx, logger.Notice, "Docker Compose",
		"Remove it, and any other job registered differently, and register them again? Their kept runs are removed.", "Y", yes)
	if err != nil || !answer {
		logger.Notice(ctx, "Not registering the job again. Run '{{|UserCommand|}}ds2 -c down{{[-]}}', then '{{|UserCommand|}}ds2 -c %s{{[-]}}'.", cmd)
		return false, err
	}
	removed, err := removeChangedJobs(ctx, dockerCLI, project, first)
	if err != nil {
		return false, err
	}
	logger.Notice(ctx, "Removed %s %s; running '%s' again to register %s.", plural(len(removed), "job", "jobs"),
		"{{|App|}}"+strings.Join(removed, "{{[-]}}, {{|App|}}")+"{{[-]}}", cmd, plural(len(removed), "it", "them"))
	return true, nil
}

// removeChangedJobs removes the project's jobs the engine has with a different spec,
// first among them: compose removes a job a create with --remove-orphans doesn't find
// in the project, once it has registered the project's other jobs. It names only the
// first job it can't register, so each pass leaves out the ones found so far, until a
// pass registers the rest and removes those. It returns the jobs removed.
func removeChangedJobs(ctx context.Context, dockerCLI command.Cli, project *types.Project, first string) ([]string, error) {
	srv, err := composev5.NewComposeService(dockerCLI, composev5.WithEventProcessor(silentEvents{}))
	if err != nil {
		return nil, err
	}
	changed := []string{first}
	for range len(project.Jobs) {
		without := *project
		without.Jobs = maps.Clone(project.Jobs)
		for _, name := range changed {
			delete(without.Jobs, name)
		}
		err := srv.Create(ctx, &without, api.CreateOptions{RemoveOrphans: true})
		name, ok := changedJobName(err)
		switch {
		case err == nil:
			slices.Sort(changed)
			return changed, nil
		case !ok || slices.Contains(changed, name):
			return nil, err
		}
		changed = append(changed, name)
	}
	return nil, fmt.Errorf("job %q is still registered with different settings", first)
}
