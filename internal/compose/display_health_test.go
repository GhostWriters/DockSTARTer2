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
	"github.com/moby/moby/api/types/events"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// newHealthTestProcessor returns an "up" processor for services web and db on one image.
func newHealthTestProcessor() *consoleEventProcessor {
	p := NewConsoleEventProcessor(context.Background(), nil, "up",
		map[string][]string{testImg: {"db", "web"}}, []string{testImg},
		map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
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

func TestHealthLineShowsHealth(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.healthCmd["db"] = "curl -fs http://localhost"

	if line := p.healthLine("db", 160); line != "" {
		t.Errorf("line = %q with no healthcheck, want none", line)
	}
	for _, tc := range []struct{ health, word, style string }{
		{healthStarting, "Checking", "{{|DockerHealthChecking|}}"},
		{healthHealthy, "Healthy", detailFinalTag},
		{healthUnhealthy, "Unhealthy", "{{|DockerHealthUnhealthy|}}"},
	} {
		p.setHealth("db", "db", tc.health)
		line := p.healthLine("db", 160)
		plain := strings.Join(strings.Fields(semstyle.ToPlain(line)), " ")
		if !strings.Contains(line, tc.style+tc.word) || !strings.HasSuffix(plain, tc.word+" healthcheck: curl -fs http://localhost") {
			t.Errorf("health %q: line %q", tc.health, plain)
		}
	}

	// The row keeps its plain action, with a warning icon while unhealthy.
	if text, _ := p.actionStatus(p.tasks["db"]); text != "Recreated" {
		t.Errorf("row = %q, want Recreated", text)
	}
	if got, want := p.serviceIcon(p.tasks["db"], api.Done), p.propagatedIcon(p.tasks["db"], api.Warning); got != want {
		t.Errorf("row icon = %q, want the warning %q", got, want)
	}
	if got, want := p.serviceIcon(p.tasks["db"], api.Error), p.propagatedIcon(p.tasks["db"], api.Error); got != want {
		t.Errorf("row icon = %q, want the error kept", got)
	}

	// Unhealthy shows the last check's output instead of the command.
	p.healthOutput["db"] = "Remote file does not exist -- broken link!!!\n"
	if plain := semstyle.ToPlain(p.healthLine("db", 160)); !strings.HasSuffix(plain, "healthcheck: Remote file does not exist -- broken link!!!") {
		t.Errorf("line = %q, want the last output", plain)
	}

	// Still on its first check when the end-of-run wait runs out.
	p.setHealth("db", "db", healthStarting)
	p.healthWaitDone = true
	if line := p.healthLine("db", 160); !strings.Contains(line, "{{|DockerHealthNoResult|}}No Result") {
		t.Errorf("line = %q, want No Result", line)
	}
}

func TestHealthCheckKeepsRowAndSectionWorking(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	ids := []string{"db", "web"}
	imageFor := func(string) string { return testImg }

	if got := p.rollupState(ids, imageFor); got != rollupComplete {
		t.Fatalf("rollup = %v with no health, want complete", got)
	}

	p.setHealth("db", "db", healthStarting)
	if line := p.healthLine("db", 160); !strings.Contains(line, p.activeSpinnerTag(p.icons().spinner)) {
		t.Errorf("line = %q while checking, want the spinner", line)
	}
	if got := p.rollupState(ids, imageFor); got != rollupProcessing {
		t.Errorf("rollup = %v while checking, want processing", got)
	}

	p.healthWaitDone = true // No Result: a warning, as its healthcheck line shows
	if got := p.rollupState(ids, imageFor); got != rollupWarning {
		t.Errorf("rollup = %v after the wait ran out, want warning", got)
	}

	p.setHealth("db", "db", healthUnhealthy)
	if got := p.rollupState(ids, imageFor); got != rollupWarning {
		t.Errorf("rollup = %v with an unhealthy container, want warning", got)
	}
}

func TestWaitForHealthReturnsOnceChecked(t *testing.T) {
	p := newHealthTestProcessor()
	p.setHealth("db", "db", healthHealthy)
	p.waitForHealth()
	if p.healthWaitDone {
		t.Errorf("the wait ran out with no container on its first check")
	}
}

func TestHealthLineSitsUnderItsService(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.setHealth("db", "db", healthHealthy)

	var plain []string
	for _, line := range p.buildLines(160, dockerlayout.VerboseDetails) {
		plain = append(plain, semstyle.ToPlain(line))
	}
	col := func(name string) int {
		for _, l := range plain {
			if i := strings.Index(l, name+":"); i >= 0 {
				return utf8.RuneCountInString(l[:i])
			}
		}
		t.Fatalf("no row for %s in %q", name, plain)
		return 0
	}
	if db, web := col("db"), col("web"); db != web {
		t.Errorf("name columns differ: db at %d, web at %d", db, web)
	}
	if hc, img := col("healthcheck"), col("image"); hc != img {
		t.Errorf("healthcheck at %d, want under image's %d", hc, img)
	}
	for i, l := range plain {
		if strings.Contains(l, "healthcheck:") && !strings.Contains(plain[i-1], "db:") {
			t.Errorf("healthcheck line follows %q, want db's row", plain[i-1])
		}
	}
}

func TestHealthFailureKeepsTheAction(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "dependency db failed to start"})
	db := p.tasks["db"]

	// Until its health is known unhealthy (it may have exited instead), it reads Error.
	if text, _ := p.actionStatus(db); text != api.StatusError {
		t.Errorf("status = %q, want Error", text)
	}
	p.setHealth("db", "db", healthUnhealthy)
	text, tag := p.actionStatus(db)
	if text != "Recreated" {
		t.Errorf("status = %q, want Recreated", text)
	}
	if line := p.healthLine("db", 160); !strings.Contains(line, "{{|DockerMarkerCause|}}"+p.icons().warn) {
		t.Errorf("health line = %q, want a warning icon in the error color: it failed a dependent", line)
	}
	if !strings.HasPrefix(tag, serviceStatusTag(api.Done, "Recreated", "up")) {
		t.Errorf("tag = %q, want the action styled as before the error", tag)
	}
	if fails := p.failures(); len(fails) != 0 {
		t.Errorf("failures = %v, want none: being unhealthy is a warning on its own", fails)
	}
}

