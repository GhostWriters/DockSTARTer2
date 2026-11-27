package compose

import (
	"context"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
)

func dep(name, condition string, required bool) serviceDep {
	return serviceDep{name: name, condition: condition, required: required}
}

// newDependsTestProcessor returns an "up" processor where web depends on the given entries.
func newDependsTestProcessor(deps ...serviceDep) *consoleEventProcessor {
	p := newHealthTestProcessor()
	p.dependsOn = map[string][]serviceDep{"web": deps}
	p.preRun = map[string]string{}
	return p
}

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(semstyle.ToPlain(l))
	}
	return out
}

func TestProjectDependencies(t *testing.T) {
	project := &types.Project{Services: types.Services{
		"web": {Name: "web", DependsOn: types.DependsOnConfig{
			"db":    {Condition: types.ServiceConditionHealthy, Required: true},
			"cache": {Required: true},
		}},
		"db": {Name: "db"},
	}}
	got := projectDependencies(project)["web"]
	want := []serviceDep{dep("cache", types.ServiceConditionStarted, true), dep("db", types.ServiceConditionHealthy, true)}
	if !slices.Equal(got, want) {
		t.Errorf("deps = %+v, want %+v", got, want)
	}
}

func TestDepStatus(t *testing.T) {
	started := dep("db", types.ServiceConditionStarted, true)
	healthy := dep("db", types.ServiceConditionHealthy, true)
	completed := dep("db", types.ServiceConditionCompletedSuccessfully, true)

	for _, tc := range []struct {
		name        string
		d           serviceDep
		setup       func(p *consoleEventProcessor)
		wantState   depState
		wantProblem string
	}{
		{"started pending", started, func(p *consoleEventProcessor) {}, depPending, ""},
		{"started met", started, func(p *consoleEventProcessor) { p.On(containerEvent("db", api.StatusStarted, api.Done)) }, depMet, ""},
		{"started never", started, func(p *consoleEventProcessor) { p.finished = true }, depFailed, "Not Started"},
		{"started outside the run", started, func(p *consoleEventProcessor) { p.preRun["db"] = "running" }, depMet, ""},
		{"healthy met", healthy, func(p *consoleEventProcessor) { p.setHealth("db", "db", healthHealthy) }, depMet, ""},
		{"healthy unhealthy", healthy, func(p *consoleEventProcessor) { p.setHealth("db", "db", healthUnhealthy) }, depFailed, "Unhealthy"},
		{"healthy checking during the end-of-run wait", healthy, func(p *consoleEventProcessor) {
			p.setHealth("db", "db", healthStarting)
			p.finished = true
		}, depPending, ""},
		{"healthy no result", healthy, func(p *consoleEventProcessor) {
			p.setHealth("db", "db", healthStarting)
			p.finished = true
			p.healthWaitDone = true
		}, depWarning, "No Result"},
		{"healthy outside the run", healthy, func(p *consoleEventProcessor) { p.preRun["db"] = healthHealthy }, depMet, ""},
		{"completed met", completed, func(p *consoleEventProcessor) { p.On(containerEvent("db", api.StatusExited, api.Done)) }, depMet, ""},
		{"completed never", completed, func(p *consoleEventProcessor) { p.finished = true }, depFailed, "Not Completed"},
		{"failed row", started, func(p *consoleEventProcessor) {
			p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "port is already allocated"})
		}, depFailed, "Failed"},
		{"interrupted", started, func(p *consoleEventProcessor) {
			p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "interrupted by the failure of Container web"})
		}, depFailed, interruptedText},
		{"skipped", started, func(p *consoleEventProcessor) {
			p.On(api.Resource{ID: "Container db", Status: api.Warning, Text: "Skipped: optional dependency"})
		}, depFailed, skippedText},
	} {
		p := newDependsTestProcessor(tc.d)
		tc.setup(p)
		if state, problem := p.depStatus(tc.d); state != tc.wantState || problem != tc.wantProblem {
			t.Errorf("%s: %v %q, want %v %q", tc.name, state, problem, tc.wantState, tc.wantProblem)
		}
	}
}

