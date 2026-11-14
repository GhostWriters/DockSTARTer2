package compose

import (
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

func TestDependsLinesGroupByCondition(t *testing.T) {
	p := newDependsTestProcessor(
		dep("cache", types.ServiceConditionStarted, true),
		dep("db", types.ServiceConditionHealthy, true),
		dep("search", types.ServiceConditionHealthy, false),
	)
	p.containerStarted["cache"] = true
	p.setHealth("db", "db", healthUnhealthy)
	p.setHealth("search", "search", healthUnhealthy)

	lines := p.dependsLines("web", 160)
	got := plainLines(lines)
	want := []string{
		"✓ Ready                   started: cache",
		"× Failed                  healthy: db (Unhealthy)",
		"✓ Ready                   healthy (optional): search (Unhealthy)",
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
		!strings.Contains(lines[1], "{{|DockerDependsFailed|}}db (Unhealthy)") ||
		!strings.Contains(lines[2], "{{|DockerDependsWaiting|}}search (Unhealthy)") {
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
	lines := plainLines(p.dependsLines("web", 70))
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the names wrapped", lines)
	}
	raw := p.dependsLines("web", 70)
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
	if lines := p.dependsLines("web", 160); lines != nil {
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
	p.setHealth("db", "db", healthStarting)
	p.finished = true
	if lines := p.dependsLines("web", 160); len(lines) != 1 || !strings.Contains(lines[0], "Waiting") {
		t.Errorf("lines = %q, want Waiting while the end-of-run wait runs", lines)
	}
	p.healthWaitDone = true
	lines := p.dependsLines("web", 160)
	if len(lines) != 1 || !strings.Contains(lines[0], "{{|DockerStatusWarn|}}No Result") ||
		!strings.Contains(lines[0], "{{|DockerDependsWaiting|}}db (No Result)") {
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
	lines := p.neededByLines("db", 160)
	if len(lines) != 1 || !strings.Contains(lines[0], "Waiting") ||
		!strings.Contains(semstyle.ToPlain(lines[0]), "needed by: cache (Still Running), web") {
		t.Fatalf("lines = %q, want web pending and cache, outside the run, still running", plainLines(lines))
	}

	p.On(containerEvent("web", api.StatusStopped, api.Done))
	p.preRun["cache"] = "stopped" // already down before the run
	lines = p.neededByLines("db", 160)
	if !strings.Contains(lines[0], detailFinalTag+"Ready") ||
		!strings.Contains(lines[0], "{{|DockerDependsMet|}}web") || !strings.Contains(lines[0], "{{|DockerDependsMet|}}cache") {
		t.Errorf("lines = %q, want Ready with both stopped", lines)
	}
	if lines := p.dependsLines("web", 160); lines != nil {
		t.Errorf("stop shows depends_on lines %q, want none", lines)
	}
}

func TestNeededByOnRestartOnlyRestartTrue(t *testing.T) {
	p := newNeededByTestProcessor("restart")
	got := p.neededBy("db")
	if len(got) != 1 || got[0].name != "web" {
		t.Fatalf("neededBy = %+v, want only web (restart: true)", got)
	}
	if lines := p.neededByLines("db", 160); !strings.Contains(lines[0], "Waiting") {
		t.Errorf("lines = %q, want Waiting until web restarts", plainLines(lines))
	}
	p.On(containerEvent("web", api.StatusRestarted, api.Done))
	if lines := p.neededByLines("db", 160); !strings.Contains(lines[0], detailFinalTag+"Ready") {
		t.Errorf("lines = %q, want Ready once web restarted", plainLines(lines))
	}
}

func TestNeededByNotOnUp(t *testing.T) {
	p := newNeededByTestProcessor("up")
	if lines := p.neededByLines("db", 160); lines != nil {
		t.Errorf("up shows %q, want no needed-by lines", lines)
	}
}
