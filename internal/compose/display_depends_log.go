package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

const (
	depLogTimeout = 5 * time.Second // bounds each dependency's container lookup, inspect and log read
	depLogTail    = "20"            // log lines read from the end of a container's output
	depLogReadMax = 64 << 10        // bytes read from them
	depLogRows    = 2               // rows each line of state or log may take
)

// downLog is what the display shows for a service whose container went wrong: its
// state, and the few log lines picked and redacted from its output (display_logredact.go).
type downLog struct {
	state   string
	problem string // set when its container ended up restarting, dead, exited non-zero or failed to start: shown after its name
	err     string // the daemon's reason it stopped or couldn't start, shown on its error line
	lines   []string
}

// containerIDInError matches a container or endpoint ID in parentheses in a daemon
// error, which only takes room on the error line.
var containerIDInError = regexp.MustCompile(` ?\([0-9a-f]{64}\)`)

// downErrRows is how many rows a down container's error line wraps to before it is cut:
// one more than a log line, a daemon error carrying its cause at the end.
const downErrRows = depLogRows + 1

// depLogProblem reports whether a failed dependency's problem leaves a container worth
// looking at; a skipped or interrupted dependency never ran.
func depLogProblem(problem string) bool {
	switch problem {
	case "Not Started", "Failed", "Unhealthy", "Not Completed":
		return true
	}
	return false
}

// crashWatchEvery is how often the live display looks for containers restarting.
var crashWatchEvery = 2 * time.Second

// checkServicesDown is the end-of-run look at the run's containers (scanServicesDown).
func (p *consoleEventProcessor) checkServicesDown() { p.scanServicesDown(false) }

// startCrashWatch looks for containers restarting every crashWatchEvery until
// stopCrashWatch, so the state and log of one that crashes show while the run is going;
// the end-of-run check drops them for any that is no longer down. Redirected output has
// no live display, so it only gets the final check.
func (p *consoleEventProcessor) startCrashWatch(ctx context.Context) {
	if p.staticOut || p.dockerClient == nil || p.projectName == "" || !commandWatchesHealth(p.command) || p.crashStop != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.crashStop = func() {
		cancel()
		<-done
	}
	go func() {
		defer close(done)
		t := time.NewTicker(crashWatchEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				p.scanServicesDown(true)
			}
		}
	}()
}

// stopCrashWatch ends the watch, waiting for a look in progress.
func (p *consoleEventProcessor) stopCrashWatch() {
	if p.crashStop != nil {
		p.crashStop()
		p.crashStop = nil
	}
}

// scanServicesDown reads the state and last log lines of each container of the run's
// services (the ones asked for and their dependencies) that is down, and, once the run is
// over, of each dependency that failed its condition, for the lines under its row and in
// the summary. Compose reports a container started even when it crashes straight away.
// Live (while the run is going) only a container restarting or dead counts: one exited
// may be waiting for compose to start it. At the end an exit non-zero counts too. A
// container down but outside the run is left to the runs that include it. Status counts
// an exit non-zero at every look and a container that failed to start, and doesn't
// look at stopped dependencies.
func (p *consoleEventProcessor) scanServicesDown(live bool) {
	if p.dockerClient == nil || p.projectName == "" || !commandWatchesHealth(p.command) {
		return
	}
	if p.command == statusCommand {
		live = false // nothing is going to start an exited container
	}
	ctx, cancel := context.WithTimeout(context.Background(), depLogTimeout)
	f := dockerclient.Filters{}
	f.Add("label", api.ProjectLabel+"="+p.projectName)
	list, err := p.dockerClient.ContainerList(ctx, dockerclient.ContainerListOptions{All: true, Filters: f})
	cancel()

	p.mtx.Lock()
	var names []string
	force := map[string]bool{} // service → it failed its condition, so its log is read whatever its state
	add := func(name string, failed bool) {
		if !contains(names, name) {
			names = append(names, name)
		}
		force[name] = force[name] || failed
	}
	for svc, deps := range p.dependsOn { // all the project's, so only those of the run's services count
		if live || !p.isKnownServiceName(svc) || p.command == statusCommand { // to status a failed condition is a stopped service
			continue
		}
		for _, d := range deps {
			if state, problem := p.depConditionStatus(d); state == depFailed && depLogProblem(problem) { // not depStatus: a service found down earlier is looked at again
				add(d.name, true)
			}
		}
	}
	if err == nil {
		for _, ctr := range list.Items {
			svc := ctr.Labels[api.ServiceLabel]
			if svc == "" || !p.isKnownServiceName(svc) {
				continue
			}
			switch {
			case ctr.State == container.StateRestarting, ctr.State == container.StateDead:
				add(svc, false)
			case ctr.State == container.StateExited && !live: // an exit of 0 is dropped when read
				add(svc, false)
			case ctr.State == container.StateCreated && p.command == statusCommand: // one that failed to start is kept when read
				add(svc, false)
			}
		}
	}
	for name := range p.downLogs {
		if !contains(names, name) {
			delete(p.downLogs, name) // it came back up since the last look
		}
	}
	p.mtx.Unlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := p.readServiceLog(name, force[name])
			p.mtx.Lock()
			defer p.mtx.Unlock()
			switch {
			case info != nil && p.downLogs == nil:
				p.downLogs = map[string]*downLog{name: info}
			case info != nil:
				p.downLogs[name] = info
			default:
				delete(p.downLogs, name)
			}
		}()
	}
	wg.Wait()
}

