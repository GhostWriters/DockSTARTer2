package compose

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GhostWriters/semstyle"
	"github.com/buger/goterm"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/version"
)

// Job run statuses, shown on the job's row while -c run runs it: how it ended uses
// the engine's names for a run's end. A failure's exit code goes without brackets,
// so that up to three digits fit the status column.
const (
	jobStartingText  = "Starting"
	jobRunningText   = "Running"
	jobSucceededText = "Succeeded"
	jobFailedText    = "Failed"
	jobCancelledText = "Cancelled"
	jobTimedOutText  = "Timed Out"
)

// jobRun is the job -c run runs: its name, image and configuration.
type jobRun struct {
	name  string
	image string
	job   types.JobConfig
}

// prepareJobRun returns project with job name materialized as a service, as
// compose's run command does (cmd/compose/run.go materializeManualJob): narrowed to
// the job and its dependencies, with every job it depends on materialized too.
func prepareJobRun(project *types.Project, name string) (*types.Project, *jobRun, error) {
	all := project.AllJobs()
	job, ok := all[name]
	if !ok {
		if len(all) == 0 {
			return nil, nil, errors.New("the compose files declare no jobs")
		}
		return nil, nil, fmt.Errorf("no such job in the compose files, which declare %s", strings.Join(slices.Sorted(maps.Keys(all)), ", "))
	}
	if composev5.ManualTriggerDisabled(job) {
		return nil, nil, composev5.ManualTriggerDisabledErr(name)
	}
	project, err := project.WithSelectedJob(name)
	if err != nil {
		return nil, nil, err
	}
	if err := materializeJobDeps(project, project.AllJobs(), job, map[string]bool{name: true}); err != nil {
		return nil, nil, err
	}
	project.Services[name] = composev5.JobAsService(project, name, job)
	return project, &jobRun{name: name, image: job.Image, job: job}, nil
}

// materializeJobDeps adds the jobs that job depends on, directly or through another,
// to project.Services, as compose's materializeJobClosure does: a job declared with
// manual: false can't run, even as another's dependency.
func materializeJobDeps(project *types.Project, jobs types.Jobs, job types.JobConfig, seen map[string]bool) error {
	for dep := range job.DependsOn {
		if seen[dep] {
			continue
		}
		seen[dep] = true
		depJob, isJob := jobs[dep]
		if !isJob {
			continue
		}
		if composev5.ManualTriggerDisabled(depJob) {
			return fmt.Errorf("job %q is declared with manual: false, it can't run even as a dependency of another job", dep)
		}
		if err := materializeJobDeps(project, jobs, depJob, seen); err != nil {
			return err
		}
		project.Services[dep] = composev5.JobAsService(project, dep, depJob)
	}
	return nil
}

// lazyDisplay passes compose's events to the display, starting it on the first
// one: RunJob has no Start (it opens no operation), and an engine without jobs
// fails it before any event, leaving the one-off fallback a display not yet shown.
type lazyDisplay struct {
	p       *consoleEventProcessor
	ctx     context.Context
	started bool
	oneOff  bool // the job runs as a one-off container, which compose starts after its Done
	mtx     sync.Mutex
}

func (l *lazyDisplay) ensureStarted() {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if !l.started {
		l.started = true
		l.p.Start(l.ctx, "run")
	}
}

// Start starts the display, once.
func (l *lazyDisplay) Start(context.Context, string) { l.ensureStarted() }

// On starts the display if it isn't yet, and passes events on.
func (l *lazyDisplay) On(events ...api.Resource) {
	l.ensureStarted()
	l.p.On(events...)
}

// Done is compose's end of its one-off preparation, on an engine without jobs: the
// container starts next, so the display goes on (finish ends it).
func (l *lazyDisplay) Done(_ string, success bool) {
	l.ensureStarted()
	if l.oneOff && success {
		l.p.setJobRunStatus(l.p.runJob, jobRunningText, api.Working, "")
	}
}

// finish ends the display once the job has ended, its output included.
func (l *lazyDisplay) finish(success bool) {
	l.ensureStarted()
	l.p.mtx.Lock()
	finished := l.p.finished
	if l.p.jobOutPartial != "" {
		l.p.jobOutLines = append(l.p.jobOutLines, cleanOutputLine(l.p.jobOutPartial))
		l.p.jobOutPartial = ""
	}
	l.p.mtx.Unlock()
	if !finished {
		l.p.Done("run", success)
	}
}

// jobOutput takes the job's output for the display, which shows it under the job.
type jobOutput struct {
	display *lazyDisplay
	name    string
	once    sync.Once
	wrote   bool
}

