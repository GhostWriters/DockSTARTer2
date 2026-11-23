package compose

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// serviceDep is one depends_on entry: the service depended on, the condition it must
// meet, whether the dependency is required, and whether restarting it restarts the
// service too (restart: true).
type serviceDep struct {
	name      string
	condition string
	required  bool
	restart   bool
}

// Each dependency (or dependent, on a needed-by line) is pending until its condition is
// met or it fails; a warning is a problem that doesn't fail it, such as a health check
// with no result by the end of the run.
type depState int

const (
	depPending depState = iota
	depMet
	depWarning
	depFailed
)

// depEntry is one name on a detail line: its state, the problem to show after it, and
// whether it is required (an optional one's failure makes the line a warning, not a failure).
type depEntry struct {
	name     string
	state    depState
	problem  string
	required bool
}

// depGroups lists a service's dependency lines in display order: label, condition
// and required, one line for each the service uses. A required dependency is needed and
// an optional one wanted, two words of one length so their names line up.
var depGroups = []struct {
	label     string
	condition string
	required  bool
}{
	{"needs started", types.ServiceConditionStarted, true},
	{"needs healthy", types.ServiceConditionHealthy, true},
	{"needs completed", types.ServiceConditionCompletedSuccessfully, true},
	{"wants started", types.ServiceConditionStarted, false},
	{"wants healthy", types.ServiceConditionHealthy, false},
	{"wants completed", types.ServiceConditionCompletedSuccessfully, false},
}

// projectDependencies returns each service's depends_on entries, sorted by name.
func projectDependencies(project *types.Project) map[string][]serviceDep {
	out := make(map[string][]serviceDep)
	for name, svc := range project.Services {
		for dep, cfg := range svc.DependsOn {
			cond := cfg.Condition
			if cond == "" {
				cond = types.ServiceConditionStarted
			}
			out[name] = append(out[name], serviceDep{name: dep, condition: cond, required: cfg.Required, restart: cfg.Restart})
		}
		slices.SortFunc(out[name], func(a, b serviceDep) int { return strings.Compare(a.name, b.name) })
	}
	return out
}

// preRunPaused is the state before the run of a paused container.
const preRunPaused = "paused"

// preRunState returns a container's state before the run: paused, else its health
// when it has a healthcheck, else running or stopped.
func preRunState(ctr container.Summary) string {
	if ctr.State == container.StatePaused {
		return preRunPaused
	}
	if ctr.State != container.StateRunning {
		return "stopped"
	}
	if h := ctr.Health; h != nil {
		switch s := string(h.Status); s {
		case healthStarting, healthHealthy, healthUnhealthy:
			return s
		}
	}
	// API versions before Health in the list report it only in Status: "Up 5 minutes (healthy)".
	switch {
	case strings.HasSuffix(ctr.Status, "(healthy)"):
		return healthHealthy
	case strings.HasSuffix(ctr.Status, "(unhealthy)"):
		return healthUnhealthy
	case strings.HasSuffix(ctr.Status, "(health: starting)"):
		return healthStarting
	}
	return "running"
}

// depStatus returns a dependency's state and, when it went wrong, the problem to show
// after its name. A dependency with no row in this run counts by its state before the run.
// One whose condition was met but whose container then went down (depLog.problem) is
// failed, for a condition that needs it up.
func (p *consoleEventProcessor) depStatus(d serviceDep) (depState, string) {
	state, problem := p.depConditionStatus(d)
	if l := p.depLogs[d.name]; state == depMet && l != nil && l.problem != "" && d.condition != types.ServiceConditionCompletedSuccessfully {
		return depFailed, l.problem
	}
	return state, problem
}