func TestUnhealthyServicesIncludesFailures(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "dependency db failed to start"})
	p.setHealth("db", "db", healthUnhealthy)
	p.setHealth("web", "web", healthUnhealthy)

	if got := p.unhealthyServices(); !slices.Equal(got, []string{"db", "web"}) {
		t.Errorf("unhealthyServices = %v, want db (also a failure) and web", got)
	}
}

func TestInterruptedRowsAreWarningsGroupedByReason(t *testing.T) {
	p := NewConsoleEventProcessor(context.Background(), nil, "up",
		map[string][]string{testImg: {"app", "cache", "db", "web"}}, []string{testImg},
		map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	const reason = "interrupted by the failure of Container db"
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "port is already allocated"})
	p.On(api.Resource{ID: "Container app", Status: api.Error, Text: reason})
	p.On(api.Resource{ID: "Container cache", Status: api.Error, Text: reason})
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "not started: " + reason})

	web := p.tasks["web"]
	if web.text != interruptedText || web.status != api.Warning || !web.interrupted {
		t.Errorf("web = %q/%v, want Interrupted as a warning: compose stopped early", web.text, web.status)
	}
	if line := p.errorLine(web); !strings.Contains(line, "{{|DockerStatusWarn|}}not started: "+reason) {
		t.Errorf("message line = %q, want the reason in the warning style", line)
	}

	if fails := p.failures(); len(fails) != 1 || fails[0].name != "db" {
		t.Errorf("failures = %v, want only db", fails)
	}
	groups := p.interruptions()
	if len(groups) != 2 || groups[0].message != reason || groups[1].message != "not started: "+reason {
		t.Fatalf("interruptions = %+v, want one group per reason", groups)
	}
	var first []string
	for _, l := range groups[0].labels {
		first = append(first, semstyle.ToPlain(l))
	}
	if !slices.Equal(first, []string{"app", "cache"}) {
		t.Errorf("first group = %v, want app and cache together", first)
	}
}