func (o *jobOutput) Write(b []byte) (int, error) {
	o.once.Do(func() {
		o.wrote = true
		o.display.ensureStarted()
		o.display.p.setJobRunStatus(o.name, jobRunningText, api.Working, "")
	})
	o.display.p.addJobOutput(string(b))
	return len(b), nil
}

// addJobOutput adds output of the run job, split into lines. Callers hold no lock.
func (p *consoleEventProcessor) addJobOutput(s string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	lines := strings.Split(p.jobOutPartial+s, "\n")
	p.jobOutPartial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		p.jobOutLines = append(p.jobOutLines, cleanOutputLine(line))
	}
}

// sgrRegex matches a colour or text-style escape sequence, the only escape
// sequence the job's output keeps; escRegex matches any.
var (
	sgrRegex = regexp.MustCompile(`^\x1b\[[0-9;:]*m$`)
	escRegex = regexp.MustCompile(`\x1b(?:\[[0-9;:?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|.?)`)
)

// cleanOutputLine returns a line of the job's output safe for the display: only
// its colours as escape sequences (anything else could move the cursor in the
// live display), what follows its last carriage return (as a terminal shows a
// progress line), braces that semstyle can't read as a tag, and a colour reset at
// its end.
func cleanOutputLine(line string) string {
	if i := strings.LastIndex(strings.TrimSuffix(line, "\r"), "\r"); i >= 0 {
		line = line[i+1:]
	}
	var b strings.Builder
	coloured := false
	last := 0
	for _, m := range escRegex.FindAllStringIndex(line, -1) {
		b.WriteString(stripControl(line[last:m[0]]))
		if seq := line[m[0]:m[1]]; sgrRegex.MatchString(seq) {
			b.WriteString(seq)
			coloured = true
		}
		last = m[1]
	}
	b.WriteString(stripControl(line[last:]))
	out := strings.ReplaceAll(strings.ReplaceAll(b.String(), "\n", ""), "{{", "{\u2060{")
	if coloured {
		out += "\x1b[0m"
	}
	return out
}

// jobOutputLines returns the run job's output under a header, or nothing before it
// has any. While the job runs, only its last lines show: those that fit under the
// display's displayLines. The log gets them without their colours.
func (p *consoleEventProcessor) jobOutputLines(displayLines int) []string {
	if p.runJob == "" || len(p.jobOutLines) == 0 && p.jobOutPartial == "" {
		return nil
	}
	lines := p.jobOutLines
	if !p.finished {
		if p.jobOutPartial != "" {
			lines = append(slices.Clip(lines), cleanOutputLine(p.jobOutPartial))
		}
		height := goterm.Height()
		if height <= 0 || p.noViewport {
			height = 24
		}
		if keep := max(height-displayLines-2, 5); len(lines) > keep {
			lines = lines[len(lines)-keep:]
		}
	}
	out := []string{"{{|DockerSection|}}──{{[-]}} " + dockerlayout.StyleServiceName(p.runJob) + " {{|DockerSection|}}output ──{{[-]}}"}
	for _, line := range lines {
		if p.forLog {
			line = semstyle.StripANSI(line)
		}
		out = append(out, line)
	}
	return out
}

// setJobRunStatus sets the row of the job -c run runs. Callers hold no lock.
func (p *consoleEventProcessor) setJobRunStatus(name, text string, status api.EventStatus, message string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	id := jobTaskID(name)
	t := p.tasks[id]
	if t == nil {
		t = &consoleTask{id: id, startTime: time.Now()}
		p.tasks[id] = t
		p.ids = append(p.ids, id)
		p.jobIDs = append(p.jobIDs, name)
	}
	t.text, t.status, t.message = text, status, message
	t.markWork(status)
	if t.completed() && t.endTime.IsZero() {
		t.endTime = time.Now()
	}
}

// jobEnd returns how a job run ended, from RunJob's exit code and error, and
// whether its output started: its row's status, the row's message, and the error
// -c run returns.
func jobEnd(name string, code int, err error, started bool) (text string, status api.EventStatus, message string, runErr error) {
	switch {
	case code == 130:
		return jobCancelledText, api.Warning, "", fmt.Errorf("job %q was cancelled", name)
	case code == 124:
		return jobTimedOutText, api.Error, "", fmt.Errorf("job %q timed out", name)
	case code != 0:
		// The row's status has the exit code; only an error of compose's adds to it.
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		return fmt.Sprintf("%s %d", jobFailedText, code), api.Error, msg, fmt.Errorf("job %q exited with code %d", name, code)
	case err != nil && started:
		return api.StatusError, api.Error, shortMessage(err.Error()), err
	case err != nil:
		return notStartedText, api.Error, shortMessage(err.Error()), err
	}
	return jobSucceededText, api.Done, "", nil
}

