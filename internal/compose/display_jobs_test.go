package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
)

func scheduledJob(cron, tz string) types.JobConfig {
	return types.JobConfig{Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: cron, Timezone: tz}}}}
}

func manualJob() types.JobConfig {
	manual := true
	return types.JobConfig{Triggers: &types.TriggerConfig{Manual: &manual}}
}

func newJobsTestProcessor(command string) *consoleEventProcessor {
	p := newHealthTestProcessor()
	p.command = command
	p.jobs = jobDecls(&types.Project{Jobs: types.Jobs{
		"backup": scheduledJob("0 3 * * *", "Europe/London"),
		"repair": manualJob(),
	}})
	return p
}

// jobLines returns the jobs' lines, plain: grouped, under changed and unchanged
// on their own, else the jobs: section.
func jobLines(p *consoleEventProcessor, level int) []string {
	if p.groupsRows() {
		lines, timers := p.groupedLines([]kindGroups{p.jobGroups(160, level)}, 160)
		return plainLines(renderGrouped(p, lines, timers))
	}
	lines, _ := p.buildJobLines(160, level)
	return plainLines(lines)
}

func TestScheduleText(t *testing.T) {
	if got := scheduleText(scheduledJob("0 3 * * *", "Europe/London")); got != "0 3 * * * (Europe/London)" {
		t.Errorf("scheduleText = %q", got)
	}
	if got := scheduleText(manualJob()); got != "" {
		t.Errorf("manual job's scheduleText = %q, want none", got)
	}
}

func TestJobRowsOnUp(t *testing.T) {
	p := newJobsTestProcessor("up")
	// Below -v only the scheduled job, which up registers, is listed: under unchanged,
	// in the Pending group.
	if lines := jobLines(p, 0); len(lines) != 2 || !strings.Contains(lines[0], "unchanged ─") ||
		!strings.Contains(lines[1], "Pending") || !strings.Contains(lines[1], "1 job: backup") {
		t.Fatalf("lines = %q, want unchanged, and backup Pending on the line standing in for its 1 job: heading", lines)
	}
	// -v adds the manual job.
	if lines := jobLines(p, dockerlayout.VerboseDetails); !strings.Contains(strings.Join(lines, "\n"), "Manual") {
		t.Errorf("lines = %q, want repair as Manual with -v", lines)
	}
	// Scheduled anew: under changed, the schedule shows without -v.
	p.On(api.Resource{ID: "Job backup", Status: api.Done, Text: api.StatusCreated})
	lines := jobLines(p, 0)
	if len(lines) != 3 || !strings.Contains(lines[0], "changed ─") || !strings.Contains(lines[1], jobScheduledText) || !strings.Contains(lines[1], "1 job: backup") || !strings.Contains(lines[2], "schedule: 0 3 * * * (Europe/London)") {
		t.Errorf("lines = %q, want backup Scheduled with its schedule under changed", lines)
	}
	// Already registered: in the Scheduled group, its schedule only with -v.
	p.On(api.Resource{ID: "Job backup", Status: api.Done, Text: api.StatusRunning})
	if lines := jobLines(p, 0); len(lines) != 2 || !strings.Contains(lines[1], jobScheduledText) || !strings.Contains(lines[1], "1 job: backup") {
		t.Errorf("lines = %q, want backup in the Scheduled group, on the line standing in for its 1 job: heading, without its schedule", lines)
	}
	// -v: one heading for the unchanged jobs, backup (with its schedule) and the manual
	// repair nested under it, the heading Complete.
	lines = jobLines(p, dockerlayout.VerboseDetails)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Complete       2 jobs:") || strings.Contains(joined, " changed ─") ||
		!strings.Contains(joined, "    backup:") || !strings.Contains(joined, "    repair:") || !strings.Contains(joined, "schedule:") {
		t.Errorf("-v lines = %q, want backup and repair nested under one Complete 2 jobs: heading, no changed", lines)
	}
}

func TestJobEventsDontBecomeServiceRows(t *testing.T) {
	p := newJobsTestProcessor("up")
	p.On(api.Resource{ID: "Job backup", Status: api.Working, Text: api.StatusCreating})
	if len(p.serviceIDs) != 0 || len(p.unknownContainers) != 0 {
		t.Errorf("serviceIDs = %q, unknown = %v: a job event made a service row", p.serviceIDs, p.unknownContainers)
	}
	if lines := jobLines(p, 0); len(lines) != 2 || !strings.Contains(lines[1], jobSchedulingText) || !strings.Contains(lines[1], "1 job: backup") {
		t.Errorf("lines = %q, want backup's own row, Scheduling", lines)
	}
}

func TestJobsOnDownComeFromEvents(t *testing.T) {
	p := newJobsTestProcessor("down")
	if lines := jobLines(p, 0); len(lines) != 0 {
		t.Fatalf("lines = %q, want no jobs: section before compose removes one", lines)
	}
	p.On(api.Resource{ID: "Job backup", Status: api.Done, Text: api.StatusRemoved})
	// A manual job is never registered: down finds nothing to remove for it.
	p.On(api.Resource{ID: "Job repair", Status: api.Done, Text: jobNotFoundText})
	if lines := jobLines(p, 0); len(lines) != 2 || !strings.Contains(lines[0], "changed ─") || !strings.Contains(lines[1], api.StatusRemoved) {
		t.Errorf("lines = %q, want only backup Removed", lines)
	}
	if n := len(p.jobsActedOn()); n != 1 {
		t.Errorf("jobsActedOn = %d, want 1: a job with nothing removed isn't counted", n)
	}
}

