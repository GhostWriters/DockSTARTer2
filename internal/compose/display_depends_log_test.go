package compose

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
)

func TestRedactLogLine(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"env pair", "DB_PASSWORD=hunter2 starting", "DB_PASSWORD=[redacted] starting"},
		{"colon pair", "password: hunter2", "password: [redacted]"},
		{"json pair", `{"api_key":"abc123","user":"bob"}`, `{"api_key":[redacted],"user":"bob"}`},
		{"quoted value with spaces", `secret="two words"`, `secret=[redacted]`},
		{"flag", "run --password hunter2 --verbose", "run --password [redacted] --verbose"},
		{"bearer header", "Authorization: Bearer abcdef123456", "Authorization: [redacted]"},
		{"url credentials", "connect postgres://user:s3cret@db:5432/app", "connect postgres://user:[redacted]@db:5432/app"},
		{"url token parameter", "GET /x?token=abc&page=2", "GET /x?token=[redacted]&page=2"},
		{"jwt", "got eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.c2lnbmF0dXJl ok", "got [redacted] ok"},
		{"github token", "using ghp_abcdefghijklmnopqrstuvwxyz0123456789", "using [redacted]"},
		{"aws key", "id AKIAABCDEFGHIJKLMNOP", "id [redacted]"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----", "[redacted]"},
		{"long hex", "digest 5d41402abc4b2a76b9719d911017c5925d41402abc4b2a76b9719d911017c592", "digest [redacted]"},
		{"already redacted", "token: [redacted]", "token: [redacted]"},

		// What must survive: the message is still readable.
		{"plain sentence", "Error: Database is uninitialized and superuser password is not specified.", "Error: Database is uninitialized and superuser password is not specified."},
		{"short word", "listening on port 5432", "listening on port 5432"},
		{"long path", "initdb: directory /var/lib/postgresql/data/pgdata/base_directory_name exists", "initdb: directory /var/lib/postgresql/data/pgdata/base_directory_name exists"},
	} {
		if got := redactLogLine(tc.in); got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, got, tc.want)
		}
	}
}

func TestCleanLogLineStripsTerminalCodes(t *testing.T) {
	got := cleanLogLine("\x1b[31mERROR\x1b[0m\tbad  {{[red]}} PASSWORD=x\r")
	want := "ERROR bad {\u2060{[red]}} PASSWORD=[redacted]"
	if got != want {
		t.Errorf("cleanLogLine = %q, want %q", got, want)
	}
}

