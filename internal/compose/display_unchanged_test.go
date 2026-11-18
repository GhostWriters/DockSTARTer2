package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
)

// renderPlain returns the processor's lines, plain, at level.
func renderPlain(p *consoleEventProcessor, level int) string {
	return strings.Join(plainLines(p.buildLines(160, level)), "\n")
}

func TestUnchangedServicesFoldOnUp(t *testing.T) {
	p := newHealthTestProcessor() // up; db and web share testImg
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	got := renderPlain(p, 0)
	if !strings.Contains(got, unchangedText) || !strings.Contains(got, "2 services: db, web") {
		t.Errorf("display =\n%s\nwant db and web folded into one Unchanged line", got)
	}
	if strings.Contains(got, "image:") {
		t.Errorf("display =\n%s\nwant their image folded with them", got)
	}
	// -v lists every row.
	if got := renderPlain(p, dockerlayout.VerboseDetails); strings.Contains(got, unchangedText+"   ") || !strings.Contains(got, "db:") {
		t.Errorf("-v display =\n%s\nwant every row", got)
	}
}

func TestAChangedServiceKeepsItsImageBlock(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", "Recreated", api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	got := renderPlain(p, 0)
	if strings.Contains(got, unchangedText+"   ") || !strings.Contains(got, "web:") || !strings.Contains(got, "image:") {
		t.Errorf("display =\n%s\nwant the block shown whole, web with it, since db changed", got)
	}
}

func TestUnhealthyServiceIsntFolded(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.setHealth("web", "web", healthUnhealthy)
	if got := renderPlain(p, 0); strings.Contains(got, unchangedText+"   ") {
		t.Errorf("display =\n%s\nwant nothing folded while web is unhealthy", got)
	}
}

func TestUpdateFoldsOnlyOnceTheImageCheckEnds(t *testing.T) {
	p := newHealthTestProcessor()
	p.command = "update"
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	if got := renderPlain(p, 0); strings.Contains(got, unchangedText+"   ") {
		t.Errorf("display =\n%s\nwant nothing folded before update's image check reports", got)
	}
	p.finished, p.succeeded = true, true // the check reported nothing to pull
	if got := renderPlain(p, 0); !strings.Contains(got, "2 services: db, web") {
		t.Errorf("display =\n%s\nwant db and web folded once update ended", got)
	}
}

func TestAFailureShowsEveryRow(t *testing.T) {
	p := newHealthTestProcessor()
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	if !p.foldsUnchanged(0) {
		t.Fatal("nothing failed, yet the rows don't fold")
	}
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "port is already allocated"})
	if p.foldsUnchanged(0) {
		t.Error("a row failed, yet the rows still fold")
	}
	q := newHealthTestProcessor()
	q.finished, q.succeeded = true, false // failed with no row of its own, e.g. a pull refused early
	if q.foldsUnchanged(0) {
		t.Error("the run failed, yet the rows still fold")
	}
	// A pull still going when the run fails ends Interrupted, not stuck Pulling.
	r := newHealthTestProcessor()
	r.On(api.Resource{ID: testImg, Status: api.Working, Text: api.StatusPulling})
	r.succeeded = false
	r.finalizeActiveTasks()
	if img := r.tasks[testImg]; img.text != interruptedText || img.status != api.Warning {
		t.Errorf("image = %q %v, want Interrupted", img.text, img.status)
	}
	// The rows the failed run never reached show Interrupted, not Pending.
	if got := renderPlain(q, 0); !strings.Contains(got, "Interrupted   db:") || strings.Contains(got, "Pending") {
		t.Errorf("display =\n%s\nwant the unreached rows Interrupted", got)
	}
}

func TestTeardownDoesntFold(t *testing.T) {
	p := newHealthTestProcessor()
	p.command = "down"
	if p.foldsUnchanged(0) {
		t.Error("down folds rows; every row it lists changed")
	}
}

func TestUnchangedJobsFold(t *testing.T) {
	p := newHealthTestProcessor()
	p.jobs = jobDecls(&types.Project{Jobs: types.Jobs{"backup": scheduledJob("0 3 * * *", "")}})
	p.On(api.Resource{ID: "Job backup", Status: api.Done, Text: api.StatusRunning}) // already registered
	lines, _ := p.buildJobLines(160, 0)
	if got := strings.Join(plainLines(lines), "\n"); !strings.Contains(got, unchangedText) || !strings.Contains(got, "1 job: backup") {
		t.Errorf("jobs =\n%s\nwant backup folded", got)
	}
	lines, _ = p.buildJobLines(160, dockerlayout.VerboseDetails)
	if got := strings.Join(plainLines(lines), "\n"); !strings.Contains(got, "backup:") {
		t.Errorf("-v jobs =\n%s\nwant backup's own row", got)
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

func TestFoldLinesWrapAllNames(t *testing.T) {
	p := newHealthTestProcessor()
	var names []string
	for i := range 30 {
		names = append(names, fmt.Sprintf("service%02d", i))
	}
	lines := p.foldLines(names, "service", "services", 100)
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
