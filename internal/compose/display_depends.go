package compose

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"

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
// whether it is required (an optional one's failure leaves the line Ready).
type depEntry struct {
	name     string
	state    depState
	problem  string
	required bool
}

// depGroups lists a service's dependency lines in display order: label, condition
// and required, one line for each the service uses.
var depGroups = []struct {
	label     string
	condition string
	required  bool
}{
	{"started", types.ServiceConditionStarted, true},
	{"healthy", types.ServiceConditionHealthy, true},
	{"completed", types.ServiceConditionCompletedSuccessfully, true},
	{"started (optional)", types.ServiceConditionStarted, false},
	{"healthy (optional)", types.ServiceConditionHealthy, false},
	{"completed (optional)", types.ServiceConditionCompletedSuccessfully, false},
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

// preRunState returns a container's state before the run for the dependency lines:
// its health when it has a healthcheck, else running or stopped.
func preRunState(ctr container.Summary) string {
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
func (p *consoleEventProcessor) depStatus(d serviceDep) (depState, string) {
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
		case "stopped", "":
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
// for each condition it uses, wrapped to fit termW.
func (p *consoleEventProcessor) dependsLines(svc string, termW int) []string {
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
		lines = append(lines, p.detailNameLines(g.label, entries, p.reachedContainers(svc), termW)...)
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
// has restarted it after svc. A dependent with no row in this run counts by its state
// before the run.
func (p *consoleEventProcessor) dependentStatus(d serviceDep) (depState, string) {
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
	return depWarning, "Still Running" // outside the run, so it stays up
}

// neededByLines returns the line under svc's row listing what depends on it, on the
// commands where that orders the run: stop and down, which stop those first, and
// restart, which restarts its restart: true dependents after it.
func (p *consoleEventProcessor) neededByLines(svc string, termW int) []string {
	switch p.command {
	case "stop", "down", "restart":
	default:
		return nil
	}
	var entries []depEntry
	for _, d := range p.neededBy(svc) {
		state, problem := p.dependentStatus(d)
		entries = append(entries, depEntry{d.name, state, problem, true})
	}
	return p.detailNameLines("needed by", entries, true, termW)
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
// name still pending reads Waiting when compose may be waiting on it, else Pending.
func (p *consoleEventProcessor) detailNameLines(label string, entries []depEntry, waiting bool, termW int) []string {
	if len(entries) == 0 {
		return nil
	}
	var names []string
	var nameWs []int
	worst := depMet
	onlyNoResult := true
	for _, e := range entries {
		state := e.state
		style := "{{|DockerDependsWaiting|}}"
		switch {
		case state == depMet:
			style = "{{|DockerDependsMet|}}"
		case state == depFailed && e.required:
			style = "{{|DockerDependsFailed|}}"
		}
		if (state == depFailed || state == depWarning) && !e.required {
			state = depMet // compose starts the service anyway; the name shows the problem
		}
		if state == depWarning && e.problem != "No Result" {
			onlyNoResult = false
		}
		if depRank(state) > depRank(worst) {
			worst = state
		}
		text := e.name
		if e.problem != "" {
			text += " (" + e.problem + ")"
		}
		names = append(names, style+text+"{{[-]}}")
		nameWs = append(nameWs, utf8.RuneCountInString(text))
	}

	ic := p.icons()
	var icon, statusText, statusTag string
	switch worst {
	case depFailed:
		icon = "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
		statusText, statusTag = "Failed", "{{|DockerStatusFail|}}Failed{{[-]}}"
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
