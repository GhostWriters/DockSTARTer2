package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
)

// newStatusTestProcessor returns a status processor for db and web, both running.
func newStatusTestProcessor() *consoleEventProcessor {
	return newGroupsTestProcessor(statusCommand)
}

// Each service goes in the group of what status found: a problem first, then its
// container's state.
func TestStatusGroupWord(t *testing.T) {
	p := newStatusTestProcessor()
	for _, tc := range []struct {
		name  string
		setup func()
		want  string
	}{
		{"running", func() {}, api.StatusRunning},
		{"stopped", func() { p.preRun["db"] = "stopped" }, notRunningText},
		{"paused", func() { p.preRun["db"] = preRunPaused }, pausedText},
		{"no container", func() { delete(p.preRun, "db") }, notCreatedText},
		{"no container or image", func() { delete(p.preRun, "db"); delete(p.imageMetaBefore, dbImg) }, notPulledText},
		{"unhealthy", func() { p.health["db"] = map[string]string{"proj-db-1": healthUnhealthy} }, unhealthyText},
		{"restarting", func() { p.downLogs = map[string]*downLog{"db": {problem: restartingText}} }, restartingText},
		{"failed to start", func() {
			p.preRun["db"] = "stopped"
			p.downLogs = map[string]*downLog{"db": {problem: failedStartText}}
		}, failedStartText},
	} {
		p = newStatusTestProcessor()
		tc.setup()
		if got := p.statusGroupWord("db", dbImg); got != tc.want {
			t.Errorf("%s: statusGroupWord = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Nothing runs, so everything is under unchanged, folded by state, and a service with a
// problem shows its own block there, as a warning: status reports, it fails nothing.
func TestStatusShowsEverythingUnchanged(t *testing.T) {
	p := newStatusTestProcessor()
	p.preRun["web"] = "stopped"
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if strings.Contains(got, " changed ") || !strings.Contains(got, "unchanged ─") ||
		!strings.Contains(got, "✓ Running         1 service: db") || !strings.Contains(got, "· Not Running     1 service: web") ||
		!strings.HasPrefix(strings.TrimSpace(got), "✓ status:") {
		t.Errorf("display =\n%s\nwant a status heading, db Running and web Not Running under unchanged, nothing changed", got)
	}

	p.downLogs = map[string]*downLog{"web": {state: "Restarting, exit code 1", problem: restartingText, lines: []string{"boom"}}}
	got = renderPlain(p, 0)
	if !strings.Contains(got, "⚠ Restarting      web:") || !strings.Contains(got, "state: Restarting, exit code 1") ||
		!strings.Contains(got, "⚠ Warning     ─ unchanged") || !strings.HasPrefix(strings.TrimSpace(got), "⚠ status:") ||
		strings.Contains(got, "×") {
		t.Errorf("display =\n%s\nwant web's own Restarting block, its state line, the headings as warnings, no error", got)
	}
}

// A network that doesn't exist stays under unchanged as Not Created: status creates
// nothing, so it isn't one that failed to be. A stopped service has no dependency lines
// below -v, its unmet needs being what stopped services have.
func TestStatusLeavesNetworksAndStoppedServicesQuiet(t *testing.T) {
	p := newStatusTestProcessor()
	p.preRun = map[string]string{}
	p.dependsOn = map[string][]serviceDep{"web": {{name: "db", condition: "service_started", required: true}}}
	p.networks = resourceDecl{names: []string{"proj_default"}, exists: map[string]bool{}}
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if strings.Contains(got, " changed ") || !strings.Contains(got, "Not Created   1 network: proj_default") ||
		strings.Contains(got, "needs") || !strings.Contains(got, "Not Created   2 services: db, web") {
		t.Errorf("display =\n%s\nwant the network and both services Not Created under unchanged, no needs line", got)
	}
	if got := renderPlain(p, dockerlayout.VerboseDetails); !strings.Contains(got, "needs started: db") {
		t.Errorf("-v display =\n%s\nwant web's needs line", got)
	}
}

// A model is Existing when the runner has pulled its reference, Not Pulled when not,
// and Unknown, a warning, when the runner couldn't be asked.
func TestStatusModelWord(t *testing.T) {
	p := newStatusTestProcessor()
	p.models = map[string]string{"llm": "ai/smollm2:latest", "embed": "ai/embed:latest"}
	p.modelsOnDisk = map[string]bool{"ai/smollm2:latest": true}
	if got := p.statusModelWord("llm"); got != existingText {
		t.Errorf("pulled model = %q, want %q", got, existingText)
	}
	if got := p.statusModelWord("embed"); got != notPulledText {
		t.Errorf("model not pulled = %q, want %q", got, notPulledText)
	}
	p.modelsUnknown = true
	p.finished = true
	if got := p.statusModelWord("llm"); got != unknownText {
		t.Errorf("model with no runner = %q, want %q", got, unknownText)
	}
	if got := p.kindState(p.modelKind(), []string{"llm"}); got != rollupWarning {
		t.Errorf("models' state with no runner = %v, want a warning", got)
	}
}

// Status lists the jobs the compose files declare: the scheduled ones Scheduled, and
// with -v the manual ones too, as up does before it registers them.
func TestStatusListsDeclaredJobs(t *testing.T) {
	p := newStatusTestProcessor()
	p.jobs = map[string]jobDecl{"tick": {schedule: "* * * * *", scheduled: true}, "hello": {}}
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Scheduled     1 job: tick") || strings.Contains(got, "hello") || strings.Contains(got, " changed ") {
		t.Errorf("display =\n%s\nwant tick Scheduled under unchanged, the manual job left out", got)
	}
	got = renderPlain(p, dockerlayout.VerboseDetails)
	if !strings.Contains(got, "Manual          hello:") || !strings.Contains(got, "Scheduled       tick:") || !strings.Contains(got, "schedule: * * * * *") {
		t.Errorf("-v display =\n%s\nwant both jobs' rows and tick's schedule", got)
	}
}

// An image whose tag the registry now points at another digest has an update: its
// service shows its block, its image line Update and an update line naming the new
// version when a tag gives it, else the digest cut short. It is information, so a stack
// that is otherwise fine stays complete, and the header, which counts what the display
// covers, leaves it out (the recap after the display lists it).
func TestStatusShowsImageUpdates(t *testing.T) {
	p := newStatusTestProcessor()
	p.imageMetaBefore[webImg] = imageMeta{digest: "sha256:" + strings.Repeat("a", 64)}
	remote := "sha256:" + strings.Repeat("b", 64)
	p.imageUpdates = map[string]string{webImg: remote}
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if !strings.Contains(got, "✓ status: 2 services (2 images) ") || !strings.Contains(got, "✓ Running     ─ unchanged") ||
		!strings.Contains(got, "Running         web:") || !strings.Contains(got, "i Update            image: example/web:1") ||
		!strings.Contains(got, "update: sha256:bbbbbbbbbbbb (not pulled)") || !strings.Contains(got, "Running         1 service: db") {
		t.Errorf("display =\n%s\nwant web's block with its update, db folded, 1 update counted, still complete", got)
	}
	p.digestVersions[remote] = "1.2.3"
	if got := renderPlain(p, 0); !strings.Contains(got, "update: 1.2.3 (not pulled)") {
		t.Errorf("display =\n%s\nwant the update named by its version", got)
	}
	p.updateMeta = map[string]imageMeta{webImg: {built: "2026-11-26 18:02:11"}}
	if got := renderPlain(p, 0); !strings.Contains(got, "update: 1.2.3 (2026-11-26 18:02:11, not pulled)") {
		t.Errorf("display =\n%s\nwant the update's date with it", got)
	}
}

// The update recap lists services: those sharing an image on one line, then the image
// and the update under them; one whose registry couldn't be asked goes in the warning
// with its image and why.
func TestStatusUpdateRecapListsServices(t *testing.T) {
	p := NewConsoleEventProcessor(context.Background(), nil, statusCommand,
		map[string][]string{"lscr.io/x/radarr:latest": {"radarr__4k", "radarr"}, "docker.io/library/redis:8": {"redis"}},
		[]string{"docker.io/library/redis:8", "lscr.io/x/radarr:latest"}, map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	p.imageUpdates = map[string]string{"lscr.io/x/radarr:latest": "sha256:" + strings.Repeat("c", 64)}
	p.updateMeta = map[string]imageMeta{"lscr.io/x/radarr:latest": {version: "6.0.1", built: "2026-11-26 10:00:00"}}
	p.digestVersions["sha256:"+strings.Repeat("c", 64)] = "6.0.1"
	p.updateErrs = map[string]string{"docker.io/library/redis:8": "registry down"}
	updates, failed := p.updateSummaryLines()
	got, gotFailed := plainLines(updates), plainLines(failed)
	want := []string{"Found 2 services with updates (not pulled):", "radarr, radarr__4k:", "image:  lscr.io/x/radarr:latest", "update: 6.0.1 (2026-11-26 10:00:00)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("updates = %q, want %q", got, want)
	}
	wantFailed := []string{"Couldn't check 1 service for updates:", "redis:", "image:  docker.io/library/redis:8", "error:  registry down"}
	if strings.Join(gotFailed, "|") != strings.Join(wantFailed, "|") {
		t.Errorf("failed = %q, want %q", gotFailed, wantFailed)
	}
}

// While its checks run, status's headings are in progress, whatever its rows show.
func TestStatusRollupWhileChecking(t *testing.T) {
	p := newStatusTestProcessor()
	if got := p.statusRollup([]string{"db", "web"}, func(string) string { return "" }); got != rollupProcessing {
		t.Errorf("statusRollup before Done = %v, want processing", got)
	}
	p.finished = true
	if got := p.statusRollup([]string{"db", "web"}, func(string) string { return "" }); got != rollupComplete {
		t.Errorf("statusRollup after Done = %v, want complete", got)
	}
}

// Status selects the services asked for alone, and runs no docker command, so its
// heading names it alone.
func TestStatusSelectionAndHeading(t *testing.T) {
	if includesDependencies(statusCommand, DepsDefault) || includesDependents(statusCommand, DepsInclude) {
		t.Error("status pulls in other services, want the ones asked for alone")
	}
	if got := composeHeading(statusCommand); got != "status" {
		t.Errorf("composeHeading(status) = %q, want status", got)
	}
}

// A daemon error's container ID is dropped from the error line.
func TestContainerIDInError(t *testing.T) {
	in := "driver failed programming external connectivity on endpoint swag (" + strings.Repeat("ab", 32) + "): Bind for 0.0.0.0:80 failed"
	want := "driver failed programming external connectivity on endpoint swag: Bind for 0.0.0.0:80 failed"
	if got := containerIDInError.ReplaceAllString(in, ""); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
