package compose

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
)

// newHealthTestProcessor returns an "up" processor for services web and db on one image.
func newHealthTestProcessor() *consoleEventProcessor {
	p := NewConsoleEventProcessor(context.Background(), nil, "up",
		map[string][]string{testImg: {"db", "web"}}, []string{testImg},
		map[string]string{}, "proj", false, false, true, nil, nil).(*consoleEventProcessor)
	return p
}

func containerEvent(svc, text string, status api.EventStatus) api.Resource {
	return api.Resource{ID: "Container " + svc, Text: text, Status: status}
}

func TestComposeHealthEventsKeepTheAction(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("db", api.StatusWaiting, api.Working))
	p.On(containerEvent("db", api.StatusHealthy, api.Done))
	if got := p.tasks["db"].text; got != "Recreated" {
		t.Errorf("db = %q, want Recreated kept", got)
	}

	// A container compose didn't touch has no action, so it shows as Running.
	p.On(containerEvent("web", api.StatusHealthy, api.Done))
	if got := p.tasks["web"].text; got != api.StatusRunning {
		t.Errorf("web = %q, want Running", got)
	}
}

func TestServiceStatusShowsHealth(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	db := p.tasks["db"]

	for _, tc := range []struct{ health, want string }{
		{"", "Recreated"},
		{healthStarting, "Recreated (Checking)"},
		{healthHealthy, "Recreated (Healthy)"},
		{healthUnhealthy, "Recreated (Unhealthy)"},
	} {
		p.health["db"] = tc.health
		text, tag := p.serviceStatus(db)
		if text != tc.want || semstyle.ToPlain(tag) != tc.want {
			t.Errorf("health %q: text %q, tag %q, want %q", tc.health, text, semstyle.ToPlain(tag), tc.want)
		}
	}

	// Still on its first check when the end-of-run wait runs out.
	p.health["db"] = healthStarting
	p.healthWaitDone = true
	text, tag := p.serviceStatus(db)
	if text != "Recreated (No Result)" || !strings.Contains(tag, "{{|DockerHealthNoResult|}}(No Result)") {
		t.Errorf("text %q, tag %q, want Recreated (No Result) in the warning style", text, tag)
	}
	p.updateStatusW()
	if got, want := p.statusW(), len("Recreated")+healthSuffixW+sectionStatusGutterW; got != want {
		t.Errorf("statusW = %d, want %d", got, want)
	}
}

func TestHealthCheckKeepsRowAndSectionWorking(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	db := p.tasks["db"]
	ids := []string{"db", "web"}
	imageFor := func(string) string { return testImg }

	if got := p.rollupState(ids, imageFor); got != rollupComplete {
		t.Fatalf("rollup = %v with no health, want complete", got)
	}

	p.health["db"] = healthStarting
	if got, want := p.serviceIcon(db, api.Done), p.activeSpinnerTag(p.icons().spinner); got != want {
		t.Errorf("icon = %q while checking, want the spinner %q", got, want)
	}
	if got := p.rollupState(ids, imageFor); got != rollupProcessing {
		t.Errorf("rollup = %v while checking, want processing", got)
	}

	p.healthWaitDone = true // No Result
	if got := p.rollupState(ids, imageFor); got != rollupComplete {
		t.Errorf("rollup = %v after the wait ran out, want complete", got)
	}

	p.health["db"] = healthUnhealthy
	if got := p.rollupState(ids, imageFor); got != rollupWarning {
		t.Errorf("rollup = %v with an unhealthy container, want warning", got)
	}
}

func TestWaitForHealthReturnsOnceChecked(t *testing.T) {
	p := newHealthTestProcessor()
	p.health["db"] = healthHealthy
	start := time.Now()
	p.waitForHealth()
	if time.Since(start) > healthWaitPoll || p.healthWaitDone {
		t.Errorf("waited with no container on its first check")
	}
}

func TestStatusWidthHoldsAsHealthChanges(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.updateStatusW()
	if got := p.statusW(); got != sectionStatusW {
		t.Fatalf("statusW = %d with no health, want the default %d", got, sectionStatusW)
	}

	want := len("Recreated") + healthSuffixW + sectionStatusGutterW
	for _, h := range []string{healthStarting, healthHealthy, healthUnhealthy} {
		p.health["db"] = h
		p.updateStatusW()
		if got := p.statusW(); got != want {
			t.Errorf("health %q: statusW = %d, want %d", h, got, want)
		}
	}
}

func TestHealthRowsAlign(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.health["db"] = healthHealthy

	col := func(name string) int {
		for _, line := range p.buildLines(160, false) {
			plain := semstyle.ToPlain(line)
			if i := strings.Index(plain, name+":"); i >= 0 {
				return utf8.RuneCountInString(plain[:i])
			}
		}
		t.Fatalf("no row for %s", name)
		return 0
	}
	if db, web := col("db"), col("web"); db != web {
		t.Errorf("name columns differ: db at %d, web at %d", db, web)
	}
}

func TestHealthFailureKeepsTheAction(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "dependency db failed to start"})
	db := p.tasks["db"]

	// Until its health is known unhealthy (it may have exited instead), it reads Error.
	if text, _ := p.serviceStatus(db); text != api.StatusError {
		t.Errorf("status = %q, want Error", text)
	}
	p.health["db"] = healthUnhealthy
	text, tag := p.serviceStatus(db)
	if text != "Recreated (Unhealthy)" {
		t.Errorf("status = %q, want Recreated (Unhealthy)", text)
	}
	if !strings.HasPrefix(tag, serviceStatusTag(api.Done, "Recreated", "up")) {
		t.Errorf("tag = %q, want the action styled as before the error", tag)
	}
	if fails := p.failures(); len(fails) != 1 || fails[0].name != "db" {
		t.Errorf("failures = %v, want db's", fails)
	}
}