// startSettle is how long after the run last started a container the check for crashes
// waits. A container that exits at once is between restarts, or running for a moment,
// for the first fraction of a second, and Docker's pause between restarts lengthens; a
// few seconds also catch one that dies when it can't reach what it needs.
var startSettle = 5 * time.Second

// waitForStartedToSettle waits until startSettle after the run last started a container,
// when a check for crashes is going to run, so one that dies at once shows as down. Time
// already spent on the end-of-run health wait counts. The display keeps updating.
func (p *consoleEventProcessor) waitForStartedToSettle() {
	if p.dockerClient == nil || p.projectName == "" || !commandWatchesHealth(p.command) {
		return
	}
	p.mtx.Lock()
	last := p.lastStart
	p.mtx.Unlock()
	if last.IsZero() {
		return
	}
	if wait := startSettle - time.Since(last); wait > 0 {
		time.Sleep(wait)
	}
}

// settleDependencies marks the end-of-run checks done, so the dependency lines collapse
// to the ones with a problem (expandDependencyLines).
func (p *consoleEventProcessor) settleDependencies() {
	p.mtx.Lock()
	p.depsSettled = true
	p.mtx.Unlock()
}

// expandDependencyLines reports whether the needs, wants and needed-by lines show even
// when met, until the run and its end-of-run checks are over: a dependency that started
// can still go down, and the lines say what is being waited on. Callers hold p.mtx.
func (p *consoleEventProcessor) expandDependencyLines() bool {
	return !p.depsSettled
}

// readServiceLog returns svc's container state, the daemon's error and picked log lines,
// or nil when it has no container, the daemon doesn't answer, or it is fine and wasn't
// failed (force).
func (p *consoleEventProcessor) readServiceLog(svc string, force bool) *downLog {
	ctx, cancel := context.WithTimeout(context.Background(), depLogTimeout)
	defer cancel()
	f := dockerclient.Filters{}
	f.Add("label", api.ProjectLabel+"="+p.projectName)
	f.Add("label", api.ServiceLabel+"="+svc)
	list, err := p.dockerClient.ContainerList(ctx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if err != nil || len(list.Items) == 0 {
		return nil
	}
	res, err := p.dockerClient.ContainerInspect(ctx, list.Items[0].ID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return nil
	}
	c := res.Container
	info := &downLog{}
	if st := c.State; st != nil {
		status := string(st.Status)
		parts := []string{strings.ToUpper(status[:min(1, len(status))]) + status[min(1, len(status)):]}
		if st.OOMKilled {
			parts = append(parts, "out of memory")
		}
		if st.ExitCode != 0 || status == "exited" || status == "dead" {
			parts = append(parts, fmt.Sprintf("exit code %d", st.ExitCode))
		}
		switch {
		case c.RestartCount == 1:
			parts = append(parts, "restarted once")
		case c.RestartCount > 1:
			parts = append(parts, fmt.Sprintf("restarted %d times", c.RestartCount))
		}
		info.state = strings.Join(parts, ", ")
		info.err = cleanLogLine(containerIDInError.ReplaceAllString(st.Error, "")) // e.g. a port already allocated
		switch {
		case st.Restarting || status == "restarting":
			info.problem = restartingText
		case status == "dead":
			info.problem = deadText
		case status == "exited" && st.ExitCode != 0:
			info.problem = exitedText
		case status == "created" && st.ExitCode != 0 && strings.Contains(st.Error, "context canceled"):
			info.problem = notStartedText // its start was cancelled when another service failed
		case status == "created" && st.ExitCode != 0:
			info.problem = failedStartText // it never got going
		}
	}
	if !force && info.problem == "" {
		return nil
	}

	rc, err := p.dockerClient.ContainerLogs(ctx, list.Items[0].ID, dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: depLogTail})
	if err != nil {
		return info
	}
	defer rc.Close()
	var raw bytes.Buffer
	src := io.LimitReader(rc, depLogReadMax)
	if c.Config != nil && c.Config.Tty {
		_, _ = io.Copy(&raw, src)
	} else {
		_, _ = stdcopy.StdCopy(&raw, &raw, src)
	}
	var lines []string
	for _, l := range strings.Split(raw.String(), "\n") {
		if l = cleanLogLine(l); l != "" {
			lines = append(lines, l)
		}
	}
	info.lines = pickLogLines(slices.Compact(lines)) // a crash loop repeats its last line
	return info
}

