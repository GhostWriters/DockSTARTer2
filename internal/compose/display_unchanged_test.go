package compose

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
)

// Two images, one service each, so one can change while the other stays in its group.
const (
	dbImg  = "example/db:1"
	webImg = "example/web:1"
)

// newGroupsTestProcessor returns a processor for db and web, both running before the
// run, their images on disk.
func newGroupsTestProcessor(command string) *consoleEventProcessor {
	p := NewConsoleEventProcessor(context.Background(), nil, command,
		map[string][]string{dbImg: {"db"}, webImg: {"web"}}, []string{dbImg, webImg},
		map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	p.preRun = map[string]string{"db": "running", "web": "running"}
	p.imageMetaBefore[dbImg] = imageMeta{}
	p.imageMetaBefore[webImg] = imageMeta{}
	return p
}

// renderPlain returns the processor's lines, plain, at level.
func renderPlain(p *consoleEventProcessor, level int) string {
	return strings.Join(plainLines(p.buildLines(160, level)), "\n")
}

func TestServicesStartInTheGroupOfTheirState(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.preRun = map[string]string{"db": "running", "web": "stopped"}
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Not Running   1 service: web") || !strings.Contains(got, "Running       1 service: db") || strings.Contains(got, "db:") {
		t.Errorf("display =\n%s\nwant web in Not Running and db in Running, without rows", got)
	}
	if strings.Index(got, "Not Running") > strings.Index(got, "Running") {
		t.Errorf("display =\n%s\nwant Not Running before Running", got)
	}
	delete(p.preRun, "web") // no container yet
	if got := renderPlain(p, 0); !strings.Contains(got, "Not Created   1 service: web") {
		t.Errorf("display =\n%s\nwant web in Not Created", got)
	}
	delete(p.imageMetaBefore, webImg) // nor its image: pulling it comes first
	if got := renderPlain(p, 0); !strings.Contains(got, "Not Pulled    1 service: web") {
		t.Errorf("display =\n%s\nwant web in Not Pulled", got)
	}
}

func TestPullStartsByWhatsOnDisk(t *testing.T) {
	p := newGroupsTestProcessor("pull")
	delete(p.imageMetaBefore, webImg) // web's image isn't on disk
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Not Pulled    1 service: web") || !strings.Contains(got, "Cached        1 service: db") {
		t.Errorf("display =\n%s\nwant web in Not Pulled and db in Cached", got)
	}
}

func TestUntouchedServicesSettleInTheRunningGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Running       2 services: db, web") || strings.Contains(got, "image:") {
		t.Errorf("display =\n%s\nwant db and web in the Running group, their images with them", got)
	}
}

func TestAChangedServiceLeavesItsGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", "Recreated", api.Done))
	got := renderPlain(p, 0)
	// db has its own row, first; web stays in Running, the group's count down to 1.
	if !strings.Contains(got, "Recreated       db:") || !strings.Contains(got, "Running       1 service: web") {
		t.Errorf("display =\n%s\nwant db's own row and only web in Running", got)
	}
	if strings.Index(got, "db:") > strings.Index(got, "1 service: web") {
		t.Errorf("display =\n%s\nwant the changed row before the groups", got)
	}
}

func TestAnImageCheckAloneDoesntChangeAService(t *testing.T) {
	p := newGroupsTestProcessor("update")
	p.On(api.Resource{ID: dbImg, Status: api.Working, Text: api.StatusPulling}) // update checks every image
	if got := renderPlain(p, 0); !strings.Contains(got, "2 services: db, web") {
		t.Errorf("display =\n%s\nwant db still pending while its image is only checked", got)
	}
	p.On(api.Resource{ID: "layer1", ParentID: dbImg, Status: api.Working, Text: api.StatusDownloading})
	if got := renderPlain(p, 0); !strings.Contains(got, "db:") || !strings.Contains(got, "1 service: web") {
		t.Errorf("display =\n%s\nwant db's own row once a layer downloads", got)
	}
}