func TestUnmetOnAnInterruptedDependencyIsAWarning(t *testing.T) {
	p := NewConsoleEventProcessor(context.Background(), nil, "up",
		map[string][]string{testImg: {"cache", "db", "queue", "web"}}, []string{testImg},
		map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	p.preRun = map[string]string{}
	p.dependsOn = map[string][]serviceDep{"web": {
		dep("cache", types.ServiceConditionStarted, true),
		dep("db", types.ServiceConditionStarted, true),
		dep("queue", types.ServiceConditionStarted, true),
	}}
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "port is already allocated"})
	p.On(api.Resource{ID: "Container cache", Status: api.Error, Text: "interrupted by the failure of Container db"})
	p.On(api.Resource{ID: "Container queue", Status: api.Warning, Text: "Skipped: optional dependency"})

	lines := p.dependsLines("web", 160, true)
	got := plainLines(lines)
	if len(got) != 2 {
		t.Fatalf("lines = %q, want the failed name and the knock-on names on lines of their own", got)
	}
	// A skipped dependency failed to start itself: a failure, as db is.
	if !strings.Contains(got[0], "db (Failed)") || !strings.Contains(got[0], "queue (Skipped)") ||
		!strings.Contains(lines[0], "DockerStatusFail") || !strings.Contains(lines[0], "DockerDependsFailed") {
		t.Errorf("failed line = %q, want db and queue Unmet as a failure", lines[0])
	}
	if !strings.Contains(got[1], "cache (Interrupted)") || !strings.Contains(got[1], "Unmet") ||
		!strings.Contains(lines[1], "DockerStatusWarn") || strings.Contains(lines[1], "DockerStatusFail") ||
		strings.Contains(lines[1], "DockerDependsFailed") {
		t.Errorf("knock-on line = %q, want cache Unmet as a warning", lines[1])
	}

	// An interrupted service's needed-by line is a warning too.
	p = newDependsTestProcessor()
	p.dependsOn["web"] = []serviceDep{dep("db", types.ServiceConditionHealthy, true)}
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "interrupted by the failure of Container proxy"})
	if lines := p.neededByLines("db", 160, true); len(lines) != 1 || !strings.Contains(lines[0], "DockerStatusWarn") || strings.Contains(lines[0], "DockerStatusFail") {
		t.Errorf("needed-by lines = %q, want one Unmet warning", lines)
	}
}

func TestDependsLinesGroupByCondition(t *testing.T) {
	p := newDependsTestProcessor(
		dep("cache", types.ServiceConditionStarted, true),
		dep("db", types.ServiceConditionHealthy, true),
		dep("search", types.ServiceConditionHealthy, false),
	)
	p.containerStarted["cache"] = true
	p.setHealth("db", "db", healthUnhealthy)
	p.setHealth("search", "search", healthUnhealthy)

	lines := p.dependsLines("web", 160, true)
	got := plainLines(lines)
	want := []string{
		"✓ Ready                    needs started: cache",
		"× Unmet                   needs healthy: db (Unhealthy)",
		"⚠ Unmet                   wants healthy: search (Unhealthy)", // a warning: optional
	}
	for i := range want {
		if i >= len(got) || strings.Join(strings.Fields(got[i]), " ") != strings.Join(strings.Fields(want[i]), " ") {
			t.Fatalf("lines = %q, want %q", got, want)
		}
	}
	if !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("Ready = %q, want the detail line's style", lines[0])
	}
	if !strings.Contains(lines[0], "{{|DockerDependsMet|}}cache") ||
		!strings.Contains(lines[1], "{{|DockerDependsFailed|}}db{{[-]}}{{|DockerDependsFailed|}} (Unhealthy)") ||
		!strings.Contains(lines[2], "{{|DockerDependsWaiting|}}search{{[-]}}{{|DockerDependsWaiting|}} (Unhealthy)") {
		t.Errorf("name styles wrong: %q", lines)
	}
}

func TestDependsLinesWrapUnderTheFirstName(t *testing.T) {
	p := newDependsTestProcessor(
		dep("nginx", types.ServiceConditionStarted, true),
		dep("radarr", types.ServiceConditionStarted, true),
		dep("radarr__4k", types.ServiceConditionStarted, true),
		dep("sonarr", types.ServiceConditionStarted, true),
		dep("sonarr__4k", types.ServiceConditionStarted, true),
	)
	lines := plainLines(p.dependsLines("web", 70, true))
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the names wrapped", lines)
	}
	raw := p.dependsLines("web", 70, true)
	first := semstyle.ToPlain(raw[0])
	second := semstyle.ToPlain(raw[1])
	col := utf8.RuneCountInString(first[:strings.Index(first, "nginx")])
	if strings.TrimLeft(second, " ") == "" || len(second)-len(strings.TrimLeft(second, " ")) != col {
		t.Errorf("continuation %q doesn't start under the first name (column %d)", second, col)
	}
	if !strings.HasSuffix(strings.TrimSpace(first), ",") {
		t.Errorf("wrapped line %q should end with a comma", first)
	}
}

func TestDependsLinesOnlyWhenStarting(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.command = "stop"
	if lines := p.dependsLines("web", 160, true); lines != nil {
		t.Errorf("stop shows %q, want no dependency lines", lines)
	}
}