func TestPickLogLines(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  []string
	}{
		{"none", nil, nil},
		{"error then a hint", []string{"starting", "Error: not initialized", "set a password", "see docs"}, []string{"Error: not initialized", "see docs"}},
		{"error is last", []string{"starting", "ready", "fatal: bind failed"}, []string{"fatal: bind failed"}},
		{"no error shows the last three", []string{"a", "b", "c", "d", "e"}, []string{"c", "d", "e"}},
		{"fewer than three", []string{"a", "b"}, []string{"a", "b"}},
		{"first error wins", []string{"error one", "error two", "tail"}, []string{"error one", "tail"}},
	} {
		got := pickLogLines(tc.lines)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// logDaemon answers the container lookup, inspect and log calls for one container.
type logDaemon struct {
	daemonClient
	filters dockerclient.Filters
	service string              // the service the container belongs to, "db" when empty
	items   []container.Summary // the project's containers when listed whole, instead of one for service
	state   container.State
	restart int
	tty     bool
	logs    []byte
	read    int // log reads
}

func (f *logDaemon) ContainerList(_ context.Context, o dockerclient.ContainerListOptions) (dockerclient.ContainerListResult, error) {
	f.filters = o.Filters
	if len(f.items) > 0 && !o.Filters["label"][api.ServiceLabel+"=lone"] && !o.Filters["label"][api.ServiceLabel+"=db"] && !o.Filters["label"][api.ServiceLabel+"=web"] {
		return dockerclient.ContainerListResult{Items: f.items}, nil
	}
	svc := f.service
	if svc == "" {
		svc = "db"
	}
	return dockerclient.ContainerListResult{Items: []container.Summary{{ID: "abc", State: f.state.Status, Labels: map[string]string{api.ServiceLabel: svc}}}}, nil
}

func (f *logDaemon) ContainerInspect(context.Context, string, dockerclient.ContainerInspectOptions) (dockerclient.ContainerInspectResult, error) {
	return dockerclient.ContainerInspectResult{Container: container.InspectResponse{
		State: &f.state, RestartCount: f.restart, Config: &container.Config{Tty: f.tty},
	}}, nil
}

func (f *logDaemon) ContainerLogs(context.Context, string, dockerclient.ContainerLogsOptions) (dockerclient.ContainerLogsResult, error) {
	f.read++
	return io.NopCloser(bytes.NewReader(f.logs)), nil
}

// muxed frames text the way Docker does for a container without a TTY.
func muxed(stream stdcopy.StdType, text string) []byte {
	header := []byte{byte(stream), 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[4:], uint32(len(text)))
	return append(header, text...)
}

func TestFetchDependencyLogs(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	d := &logDaemon{
		state:   container.State{Status: "restarting", ExitCode: 1},
		restart: 5,
		logs: append(muxed(stdcopy.Stdout, "starting up\nDB_PASSWORD=hunter2\n"),
			muxed(stdcopy.Stderr, "Error: Database is uninitialized\nYou may also use trust\n")...),
	}
	p.dockerClient = d
	p.finished = true // db never started

	p.checkServicesDown()

	got := p.downLogs["db"]
	if got == nil {
		t.Fatal("no log read for db")
	}
	if want := "Restarting, exit code 1, restarted 5 times"; got.state != want {
		t.Errorf("state = %q, want %q", got.state, want)
	}
	if want := "Error: Database is uninitialized|You may also use trust"; strings.Join(got.lines, "|") != want {
		t.Errorf("lines = %q, want %q", got.lines, want)
	}
	if !d.filters["label"]["com.docker.compose.project=proj"] || !d.filters["label"][api.ServiceLabel+"=db"] {
		t.Errorf("container lookup filters = %v, want the project and service labels", d.filters)
	}
}

func TestFetchDependencyLogsKeepsNoSecrets(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	p.dockerClient = &logDaemon{state: container.State{Status: "exited", ExitCode: 2}, tty: true,
		logs: []byte("connecting with DB_PASSWORD=hunter2 to postgres://u:pw@h/db failed\n")}
	p.finished = true

	p.checkServicesDown()

	lines := strings.Join(p.downLogs["db"].lines, "\n")
	if strings.Contains(lines, "hunter2") || strings.Contains(lines, ":pw@") {
		t.Errorf("stored lines hold a secret: %q", lines)
	}
	if !strings.Contains(lines, "[redacted]") {
		t.Errorf("stored lines = %q, want the redaction marked", lines)
	}
}

func TestFetchDependencyLogsSkipsMetDependencies(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	d := &logDaemon{state: container.State{Status: "running"}, logs: []byte("fine\n")}
	p.dockerClient = d
	p.preRun["db"] = "running" // db was up before the run
	p.finished = true

	p.checkServicesDown()

	if p.downLogs["db"] != nil || d.read != 0 {
		t.Errorf("read the log of a dependency that is up: %+v, %d reads", p.downLogs["db"], d.read)
	}
}

func TestFetchDependencyLogsFindsADependencyThatStartedThenFailed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   container.State
		problem string
	}{
		{"restarting", container.State{Status: "restarting", Restarting: true, ExitCode: 1}, "Restarting"},
		{"exited non-zero", container.State{Status: "exited", ExitCode: 3}, "Exited"},
		{"dead", container.State{Status: "dead"}, "Dead"},
	} {
		p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
		p.projectName = "proj"
		p.On(containerEvent("db", api.StatusStarted, api.Done)) // compose reports it started
		p.dockerClient = &logDaemon{state: tc.state, tty: true, logs: []byte("Error: cannot open the data directory\n")}
		p.finished = true

		p.checkServicesDown()

		if got := p.downLogs["db"]; got == nil || got.problem != tc.problem || len(got.lines) != 1 {
			t.Errorf("%s: read %+v, want problem %q and its log", tc.name, got, tc.problem)
		}
		lines := plainLines(p.dependsLines("web", 160, false))
		if len(lines) != 1 || !strings.Contains(lines[0], "Crashed") || !strings.Contains(lines[0], "db ("+tc.problem+")") {
			t.Errorf("%s: lines = %q, want the needs line crashed with the problem", tc.name, lines)
		}
		if got := p.serviceLogLines("db", 160); len(got) != 2 {
			t.Errorf("%s: db's own lines = %q, want its state and log", tc.name, plainLines(got))
		}
	}
}

func TestFetchDependencyLogsIgnoresACleanExit(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.dockerClient = &logDaemon{state: container.State{Status: "exited", ExitCode: 0}, logs: []byte("done\n")}
	p.finished = true

	p.checkServicesDown()

	if p.downLogs["db"] != nil {
		t.Errorf("read a one-shot dependency that exited 0: %+v", p.downLogs["db"])
	}
}