// serviceLogLines returns the state, error and log lines under svc's row when its
// container went down or couldn't start, at the row's detail level, led by Crashed, or
// by Start Failed or Not Started for one that never started. The state takes the colour
// its row's icon does (the error colour when the row itself failed or, outside status, a
// service requires it, the cause marker, else the warning colour), and the error and
// log, as messages, the warning colour. Each line of text wraps to two rows (an error
// line, downErrRows), then is cut.
func (p *consoleEventProcessor) serviceLogLines(svc string, termW int) []string {
	d := p.downLogs[svc]
	if d == nil || (d.state == "" && len(d.lines) == 0) {
		return nil
	}
	// As on a ports line: the fact the icon stands for follows it, the message is in the
	// warning colour, and a row that failed itself is red throughout.
	errored := false
	if t := p.tasks[svc]; t != nil && t.status == api.Error && !t.healthErr {
		errored = true
	}
	stateTag, logTag := "{{|DockerMarkerWarn|}}", "{{|DockerStatusWarn|}}"
	if errored || p.dependencyCause(svc) && p.command != statusCommand {
		stateTag = "{{|DockerMarkerError|}}"
	}
	if errored {
		logTag = stateTag
	}
	// The block's first row carries the icon and status word in the left columns, as the
	// healthcheck and needs lines do: the cause marker when a service requires it.
	ic := p.icons()
	icon, statusTag := "{{|DockerMarkerWarn|}}"+ic.warn+"{{[-]}}", "{{|DockerStatusWarn|}}"
	switch {
	case errored:
		icon, statusTag = "{{|DockerMarkerError|}}"+ic.error+"{{[-]}}", "{{|DockerStatusFail|}}"
	case p.dependencyCause(svc):
		icon = p.causeMarker()
	}
	word := "Crashed"
	if d.problem == failedStartText || d.problem == notStartedText {
		word = d.problem // it never started, so it didn't crash
	}
	leadW := utf8.RuneCountInString(semstyle.ToPlain(globalIndent+icon+" ")) + sectionStatusW
	lead := globalIndent + icon + " " + statusTag + word + "{{[-]}}" + strutil.Repeat(" ", sectionStatusW-len(word))
	indent := leadW + 2*sectionChildIndentW
	const labelW = len("state")
	room := max(termW-timerReserveW-summaryHeaderIndentW-indent-labelW-2, minDetailW)
	started := false // the block's first row has been drawn
	row := func(label, text, tag string, first bool) string {
		prefix := strutil.Repeat(" ", indent+labelW+2)
		if first {
			left := strutil.Repeat(" ", leadW)
			if !started {
				left, started = lead, true
			}
			prefix = left + strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} " +
				strutil.Repeat(" ", labelW-len(label))
		}
		// The reset keeps a continuation row from matching errorLinePrefixes, which
		// placeErrorLines would take for a message and re-wrap.
		return prefix + "{{[-]}}" + styleLogText(text, tag)
	}
	var lines []string
	for i, text := range wrapWords(d.state, room, depLogRows) {
		lines = append(lines, row("state", text, stateTag, i == 0))
	}
	for i, text := range wrapWords(d.err, room, downErrRows) {
		lines = append(lines, row("error", text, logTag, i == 0))
	}
	first := true
	for _, l := range d.lines {
		for _, text := range wrapWords(l, room, depLogRows) {
			lines = append(lines, row("log", text, logTag, first))
			first = false
		}
	}
	return lines
}