func TestUnhealthyServicesIncludesFailures(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "dependency db failed to start"})
	p.health["db"] = healthUnhealthy
	p.health["web"] = healthUnhealthy

	if got := p.unhealthyServices(); !slices.Equal(got, []string{"db", "web"}) {
		t.Errorf("unhealthyServices = %v, want db (also a failure) and web", got)
	}
}

func TestInterruptedRowsAreWarningsGroupedByReason(t *testing.T) {
	p := NewConsoleEventProcessor(context.Background(), nil, "up",
		map[string][]string{testImg: {"app", "db", "web"}}, []string{testImg},
		map[string]string{}, "proj", false, false, true, nil, nil).(*consoleEventProcessor)
	const reason = "interrupted by the failure of Container db"
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "port is already allocated"})
	p.On(api.Resource{ID: "Container app", Status: api.Error, Text: reason})
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "not started: " + reason})

	web := p.tasks["web"]
	if web.text != interruptedText || web.status != api.Warning || !web.interrupted {
		t.Errorf("web = %q/%v, want Interrupted as a warning", web.text, web.status)
	}
	if line := p.errorLine(web, 160); !strings.Contains(line, "{{|DockerStatusWarn|}}not started: "+reason) {
		t.Errorf("message line = %q, want the reason in the warning style", line)
	}

	if fails := p.failures(); len(fails) != 1 || fails[0].name != "db" {
		t.Errorf("failures = %v, want only db", fails)
	}
	groups := p.interruptions()
	if len(groups) != 2 || groups[0].message != reason || groups[1].message != "not started: "+reason {
		t.Fatalf("interruptions = %+v, want one group per reason", groups)
	}
	if got := semstyle.ToPlain(groups[0].labels[0]); got != "app" {
		t.Errorf("first group = %q, want app", got)
	}
}

func TestStyleReasonStylesTheFailedService(t *testing.T) {
	p := newHealthTestProcessor()
	for _, tc := range []struct{ reason, want string }{
		{"Interrupted by the failure of Container db", "Interrupted by the failure of Container " + "{{|UserApp|}}db{{[-]}}"},
		{"Interrupted by the failure of Container proj-web-1", "Interrupted by the failure of Container " + "{{|UserApp|}}web{{[-]}}"},
		{"Interrupted by the failure of Container stranger", "Interrupted by the failure of Container stranger"},
		{`Not started: condition "service_healthy" of service "db" not met`, `Not started: condition "service_healthy" of service "db" not met`},
	} {
		got := p.styleReason(tc.reason)
		if semstyle.ToPlain(got) != semstyle.ToPlain(tc.want) || strings.Contains(tc.want, "{{") != strings.Contains(got, "{{") {
			t.Errorf("styleReason(%q) = %q, want %q", tc.reason, got, tc.want)
		}
	}
}

func TestSkippedRowShowsReasonUnderIt(t *testing.T) {
	p := newHealthTestProcessor()
	const reason = `condition "service_healthy" of service "db" not reached: interrupted by the failure of Container web`
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Warning, Text: "Skipped: " + reason})
	p.health["db"] = healthStarting

	db := p.tasks["db"]
	if db.text != skippedText || db.message != reason {
		t.Errorf("db = %q / %q, want Skipped with the reason as its message", db.text, db.message)
	}
	if line := p.errorLine(db, 200); !strings.Contains(line, "{{|DockerStatusWarn|}}"+reason) {
		t.Errorf("message line = %q, want the reason in the warning style", line)
	}
	p.updateStatusW()
	if got, want := p.statusW(), len(skippedText)+healthSuffixW+sectionStatusGutterW; got != want {
		t.Errorf("statusW = %d, want %d", got, want)
	}
}

func TestFinalStatusTakesItsRowStyle(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"service", serviceStatusTag(api.Done, "Recreated", "up"), "{{|App|}}Recreated"},
		{"image", imageStatusTag(api.Done, api.StatusPulled, "up"), "{{|DockerMarkerDone|}}Pulled"},
		{"layer", layerStatusTag(api.Done, "Already exists"), "{{[::D]}}"},
		{"network", networkStatusTag(api.Done, api.StatusCreated, "up"), "{{|App|}}Created"},
		{"volume", volumeStatusTag(api.Done, api.StatusCreated, "up"), "{{|App|}}Created"},
	} {
		if !strings.HasPrefix(tc.got, tc.want) {
			t.Errorf("%s: %q, want it to start with %q", tc.name, tc.got, tc.want)
		}
	}
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.health["db"] = healthHealthy
	if _, tag := p.serviceStatus(p.tasks["db"]); !strings.HasSuffix(tag, "{{|DockerHealthHealthy|}}(Healthy){{[-]}}") {
		t.Errorf("tag = %q, want (Healthy) in the health style", tag)
	}
}

func TestContainerHealth(t *testing.T) {
	hc := func(test ...string) *container.Config {
		return &container.Config{Healthcheck: &container.HealthConfig{Test: test}}
	}
	running := &container.State{Running: true}
	for _, tc := range []struct {
		name string
		c    container.InspectResponse
		want string
	}{
		{"stopped", container.InspectResponse{State: &container.State{}, Config: hc("CMD", "true")}, ""},
		{"no healthcheck", container.InspectResponse{State: running, Config: &container.Config{}}, ""},
		{"disabled", container.InspectResponse{State: running, Config: hc("NONE")}, ""},
		{"first check pending", container.InspectResponse{State: running, Config: hc("CMD", "true")}, healthStarting},
		{"healthy", container.InspectResponse{State: &container.State{Running: true, Health: &container.Health{Status: container.Healthy}}}, healthHealthy},
	} {
		if got := containerHealth(tc.c); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
