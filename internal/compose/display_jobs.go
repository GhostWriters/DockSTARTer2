package compose

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
)

// jobEventPrefix starts the ID of compose's events for a job ("Job backup").
const jobEventPrefix = "Job "

// Job row statuses: compose's Creating, Created and Running for a job registered
// with the engine read as Scheduling, Scheduled and Unchanged, by what they mean for
// it: it now runs on its schedule, or already did and is the same as before.
const (
	jobSchedulingText = "Scheduling"
	jobScheduledText  = "Scheduled"
	jobUnchangedText  = "Unchanged"
	jobManualText     = "Manual"
)

// jobNotFoundText is compose's Done for a job down found nothing to remove
// (removeResource): every manual job, never registered, so its row is left out.
const jobNotFoundText = "Warning: No resource found to remove"

// isJobsRefusal reports whether a compose error is it refusing a project's scheduled
// jobs, the engine being too old or its jobs extension off (rejectUnsupportedScheduledJobs).
func isJobsRefusal(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "scheduled jobs ")
}

// jobsEngineHint says what an engine too old for jobs needs: compose's message for
// one names neither the version nor the extension (API 1.57, apiVersionJobs in compose).
const jobsEngineHint = "Jobs need Docker Engine 29.10 or later, with its jobs extension enabled " +
	"(start dockerd with {{|UsageCommand|}}--extension-enable org.mobyproject.jobs.v0{{[-]}})."

// reportJobsHint logs jobsEngineHint under a compose failure that is the engine
// being too old for jobs, wrapped to wrapW.
func reportJobsHint(ctx context.Context, err error, wrapW int) {
	if !strings.Contains(err.Error(), "not supported by this version of the engine") {
		return
	}
	for _, line := range wrapStyled(jobsEngineHint, wrapW) {
		logger.Error(ctx, "  %s", line)
	}
}

// jobDecl is what the compose project declares about a job: its schedule, shown
// under its row, and whether it has one, which is what up registers.
type jobDecl struct {
	schedule  string
	scheduled bool
}

// jobDecls returns the project's jobs, by name.
func jobDecls(project *types.Project) map[string]jobDecl {
	out := make(map[string]jobDecl, len(project.Jobs))
	for name, job := range project.Jobs {
		out[name] = jobDecl{schedule: scheduleText(job), scheduled: composev5.HasSchedule(job)}
	}
	return out
}