// serviceDown reports whether svc's container ended up restarting, dead or exited
// non-zero.
func (p *consoleEventProcessor) serviceDown(svc string) bool {
	d := p.downLogs[svc]
	return d != nil && d.problem != ""
}

// dependencyCause reports whether svc is down (serviceDown) and a service requires
// it: the cause of that service's unmet condition, as opposed to a failure of its own.
func (p *consoleEventProcessor) dependencyCause(svc string) bool {
	return p.serviceDown(svc) && slices.ContainsFunc(p.neededBy(svc), func(d serviceDep) bool { return d.required })
}

// styleLogText colours text with tag, its redacted parts dim.
func styleLogText(text, tag string) string {
	parts := strings.Split(text, redactedText)
	for i, part := range parts {
		if part != "" {
			parts[i] = tag + part + "{{[-]}}"
		}
	}
	return strings.Join(parts, "{{[::D]}}"+redactedText+"{{[-]}}")
}

// downServices returns the services whose containers ended up down (serviceDown),
// in display order.
func (p *consoleEventProcessor) downServices() []string {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	var out []string
	add := func(svc string) {
		if p.serviceDown(svc) && !contains(out, svc) {
			out = append(out, svc)
		}
	}
	for _, img := range p.imageOrder {
		for _, svc := range p.serviceIDsForImage(img) {
			add(svc)
		}
	}
	for _, svc := range p.serviceIDs {
		add(svc)
	}
	for _, svc := range slices.Sorted(maps.Keys(p.downLogs)) {
		add(svc)
	}
	return out
}

// downSummaryLines returns the summary's list of the containers that ended up down: each
// one's name, then its state, error and log lines as the display shows them, wrapped to
// wrapW under their labels. Nil when none did.
func (p *consoleEventProcessor) downSummaryLines(wrapW int) []string {
	down := p.downServices()
	if len(down) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("%s %d %s down:", p.recapLead(), len(down), plural(len(down), "container", "containers"))}
	const labelW = len("state")
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, svc := range down {
		d := p.downLogs[svc]
		lines = append(lines, "  "+dockerlayout.StyleServiceName(svc)+"{{|DockerColon|}}:{{[-]}}")
		const indent = "    "
		textW := max(wrapW-len(indent)-labelW-2, minDetailW)
		row := func(label, text string, rows int) {
			for i, part := range wrapWords(text, textW, rows) {
				prefix := indent + strutil.Repeat(" ", labelW+2)
				if i == 0 && label != "" {
					prefix = indent + "{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} " + strutil.Repeat(" ", labelW-len(label))
				}
				lines = append(lines, prefix+part)
			}
		}
		if d.state != "" {
			row("state", d.state, depLogRows)
		}
		if d.err != "" {
			row("error", d.err, downErrRows)
		}
		for i, l := range d.lines {
			label := "log"
			if i > 0 {
				label = ""
			}
			row(label, l, depLogRows)
		}
		// Who needs it, as on its row.
		for _, g := range depGroups {
			var names []string
			for _, dep := range p.neededBy(svc) {
				if dep.condition == g.condition && dep.required == g.required {
					names = append(names, dockerlayout.StyleServiceName(dep.name))
				}
			}
			if len(names) == 0 {
				continue
			}
			label := g.label + " by"
			text := strings.Join(names, "{{|DockerColon|}},{{[-]}} ")
			for i, part := range wrapStyled(text, max(wrapW-len(label)-2, minDetailW)) {
				prefix := indent + strutil.Repeat(" ", len(label)+2)
				if i == 0 {
					prefix = indent + "{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
				}
				lines = append(lines, prefix+part)
			}
		}
	}
	return lines
}