// depConditionStatus returns whether d's condition was met, by the run's events.
func (p *consoleEventProcessor) depConditionStatus(d serviceDep) (depState, string) {
	t := p.tasks[d.name]
	if t != nil && t.text == skippedText {
		return depFailed, skippedText
	}
	if t != nil && t.interrupted {
		return depFailed, interruptedText
	}
	if t != nil && t.status == api.Error && !t.healthErr {
		return depFailed, "Failed"
	}
	health := p.serviceHealth(d.name)
	started := p.containerStarted[d.name]
	if t == nil && !started {
		switch s := p.preRun[d.name]; s {
		case "stopped", preRunPaused, "": // a paused dependency isn't serving
			started = false
		default:
			started = true
			if health == "" && s != "running" {
				health = s
			}
		}
	}
	switch d.condition {
	case types.ServiceConditionHealthy:
		switch {
		case health == healthHealthy:
			return depMet, ""
		case health == healthUnhealthy:
			return depFailed, "Unhealthy"
		case health == healthStarting && p.healthWaitDone:
			return depWarning, "No Result"
		case health == healthStarting:
			return depPending, "" // the end-of-run wait may still bring a result
		}
	case types.ServiceConditionCompletedSuccessfully:
		if t != nil && t.text == api.StatusExited {
			return depMet, ""
		}
		if p.finished {
			return depFailed, "Not Completed"
		}
	default:
		if started {
			return depMet, ""
		}
	}
	if p.finished {
		return depFailed, "Not Started"
	}
	return depPending, ""
}

// dependsLines returns the lines under svc's row listing its dependencies, one line
// for each condition it uses, wrapped to fit termW; without all, only the lines still
// pending or with a problem.
func (p *consoleEventProcessor) dependsLines(svc string, termW int, all bool) []string {
	deps := p.dependsOn[svc]
	if len(deps) == 0 || !commandWatchesHealth(p.command) {
		return nil
	}
	var lines []string
	for _, g := range depGroups {
		var entries []depEntry
		for _, d := range deps {
			if d.condition == g.condition && d.required == g.required {
				state, problem := p.depStatus(d)
				entries = append(entries, depEntry{d.name, state, problem, d.required})
			}
		}
		lines = append(lines, p.detailNameLines(g.label, entries, p.reachedContainers(svc), all, false, termW)...)
	}
	return lines
}

// neededBy returns the services that depend on svc, sorted by name, with the condition
// each needs; on restart only those restarting svc restarts too (restart: true).
func (p *consoleEventProcessor) neededBy(svc string) []serviceDep {
	var out []serviceDep
	for dependent, deps := range p.dependsOn {
		for _, d := range deps {
			if d.name == svc && (p.command != "restart" || d.restart) {
				out = append(out, serviceDep{name: dependent, condition: d.condition, required: d.required, restart: d.restart})
			}
		}
	}
	slices.SortFunc(out, func(a, b serviceDep) int { return strings.Compare(a.name, b.name) })
	return out
}

// dependentStatus returns a dependent's state on svc's needed-by line: on stop and
// down, whether it has stopped (compose stops it first); on restart, whether compose
// has restarted it after svc. A dependent in the run that compose hasn't reached yet is
// pending; one outside the run counts by its state before the run.
func (p *consoleEventProcessor) dependentStatus(svc string, d serviceDep) (depState, string) {
	if !p.orderedByDependents() {
		// Starting: whether svc meets what the dependent needs of it, which its own row
		// and the dependent's needs line explain.
		state, _ := p.depStatus(serviceDep{name: svc, condition: d.condition, required: d.required})
		return state, ""
	}
	t := p.tasks[d.name]
	if t != nil && (t.status == api.Error && !t.healthErr || t.interrupted) {
		return depFailed, "Failed"
	}
	if p.command == "restart" {
		if p.containerStarted[d.name] {
			return depMet, ""
		}
		if p.finished {
			return depFailed, "Not Restarted"
		}
		return depPending, ""
	}
	if t != nil {
		switch t.text {
		case api.StatusStopped, api.StatusRemoved, api.StatusKilled, api.StatusExited:
			return depMet, ""
		}
		if p.finished {
			return depWarning, "Still Running"
		}
		return depPending, ""
	}
	switch p.preRun[d.name] {
	case "stopped", "":
		return depMet, "" // already down
	}
	if p.isKnownServiceName(d.name) { // part of the run, not reached yet
		switch {
		case !p.finished:
			return depPending, ""
		case !p.succeeded:
			return depWarning, "Still Running"
		}
		return depMet, ""
	}
	return depWarning, "Still Running" // outside the run, so it stays up
}