func TestServiceLogLinesShowStateAndLog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		failed bool
		tag    string // the state's colour
		logTag string // the log's colour
	}{
		{"a dependency that went down takes the warning colour", false, "{{|DockerMarkerWarn|}}", "{{|DockerStatusWarn|}}"},
		{"a dependency whose row failed is red throughout", true, "{{|DockerMarkerError|}}", "{{|DockerMarkerError|}}"},
	} {
		p := newDependsTestProcessor()
		p.downLogs = map[string]*downLog{"db": {
			state: "Restarting, exit code 1",
			lines: []string{"Error: password: [redacted] missing", "see docs"},
		}}
		if tc.failed {
			p.On(containerEvent("db", "Error", api.Error))
		}

		lines := p.serviceLogLines("db", 160)
		plain := plainLines(lines)
		if len(lines) != 3 || !strings.HasSuffix(plain[0], "Crashed          state: Restarting, exit code 1") {
			t.Fatalf("%s: lines = %q, want the state, with its Crashed status, and two log lines", tc.name, plain)
		}
		if plain[1] != "log:   Error: password: [redacted] missing" || plain[2] != "see docs" {
			t.Errorf("%s: log lines %q", tc.name, plain[1:])
		}
		if !strings.Contains(lines[0], tc.tag+"Restarting") || !strings.Contains(lines[2], tc.logTag+"see docs") {
			t.Errorf("%s: lines not coloured with %s and %s:\n%q\n%q", tc.name, tc.tag, tc.logTag, lines[0], lines[2])
		}
		if !strings.Contains(lines[1], "{{[::D]}}[redacted]{{[-]}}") {
			t.Errorf("%s: redaction not dim: %q", tc.name, lines[1])
		}
	}
}

func TestServiceLogLinesAreEmptyWithoutALog(t *testing.T) {
	p := newDependsTestProcessor()
	if lines := p.serviceLogLines("db", 160); lines != nil {
		t.Errorf("lines = %q, want none", plainLines(lines))
	}
}

func TestServiceLogLinesWrapToTwoRows(t *testing.T) {
	p := newDependsTestProcessor()
	p.downLogs = map[string]*downLog{"db": {lines: []string{strings.Repeat("word ", 40)}}}

	lines := plainLines(p.serviceLogLines("db", 70))

	if len(lines) != 2 || !strings.HasSuffix(lines[1], "…") {
		t.Errorf("lines = %q, want two rows ending in an ellipsis", lines)
	}
}

func TestDependencyThatWentDownShowsItsLogsUnderItsOwnRow(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting, exit code 1", lines: []string{"Error: no password"}}}
	p.finished = true

	p.dependsOn["cache"] = []serviceDep{dep("db", types.ServiceConditionStarted, true)}
	conditions := plainLines(p.serviceConditionLines("db", 160, 0, true, false))
	if len(conditions) != 3 || !strings.Contains(conditions[0], "Crashed") || !strings.HasSuffix(conditions[0], "state: Restarting, exit code 1") || conditions[1] != "log:   Error: no password" ||
		!strings.Contains(conditions[2], "Crashed") || !strings.Contains(conditions[2], "needs started by: cache, web") {
		t.Errorf("db condition lines = %q, want its state and log, then who needs it", conditions)
	}
	needs := plainLines(p.dependsLines("web", 160, false))
	if len(needs) != 1 || !strings.Contains(needs[0], "db (Restarting)") {
		t.Errorf("web needs lines = %q, want one line pointing at db's problem", needs)
	}
	if !p.serviceDown("db") {
		t.Error("db should count as down, giving its row the warning icon")
	}
}

// indentOf returns how many columns a line is indented in plain text.
func indentOf(line string) int {
	plain := semstyle.ToPlain(line)
	return len(plain) - len(strings.TrimLeft(plain, " "))
}

// columnOf returns the column, in characters, where word starts in a line's plain text.
func columnOf(line, word string) int {
	plain := semstyle.ToPlain(line)
	return utf8.RuneCountInString(plain[:strings.Index(plain, word)])
}

// dependencyWithEvidence returns a processor where db is a dependency of web and cache
// that is down and unhealthy.
func dependencyWithEvidence(webNeeds, cacheNeeds string) *consoleEventProcessor {
	p := newDependsTestProcessor(dep("db", webNeeds, true))
	p.dependsOn["cache"] = []serviceDep{dep("db", cacheNeeds, true)}
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.setHealth("db", "db", healthUnhealthy)
	p.healthOutput = map[string]string{"db": "connection refused"}
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting", lines: []string{"boom"}}}
	p.finished = true
	return p
}