func TestJobsRefusalFailsTheJobsNotTheServices(t *testing.T) {
	p := newJobsTestProcessor("up")
	p.setRunError(errors.New("scheduled jobs are not supported by this version of the engine: backup"))
	p.finished, p.succeeded = true, false // what Done sets
	lines := jobLines(p, 0)
	if !strings.Contains(lines[0], "Error") || !strings.Contains(lines[1], "Error") {
		t.Errorf("lines = %q, want the jobs section and backup failed", lines)
	}
	if text, _ := p.impliedStatus(); text != notStartedText {
		t.Errorf("services' implied status = %q, want %q", text, notStartedText)
	}
	if fails := p.failures(); len(fails) != 1 || fails[0].name != "job backup" {
		t.Errorf("failures = %+v, want backup's", fails)
	}
}

func TestChangedJobName(t *testing.T) {
	err := errors.New("job \"ds2-tick\" has changed: run `docker compose down` to remove it, then `up` again")
	if name, ok := changedJobName(err); !ok || name != "ds2-tick" {
		t.Errorf("changedJobName = %q, %v; want ds2-tick", name, ok)
	}
	for _, other := range []error{nil, errors.New("port is already allocated"), errors.New(`job "x" not found`)} {
		if name, ok := changedJobName(other); ok {
			t.Errorf("changedJobName(%v) = %q, want no match", other, name)
		}
	}
}

func TestChangedJobFailsItsRow(t *testing.T) {
	p := newJobsTestProcessor("up")
	p.On(api.Resource{ID: "Job backup", Status: api.Working, Text: api.StatusCreating})
	p.setRunError(errors.New("job \"backup\" has changed: run `docker compose down` to remove it, then `up` again"))
	p.finished, p.succeeded = true, false // what Done sets
	got := strings.Join(jobLines(p, 0), "\n")
	if !strings.Contains(got, "× Error") || !strings.Contains(got, jobChangedText) {
		t.Errorf("lines =\n%s\nwant backup failed as registered with different settings", got)
	}
	if fails := p.failures(); len(fails) != 1 || fails[0].name != "job backup" {
		t.Errorf("failures = %+v, want backup's, not the services'", fails)
	}
}

func TestInterruptedJobIsAWarning(t *testing.T) {
	p := newJobsTestProcessor("up")
	p.On(api.Resource{ID: "Job backup", Status: api.Working, Text: api.StatusCreating})
	p.setRunError(errors.New("port is already allocated"))
	p.finished, p.succeeded = true, false
	p.finalizeActiveTasks()
	if text, _, icon := p.jobStatus("backup"); text != interruptedText || !strings.Contains(icon, "DockerMarkerWarn") {
		t.Errorf("jobStatus = %q, %q; want Interrupted with the warning mark", text, icon)
	}
}

func TestOtherErrorsLeaveJobsInterrupted(t *testing.T) {
	p := newJobsTestProcessor("up")
	p.setRunError(errors.New("port is already allocated"))
	p.finished, p.succeeded = true, false // what Done sets
	if p.refused {
		t.Fatal("a non-jobs error marked the run refused")
	}
	if lines := jobLines(p, 0); !strings.Contains(strings.Join(lines, "\n"), interruptedText+"    1 job: backup") {
		t.Errorf("lines = %q, want backup folded Interrupted", lines)
	}
}

func TestPrepareJobRun(t *testing.T) {
	disabled := false
	project := &types.Project{
		Name:     "proj",
		Services: types.Services{"db": {Name: "db"}},
		Jobs: types.Jobs{
			"repair": {Name: "repair", Triggers: &types.TriggerConfig{Manual: new(bool)}, ContainerSpec: types.ContainerSpec{Image: "alpine:3.20"}},
			"cron":   {Name: "cron", Triggers: &types.TriggerConfig{Manual: &disabled, Schedule: []types.ScheduleConfig{{Cron: "* * * * *"}}}},
		},
	}
	*project.Jobs["repair"].Triggers.Manual = true
	p, job, err := prepareJobRun(project, "repair")
	if err != nil {
		t.Fatal(err)
	}
	if job.name != "repair" || job.image != "alpine:3.20" {
		t.Errorf("job = %+v", job)
	}
	if svc, ok := p.Services["repair"]; !ok || svc.CustomLabels[api.ProjectLabel] != "proj" {
		t.Errorf("services = %v, want repair materialized with the project label", p.Services)
	}
	if _, _, err := prepareJobRun(project, "cron"); err == nil || !strings.Contains(err.Error(), "manual: false") {
		t.Errorf("err = %v, want manual: false refused", err)
	}
	if _, _, err := prepareJobRun(project, "nope"); err == nil || !strings.Contains(err.Error(), "which declare cron, repair") {
		t.Errorf("err = %v, want the declared jobs listed", err)
	}
}