func TestUnhealthyServiceGetsItsOwnRow(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.setHealth("web", "web", healthUnhealthy)
	if got := renderPlain(p, 0); !strings.Contains(got, "web:") || !strings.Contains(got, "1 service: db") {
		t.Errorf("display =\n%s\nwant web's own row and db in the Running group", got)
	}
}

func TestVerboseNestsRowsUnderHeadings(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	got := renderPlain(p, dockerlayout.VerboseDetails)
	changed, unchanged := strings.Index(got, "Complete    changed:"), strings.Index(got, "Complete    unchanged:")
	if changed < 0 || unchanged < changed || strings.Count(got, "Complete      1 service:") != 2 {
		t.Fatalf("-v display =\n%s\nwant changed:, then unchanged:, each with a 1 service: heading, all Complete", got)
	}
	// Each kind heading's rows sit nestW further in than its count, the status column
	// staying put: "Recreated" and "Running" start where the headings' status does.
	if db := strings.Index(got, "Recreated       db:"); db < changed || db > unchanged {
		t.Errorf("-v display =\n%s\nwant db nested under changed:", got)
	}
	if web := strings.Index(got, "Running         web:"); web < unchanged {
		t.Errorf("-v display =\n%s\nwant web nested under unchanged:", got)
	}
}

func TestNetworksGroupLikeServices(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.networks = resourceDecl{names: []string{"proj_default", "proj_backend"}, exists: map[string]bool{"proj_default": true}}
	got := renderPlain(p, 0)
	// Before the run reaches them: one exists, one isn't created yet; both unchanged so far.
	if strings.Contains(got, " changed:") || !strings.Contains(got, "Not Created   1 network: proj_backend") || !strings.Contains(got, "Existing      1 network: proj_default") {
		t.Errorf("display =\n%s\nwant proj_backend Not Created and proj_default Existing, both unchanged", got)
	}
	// compose creates proj_backend: it gets its own row under changed:.
	p.On(api.Resource{ID: "Network proj_backend", Status: api.Done, Text: api.StatusCreated})
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.finished, p.succeeded = true, true
	got = renderPlain(p, 0)
	changed, unchanged := strings.Index(got, "Complete    changed:"), strings.Index(got, "Complete    unchanged:")
	if changed < 0 || unchanged < changed || !strings.Contains(got, "Complete      1 network:") || !strings.Contains(got, "Created         proj_backend:") || !strings.Contains(got, "Existing      1 network: proj_default") {
		t.Errorf("display =\n%s\nwant proj_backend Created under changed:, proj_default still Existing under unchanged:", got)
	}
}