func TestPreRunState(t *testing.T) {
	for _, tc := range []struct {
		c    container.Summary
		want string
	}{
		{container.Summary{State: container.StateExited}, "stopped"},
		{container.Summary{State: container.StateRunning, Status: "Up 2 hours"}, "running"},
		{container.Summary{State: container.StateRunning, Status: "Up 2 hours (healthy)"}, healthHealthy},
		{container.Summary{State: container.StateRunning, Status: "Up 5 seconds (health: starting)"}, healthStarting},
		{container.Summary{State: container.StateRunning, Health: &container.HealthSummary{Status: container.Unhealthy}}, healthUnhealthy},
	} {
		if got := preRunState(tc.c); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestDependsLineNoResultIsAWarning(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionHealthy, true))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.setHealth("db", "db", healthStarting)
	p.finished = true
	if lines := p.dependsLines("web", 160, true); len(lines) != 1 || !strings.Contains(lines[0], "{{|DockerStatusActive|}}Waiting") {
		t.Errorf("lines = %q, want Waiting while the end-of-run wait runs", lines)
	}
	p.healthWaitDone = true
	lines := p.dependsLines("web", 160, true)
	if len(lines) != 1 || !strings.Contains(lines[0], "{{|DockerStatusWarn|}}No Result") ||
		!strings.Contains(lines[0], "{{|DockerDependsWaiting|}}db{{[-]}}{{|DockerDependsWaiting|}} (No Result)") {
		t.Errorf("lines = %q, want a No Result warning with db in the waiting style", lines)
	}
}

// newNeededByTestProcessor returns a processor running command, where web depends on
// db (restart: true) and cache depends on db without it.
func newNeededByTestProcessor(command string) *consoleEventProcessor {
	p := newHealthTestProcessor()
	p.command = command
	p.preRun = map[string]string{"web": "running", "cache": "running"}
	p.dependsOn = map[string][]serviceDep{
		"web":   {{name: "db", condition: types.ServiceConditionStarted, required: true, restart: true}},
		"cache": {{name: "db", condition: types.ServiceConditionStarted, required: true}},
	}
	return p
}

func TestNeededByOnStop(t *testing.T) {
	p := newNeededByTestProcessor("stop")
	p.On(containerEvent("web", api.StatusStopping, api.Working))
	lines := p.neededByLines("db", 160, true)
	if len(lines) != 2 || !strings.Contains(semstyle.ToPlain(lines[0]), "needed by: cache (Still Running)") ||
		!strings.Contains(lines[1], "{{|DockerStatusActive|}}Waiting") || !strings.Contains(semstyle.ToPlain(lines[1]), "needed by: web") {
		t.Fatalf("lines = %q, want a line for cache, outside the run and still running, and another for web, pending", plainLines(lines))
	}

	p.On(containerEvent("web", api.StatusStopped, api.Done))
	p.preRun["cache"] = "stopped" // already down before the run
	lines = p.neededByLines("db", 160, true)
	if !strings.Contains(lines[0], detailFinalTag+"Ready") ||
		!strings.Contains(lines[0], "{{|DockerDependsMet|}}web") || !strings.Contains(lines[0], "{{|DockerDependsMet|}}cache") {
		t.Errorf("lines = %q, want Ready with both stopped", lines)
	}
	if lines := p.dependsLines("web", 160, true); lines != nil {
		t.Errorf("stop shows depends_on lines %q, want none", lines)
	}
}

func TestNeededByOnRestartOnlyRestartTrue(t *testing.T) {
	p := newNeededByTestProcessor("restart")
	got := p.neededBy("db")
	if len(got) != 1 || got[0].name != "web" {
		t.Fatalf("neededBy = %+v, want only web (restart: true)", got)
	}
	if lines := p.neededByLines("db", 160, true); !strings.Contains(lines[0], "{{|DockerStatusActive|}}Waiting") {
		t.Errorf("lines = %q, want Waiting until web restarts", plainLines(lines))
	}
	p.On(containerEvent("web", api.StatusRestarted, api.Done))
	if lines := p.neededByLines("db", 160, true); !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("lines = %q, want Ready once web restarted", plainLines(lines))
	}
}

func TestNeededByOnUpFollowsTheDependency(t *testing.T) {
	p := newNeededByTestProcessor("up")
	if lines := plainLines(p.neededByLines("db", 160, true)); len(lines) != 1 || !strings.Contains(lines[0], "Waiting") || !strings.Contains(lines[0], "needs started by: cache, web") {
		t.Fatalf("lines = %q, want cache and web waiting on db", lines)
	}

	p.On(containerEvent("db", api.StatusStarted, api.Done))
	if lines := p.neededByLines("db", 160, false); lines != nil {
		t.Errorf("a met dependency shows %q without -v, want none", plainLines(lines))
	}
	if lines := p.neededByLines("db", 160, true); len(lines) != 1 || !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("lines = %q, want Ready with -v", plainLines(lines))
	}

	p.finished = true
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting"}} // it started, then went down
	lines := plainLines(p.neededByLines("db", 160, false))
	if len(lines) != 1 || !strings.Contains(lines[0], "Crashed") || !strings.Contains(lines[0], "needs started by: cache, web") {
		t.Errorf("lines = %q, want the needed-by line crashed, shown without -v", lines)
	}
}