// orderedByDependents reports whether the command's order follows what depends on a
// service: stop and down stop the dependents first, restart restarts the restart: true
// ones after it.
func (p *consoleEventProcessor) orderedByDependents() bool {
	switch p.command {
	case "stop", "down", "restart":
		return true
	}
	return false
}

// neededGroup is one needed-by line, with the condition it lists dependents for ("" on
// the commands whose single line orders the run).
type neededGroup struct {
	condition string
	lines     []string
}

// neededByLines returns the lines under svc's row listing what depends on it, flattened
// from neededByGroups.
func (p *consoleEventProcessor) neededByLines(svc string, termW int, all bool) []string {
	var lines []string
	for _, g := range p.neededByGroups(svc, termW, all) {
		lines = append(lines, g.lines...)
	}
	return lines
}

// neededByGroups returns the needed-by lines under svc's row: on stop, down and restart
// one line, where that orders the run (orderedByDependents); on the commands that start
// containers a line for each condition, whether svc meets what its dependents need of
// it. Without all, only while still pending or with a problem. The state, log and
// healthcheck lines nest under them (serviceConditionLines).
func (p *consoleEventProcessor) neededByGroups(svc string, termW int, all bool) []neededGroup {
	if p.orderedByDependents() {
		var entries []depEntry
		for _, d := range p.neededBy(svc) {
			state, problem := p.dependentStatus(svc, d)
			entries = append(entries, depEntry{d.name, state, problem, true})
		}
		if lines := p.detailNameLines("needed by", entries, true, all, false, termW); lines != nil {
			return []neededGroup{{lines: lines}}
		}
		return nil
	}
	if !commandWatchesHealth(p.command) {
		return nil
	}
	// One line for each condition the dependents need of svc, as dependsLines has.
	var groups []neededGroup
	for _, g := range depGroups {
		label := g.label + " by"
		var entries []depEntry
		for _, d := range p.neededBy(svc) {
			if d.condition == g.condition && d.required == g.required {
				state, problem := p.dependentStatus(svc, d)
				entries = append(entries, depEntry{d.name, state, problem, d.required})
			}
		}
		cause := g.required && p.dependencyDown(svc)
		if lines := p.detailNameLines(label, entries, true, all, cause, termW); lines != nil {
			groups = append(groups, neededGroup{g.condition, lines})
		}
	}
	return groups
}

// depRank orders states for a line's status: a failure, then work still pending, then a
// warning, then met.
func depRank(s depState) int {
	switch s {
	case depFailed:
		return 3
	case depPending:
		return 2
	case depWarning:
		return 1
	}
	return 0
}

// reachedContainers reports whether svc's row has reached its container stage, past
// pulling or building its image, so compose may be waiting on its dependencies.
func (p *consoleEventProcessor) reachedContainers(svc string) bool {
	t := p.tasks[svc]
	if t == nil {
		return false
	}
	switch t.text {
	case "", api.StatusPulling, api.StatusPulled, api.StatusBuilding, api.StatusBuilt:
		return false
	}
	return true
}