// runJobCommand runs job with srv: the display shows its dependencies starting,
// then its output under it, and keeps going until the job ends. On an engine without
// jobs it runs as a one-off container, as compose's run does, except inside the
// TUI: compose's one-off run resets the process's signal handling, which the TUI
// needs.
func runJobCommand(ctx, sdkCtx context.Context, srv api.Compose, project *types.Project, job *jobRun, display *lazyDisplay, out *jobOutput, inTUI bool) error {
	p := display.p
	p.setJobRunStatus(job.name, jobStartingText, api.Working, "")
	opts := api.RunOptions{Service: job.name}
	code, err := srv.RunJob(sdkCtx, project, job.name, opts)
	if errors.Is(err, api.ErrJobsNotSupported) && !display.started {
		if inTUI {
			logger.Error(ctx, "Can't run job {{|App|}}%s{{[-]}}: %v", job.name, err)
			if !strings.Contains(err.Error(), "--extension-enable") {
				logger.Error(ctx, "%s", jobsEngineHint)
			}
			return reportedError{err}
		}
		logger.Warn(ctx, "Running job {{|App|}}%s{{[-]}} as a one-off container, since %v.", job.name, err)
		if !strings.Contains(err.Error(), "--extension-enable") {
			logger.Warn(ctx, "%s", jobsEngineHint) // compose names the flag only for an engine new enough
		}
		display.oneOff = true
		if composev5.HasSchedule(job.job) {
			// The one-off publishes none of a scheduled job's ports, as compose's run does.
			svc := project.Services[job.name]
			svc.Ports = nil
			project.Services[job.name] = svc
		}
		opts.AutoRemove = true
		code, err = srv.RunOneOffContainer(sdkCtx, project, opts)
	}
	if sdkCtx.Err() != nil {
		// Ctrl+C: compose cancels the run on the engine, and its error is only the
		// context's, whatever exit code the run had reached.
		code = 130
	}
	err = runRefusal(err)
	// The display shows even a run refused before compose did anything, so that a
	// refusal looks the same whether or not compose created something first.
	text, status, message, runErr := jobEnd(job.name, code, err, out.wrote)
	p.setJobRunStatus(job.name, text, status, message)
	switch {
	case errors.As(runErr, new(notScheduledError)):
		p.setJobRunSummary(job.name, notScheduledError{}.styled())
	// The row's status says these; the summary still needs a message.
	case code == 124:
		p.setJobRunSummary(job.name, "timed out")
	case code != 0 && code != 130 && message == "":
		p.setJobRunSummary(job.name, fmt.Sprintf("exited with code %d", code))
	}
	display.finish(runErr == nil)
	if runErr == nil {
		return nil
	}
	if status == api.Warning {
		logger.Warn(ctx, "Job {{|App|}}%s{{[-]}} was cancelled.", job.name)
		return reportedError{runErr}
	}
	return reportFailures(ctx, p, runErr)
}

// runRefusal returns RunJob's error, except that for a scheduled job not registered
// yet it is a notScheduledError, which names DS2's command for registering it where
// compose's names its own (docker compose up).
func runRefusal(err error) error {
	if err != nil && strings.Contains(err.Error(), "is not registered with the engine") {
		return notScheduledError{}
	}
	return err
}

// notScheduledError is a scheduled job's run refused for not being registered yet.
// Error is plain text, for the row under the job; styled is the failure summary's,
// the command styled word by word so that wrapping between words splits no tag.
type notScheduledError struct{}

func (notScheduledError) Error() string {
	return fmt.Sprintf("Not scheduled yet. Run '%s -c up' first.", version.CommandName)
}

func (notScheduledError) styled() string {
	words := strings.Fields(version.CommandName + " -c up")
	for i, w := range words {
		words[i] = "{{|UserCommand|}}" + w + "{{[-]}}"
	}
	return "Not scheduled yet. Run '" + strings.Join(words, " ") + "' first."
}

// setJobRunSummary sets the failure summary's message for the run job, where it
// differs from the one under its row. Callers hold no lock.
func (p *consoleEventProcessor) setJobRunSummary(name, summary string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if t := p.tasks[jobTaskID(name)]; t != nil {
		t.summary = summary
	}
}

// checkJob reports whether the compose files as they are declare job name, so
// -c run can refuse an unknown job before asking; nil when they can't be read yet.
func checkJob(ctx context.Context, name string) error {
	project, err := loadComposeProject(ctx)
	if err != nil {
		return nil
	}
	_, _, err = prepareJobRun(project, name)
	return err
}
