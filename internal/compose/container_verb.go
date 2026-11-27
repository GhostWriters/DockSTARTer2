package compose

import (
	"context"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/docker/compose/v5/pkg/api"
	mobyclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
)

// containerVerbWords are each container command's statuses: while its call runs, and
// once it returns.
var containerVerbWords = map[string][2]string{
	"start":   {api.StatusStarting, api.StatusStarted},
	"stop":    {api.StatusStopping, api.StatusStopped},
	"restart": {api.StatusRestarting, api.StatusRestarted},
}

// ContainerVerbDisplay is the live display of docker start, stop or restart acting on
// containers by name: the compose display in its container mode, its heading the
// docker command, each container a row under changed or unchanged.
type ContainerVerbDisplay struct {
	p    *consoleEventProcessor
	verb string
}

// NewContainerVerbDisplay starts the display of docker verb (start, stop or restart)
// on the containers names. It reads each container's state before the run, which
// decides whether the command changes it, and what runs it (its compose service and
// project), shown with -v.
func NewContainerVerbDisplay(ctx context.Context, verb string, names []string) *ContainerVerbDisplay {
	var out io.Writer = os.Stdout
	tuiWriter, hasTUIWriter := ctx.Value(console.TUIWriterKey).(io.Writer)
	staticOut := !hasTUIWriter && (!console.IsStdoutTTY() || !console.IsTTY())
	var updateFn func([]string)
	if hasTUIWriter {
		out = tuiWriter
		updateFn = console.ReplaceOutputFuncFromContext(ctx)
		if updateFn == nil {
			updateFn = console.ReplaceOutputLinesFn
		}
	}
	conf := config.LoadAppConfig()
	ascii := !conf.Appearance.ForConnType(console.ConnTypeFromContext(ctx)).LineCharacters
	project := ProjectName()
	p := NewConsoleEventProcessor(ctx, out, verb, map[string][]string{}, nil, map[string]string{}, project, ascii,
		console.GlobalVerbose, staticOut, updateFn, nil).(*consoleEventProcessor)
	p.table = true
	p.containerVerb = "docker " + verb
	p.containers = slices.Clone(names)
	slices.Sort(p.containers)
	p.containerPreRun = map[string]string{}
	p.containerDetail = map[string][2]string{}
	if cli, err := mobyclient.New(mobyclient.FromEnv); err == nil {
		defer cli.Close()
		if res, err := cli.ContainerList(ctx, mobyclient.ContainerListOptions{All: true}); err == nil {
			for _, ctr := range res.Items {
				for _, raw := range ctr.Names {
					name := strings.TrimPrefix(raw, "/")
					if !slices.Contains(names, name) {
						continue
					}
					p.containerPreRun[name] = preRunState(ctr)
					switch proj, svc := ctr.Labels[api.ProjectLabel], ctr.Labels[api.ServiceLabel]; {
					case proj == project && svc != "":
						p.containerDetail[name] = [2]string{"service", svc}
					case proj != "":
						p.containerDetail[name] = [2]string{"project", proj}
					}
				}
			}
		}
	}
	p.Start(ctx, verb)
	return &ContainerVerbDisplay{p: p, verb: verb}
}

// Starting reports that the command's call for container name has started.
func (d *ContainerVerbDisplay) Starting(name string) {
	d.p.On(api.Resource{ID: "Container " + name, Status: api.Working, Text: containerVerbWords[d.verb][0]})
}

// Finished reports that the command's call for container name returned, with err when
// it failed.
func (d *ContainerVerbDisplay) Finished(name string, err error) {
	if err != nil {
		d.p.On(api.Resource{ID: "Container " + name, Status: api.Error, Text: err.Error()})
		return
	}
	d.p.On(api.Resource{ID: "Container " + name, Status: api.Done, Text: containerVerbWords[d.verb][1]})
}

// Done ends the display, its final frame drawn; ok reports whether every call
// succeeded.
func (d *ContainerVerbDisplay) Done(ok bool) {
	d.p.Done(d.verb, ok)
}