func TestConditionLinesAreInCauseAndEffectOrderAndFlat(t *testing.T) {
	p := dependencyWithEvidence(types.ServiceConditionStarted, types.ServiceConditionHealthy)

	lines := p.serviceConditionLines("db", 160, 0, true, false)

	// The service itself (healthcheck, state, log), then who waits on it.
	plain := plainLines(lines)
	want := []string{"healthcheck: connection refused", "state: Restarting", "log:   boom", "needs started by: web", "needs healthy by: cache"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %d", plain, len(want))
	}
	for i, w := range want {
		if !strings.Contains(plain[i], w) {
			t.Errorf("line %d = %q, want it to hold %q", i, plain[i], w)
		}
	}
	// No nesting: every label starts in the same column.
	health, state, needs := columnOf(lines[0], "healthcheck"), columnOf(lines[1], "state"), columnOf(lines[3], "needs")
	if state != health || needs != health {
		t.Errorf("columns: healthcheck %d, state %d, needed-by %d, want them level", health, state, needs)
	}
}

func TestConditionLinesPutWhatItWaitsOnBeforeItsOwnHealth(t *testing.T) {
	p := dependencyWithEvidence(types.ServiceConditionStarted, types.ServiceConditionStarted)
	p.On(containerEvent("web", api.StatusStarted, api.Done))
	p.setHealth("web", "web", healthUnhealthy)
	p.healthOutput["web"] = "cannot reach db"

	plain := plainLines(p.serviceConditionLines("web", 160, 0, true, false))

	if len(plain) != 2 || !strings.Contains(plain[0], "needs started: db") || !strings.Contains(plain[1], "healthcheck: cannot reach db") {
		t.Errorf("web lines = %q, want what it waits on, then its healthcheck", plain)
	}
}

func TestConditionLinesWithoutDependentsAreUnchanged(t *testing.T) {
	p := newDependsTestProcessor()
	p.setHealth("db", "db", healthUnhealthy)
	p.healthOutput = map[string]string{"db": "connection refused"}

	lines := p.serviceConditionLines("db", 160, 0, true, false)

	if len(lines) != 1 || indentOf(lines[0]) != indentOf(p.healthLine("db", 160)) {
		t.Errorf("lines = %q, want only the top-level healthcheck line", plainLines(lines))
	}
}

func TestDownDependencyThatIsRequiredIsShownAsTheCause(t *testing.T) {
	p := dependencyWithEvidence(types.ServiceConditionStarted, types.ServiceConditionStarted)
	warn := p.icons().warn
	row := p.tasks["db"]

	if got := p.serviceIcon(row, row.status); got != "{{|DockerMarkerCause|}}"+warn+"{{[-]}}" {
		t.Errorf("db row icon = %q, want the cause marker", got)
	}
	lines := p.serviceConditionLines("db", 160, 0, true, false)
	byLabel := func(label string) string {
		for _, l := range lines {
			if strings.Contains(semstyle.ToPlain(l), label) {
				return l
			}
		}
		return ""
	}
	if l := byLabel("needs started by"); !strings.Contains(l, "{{|DockerMarkerCause|}}"+warn) || !strings.Contains(l, "{{|DockerStatusWarn|}}Crashed") ||
		!strings.Contains(l, "{{|DockerDependsWaiting|}}web") {
		t.Errorf("needed-by line = %q, want the cause marker with Crashed and the blocked dependents in the warning colour", l)
	}
	if l := byLabel("state:"); !strings.Contains(l, "{{|DockerMarkerError|}}Restarting") {
		t.Errorf("state line = %q, want the cause's red", l)
	}
	if l := byLabel("log:"); !strings.Contains(l, "{{|DockerStatusWarn|}}boom") {
		t.Errorf("log line = %q, want the message in the warning colour", l)
	}
	needs := p.dependsLines("web", 160, false)
	if len(needs) != 1 || !strings.Contains(needs[0], "{{|DockerMarkerWarn|}}"+warn) || !strings.Contains(needs[0], "{{|DockerStatusWarn|}}Crashed") ||
		!strings.Contains(needs[0], "{{|DockerDependsWaiting|}}db") {
		t.Errorf("web's needs line = %q, want a warning: DS2 inferred it, compose reported no failure", needs)
	}
}

func TestDownDependencyThatIsOnlyOptionalStaysAWarning(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, false))
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting", lines: []string{"boom"}}}
	p.finished = true
	row := p.tasks["db"]

	if got := p.serviceIcon(row, row.status); got != "{{|DockerMarkerWarn|}}"+p.icons().warn+"{{[-]}}" {
		t.Errorf("db row icon = %q, want a plain warning", got)
	}
	lines := p.serviceConditionLines("db", 160, 0, true, false)
	if strings.Contains(lines[2], "DockerMarkerCause") || !strings.Contains(lines[0], "{{|DockerMarkerWarn|}}Restarting") {
		t.Errorf("lines = %q, want warning colours for an optional dependent", lines)
	}
}