func TestChangedHoldsEachKindThatChanged(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.networks = resourceDecl{names: []string{"proj_default"}, exists: map[string]bool{}}
	p.On(api.Resource{ID: "Network proj_default", Status: api.Done, Text: api.StatusCreated})
	p.On(containerEvent("db", api.StatusCreated, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	changed, svcs, nets, unchanged := strings.Index(got, "changed:"), strings.Index(got, "1 service:\n"),
		strings.Index(got, "1 network:"), strings.Index(got, "unchanged:")
	if strings.Count(got, " changed:") != 1 || changed > svcs || svcs > nets || nets > unchanged ||
		!strings.Contains(got, "Running       1 service: web") || strings.Contains(got, "services:") {
		t.Errorf("display =\n%s\nwant one changed: holding 1 service: then 1 network:, web under unchanged:, no section headings", got)
	}
}

func TestCombineStates(t *testing.T) {
	for _, tc := range []struct {
		in   []sectionRollupState
		want sectionRollupState
	}{
		{[]sectionRollupState{rollupComplete, rollupComplete}, rollupComplete},
		{[]sectionRollupState{rollupPending, rollupPending}, rollupPending},
		{[]sectionRollupState{rollupPending, rollupComplete}, rollupProcessing},
		{[]sectionRollupState{rollupComplete, rollupWarning, rollupProcessing}, rollupWarning},
		{[]sectionRollupState{rollupWarning, rollupError}, rollupError},
	} {
		if got := combineStates(tc.in); got != tc.want {
			t.Errorf("combineStates(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHealthNoResultIsAWarning(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	p.setHealth("db", "db", healthStarting)
	if p.healthNoResult("db") {
		t.Fatal("no result while the run still waits for the first check")
	}
	p.healthWaitDone = true // the wait ran out with no result
	if icon := semstyle.ToPlain(p.serviceIcon(p.tasks["db"], api.Done)); icon != p.icons().warn {
		t.Errorf("db's icon = %q, want a warning", icon)
	}
	if state := p.rollupState([]string{"db"}, nil); state != rollupWarning {
		t.Errorf("rollup = %v, want a warning", state)
	}
}

func TestNestLineKeepsTheStatusColumn(t *testing.T) {
	pad := strings.Repeat(" ", sectionStatusW-len("Running"))
	line := globalIndent + "{{|DockerMarkerDone|}}✓{{[-]}} {{|App|}}Running{{[-]}}" + pad + sectionChildIndent + "{{|App|}}db{{[-]}}:"
	got := strings.Join(plainLines([]string{nestLine(line)}), "")
	if want := "✓ Running" + pad + sectionChildIndent + strings.Repeat(" ", nestW) + "db:"; got != want {
		t.Errorf("nestLine = %q, want %q", got, want)
	}
}

func TestPullGroupsCachedImages(t *testing.T) {
	p := newGroupsTestProcessor("pull")
	p.finished, p.succeeded = true, true // both checks found nothing to pull
	if got := renderPlain(p, 0); !strings.Contains(got, "Cached        2 services: db, web") {
		t.Errorf("display =\n%s\nwant db and web in the Cached group", got)
	}
}

func TestAFailureShowsEveryRow(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	if !p.groupsRows() {
		t.Fatal("nothing failed, yet the rows aren't grouped")
	}
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "port is already allocated"})
	if p.groupsRows() {
		t.Error("a row failed, yet the rows are still grouped")
	}
	q := newGroupsTestProcessor("up")
	q.finished, q.succeeded = true, false // failed with no row of its own, e.g. a pull that failed first
	if q.groupsRows() {
		t.Error("the run failed, yet the rows are still grouped")
	}
	// A pull still going when the run fails ends Interrupted, not stuck Pulling.
	r := newGroupsTestProcessor("up")
	r.On(api.Resource{ID: dbImg, Status: api.Working, Text: api.StatusPulling})
	r.succeeded = false
	r.finalizeActiveTasks()
	if img := r.tasks[dbImg]; img.text != interruptedText || img.status != api.Warning {
		t.Errorf("image = %q %v, want Interrupted", img.text, img.status)
	}
	// The rows the failed run never reached show Interrupted, not Pending.
	if got := renderPlain(q, 0); !strings.Contains(got, "Interrupted   db:") || strings.Contains(got, "Pending") {
		t.Errorf("display =\n%s\nwant the unreached rows Interrupted", got)
	}
}

func TestTeardownGroups(t *testing.T) {
	// down: db removed under changed:; web, with no container, Not Created under unchanged:.
	p := newGroupsTestProcessor("down")
	delete(p.preRun, "web")
	if got := renderPlain(p, 0); !strings.Contains(got, "Running       1 service: db") || !strings.Contains(got, "Not Created   1 service: web") || strings.Contains(got, " changed:") {
		t.Errorf("down display before any event =\n%s\nwant db Running (to be removed) and web Not Created, nothing changed yet", got)
	}
	p.On(containerEvent("db", api.StatusRemoved, api.Done))
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	changed, unchanged := strings.Index(got, "Complete    changed:"), strings.Index(got, "Complete    unchanged:")
	if changed < 0 || unchanged < changed || !strings.Contains(got, "Removed         db:") || !strings.Contains(got, "Not Created   1 service: web") || strings.Contains(got, "services:") {
		t.Errorf("down display =\n%s\nwant db Removed under changed:, web Not Created under unchanged:, no services: section", got)
	}

	// stop: web, already stopped, stays unchanged though compose reports stopping it.
	q := newGroupsTestProcessor("stop")
	q.preRun["web"] = "stopped"
	q.On(containerEvent("db", api.StatusStopped, api.Done))
	q.On(containerEvent("web", api.StatusStopped, api.Done))
	q.finished, q.succeeded = true, true
	if got := renderPlain(q, 0); !strings.Contains(got, "Stopped         db:") || !strings.Contains(got, "Not Running   1 service: web") {
		t.Errorf("stop display =\n%s\nwant db Stopped under changed: and web in the Not Running group", got)
	}

	// restart: web, not running before, is started rather than restarted.
	r := newGroupsTestProcessor("restart")
	r.preRun["web"] = "stopped"
	r.On(containerEvent("db", api.StatusRestarted, api.Done))
	r.On(containerEvent("web", api.StatusRestarted, api.Done))
	r.finished, r.succeeded = true, true
	if got := renderPlain(r, 0); !strings.Contains(got, "Restarted       db:") || !strings.Contains(got, "Started         web:") {
		t.Errorf("restart display =\n%s\nwant db Restarted and web Started", got)
	}

	// A failed teardown shows every row under the services: section.
	q.On(api.Resource{ID: "Container db", Status: api.Error, Text: "permission denied"})
	if got := renderPlain(q, 0); !strings.Contains(got, "services:") || strings.Contains(got, "changed:") {
		t.Errorf("failed stop display =\n%s\nwant the services: section, ungrouped", got)
	}
}

func TestScheduledJobsGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.jobs = jobDecls(&types.Project{Jobs: types.Jobs{
		"backup": scheduledJob("0 3 * * *", ""),
		"report": scheduledJob("0 4 * * *", ""),
	}})
	p.On(api.Resource{ID: "Job backup", Status: api.Done, Text: api.StatusRunning}) // registered by an earlier up
	p.On(api.Resource{ID: "Job report", Status: api.Done, Text: api.StatusCreated}) // registered now
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Scheduled       report:") || !strings.Contains(got, "Scheduled     1 job: backup") {
		t.Errorf("jobs =\n%s\nwant report's own row and backup in the Scheduled group", got)
	}
}

func TestSummaryLine(t *testing.T) {
	p := newHealthTestProcessor() // 2 services sharing 1 image
	p.projectNetworks = 1
	p.jobs = jobDecls(&types.Project{Jobs: types.Jobs{"backup": scheduledJob("* * * * *", "")}})
	got := strings.Join(plainLines([]string{p.buildSummaryLine()}), "")
	if want := "up: 2 services (1 image), 1 network, 1 job"; !strings.HasSuffix(got, want) {
		t.Errorf("summary = %q, want it to end %q", got, want)
	}
	p.command = "down" // a teardown counts the networks it removed, none here
	if got := strings.Join(plainLines([]string{p.buildSummaryLine()}), ""); strings.Contains(got, "network") {
		t.Errorf("down summary = %q, want no network it didn't remove", got)
	}
}

func TestGroupLinesWrapAllNames(t *testing.T) {
	p := newHealthTestProcessor()
	var names []string
	for i := range 30 {
		names = append(names, fmt.Sprintf("service%02d", i))
	}
	lines := p.groupLines("Running", false, names, "service", "services", 100)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the names wrapped", plainLines(lines))
	}
	// Columns in runes: the first line's ✓ is one column but three bytes.
	column := func(s, sub string) int { return len([]rune(s[:strings.Index(s, sub)])) }
	first := semstyle.ToPlain(lines[0])
	start := column(first, "service00")
	all := ""
	for i, l := range lines {
		plain := semstyle.ToPlain(l)
		if w := len([]rune(plain)); w > 100 {
			t.Errorf("line %q is %d wide, more than 100", plain, w)
		}
		if i > 0 && column(plain, "service") != start {
			t.Errorf("line %q doesn't line up under the first name", plain)
		}
		all += plain
	}
	if !strings.Contains(first, "30 services:") || !strings.Contains(all, "service29") || strings.Contains(all, "…") {
		t.Errorf("lines = %q, want the count and every name", plainLines(lines))
	}
}