func TestNeededByNotOnCommandsThatLeaveNothingRunning(t *testing.T) {
	for _, command := range []string{"pull", "create", "rm", "kill"} {
		p := newNeededByTestProcessor(command)
		if lines := p.neededByLines("db", 160, true); lines != nil {
			t.Errorf("%s shows %q, want no needed-by lines", command, plainLines(lines))
		}
	}
}

func TestDependsLinePendingUntilContainers(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))

	// web hasn't reached its containers: compose isn't waiting on db yet.
	lines := p.dependsLines("web", 160, true)
	if len(lines) != 1 || !strings.Contains(lines[0], "{{|DockerStatusPending|}}Pending") {
		t.Fatalf("lines = %q, want Pending before web's containers", plainLines(lines))
	}
	if !strings.Contains(lines[0], "{{|DockerDependsWaiting|}}db") {
		t.Errorf("lines = %q, want db still in the waiting style", lines)
	}

	p.On(containerEvent("web", api.StatusCreating, api.Working))
	if lines := p.dependsLines("web", 160, true); !strings.Contains(lines[0], "{{|DockerStatusActive|}}Waiting") {
		t.Errorf("lines = %q, want Waiting once web is creating", plainLines(lines))
	}

	p.On(containerEvent("db", api.StatusStarted, api.Done))
	if lines := p.dependsLines("web", 160, true); !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("lines = %q, want Ready once db started", plainLines(lines))
	}
}

func TestNeededByPendingUntilReached(t *testing.T) {
	// A full down: web and db are both in the run, and compose hasn't reached web yet.
	p := newNeededByTestProcessor("down")
	p.dependsOn = map[string][]serviceDep{"web": {{name: "db", condition: types.ServiceConditionStarted, required: true}}}
	lines := p.neededByLines("db", 160, true)
	if len(lines) != 1 || !strings.Contains(lines[0], "{{|DockerStatusActive|}}Waiting") || strings.Contains(lines[0], "Still Running") {
		t.Fatalf("lines = %q, want web pending, not still running", plainLines(lines))
	}

	p.On(containerEvent("web", api.StatusRemoved, api.Done))
	if lines := p.neededByLines("db", 160, true); !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("lines = %q, want Ready once web is removed", plainLines(lines))
	}
}

func TestDetailLinesWithoutVerboseOnlyWhilePendingOrFailed(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	if lines := p.dependsLines("web", 160, false); len(lines) != 1 {
		t.Fatalf("lines = %q, want the Pending line shown without -v", plainLines(lines))
	}
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	if lines := p.dependsLines("web", 160, false); lines != nil {
		t.Errorf("lines = %q, want a Ready line hidden without -v", plainLines(lines))
	}
	if lines := p.dependsLines("web", 160, true); len(lines) != 1 {
		t.Errorf("lines = %q, want the Ready line shown with -v", plainLines(lines))
	}
}

func TestOptionalProblemShowsWithoutVerbose(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, false))
	p.finished = true // db never started: a warning, since it's optional, with the problem
	if lines := p.dependsLines("web", 160, false); len(lines) != 1 || !strings.Contains(semstyle.ToPlain(lines[0]), "Not Started") {
		t.Errorf("lines = %q, want the optional dependency's problem shown without -v", plainLines(lines))
	}
}

func TestNeededByOnUpHasALineForEachCondition(t *testing.T) {
	p := newNeededByTestProcessor("up")
	p.dependsOn["web"] = []serviceDep{{name: "db", condition: types.ServiceConditionHealthy, required: true}}
	p.dependsOn["cache"] = []serviceDep{{name: "db", condition: types.ServiceConditionStarted, required: false}}

	lines := plainLines(p.neededByLines("db", 160, true))

	if len(lines) != 2 || !strings.Contains(lines[0], "needs healthy by: web") || !strings.Contains(lines[1], "wants started by: cache") {
		t.Errorf("lines = %q, want one line for each condition, the optional one saying wants", lines)
	}
}

func TestNeedsAndWantsLinesAlignTheirNames(t *testing.T) {
	p := newDependsTestProcessor(
		dep("db", types.ServiceConditionStarted, true),
		dep("cache", types.ServiceConditionStarted, false),
	)
	p.finished = true

	lines := p.dependsLines("web", 160, true)

	if len(lines) != 2 {
		t.Fatalf("lines = %q, want a needs line and a wants line", plainLines(lines))
	}
	if needs, wants := columnOf(lines[0], "db"), columnOf(lines[1], "cache"); needs != wants {
		t.Errorf("names start at columns %d and %d, want them lined up: %q", needs, wants, plainLines(lines))
	}
}