func TestDependencyLinesShowWhenMetUntilTheRunSettles(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.On(containerEvent("db", api.StatusStarted, api.Done)) // db is up, so the line is Ready

	lines := plainLines(p.serviceDetailLines("web", 160, 0, true))
	if len(lines) != 1 || !strings.Contains(lines[0], "Ready") || !strings.Contains(lines[0], "needs started: db") {
		t.Errorf("while running: %q, want the met needs line shown", lines)
	}

	p.settleDependencies()
	if lines := p.serviceDetailLines("web", 160, 0, true); len(lines) != 0 {
		t.Errorf("once settled: %q, want the met line collapsed", plainLines(lines))
	}
}

func TestExpandedLinesDoNotChangeWhichRowsAreChanges(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.On(containerEvent("db", api.StatusStarted, api.Done))

	if lines := p.serviceConditionLines("web", 160, 0, false, false); len(lines) != 0 {
		t.Errorf("the grouping check sees %q while running, want no lines so rows keep their group", plainLines(lines))
	}
}

func TestFetchDependencyLogsForgetsADependencyThatCameBackUp(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	d := &logDaemon{state: container.State{Status: "restarting", Restarting: true, ExitCode: 1}, tty: true, logs: []byte("boom\n")}
	p.dockerClient = d
	p.finished = true

	p.checkServicesDown()
	if p.downLogs["db"] == nil {
		t.Fatal("no problem found while it restarts")
	}
	d.state = container.State{Status: "running", Running: true}
	p.checkServicesDown()

	if p.downLogs["db"] != nil {
		t.Errorf("db still marked down after it came back: %+v", p.downLogs["db"])
	}
}

func TestStoppedDependencyIsAChangeFromTheFirstFrame(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.command = "up"
	p.preRun["db"] = "stopped" // up is going to start it

	if got := p.serviceGroup("db", "", 160); got != groupChanged {
		t.Errorf("db group = %v, want its own row under changed while it starts", got)
	}
	p.preRun["cache"] = "stopped" // starting too, but nothing depends on it
	if got := p.serviceGroup("cache", "", 160); got != groupPending {
		t.Errorf("cache group = %v, want it pending in the group of its state", got)
	}
	lines := plainLines(p.serviceDetailLines("db", 160, 0, true))
	if len(lines) != 1 || !strings.Contains(lines[0], "needs started by: web") {
		t.Errorf("db lines = %q, want who waits on it while it starts", lines)
	}

	p.preRun["db"] = "running"
	if got := p.serviceGroup("db", "", 160); got != groupPending {
		t.Errorf("a running dependency is group %v, want it pending until something happens", got)
	}
}

func TestPendingDependencyIsNotAChangeToWhatWaitsOnIt(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.command = "up"
	p.preRun["web"] = "running"
	p.preRun["db"] = "stopped" // up is starting it; web, already running, only checks

	if p.hasConditionLines("web", 160) {
		t.Error("a pending needs line makes web a change, want it to stay with the unchanged services")
	}
	if got := p.serviceGroup("web", "", 160); got != groupPending {
		t.Errorf("web group = %v, want pending, in the group of its state", got)
	}

	p.finished = true // db never started
	if !p.hasConditionLines("web", 160) {
		t.Error("an unmet dependency is a problem under web, want it to count")
	}
}