func TestStyleReasonStylesTheFailedService(t *testing.T) {
	p := newHealthTestProcessor()
	const prefix = "Interrupted by the failure of Container "
	for _, tc := range []struct{ reason, want string }{
		{prefix + "db", prefix + dockerlayout.StyleServiceName("db")},
		{prefix + "proj-web-1", prefix + dockerlayout.StyleServiceName("web")},
		{prefix + "stranger", prefix + "stranger"},
		{`Not started: condition "service_healthy" of service "db" not met`, `Not started: condition "service_healthy" of service "db" not met`},
	} {
		if got := p.styleReason(tc.reason); got != tc.want {
			t.Errorf("styleReason(%q) = %q, want %q", tc.reason, got, tc.want)
		}
	}
}

func TestSkippedRowShowsReasonUnderIt(t *testing.T) {
	p := newHealthTestProcessor()
	const reason = `condition "service_healthy" of service "db" not reached: interrupted by the failure of Container web`
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Warning, Text: "Skipped: " + reason})
	p.setHealth("db", "db", healthStarting)

	db := p.tasks["db"]
	if db.text != skippedText || db.message != reason {
		t.Errorf("db = %q / %q, want Skipped with the reason as its message", db.text, db.message)
	}
	if line := p.errorLine(db); !strings.Contains(line, "{{|DockerStatusWarn|}}"+reason) {
		t.Errorf("message line = %q, want the reason in the warning style", line)
	}
}