// scheduleText returns a job's schedule as its schedule line shows it: the cron
// expression, then its time zone when it sets one, or "" for a job with none.
func scheduleText(job types.JobConfig) string {
	if job.Triggers == nil || len(job.Triggers.Schedule) == 0 {
		return ""
	}
	var parts []string
	for _, s := range job.Triggers.Schedule {
		text := s.Cron
		if s.Timezone != "" {
			text += " (" + s.Timezone + ")"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", ")
}

// registersJobs reports whether command registers the project's scheduled jobs, so
// its display lists them from the start.
func registersJobs(command string) bool {
	switch command {
	case "up", "update", "create":
		return true
	}
	return false
}

// jobTaskID returns the task ID of a job's events, which keeps a job apart from a
// service of the same name.
func jobTaskID(name string) string {
	return jobEventPrefix + name
}

// upsertJob records an event for job name: its status, and an error's message.
func (p *consoleEventProcessor) upsertJob(name string, e api.Resource) {
	id := jobTaskID(name)
	t, exists := p.tasks[id]
	if !exists {
		t = &consoleTask{id: id, startTime: time.Now()}
		p.tasks[id] = t
		p.ids = append(p.ids, id)
		p.jobIDs = append(p.jobIDs, name)
	}
	switch {
	case e.Status == api.Error:
		t.message = e.Text
		if t.message == "" || t.message == api.StatusError {
			t.message = e.Details
		}
		t.message = stripControl(t.message)
		t.text = api.StatusError
	case e.Text != "":
		t.text = e.Text
	}
	t.status = e.Status
	t.markWork(e.Status)
	if t.completed() && t.endTime.IsZero() {
		t.endTime = time.Now()
	}
}

// jobRows returns the jobs the display lists, by name: those the command registers,
// at verbosity level the manual ones too, every job compose reported on but one it
// found nothing to remove for, and the job -c run runs.
func (p *consoleEventProcessor) jobRows(level int) []string {
	var names []string
	if registersJobs(p.command) {
		for name, d := range p.jobs {
			if d.scheduled || level >= dockerlayout.VerboseDetails {
				names = append(names, name)
			}
		}
	}
	for _, name := range p.jobIDs {
		if t := p.tasks[jobTaskID(name)]; t != nil && t.text == jobNotFoundText {
			continue
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if p.runJob != "" && !slices.Contains(names, p.runJob) {
		names = append(names, p.runJob)
	}
	slices.Sort(names)
	return names
}

// setRunError gives the processor the SDK call's error before Done draws the final
// frame: when it is compose refusing the scheduled jobs, each of them fails with it.
// Callers hold no lock.
func (p *consoleEventProcessor) setRunError(err error) {
	if !isJobsRefusal(err) {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.refused = true
	for name, d := range p.jobs {
		if d.scheduled && p.tasks[jobTaskID(name)] == nil {
			p.upsertJob(name, api.Resource{ID: jobTaskID(name), Status: api.Error, Text: err.Error()})
		}
	}
}

// jobsActedOn returns the jobs the command registers or removes, by name: the
// rows of the summary's job count and the section's rollup, without the manual
// jobs it leaves alone.
func (p *consoleEventProcessor) jobsActedOn() []string {
	var names []string
	for _, name := range p.jobRows(0) {
		if p.tasks[jobTaskID(name)] != nil || p.jobs[name].scheduled {
			names = append(names, name)
		}
	}
	return names
}

// jobStatus returns job name's row status: its last event's, Pending before it has
// one, Interrupted when the command ended without registering it, and Manual for a job
// with no schedule, which up leaves for -c run.
func (p *consoleEventProcessor) jobStatus(name string) (text, tag, icon string) {
	ic := p.icons()
	t := p.tasks[jobTaskID(name)]
	if t == nil {
		switch {
		case !p.jobs[name].scheduled:
			return jobManualText, "{{|DockerInfo|}}" + jobManualText + "{{[-]}}", "{{|DockerInfo|}}i{{[-]}}"
		case p.finished:
			// The run stopped at a failure before compose registered it, as the
			// services it never reached.
			return interruptedText, "{{|DockerStatusWarn|}}" + interruptedText + "{{[-]}}", "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
		default:
			return "Pending", "{{|DockerStatusPending|}}Pending{{[-]}}", "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
		}
	}
	text = t.text
	switch {
	case name == p.runJob:
		// -c run sets the row's status itself (setJobRunStatus).
		switch t.status {
		case api.Error:
			return text, "{{|DockerStatusFail|}}" + text + "{{[-]}}", "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
		case api.Warning:
			return text, "{{|DockerStatusWarn|}}" + text + "{{[-]}}", "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
		}
	case t.status == api.Error:
		return api.StatusError, "{{|DockerStatusFail|}}" + api.StatusError + "{{[-]}}", "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
	case text == api.StatusCreating:
		text = jobSchedulingText
	case text == api.StatusCreated:
		text = jobScheduledText
	case text == api.StatusRunning:
		text = jobUnchangedText
	}
	if t.completed() {
		return text, serviceFinalTag + text + "{{[-]}}", "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
	}
	return text, "{{|DockerStatusActive|}}" + text + "{{[-]}}", p.activeSpinnerTag(ic.spinner)
}

// jobRollup returns the jobs: section's state: an error when a job failed, a
// warning when one was cancelled or the command ended without registering it, done
// once every job is.
func (p *consoleEventProcessor) jobRollup() sectionRollupState {
	state := rollupComplete
	for _, name := range p.jobsActedOn() {
		t := p.tasks[jobTaskID(name)]
		switch {
		case t != nil && t.status == api.Error:
			return rollupError
		case t != nil && t.status == api.Warning, t == nil && p.finished:
			state = rollupWarning
		case (t == nil || !t.completed()) && state != rollupWarning:
			state = rollupProcessing
		}
	}
	return state
}

// buildJobLines returns the jobs: section, or nothing when the display lists no job:
// each job's row, its error, the image of the job -c run runs, and its schedule, at
// verbosity level always, below it only when the run registered it anew. Jobs
// already registered fold into one Unchanged line when the display folds them.
func (p *consoleEventProcessor) buildJobLines(termW, level int) ([]string, []timerEntry) {
	names := p.jobRows(level)
	if len(names) == 0 {
		return nil, nil
	}
	state := p.jobRollup()
	text, stTag, iconTag := sectionStatusText(state)
	icon := iconTag + p.sectionRollupIcon(state) + "{{[-]}}"
	if state == rollupProcessing {
		icon = p.activeSpinnerTag(p.icons().spinner)
	}
	var taskIDs []string
	for _, name := range p.jobsActedOn() {
		taskIDs = append(taskIDs, jobTaskID(name))
	}
	lines := []string{globalIndent + icon + " " + stTag + text + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text)) +
		"{{|DockerSection|}}jobs{{[-]}}{{|DockerColon|}}:{{[-]}}"}
	timers := []timerEntry{{task: p.sectionTaskFor(taskIDs), style: timerSection}}
	var unchanged []string // jobs folded into the section's Unchanged line
	for _, name := range names {
		text, tag, icon := p.jobStatus(name)
		t := p.tasks[jobTaskID(name)]
		if text == jobUnchangedText && p.foldsUnchanged(level) {
			unchanged = append(unchanged, name)
			continue
		}
		lines = append(lines, globalIndent+icon+" "+tag+strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text))+
			sectionChildIndent+dockerlayout.StyleServiceName(name)+"{{|DockerColon|}}:{{[-]}}")
		timers = append(timers, timerEntry{task: t, style: timerService})
		if line := p.errorLine(t); line != "" {
			lines = append(lines, line)
			timers = append(timers, timerEntry{})
		}
		if name == p.runJob && p.runJobImage != "" {
			img := p.runJobImage
			layers := p.layersForImage(img)
			lines = append(lines, p.buildImageLine(img, p.tasks[img], layers, utf8.RuneCountInString(img)+p.layerCountWidth(img), termW))
			timers = append(timers, timerEntry{task: p.tasks[img], style: timerImage})
		}
		if schedule := p.jobs[name].schedule; schedule != "" && (level >= dockerlayout.VerboseDetails || text == jobScheduledText) {
			lines = append(lines, p.scheduleLine(schedule, termW))
			timers = append(timers, timerEntry{})
		}
	}
	if len(unchanged) > 0 {
		for _, line := range p.foldLines(unchanged, "job", "jobs", termW) {
			lines = append(lines, line)
			timers = append(timers, timerEntry{})
		}
	}
	return lines, timers
}

// scheduleLine returns the line under a job's row showing its schedule.
func (p *consoleEventProcessor) scheduleLine(schedule string, termW int) string {
	const icon, word, label = "i", "Info", "schedule"
	prefix := globalIndent + "{{|DockerInfo|}}" + icon + "{{[-]}} {{|DockerInfo|}}" + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) +
		strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerInfo|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
	room := max(termW-timerReserveW-summaryHeaderIndentW-utf8.RuneCountInString(semstyle.ToPlain(prefix)), minDetailW)
	return prefix + "{{[::D]}}" + cutRunes(stripControl(schedule), room) + "{{[-]}}"
}