// nginxScenario returns an up run where nginx is stopped and organizr and seerr, already
// running, depend on it (seerr also on radarr, running).
func nginxScenario() *consoleEventProcessor {
	imgs := map[string][]string{"i/nginx:1": {"nginx"}, "i/organizr:1": {"organizr"}, "i/seerr:1": {"seerr"}, "i/radarr:1": {"radarr"}}
	order := []string{"i/nginx:1", "i/organizr:1", "i/seerr:1", "i/radarr:1"}
	p := NewConsoleEventProcessor(context.Background(), nil, "up", imgs, order, map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	p.preRun = map[string]string{"nginx": "stopped", "organizr": "running", "seerr": "running", "radarr": "running"}
	for _, img := range order {
		p.imageMetaBefore[img] = imageMeta{}
	}
	p.dependsOn = map[string][]serviceDep{
		"organizr": {dep("nginx", types.ServiceConditionStarted, true)},
		"seerr":    {dep("nginx", types.ServiceConditionStarted, true), dep("radarr", types.ServiceConditionStarted, true)},
	}
	return p
}

func TestUnchangedServicesCheckingADependencyShowTheirLinesUntilItPasses(t *testing.T) {
	p := nginxScenario()

	got := renderPlain(p, 0)

	for _, want := range []string{
		"Pending          nginx:",
		"needs started by: organizr, seerr",
		"2 services: organizr, seerr",
		"needs started: nginx",
		"1 service: radarr",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("frame lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "organizr:") {
		t.Errorf("organizr has its own row, want it to stay in the unchanged group:\n%s", got)
	}

	p.On(containerEvent("nginx", api.StatusStarted, api.Done)) // the check passes
	p.On(containerEvent("nginx", api.StatusStarted, api.Done))
	after := renderPlain(p, 0)
	if strings.Contains(after, "needs started: nginx") {
		t.Errorf("the lines stay after nginx started:\n%s", after)
	}
}

func TestDownSummaryListsEachDownContainerWithItsStateAndLog(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true), dep("cache", types.ServiceConditionStarted, true))
	p.serviceIDs = []string{"web", "db", "cache"}
	p.downLogs = map[string]*downLog{
		"db":    {problem: "Restarting", state: "Restarting, exit code 1, restarted 45 times", lines: []string{"Error: no password", "see docs"}},
		"cache": {problem: "Exited", state: "Exited, exit code 3"},
	}

	got := plainLines(p.downSummaryLines(120))

	want := []string{
		"Compose finished with 2 containers down:",
		"db:",
		"state: Restarting, exit code 1, restarted 45 times",
		"log:   Error: no password",
		"see docs",
		"needs started by: web",
		"cache:",
		"state: Exited, exit code 3",
		"needs started by: web",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestDownSummaryIsEmptyWhenNothingWentDown(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.downLogs = map[string]*downLog{"db": {state: "Running", lines: []string{"fine"}}} // read, but not down
	if got := p.downSummaryLines(120); got != nil {
		t.Errorf("summary = %q, want none", plainLines(got))
	}
}

func TestDownSummaryWrapsLongLogsUnderTheirLabel(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.serviceIDs = []string{"db"}
	p.downLogs = map[string]*downLog{"db": {problem: "Dead", lines: []string{strings.Repeat("word ", 30)}}}

	got := plainLines(p.downSummaryLines(40))

	if len(got) != 5 || !strings.HasPrefix(got[2], "log:") || !strings.HasSuffix(got[3], "…") || !strings.HasPrefix(got[4], "needs started by") {
		t.Errorf("summary = %q, want the header, the name, two wrapped rows ending in an ellipsis, and who needs it", got)
	}
}

func TestDownSummaryPutsStateAndLogBeforeWhoNeedsTheContainer(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.serviceIDs = []string{"db"}
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting", lines: []string{"boom"}}}

	lines := p.downSummaryLines(120)

	if len(lines) != 5 {
		t.Fatalf("summary = %q, want 5 lines", plainLines(lines))
	}
	if !strings.Contains(semstyle.ToPlain(lines[2]), "state:") || !strings.Contains(semstyle.ToPlain(lines[4]), "needs started by:") {
		t.Errorf("summary = %q, want the state and log, then who needs it", plainLines(lines))
	}
	if state, needs := indentOf(lines[2]), indentOf(lines[4]); state != needs {
		t.Errorf("state is indented %d, needed-by %d, want them level: the state is the container's own", state, needs)
	}
}

func TestDownDependencyIsAWarningOnTheDependentsNeedsLine(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.On(containerEvent("db", api.StatusStarted, api.Done)) // compose reports it started
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting"}}
	p.finished = true

	line := p.dependsLines("web", 160, false)[0]

	if !strings.Contains(line, "{{|DockerStatusWarn|}}Crashed") || strings.Contains(line, "DockerStatusFail") || strings.Contains(line, "DockerDependsFailed") {
		t.Errorf("needs line = %q, want a yellow Crashed: DS2 inferred it from a snapshot", line)
	}

	// A failure compose itself reports stays red.
	p2 := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p2.finished = true // db never started
	if line := p2.dependsLines("web", 160, false)[0]; !strings.Contains(line, "{{|DockerStatusFail|}}Unmet") {
		t.Errorf("needs line = %q, want a red Unmet for a dependency that never started", line)
	}
}

func TestALineSplitsWhenItsNamesHaveDifferentStatuses(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true), dep("cache", types.ServiceConditionStarted, true))
	p.containerStarted["cache"] = true // met
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting"}} // went down
	p.finished = true

	lines := plainLines(p.dependsLines("web", 160, true))

	if len(lines) != 2 || !strings.Contains(lines[0], "Crashed") || !strings.Contains(lines[0], "needs started: db (Restarting)") ||
		!strings.Contains(lines[1], "Ready") || !strings.Contains(lines[1], "needs started: cache") {
		t.Errorf("lines = %q, want a Crashed line for db and a Ready line for cache", lines)
	}
	if lines := plainLines(p.dependsLines("web", 160, false)); len(lines) != 1 || !strings.Contains(lines[0], "db (Restarting)") {
		t.Errorf("settled: %q, want only the crashed line, the met name collapsing", lines)
	}
}