func TestFinalStatusTakesItsRowStyle(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"section", func() string { _, tag, _ := sectionStatusText(rollupComplete); return tag }(), "{{|DockerSection|}}"},
		{"service", serviceStatusTag(api.Done, "Recreated", "up"), "{{|App|}}Recreated"},
		{"image", imageStatusTag(api.Done, api.StatusPulled, "up"), "{{|DockerImage|}}Pulled"},
		{"layer", layerStatusTag(api.Done, "Already exists"), "{{[::D]}}"},
		{"network", networkStatusTag(api.Done, api.StatusCreated, "up"), "{{|App|}}Created"},
		{"volume", volumeStatusTag(api.Done, api.StatusCreated, "up"), "{{|App|}}Created"},
	} {
		if !strings.HasPrefix(tc.got, tc.want) {
			t.Errorf("%s: %q, want it to start with %q", tc.name, tc.got, tc.want)
		}
	}
	// A service row showing its image's status keeps the service row's styles.
	for _, tc := range []struct{ name, got, want string }{
		{"pulling", serviceStatusTag(api.Working, api.StatusPulling, "update"), "{{|DockerStatusActive|}}Pulling"},
		{"pulled midway", serviceStatusTag(api.Done, api.StatusPulled, "update"), "{{|DockerStatusSuccess|}}Pulled"},
		{"pulled final", serviceStatusTag(api.Done, api.StatusPulled, "pull"), "{{|App|}}Pulled"},
	} {
		if !strings.HasPrefix(tc.got, tc.want) {
			t.Errorf("service %s: %q, want it to start with %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestContainerHealthInfo(t *testing.T) {
	c := container.InspectResponse{
		State: &container.State{Running: true, Health: &container.Health{
			Status: container.Unhealthy,
			Log:    []*container.HealthcheckResult{{Output: "old"}, {Output: "broken link"}},
		}},
		Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD-SHELL", "wget --spider x"}}},
	}
	if got := containerHealthInfo(c); got != (healthInfo{healthUnhealthy, "wget --spider x", "broken link"}) {
		t.Errorf("info = %+v", got)
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

// fakeDaemon answers container inspects from health and streams events, for the
// health watch tests.
type fakeDaemon struct {
	daemonClient
	health map[string]string // container → state its inspect reports
	events chan events.Message
}

func (f *fakeDaemon) ContainerInspect(_ context.Context, name string, _ dockerclient.ContainerInspectOptions) (dockerclient.ContainerInspectResult, error) {
	state := &container.State{Running: true}
	if h, ok := f.health[name]; ok {
		state.Health = &container.Health{Status: container.HealthStatus(h)}
	}
	return dockerclient.ContainerInspectResult{Container: container.InspectResponse{State: state}}, nil
}

func (f *fakeDaemon) Events(context.Context, dockerclient.EventsListOptions) dockerclient.EventsResult {
	return dockerclient.EventsResult{Messages: f.events, Err: make(chan error)}
}

func healthEvent(svc, name, status string) events.Message {
	return events.Message{
		Action: events.Action("health_status: " + status),
		Actor:  events.Actor{Attributes: map[string]string{api.ServiceLabel: svc, "name": name}},
	}
}

func TestReplicaHealthShowsTheWorst(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.setHealth("db", "proj-db-1", healthHealthy)
	p.setHealth("db", "proj-db-2", healthUnhealthy)
	if got := p.serviceHealth("db"); got != healthUnhealthy {
		t.Errorf("serviceHealth = %q, want unhealthy from either replica", got)
	}
	p.setHealth("db", "proj-db-2", healthStarting)
	if got := p.serviceHealth("db"); got != healthStarting {
		t.Errorf("serviceHealth = %q, want starting while a replica checks", got)
	}
}

func TestHealthEventsOnlyForTrackedContainers(t *testing.T) {
	fake := &fakeDaemon{health: map[string]string{"db": healthStarting}, events: make(chan events.Message)}
	p := newHealthTestProcessor()
	p.dockerClient = fake
	p.startHealthWatch(context.Background())
	p.On(containerEvent("db", api.StatusStarted, api.Done)) // tracks and inspects db
	p.healthWG.Wait()

	fake.events <- healthEvent("web", "web", healthStarting) // web wasn't started by this run
	fake.events <- healthEvent("db", "db", healthHealthy)
	fake.health["db"] = healthHealthy
	p.stopHealthWatch()

	if got := p.serviceHealth("db"); got != healthHealthy {
		t.Errorf("db = %q, want healthy from its event", got)
	}
	if got := p.serviceHealth("web"); got != "" {
		t.Errorf("web = %q, want no health for a container the run didn't start", got)
	}
}

func TestStopHealthWatchRefreshesTrackedContainers(t *testing.T) {
	fake := &fakeDaemon{health: map[string]string{"db": healthStarting}, events: make(chan events.Message)}
	p := newHealthTestProcessor()
	p.dockerClient = fake
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.healthWG.Wait()

	fake.health["db"] = healthUnhealthy // an event the watch never saw
	p.stopHealthWatch()
	if got := p.serviceHealth("db"); got != healthUnhealthy {
		t.Errorf("db = %q, want the refreshed unhealthy", got)
	}
}

func TestHealthyLineHiddenWithoutVerbose(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.setHealth("db", "db", healthStarting)
	if lines := p.serviceDetailLines("db", 160, 0, false); len(lines) != 1 {
		t.Fatalf("lines = %q, want the Checking line shown without -v", lines)
	}
	p.setHealth("db", "db", healthHealthy)
	if lines := p.serviceDetailLines("db", 160, 0, false); lines != nil {
		t.Errorf("lines = %q, want the Healthy line hidden without -v", lines)
	}
	if lines := p.serviceDetailLines("db", 160, dockerlayout.VerboseDetails, false); len(lines) != 1 {
		t.Errorf("lines = %q, want the Healthy line shown with -v", lines)
	}
	p.setHealth("db", "db", healthUnhealthy)
	if lines := p.serviceDetailLines("db", 160, 0, false); len(lines) != 1 {
		t.Errorf("lines = %q, want the Unhealthy line shown without -v", lines)
	}
}

func TestUnmetConditionIsTheDependentsError(t *testing.T) {
	p := newHealthTestProcessor()
	p.dependsOn = map[string][]serviceDep{
		"web":   {{name: "db", condition: types.ServiceConditionHealthy, required: true}},
		"cache": {{name: "db", condition: types.ServiceConditionHealthy, required: false}},
	}
	p.serviceIDs = append(p.serviceIDs, "cache")
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(api.Resource{ID: "Container db", Status: api.Error, Text: "dependency db failed to start"})
	p.setHealth("db", "db", healthUnhealthy)
	const unmet = `not started: condition "service_healthy" of service "db" not met`
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: unmet})
	p.On(api.Resource{ID: "Container cache", Status: api.Error, Text: unmet})

	db, web, cache := p.tasks["db"], p.tasks["web"], p.tasks["cache"]
	if db.status != api.Warning || p.serviceIcon(db, db.status) != "{{|DockerMarkerCause|}}"+p.icons().warn+"{{[-]}}" {
		t.Errorf("db: status %v, icon %q; want a warning, its icon in the error color", db.status, p.serviceIcon(db, db.status))
	}
	if web.status != api.Error || web.text != notStartedText || !strings.Contains(p.errorLine(web), unmet) {
		t.Errorf("web: %v %q, line %q; want Not Started as the error, with compose's message", web.status, web.text, p.errorLine(web))
	}
	if !cache.interrupted || cache.status != api.Warning || cache.text != interruptedText || cache.message != unmet {
		t.Errorf("cache: %v %q %q; want Interrupted with compose's message: its dependency on db is optional, so compose stopped early", cache.status, cache.text, cache.message)
	}
	fails := p.failures()
	if len(fails) != 1 || fails[0].name != "web" || fails[0].message != unmet {
		t.Errorf("failures = %v, want only web, with compose's message", fails)
	}
}
func TestErrorPlacement(t *testing.T) {
	p := newHealthTestProcessor()
	prefix := p.errorLinePrefixes()[0]
	row := "   ✓ Started     web:"
	wide := "   ✓ Cached        image: example/app:latest                                       [####]"
	long := "failed to set up container networking: driver failed programming external connectivity on endpoint web: Bind for 0.0.0.0:8000 failed: port is already allocated"
	lines, timers := p.placeErrorLines(
		[]string{row, prefix + "Not Started{{[-]}}", wide, row, prefix + long + "{{[-]}}"},
		make([]timerEntry, 5), 80) // a terminal with no room past the table
	if len(lines) != len(timers) {
		t.Fatalf("%d lines but %d timers", len(lines), len(timers))
	}
	if plain := semstyle.ToPlain(lines[0]); plain != "   ✓ Started     web:  Not Started" {
		t.Errorf("row = %q, want the short message two spaces past the name", plain)
	}
	wrapped := lines[3:]
	if len(wrapped) < 3 {
		t.Fatalf("lines = %q, want the long message wrapped under its row", plainLines(lines))
	}
	var parts []string
	for _, l := range wrapped {
		if !strings.HasPrefix(l, prefix) {
			t.Errorf("wrapped line %q, want it at the error indent", l)
		}
		if w := utf8.RuneCountInString(semstyle.ToPlain(l)); w > utf8.RuneCountInString(wide) {
			t.Errorf("wrapped line %q is wider than the table", semstyle.ToPlain(l))
		}
		parts = append(parts, strings.TrimSpace(semstyle.ToPlain(l)))
	}
	if got := strings.Join(parts, " "); got != long {
		t.Errorf("wrapped text = %q, want the whole message", got)
	}
}

func TestErrorWrapUsesTheTerminalsRoom(t *testing.T) {
	p := newHealthTestProcessor()
	prefix := p.errorLinePrefixes()[0]
	row := "   × Error       web:"
	long := strutil.Repeat("word ", 30) // 149 runes
	lines, _ := p.placeErrorLines([]string{row, prefix + long + "{{[-]}}"}, make([]timerEntry, 2), 200)
	if len(lines) != 3 {
		t.Fatalf("lines = %q, want the message in two lines of up to %d", plainLines(lines), errorWrapW)
	}
	for _, l := range lines[1:] {
		if w := utf8.RuneCountInString(strings.TrimSpace(semstyle.ToPlain(l))); w > errorWrapW {
			t.Errorf("line %q is %d wide, more than %d", semstyle.ToPlain(l), w, errorWrapW)
		}
	}
}

func TestWrapWordsCapsLines(t *testing.T) {
	got := wrapWords(strutil.Repeat("word ", 40), 20, 6)
	if len(got) != 6 || !strings.HasSuffix(got[5], "…") {
		t.Errorf("wrapWords = %q, want 6 lines, the last ending in …", got)
	}
	for _, l := range got {
		if utf8.RuneCountInString(l) > 20 {
			t.Errorf("line %q is wider than 20", l)
		}
	}
	if got := wrapWords("sha256:"+strutil.Repeat("a", 30), 20, 6); len(got) != 2 || got[0] != "sha256:"+strutil.Repeat("a", 13) {
		t.Errorf("long word = %q, want it broken at 20", got)
	}
}

func TestWrapStyledMeasuresWithoutTags(t *testing.T) {
	name := dockerlayout.StyleServiceName("beets")
	got := wrapStyled("unhealthy, and "+name+" needs it healthy", 16)
	want := []string{"unhealthy, and", name + " needs it", "healthy"}
	if !slices.Equal(got, want) {
		t.Errorf("wrapStyled = %q, want %q", got, want)
	}
	if got := wrapStyled("abcdefghij", 4); !slices.Equal(got, []string{"abcd", "efgh", "ij"}) {
		t.Errorf("long plain word = %q, want it broken at 4", got)
	}
}

func TestInlineErrorsShareAColumn(t *testing.T) {
	p := newHealthTestProcessor()
	warn := p.errorLinePrefixes()[1]
	wide := "   ✓ Cached        image: example/app:latest                                       [####]"
	lines, _ := p.placeErrorLines([]string{
		"   ⚠ Interrupted   web:", warn + "interrupted by db{{[-]}}",
		"   ⚠ Interrupted   calibreweb:", warn + "interrupted by db{{[-]}}",
		wide, warn + "too wide to share{{[-]}}",
	}, make([]timerEntry, 6), 200)
	var cols []int
	for _, l := range lines[:2] {
		plain := semstyle.ToPlain(l)
		cols = append(cols, utf8.RuneCountInString(plain[:strings.Index(plain, "interrupted by")]))
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Errorf("messages start at %v, want one column: %q", cols, plainLines(lines))
	}
	if got := strings.TrimSpace(semstyle.ToPlain(lines[len(lines)-1])); got != "too wide to share" {
		t.Errorf("last line = %q, want the wide row's message on its own line", got)
	}
}

func TestOptionalDependencyFailureLeavesItsRow(t *testing.T) {
	p := newHealthTestProcessor()
	p.dependsOn = map[string][]serviceDep{"web": {{name: "db", condition: types.ServiceConditionHealthy, required: false}}}
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.setHealth("db", "db", healthUnhealthy)
	p.On(api.Resource{ID: "Container db", Status: api.Warning, Text: `Skipped: optional dependency "db" failed to start`})
	p.On(containerEvent("web", api.StatusStarted, api.Done))

	if db := p.tasks["db"]; db.text != api.StatusRunning || db.message != "" {
		t.Errorf("db = %q %q, want it left Running: compose files its dependents' warning under it", db.text, db.message)
	}
	if lines := plainLines(p.dependsLines("web", 160, true)); len(lines) != 1 || !strings.Contains(lines[0], "⚠ Unmet") || !strings.Contains(lines[0], "db (Unhealthy)") {
		t.Errorf("web's needs line = %q, want an optional Unmet naming db unhealthy", lines)
	}
}

func TestHealthLineRedactsSecrets(t *testing.T) {
	p := newHealthTestProcessor()
	p.setHealth("db", "db", healthUnhealthy)
	p.healthOutput = map[string]string{"db": "curl: (22) 401 using Authorization: Bearer abcdef123456 and DB_PASSWORD=hunter2"}
	got := plainLines([]string{p.healthLine("db", 200)})[0]
	if strings.Contains(got, "abcdef123456") || strings.Contains(got, "hunter2") {
		t.Errorf("health line holds a secret: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Errorf("health line = %q, want the redaction marked", got)
	}
}