// detailNameLines returns a detail line listing entries under label, wrapped to fit
// termW with continuation lines under the first name, or nil when there are none. Each
// name takes its state's style; the line's status is the worst of them. A line with a
// name still pending reads Waiting when compose may be waiting on it, else Pending. A
// line whose only problems are optional names reads Unmet as a warning. With cause, an
// unmet line is shown as the cause of another service's problem, not as a failure.
// Without all, a line that is Ready with no name showing a problem is left out.
func (p *consoleEventProcessor) detailNameLines(label string, entries []depEntry, waiting, all, cause bool, termW int) []string {
	if len(entries) == 0 {
		return nil
	}
	if !all && !slices.ContainsFunc(entries, func(e depEntry) bool { return e.state != depMet || e.problem != "" }) {
		return nil
	}
	var names []string
	var nameWs []int
	worst := depMet
	onlyNoResult := true
	optionalProblem := false // an optional name failed or warned: the line is a warning
	for _, e := range entries {
		state := e.state
		style := "DockerDependsWaiting"
		switch {
		case state == depMet:
			style = "DockerDependsMet"
		case state == depFailed && e.required && cause:
			style = "DockerDependsWaiting" // named as the one it blocks, in the warning colour
		case state == depFailed && e.required:
			style = "DockerDependsFailed"
		}
		if (state == depFailed || state == depWarning) && !e.required {
			state = depMet // compose starts the service anyway; the name shows the problem
			optionalProblem = true
		}
		if state == depWarning && e.problem != "No Result" {
			onlyNoResult = false
		}
		if depRank(state) > depRank(worst) {
			worst = state
		}
		text := e.name
		name := dockerlayout.StyleServiceName(e.name, style)
		if e.problem != "" {
			text += " (" + e.problem + ")"
			name += "{{|" + style + "|}} (" + e.problem + "){{[-]}}"
		}
		names = append(names, name)
		nameWs = append(nameWs, utf8.RuneCountInString(text))
	}

	ic := p.icons()
	var icon, statusText, statusTag string
	switch worst {
	case depFailed:
		icon = "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
		statusText, statusTag = "Unmet", "{{|DockerStatusFail|}}Unmet{{[-]}}"
		if cause {
			// The unmet condition is this service's own fault: the cause marker, as on a
			// ports line that holds a port another service wanted.
			icon = "{{|DockerMarkerCause|}}" + ic.warn + "{{[-]}}"
			statusTag = "{{|DockerStatusWarn|}}Unmet{{[-]}}"
		}
	case depPending:
		if waiting {
			icon = p.activeSpinnerTag(ic.spinner)
			statusText, statusTag = "Waiting", "{{|DockerStatusActive|}}Waiting{{[-]}}"
		} else {
			icon = "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
			statusText, statusTag = "Pending", "{{|DockerStatusPending|}}Pending{{[-]}}"
		}
	case depWarning:
		icon = "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
		statusText = "Warning"
		if onlyNoResult {
			statusText = "No Result"
		}
		statusTag = "{{|DockerStatusWarn|}}" + statusText + "{{[-]}}"
	case depMet:
		if !optionalProblem {
			icon = "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
			statusText, statusTag = "Ready", detailFinalTag+"Ready{{[-]}}"
			break
		}
		// Unmet, but optional: a warning, since it doesn't stop the service starting.
		icon = "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
		statusText, statusTag = "Unmet", "{{|DockerStatusWarn|}}Unmet{{[-]}}"
	default:
		icon = "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
		statusText, statusTag = "Ready", detailFinalTag+"Ready{{[-]}}"
	}
	labelIndent := strutil.Repeat(" ", 2*sectionChildIndentW)
	labelTag := "{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
	prefix := globalIndent + icon + " " + statusTag + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText)) + labelIndent + labelTag
	namesCol := utf8.RuneCountInString(semstyle.ToPlain(prefix))
	maxW := max(termW-timerReserveW-summaryHeaderIndentW-namesCol, minDetailW)

	// Wrap the names, continuation lines starting under the first name.
	var lines []string
	line, lineW := prefix, 0
	for i, name := range names {
		sep, sepW := "", 0
		if i > 0 {
			sep, sepW = "{{|DockerColon|}},{{[-]}} ", 2
		}
		if lineW > 0 && lineW+sepW+nameWs[i] > maxW {
			lines = append(lines, line+"{{|DockerColon|}},{{[-]}}")
			line, lineW = strutil.Repeat(" ", namesCol), 0
			sep, sepW = "", 0
		}
		line += sep + name
		lineW += sepW + nameWs[i]
	}
	return append(lines, line)
}