func TestAServiceNothingDependsOnIsFoundWhenItsContainerIsDown(t *testing.T) {
	p := newDependsTestProcessor() // web and db are the run's services, with no dependencies
	p.projectName = "proj"
	d := &logDaemon{
		service: "web",
		state:   container.State{Status: "restarting", Restarting: true, ExitCode: 1},
		tty:     true,
		logs:    []byte("Error: bad config\n"),
		items: []container.Summary{
			{ID: "1", State: "running", Labels: map[string]string{api.ServiceLabel: "db"}},
			{ID: "2", State: "restarting", Labels: map[string]string{api.ServiceLabel: "web"}},
		},
	}
	p.dockerClient = d
	p.finished = true

	p.checkServicesDown()

	if got := p.downLogs["web"]; got == nil || got.problem != "Restarting" || len(got.lines) != 1 {
		t.Errorf("web: %+v, want it found down with its log", got)
	}
	if p.downLogs["db"] != nil {
		t.Errorf("a running container was read: %+v", p.downLogs["db"])
	}
	if got := p.downServices(); len(got) != 1 || got[0] != "web" {
		t.Errorf("down services = %v, want only web, for the summary", got)
	}
}

func TestAContainerOutsideTheRunIsLeftAlone(t *testing.T) {
	p := newDependsTestProcessor() // the run is web and db; "elsewhere" is another service of the project
	p.projectName = "proj"
	p.dockerClient = &logDaemon{
		service: "web",
		state:   container.State{Status: "restarting", Restarting: true, ExitCode: 1},
		tty:     true,
		logs:    []byte("crash\n"),
		items: []container.Summary{
			{ID: "1", State: "restarting", Labels: map[string]string{api.ServiceLabel: "elsewhere"}},
			{ID: "2", State: "dead", Labels: map[string]string{api.ServiceLabel: "also-elsewhere"}},
		},
	}
	p.finished = true

	p.checkServicesDown()

	if len(p.downLogs) != 0 {
		t.Errorf("read containers outside the run: %+v, want the summary to list only what this run covers", p.downLogs)
	}
}

func TestAnExitedContainerOfTheRunIsFoundOnlyWhenItExitedNonZero(t *testing.T) {
	p := newDependsTestProcessor()
	p.projectName = "proj"
	d := &logDaemon{
		service: "db",
		state:   container.State{Status: "exited", ExitCode: 2},
		tty:     true,
		logs:    []byte("died\n"),
		items:   []container.Summary{{ID: "1", State: "exited", Labels: map[string]string{api.ServiceLabel: "db"}}},
	}
	p.dockerClient = d
	p.finished = true

	p.checkServicesDown()
	if p.downLogs["db"] == nil || p.downLogs["db"].problem != "Exited" {
		t.Errorf("db: %+v, want it found, exited non-zero in a service of the run", p.downLogs["db"])
	}

	d.state = container.State{Status: "exited", ExitCode: 0}
	p.checkServicesDown()
	if p.downLogs["db"] != nil {
		t.Errorf("db: %+v, want a clean exit left alone", p.downLogs["db"])
	}
}

func TestLogRowsAreNotTakenForErrorMessages(t *testing.T) {
	p := newDependsTestProcessor()
	p.downLogs = map[string]*downLog{"db": {problem: "Restarting", state: "Restarting", lines: []string{"first", "second", "third"}}}
	prefixes := p.errorLinePrefixes()

	for _, line := range p.serviceLogLines("db", 80) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				t.Errorf("line %q starts like an error message, so placeErrorLines would re-wrap it", line)
			}
		}
	}
}

func TestRepeatedLogLinesShowOnce(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	p.dockerClient = &logDaemon{state: container.State{Status: "restarting", Restarting: true, ExitCode: 1}, tty: true,
		logs: []byte("oops broken\noops broken\noops broken\n")}
	p.finished = true

	p.checkServicesDown()

	if got := p.downLogs["db"]; got == nil || len(got.lines) != 1 || got.lines[0] != "oops broken" {
		t.Errorf("lines = %+v, want the repeated line once", got)
	}
}

func TestWaitForStartedToSettleCountsFromTheLastStart(t *testing.T) {
	old := startSettle
	startSettle = 150 * time.Millisecond
	defer func() { startSettle = old }()

	p := newDependsTestProcessor()
	p.projectName = "proj"
	p.dockerClient = &logDaemon{}

	began := time.Now()
	p.waitForStartedToSettle() // nothing started: no wait
	if took := time.Since(began); took > 50*time.Millisecond {
		t.Errorf("waited %v with nothing started", took)
	}

	p.On(containerEvent("db", api.StatusStarted, api.Done))
	time.Sleep(100 * time.Millisecond) // other end-of-run work already used part of it
	began = time.Now()
	p.waitForStartedToSettle()
	if took := time.Since(began); took < 20*time.Millisecond || took > 120*time.Millisecond {
		t.Errorf("waited %v, want about the 50ms left of the settle", took)
	}
}