func TestJobEnd(t *testing.T) {
	for _, tc := range []struct {
		code   int
		err    error
		text   string
		status api.EventStatus
		fails  bool
	}{
		{0, nil, jobSucceededText, api.Done, false},
		{3, nil, "Failed 3", api.Error, true},
		{255, nil, "Failed 255", api.Error, true},
		{130, errors.New("cancelled"), jobCancelledText, api.Warning, true},
		{124, errors.New("timed out"), jobTimedOutText, api.Error, true},
		{0, errors.New("dependency failed to start"), notStartedText, api.Error, true},
	} {
		text, status, _, err := jobEnd("repair", tc.code, tc.err, false)
		if text != tc.text || status != tc.status || (err != nil) != tc.fails {
			t.Errorf("jobEnd(%d, %v) = %q, %v, %v", tc.code, tc.err, text, status, err)
		}
		if w := len(text); w > sectionStatusTextW {
			t.Errorf("%q is %d wide, more than the status column", text, w)
		}
	}
}

func TestRunRefusal(t *testing.T) {
	notRegistered := errors.New(`job "backup" is not registered with the engine: run ` + "`docker compose up`" + ` (or ` + "`create`" + `) first`)
	refusal := runRefusal(notRegistered)
	got := refusal.Error()
	if !strings.Contains(got, "-c up") || strings.Contains(got, "docker compose") || strings.Contains(got, "{{") {
		t.Errorf("runRefusal = %q, want DS2's command, not compose's, as plain text", got)
	}
	if styled := (notScheduledError{}).styled(); !strings.Contains(styled, "{{|UserCommand|}}") || semstyle.ToPlain(styled) != got {
		t.Errorf("styled = %q, want the same text with the command styled", styled)
	}
	for _, line := range wrapStyled((notScheduledError{}).styled(), 25) {
		if strings.Count(line, "{{|UserCommand|}}") != strings.Count(line, "{{[-]}}") {
			t.Errorf("wrapped line %q opens a style it doesn't close", line)
		}
	}
	other := errors.New("some other error")
	if got := runRefusal(other); got != other {
		t.Errorf("runRefusal = %v, want compose's error unchanged", got)
	}
	if runRefusal(nil) != nil {
		t.Error("runRefusal(nil) isn't nil")
	}
}

func TestCleanOutputLine(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain text", "plain text"},
		{"\x1b[31mred\x1b[0m", "\x1b[31mred\x1b[0m\x1b[0m"},    // colours stay, reset added
		{"\x1b[2J\x1b[Hcleared", "cleared"},                    // cursor and screen moves go
		{"10%\r50%\r100%", "100%"},                             // a progress line shows its last state
		{"done\r", "done"},                                     // a CRLF line ending
		{"{{|Error|}}not a tag", "{\u2060{|Error|}}not a tag"}, // braces semstyle can't read
		{"bell\x07 here", "bell here"},
	} {
		if got := cleanOutputLine(tc.in); got != tc.want {
			t.Errorf("cleanOutputLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJobOutputLines(t *testing.T) {
	p := newJobsTestProcessor("run")
	p.runJob = "repair"
	if lines := p.jobOutputLines(5); lines != nil {
		t.Fatalf("lines = %q, want none before any output", lines)
	}
	p.addJobOutput("one\ntw")
	p.addJobOutput("o\nthree")
	got := plainLines(p.jobOutputLines(5))
	if want := []string{"── repair output ──", "one", "two", "three"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("live lines = %q, want %q (the unfinished line too)", got, want)
	}
	p.asciiMode = true
	if got := plainLines(p.jobOutputLines(5))[0]; got != "-- repair output --" {
		t.Errorf("header without line characters = %q, want %q", got, "-- repair output --")
	}
	p.asciiMode = false
	// While running, only the last lines that fit show; once finished, all of them.
	for range 100 {
		p.addJobOutput("more\n")
	}
	if n := len(p.jobOutputLines(5)); n >= 100 {
		t.Errorf("live output shows %d lines, want only the last screenful", n)
	}
	p.finished = true
	if n := len(p.jobOutputLines(5)); n != 103 { // the header, one, two, "three" joined by the first "more", and 99 more
		t.Errorf("finished output shows %d lines, want all 103", n)
	}
}

func TestRunJobRowShowsItsOwnStatus(t *testing.T) {
	p := newJobsTestProcessor("run")
	p.runJob = "repair"
	p.setJobRunStatus("repair", jobRunningText, api.Working, "")
	lines := jobLines(p, 0)
	if len(lines) != 2 || !strings.Contains(lines[0], "changed ─") || !strings.Contains(lines[1], jobRunningText) || !strings.Contains(lines[1], "1 job: repair") {
		t.Errorf("lines = %q, want repair's own row, Running, under changed", lines)
	}
	if got := semstyle.ToPlain(strings.Join(lines, "")); strings.Contains(got, "backup") {
		t.Errorf("lines = %q: run listed a job it doesn't run", lines)
	}
}
