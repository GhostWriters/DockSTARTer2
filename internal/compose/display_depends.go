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
// meet, and whether the dependency is required.
type serviceDep struct {
	name      string
	condition string
	required  bool
}

// Each dependency is pending until its condition is met or it fails; one whose health
// had no result by the end of the run is unknown.
type depState int

const (
	depPending depState = iota
	depMet
	depUnknown
	depFailed
)

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
			out[name] = append(out[name], serviceDep{name: dep, condition: cond, required: cfg.Required})
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
			return depUnknown, "No Result"
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
	ic := p.icons()
	var lines []string
	for _, g := range depGroups {
		var names []string
		var nameWs []int
		worst := depMet
		for _, d := range deps {
			if d.condition != g.condition || d.required != g.required {
				continue
			}
			state, problem := p.depStatus(d)
			style := "{{|DockerDependsWaiting|}}"
			switch {
			case state == depMet:
				style = "{{|DockerDependsMet|}}"
			case state == depFailed && d.required:
				style = "{{|DockerDependsFailed|}}"
			}
			if (state == depFailed || state == depUnknown) && !d.required {
				state = depMet // compose starts the service anyway
			}
			if state > worst || state == depPending && worst == depMet {
				worst = state
			}
			text := d.name
			if problem != "" {
				text += " (" + problem + ")"
			}
			names = append(names, style+text+"{{[-]}}")
			nameWs = append(nameWs, utf8.RuneCountInString(text))
		}
		if len(names) == 0 {
			continue
		}

		var icon, statusText, statusTag string
		switch worst {
		case depFailed:
			icon = "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
			statusText, statusTag = "Failed", "{{|DockerStatusFail|}}Failed{{[-]}}"
		case depPending:
			icon = p.activeSpinnerTag(ic.spinner)
			statusText, statusTag = "Waiting", "{{|DockerStatusActive|}}Waiting{{[-]}}"
		case depUnknown:
			icon = "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
			statusText, statusTag = "No Result", "{{|DockerStatusWarn|}}No Result{{[-]}}"
		default:
			icon = "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
			statusText, statusTag = "Ready", detailFinalTag+"Ready{{[-]}}"
		}
		labelIndent := strutil.Repeat(" ", 2*sectionChildIndentW)
		label := "{{|DockerDetailLabel|}}" + g.label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
		prefix := globalIndent + icon + " " + statusTag + "{{[-]}}" +
			strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText)) + labelIndent + label
		namesCol := utf8.RuneCountInString(semstyle.ToPlain(prefix))
		maxW := max(termW-timerReserveW-summaryHeaderIndentW-namesCol, minDetailW)

		// Wrap the names, continuation lines starting under the first name.
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
		lines = append(lines, line)
	}
	return lines
}