func TestOnlyAStartedContainerStartsTheSettleClock(t *testing.T) {
	p := newDependsTestProcessor()
	p.On(containerEvent("db", api.StatusRunning, api.Done)) // already running: nothing this run started
	if !p.lastStart.IsZero() {
		t.Error("a container that was already running started the settle clock")
	}
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	if p.lastStart.IsZero() {
		t.Error("a started container didn't start the settle clock")
	}
}

func TestADependencyOfAServiceOutsideTheRunIsLeftAlone(t *testing.T) {
	p := newDependsTestProcessor(dep("db", types.ServiceConditionStarted, true))
	p.projectName = "proj"
	p.dependsOn["elsewhere"] = []serviceDep{dep("never-started", types.ServiceConditionStarted, true)} // not a service of the run
	d := &logDaemon{service: "db", state: container.State{Status: "running", Running: true}, tty: true}
	p.dockerClient = d
	p.preRun["db"] = "running"
	p.finished = true

	p.checkServicesDown()

	if p.downLogs["never-started"] != nil || d.read != 0 {
		t.Errorf("read a dependency of a service outside the run: %+v", p.downLogs["never-started"])
	}
}

func TestAStateWithoutLogLinesStillGetsTheStatusWord(t *testing.T) {
	p := newDependsTestProcessor()
	p.downLogs = map[string]*downLog{"db": {problem: "Dead", lines: []string{"only a log"}}} // no state text
	got := plainLines(p.serviceLogLines("db", 160))
	if len(got) != 1 || !strings.Contains(got[0], "Crashed") || !strings.HasSuffix(got[0], "log:   only a log") {
		t.Errorf("lines = %q, want the status word on the first row, which is the log", got)
	}
}

func TestALiveLookCountsRestartingAndDeadButNotExited(t *testing.T) {
	newRun := func(state container.State, summary container.ContainerState) *consoleEventProcessor {
		p := newDependsTestProcessor()
		p.projectName = "proj"
		p.dockerClient = &logDaemon{service: "db", state: state, tty: true, logs: []byte("x\n"),
			items: []container.Summary{{ID: "1", State: summary, Labels: map[string]string{api.ServiceLabel: "db"}}}}
		return p
	}

	p := newRun(container.State{Status: "restarting", Restarting: true, ExitCode: 1}, "restarting")
	p.scanServicesDown(true)
	if p.downLogs["db"] == nil {
		t.Error("a live look missed a container restarting")
	}

	// An exited container may be waiting for compose to start it, so only the final look counts it.
	p = newRun(container.State{Status: "exited", ExitCode: 2}, "exited")
	p.scanServicesDown(true)
	if p.downLogs["db"] != nil {
		t.Errorf("a live look counted an exited container: %+v", p.downLogs["db"])
	}
	p.scanServicesDown(false)
	if p.downLogs["db"] == nil {
		t.Error("the final look missed an exited container")
	}
}

func TestTheCrashWatchShowsACrashLiveAndTheFinalCheckDropsItOnceRecovered(t *testing.T) {
	old := crashWatchEvery
	crashWatchEvery = 10 * time.Millisecond
	defer func() { crashWatchEvery = old }()

	p := newDependsTestProcessor()
	p.staticOut = false
	p.projectName = "proj"
	d := &logDaemon{service: "db", state: container.State{Status: "restarting", Restarting: true, ExitCode: 1}, tty: true, logs: []byte("boom\n"),
		items: []container.Summary{{ID: "1", State: "restarting", Labels: map[string]string{api.ServiceLabel: "db"}}}}
	p.dockerClient = d

	p.startCrashWatch(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p.mtx.Lock()
		found := p.downLogs["db"] != nil
		p.mtx.Unlock()
		if found {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.stopCrashWatch()
	p.stopCrashWatch() // safe to repeat
	if p.downLogs["db"] == nil {
		t.Fatal("the watch didn't show the crash while the run was going")
	}

	// The container came back up and stayed: the final check drops the lines.
	d.state = container.State{Status: "running", Running: true}
	d.items = []container.Summary{{ID: "1", State: "running", Labels: map[string]string{api.ServiceLabel: "db"}}}
	p.finished = true
	p.checkServicesDown()
	if p.downLogs["db"] != nil {
		t.Errorf("db still shown as down after it recovered: %+v", p.downLogs["db"])
	}
}
