package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/pkg/stdcopy"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/strutil"
)

const (
	depLogTimeout = 5 * time.Second // bounds each dependency's container lookup, inspect and log read
	depLogTail    = "20"            // log lines read from the end of a container's output
	depLogReadMax = 64 << 10        // bytes read from them
	depLogRows    = 2               // rows each line of state or log may take
)

// depLog is what the display shows under a dependency that went wrong: its container's
// state, and the few log lines picked and redacted from its output (display_logredact.go).
type depLog struct {
	state   string
	problem string // set when its container ended up restarting, dead or exited non-zero: shown after its name
	lines   []string
}

// depLogProblem reports whether a failed dependency's problem leaves a container worth
// looking at; a skipped or interrupted dependency never ran.
func depLogProblem(problem string) bool {
	switch problem {
	case "Not Started", "Failed", "Unhealthy", "Not Completed":
		return true
	}
	return false
}

// fetchDependencyLogs reads the state and last log lines of each dependency that failed
// its condition, or met it and then went down (compose reports a container started even
// when it crashes straight away), once the run is over, for the lines under its row and
// its dependents' needs lines.
func (p *consoleEventProcessor) fetchDependencyLogs() {
	if p.dockerClient == nil || p.projectName == "" || !commandWatchesHealth(p.command) {
		return
	}
	p.mtx.Lock()
	var names []string
	failed := map[string]bool{} // dependency → it failed its condition, so its log is read whatever its state
	for _, deps := range p.dependsOn {
		for _, d := range deps {
			state, problem := p.depConditionStatus(d) // not depStatus: a dependency found down earlier is looked at again
			if (state == depMet || state == depFailed && depLogProblem(problem)) && !contains(names, d.name) {
				failed[d.name] = state == depFailed
				names = append(names, d.name)
			}
		}
	}
	p.mtx.Unlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := p.readDependencyLog(name, failed[name])
			p.mtx.Lock()
			defer p.mtx.Unlock()
			switch {
			case info != nil && p.depLogs == nil:
				p.depLogs = map[string]*depLog{name: info}
			case info != nil:
				p.depLogs[name] = info
			default:
				delete(p.depLogs, name) // it came back up since the last look
			}
		}()
	}
	wg.Wait()
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

// readDependencyLog returns svc's container state and picked log lines, or nil when it
// has no container, the daemon doesn't answer, or it is fine and wasn't failed (force).
func (p *consoleEventProcessor) readDependencyLog(svc string, force bool) *depLog {
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
	info := &depLog{}
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
		switch {
		case st.Restarting || status == "restarting":
			info.problem = "Restarting"
		case status == "dead":
			info.problem = "Dead"
		case status == "exited" && st.ExitCode != 0:
			info.problem = "Exited"
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
	info.lines = pickLogLines(lines)
	return info
}

// serviceLogLines returns the state and log lines under svc's row when it is a
// dependency whose container went wrong. The state takes the colour its row's icon does
// (the error colour when the row itself failed or a service requires it, the cause
// marker, else the warning colour) and the log, as a message, the warning colour. Each
// line of text wraps to two rows, then is cut.
func (p *consoleEventProcessor) serviceLogLines(svc string, termW int) []string {
	return p.serviceLogLinesAt(svc, termW, 0)
}

// serviceLogLinesAt is serviceLogLines nested nest columns in, under the line it explains.
func (p *consoleEventProcessor) serviceLogLinesAt(svc string, termW, nest int) []string {
	d := p.depLogs[svc]
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
	if errored || p.dependencyCause(svc) {
		stateTag = "{{|DockerMarkerError|}}"
	}
	if errored {
		logTag = stateTag
	}
	indent := utf8.RuneCountInString(semstyle.ToPlain(globalIndent+p.icons().error+" ")) + sectionStatusW + 2*sectionChildIndentW + nest
	const labelW = len("state")
	room := max(termW-timerReserveW-summaryHeaderIndentW-indent-labelW-2, minDetailW)
	row := func(label, text, tag string, first bool) string {
		prefix := strutil.Repeat(" ", indent+labelW+2)
		if first {
			prefix = strutil.Repeat(" ", indent) + "{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} " +
				strutil.Repeat(" ", labelW-len(label))
		}
		return prefix + styleLogText(text, tag)
	}
	var lines []string
	for i, text := range wrapWords(d.state, room, depLogRows) {
		lines = append(lines, row("state", text, stateTag, i == 0))
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

// dependencyDown reports whether svc is a dependency whose container ended up
// restarting, dead or exited non-zero.
func (p *consoleEventProcessor) dependencyDown(svc string) bool {
	d := p.depLogs[svc]
	return d != nil && d.problem != ""
}

// dependencyCause reports whether svc is down (dependencyDown) and a service requires
// it: the cause of that service's unmet condition, as opposed to a failure of its own.
func (p *consoleEventProcessor) dependencyCause(svc string) bool {
	return p.dependencyDown(svc) && slices.ContainsFunc(p.neededBy(svc), func(d serviceDep) bool { return d.required })
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
