package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/spf13/cobra"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// Event ID prefixes of the task IDs that keep a model, an orphan or a container outside
// the project apart from a service of the same name.
const (
	modelTaskPrefix     = "Model "
	orphanTaskPrefix    = "Orphan "
	containerTaskPrefix = "Container task "
)

// modelPulledText is the status of a model the run pulled, once configured: the change.
// One already pulled reads existingText once configured, as compose configures every
// model on every run.
const modelPulledText = "Pulled"

// rowKind is a kind of simple row (a model, an orphan container): a name, a status,
// and lines under it.
type rowKind struct {
	section, singular, plural string
	names                     []string                                      // its rows, in display order
	taskID                    func(name string) string                      // a row's task
	group                     func(name string) (word string, pending bool) // its unchanged group, "" for changed
	order                     []string                                      // the unchanged groups' order
	rowLines                  func(name string, termW, level int) ([]string, []timerEntry)
	// rowStatus and rowDetail, when set, split a row into its marker and status and the
	// lines under it, so rows that show the same thing fold (kindRows).
	rowStatus func(name string) (icon, tag, text string)
	rowDetail func(name string, termW, level int) []string
}

// taskIDs returns the task IDs of names.
func (k rowKind) taskIDs(names []string) []string {
	ids := make([]string, len(names))
	for i, name := range names {
		ids[i] = k.taskID(name)
	}
	return ids
}

// kindState returns the rolled-up state of k's rows names: their tasks', and pending
// for a row with none yet, or a warning once the run ended without reaching it.
func (p *consoleEventProcessor) kindState(k rowKind, names []string) sectionRollupState {
	var reported []string
	states := []sectionRollupState{}
	for _, name := range names {
		switch {
		case p.tasks[k.taskID(name)] != nil:
			reported = append(reported, k.taskID(name))
		case p.finished:
			states = append(states, rollupWarning)
		default:
			states = append(states, rollupPending)
		}
	}
	if len(reported) > 0 {
		states = append(states, p.rollupState(reported, nil))
	}
	if len(states) == 0 {
		return rollupComplete
	}
	return combineStates(states)
}

// kindSection returns k's section, for a display that doesn't group rows: its heading
// with its rows' rolled-up state, then each row's lines; nothing when it has no rows.
func (p *consoleEventProcessor) kindSection(k rowKind, termW, level int) ([]string, []timerEntry) {
	if len(k.names) == 0 {
		return nil, nil
	}
	icon, tag, text := p.stateLine(p.kindState(k, k.names))
	lines := []string{globalIndent + icon + " " + tag + strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text)) +
		"{{|DockerSection|}}" + k.section + "{{[-]}}{{|DockerColon|}}:{{[-]}}"}
	timers := []timerEntry{{task: p.sectionTaskFor(k.taskIDs(k.names)), style: timerSection}}
	l, t := p.kindRows(k, k.names, termW, level)
	return append(lines, l...), append(timers, t...)
}

// kindRows returns k's rows names, in order. Below -v, when k splits its rows
// (rowStatus, rowDetail), rows that show the same thing fold into one line naming them,
// their lines once and their timers merged, as appendServiceRows does for services.
func (p *consoleEventProcessor) kindRows(k rowKind, names []string, termW, level int) ([]string, []timerEntry) {
	var lines []string
	var timers []timerEntry
	fold := k.rowStatus != nil && k.rowDetail != nil && level < dockerlayout.VerboseDetails
	key := func(name string) string {
		icon, tag, _ := k.rowStatus(name)
		return icon + tag + "\x00" + strings.Join(k.rowDetail(name, termW, level), "\n")
	}
	folded := map[string][]string{}
	if fold {
		for _, name := range names {
			folded[key(name)] = append(folded[key(name)], name)
		}
	}
	drawn := map[string]bool{}
	for _, name := range names {
		if !fold || len(folded[key(name)]) < 2 {
			l, t := k.rowLines(name, termW, level)
			lines, timers = append(lines, l...), append(timers, t...)
			continue
		}
		if drawn[key(name)] {
			continue
		}
		drawn[key(name)] = true
		group := folded[key(name)]
		icon, tag, text := k.rowStatus(name)
		tasks := make([]*consoleTask, 0, len(group))
		for _, n := range group {
			if t := p.tasks[k.taskID(n)]; t != nil {
				tasks = append(tasks, t)
			}
		}
		for i, line := range p.foldedRowLines(icon, tag, text, group, k.plural, termW) {
			timer := timerEntry{}
			if i == 0 && len(tasks) > 0 {
				timer = timerEntry{task: mergeTimers(tasks), style: timerService}
			}
			lines, timers = append(lines, line), append(timers, timer)
		}
		for _, line := range k.rowDetail(name, termW, level) {
			lines, timers = append(lines, line), append(timers, timerEntry{})
		}
	}
	return lines, timers
}

// kindGroups returns k's changed and unchanged sets: the changed rows, and the rest in
// their groups (k.group), each one folded line of names, or with -v their rows.
func (p *consoleEventProcessor) kindGroups(k rowKind, termW, level int) kindGroups {
	open := level >= dockerlayout.VerboseDetails
	out := kindGroups{singular: k.singular, plural: k.plural}
	var changed []string
	groups := map[string][]string{}
	groupPending := map[string]bool{}
	for _, name := range k.names {
		word, pending := k.group(name)
		if word == "" {
			changed = append(changed, name)
			continue
		}
		groups[word] = append(groups[word], name)
		groupPending[word] = groupPending[word] || pending
	}
	out.changed = groupSet{count: len(changed), state: p.kindState(k, changed), timer: p.sectionTaskFor(k.taskIDs(changed))}
	out.changed.lines, out.changed.timers = p.kindRows(k, changed, termW, level)
	var unchanged []string
	for _, word := range k.order {
		unchanged = append(unchanged, groups[word]...)
	}
	out.unchanged = groupSet{count: len(unchanged), state: p.kindState(k, unchanged), timer: p.sectionTaskFor(k.taskIDs(unchanged))}
	for _, word := range k.order {
		if len(groups[word]) == 0 {
			continue
		}
		if open {
			l, t := p.kindRows(k, groups[word], termW, level)
			out.unchanged.lines, out.unchanged.timers = append(out.unchanged.lines, l...), append(out.unchanged.timers, t...)
			continue
		}
		for _, line := range p.groupLines(word, groupPending[word], groups[word], k.singular, k.plural, termW) {
			out.unchanged.lines = append(out.unchanged.lines, line)
			out.unchanged.timers = append(out.unchanged.timers, timerEntry{})
		}
	}
	return out
}

// upsertKindRow records an event for the row of a simple kind: its task (taskID, made
// and listed in *list under name on its first event), its status and an error's message.
func (p *consoleEventProcessor) upsertKindRow(taskID, name string, list *[]string, e api.Resource) *consoleTask {
	t, exists := p.tasks[taskID]
	if !exists {
		t = &consoleTask{id: taskID, startTime: time.Now()}
		p.tasks[taskID] = t
		p.ids = append(p.ids, taskID)
		*list = append(*list, name)
	}
	if t.status == api.Error && e.Status != api.Error {
		return t // a failed row keeps its error: compose follows a failed model pull with Pulled
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
	return t
}

// kindRowLine returns a simple row's line: its icon and status, then its name, styled
// as a service's.
func kindRowLine(icon, tag, text, name string) string {
	return globalIndent + icon + " " + tag + strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text)) +
		sectionChildIndent + dockerlayout.StyleServiceName(name) + "{{|DockerColon|}}:{{[-]}}"
}

// ── Models ──────────────────────────────────────────────────────────────────

// modelDecls returns the project's models, by the name compose's events give them
// (their name:, else their key), each mapped to the model it runs.
func modelDecls(project *types.Project) map[string]string {
	out := make(map[string]string, len(project.Models))
	for key, m := range project.Models {
		name := m.Name
		if name == "" {
			name = key
		}
		out[name] = m.Model
	}
	return out
}

// serviceModel is one model a service uses: the model's name, as its row shows it, and
// the variables compose sets in the service to the model's endpoint and to the model.
type serviceModel struct {
	name, urlVar, modelVar string
}

// serviceModelDecls returns the models each service uses, sorted by name. A variable
// the service doesn't name (endpoint_var, model_var) is compose's default: the model's
// key in upper case, - as _, then _URL or _MODEL.
func serviceModelDecls(project *types.Project) map[string][]serviceModel {
	out := make(map[string][]serviceModel)
	for svcName, svc := range project.Services {
		for key, cfg := range svc.Models {
			prefix := strings.ReplaceAll(strings.ToUpper(key), "-", "_")
			m := serviceModel{name: key, urlVar: prefix + "_URL", modelVar: prefix + "_MODEL"}
			if decl, ok := project.Models[key]; ok && decl.Name != "" {
				m.name = decl.Name
			}
			if cfg != nil && cfg.EndpointVariable != "" {
				m.urlVar = cfg.EndpointVariable
			}
			if cfg != nil && cfg.ModelVariable != "" {
				m.modelVar = cfg.ModelVariable
			}
			out[svcName] = append(out[svcName], m)
		}
		slices.SortFunc(out[svcName], func(a, b serviceModel) int { return strings.Compare(a.name, b.name) })
	}
	return out
}

// modelLines returns the lines under svc's row for the models it uses, on the commands
// that ensure models, as the file writes them: a model: line for each, with the model's
// own status, then with all the variables compose sets for it.
func (p *consoleEventProcessor) modelLines(svc string, termW int, all bool) []string {
	if !ensuresModels(p.command) {
		return nil
	}
	const varLabelW = len("endpoint_var")
	label := strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerMarkerDone|}}model{{[-]}}{{|DockerColon|}}:{{[-]}} "
	var lines []string
	for _, m := range p.serviceModels[svc] {
		text, tag, icon := p.modelStatus(m.name)
		lines = append(lines, globalIndent+icon+" "+tag+strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text))+
			label+dockerlayout.StyleServiceName(m.name))
		if all {
			lines = append(lines,
				p.infoLine("endpoint_var", varLabelW, []infoPart{{m.urlVar, ""}}, termW),
				p.infoLine("model_var", varLabelW, []infoPart{{m.modelVar, ""}}, termW))
		}
	}
	return lines
}

// ensuresModels reports whether command makes compose pull and configure the
// project's models: the commands that create containers.
func ensuresModels(command string) bool {
	return registersJobs(command) || command == "run"
}

// modelPlugin returns the Docker Model plugin's path, or why compose can't reach it, in
// compose's own words. Compose fails such a run before it reports on any model.
func modelPlugin(cli dockerconfig.Provider) (path, problem string) {
	plugin, err := manager.GetPlugin("model", cli, &cobra.Command{})
	switch {
	case errdefs.IsNotFound(err):
		return "", "'models' support requires Docker Model plugin"
	case err != nil:
		return "", err.Error()
	case plugin.Err != nil:
		return "", "failed to load Docker Model plugin: " + plugin.Err.Error()
	}
	return plugin.Path, ""
}

// modelSettings are the runtime settings compose configures a model with.
type modelSettings struct {
	contextSize int
	flags       []string
}

func (s modelSettings) equal(o modelSettings) bool {
	return s.contextSize == o.contextSize && slices.Equal(s.flags, o.flags)
}

// modelSettingsDecls returns the settings the project gives each model, by name.
func modelSettingsDecls(project *types.Project) map[string]modelSettings {
	out := make(map[string]modelSettings, len(project.Models))
	for key, m := range project.Models {
		name := m.Name
		if name == "" {
			name = key
		}
		out[name] = modelSettings{contextSize: m.ContextSize, flags: m.RuntimeFlags}
	}
	return out
}

// readModelSettings returns the settings the model runner holds for each model in
// models (name → model), read with the plugin at pluginPath; a model it can't read is
// left out. One not pulled reads as no settings. Run on its own, the plugin writes
// the settings to stderr (through docker model, to stdout), so either stream is read.
func readModelSettings(ctx context.Context, pluginPath string, models map[string]string) map[string]modelSettings {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "DOCKER_CLI_PLUGIN") })
	out := make(map[string]modelSettings, len(models))
	for name, model := range models {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		cmd := exec.CommandContext(cctx, pluginPath, "configure", "show", model)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		cancel()
		var shown []struct {
			Config struct {
				ContextSize  int      `json:"context-size"`
				RuntimeFlags []string `json:"runtime-flags"`
			}
		}
		if err != nil || json.Unmarshal(stdout.Bytes(), &shown) != nil && json.Unmarshal(stderr.Bytes(), &shown) != nil {
			continue
		}
		var s modelSettings
		if len(shown) > 0 {
			s = modelSettings{contextSize: shown[0].Config.ContextSize, flags: shown[0].Config.RuntimeFlags}
		}
		out[name] = s
	}
	return out
}

// modelSettingsChanged reports whether compose's configure changed model name's
// settings: the runner held others before the run. False when they couldn't be read.
func (p *consoleEventProcessor) modelSettingsChanged(name string) bool {
	had, ok := p.modelSettingsBefore[name]
	return ok && !had.equal(p.modelSettings[name])
}

// failModels marks each of the project's models failed with problem, for a run
// compose can't ensure them on.
func (p *consoleEventProcessor) failModels(problem string) {
	for _, name := range slices.Sorted(maps.Keys(p.models)) {
		p.On(api.Resource{ID: name, Status: api.Error, Text: problem})
	}
}

func modelTaskID(name string) string { return modelTaskPrefix + name }

// upsertModel records an event for model name: Pulling marks it pulled by this run.
func (p *consoleEventProcessor) upsertModel(name string, e api.Resource) {
	p.upsertKindRow(modelTaskID(name), name, &p.modelIDs, e)
	if e.Text == api.StatusPulling || e.Text == api.StatusPulled {
		p.modelPulled[name] = true
	}
}

// modelRows returns the models the display lists: the project's on the commands that
// bring services up (compose ensures every one), and any compose reported on.
func (p *consoleEventProcessor) modelRows() []string {
	var names []string
	if ensuresModels(p.command) {
		for name := range p.models {
			names = append(names, name)
		}
	}
	for _, name := range p.modelIDs {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// modelStatus returns model name's row status: Pending before compose reports on it,
// Interrupted when the run ended first, Pulled once a model the run pulled is
// configured, Existing once one already pulled is, else compose's own (Pulling,
// Configuring, Error).
func (p *consoleEventProcessor) modelStatus(name string) (text, tag, icon string) {
	ic := p.icons()
	t := p.tasks[modelTaskID(name)]
	switch {
	case t == nil && p.finished:
		return interruptedText, "{{|DockerStatusWarn|}}" + interruptedText + "{{[-]}}", "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
	case t == nil:
		return pendingText, "{{|DockerStatusPending|}}" + pendingText + "{{[-]}}", "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
	case t.status == api.Error:
		return api.StatusError, "{{|DockerStatusFail|}}" + api.StatusError + "{{[-]}}", "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}"
	}
	text = t.text
	if t.completed() {
		switch {
		case p.modelPulled[name]:
			text = modelPulledText
		case t.text == api.StatusConfigured && !p.modelSettingsChanged(name):
			text = existingText
		}
		return text, serviceFinalTag + text + "{{[-]}}", "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
	}
	return text, "{{|DockerStatusActive|}}" + text + "{{[-]}}", p.activeSpinnerTag(ic.spinner)
}

// modelKind returns the models as a simple row kind: a model the run pulled, or one
// that failed or the run didn't reach, changed; one already pulled, which it only
// configured, in the Existing group, and one not reported on yet in the Pending group. Under a row, its
// model: line shows the model it runs, with -v or once the run pulled it.
func (p *consoleEventProcessor) modelKind() rowKind {
	return rowKind{
		section: "models", singular: "model", plural: "models",
		names:  p.modelRows(),
		taskID: modelTaskID,
		order:  []string{pendingText, existingText},
		group: func(name string) (string, bool) {
			t := p.tasks[modelTaskID(name)]
			switch {
			case t == nil && !p.finished:
				return pendingText, true
			case t != nil && t.status == api.Done && t.text == api.StatusConfigured && !p.modelPulled[name] && !p.modelSettingsChanged(name):
				return existingText, false
			}
			return "", false
		},
		rowLines: func(name string, termW, level int) ([]string, []timerEntry) {
			text, tag, icon := p.modelStatus(name)
			t := p.tasks[modelTaskID(name)]
			lines := []string{kindRowLine(icon, tag, text, name)}
			timers := []timerEntry{{task: t, style: timerService}}
			if line := p.errorLine(t); line != "" {
				lines, timers = append(lines, line), append(timers, timerEntry{})
			}
			if model := p.models[name]; model != "" && (level >= dockerlayout.VerboseDetails || p.modelPulled[name]) {
				lines, timers = append(lines, p.infoDetailLine("model", model, termW)), append(timers, timerEntry{})
			}
			return lines, timers
		},
	}
}

// ── Orphans ─────────────────────────────────────────────────────────────────

func orphanTaskID(name string) string { return orphanTaskPrefix + name }

// isOrphanEvent reports whether a container event is compose stopping or removing a
// container of a service the project no longer has (--remove-orphans).
func isOrphanEvent(e api.Resource) bool {
	switch e.Text {
	case api.StatusStopping, api.StatusStopped, api.StatusRemoving, api.StatusRemoved:
		return true
	}
	return e.Status == api.Error
}

// orphanKind returns the orphan containers compose stopped or removed as a simple row
// kind: every one changed.
func (p *consoleEventProcessor) orphanKind() rowKind {
	names := slices.Sorted(slices.Values(p.orphanIDs))
	return rowKind{
		section: "orphans", singular: "orphan", plural: "orphans",
		names:  names,
		taskID: orphanTaskID,
		group:  func(string) (string, bool) { return "", false },
		rowLines: func(name string, termW, level int) ([]string, []timerEntry) {
			t := p.tasks[orphanTaskID(name)]
			lines := []string{kindRowLine(p.spinnerIcon(t), serviceStatusTag(t.status, t.text, "down"), abbreviateStatus(t.text), name)}
			timers := []timerEntry{{task: t, style: timerService}}
			if line := p.errorLine(t); line != "" {
				lines, timers = append(lines, line), append(timers, timerEntry{})
			}
			return lines, timers
		},
	}
}

// ── Containers (docker start, stop or restart) ──────────────────────────────

func containerTaskID(name string) string { return containerTaskPrefix + name }

// containerStateWord returns the group of a container by its state before the run.
func containerStateWord(state string) string {
	switch state {
	case "":
		return notCreatedText
	case "stopped":
		return notRunningText
	case preRunPaused:
		return pausedText
	}
	return api.StatusRunning
}

// containerKind returns the containers a docker start, stop or restart acts on, by
// name, as a simple row kind: one the command leaves as it was in the group of its
// state (verbLeavesAlone), one not reported on yet in the group of its state before the
// run, the rest changed. Rows that show the same thing fold. With -v, a row names what
// runs the container: its service, or another project.
func (p *consoleEventProcessor) containerKind() rowKind {
	status := func(name string) (icon, tag, text string) {
		t := p.tasks[containerTaskID(name)]
		if t == nil {
			word := containerStateWord(p.containerPreRun[name])
			return "{{|DockerStatusPending|}}" + p.icons().pending + "{{[-]}}", "{{|DockerStatusPending|}}" + word + "{{[-]}}", word
		}
		return p.spinnerIcon(t), serviceStatusTag(t.status, t.text, p.command), abbreviateStatus(t.text)
	}
	detail := func(name string, termW, level int) []string {
		var lines []string
		if line := p.errorLine(p.tasks[containerTaskID(name)]); line != "" {
			lines = append(lines, line)
		}
		if d := p.containerDetail[name]; d[0] != "" && level >= dockerlayout.VerboseDetails {
			lines = append(lines, p.infoDetailLine(d[0], d[1], termW))
		}
		return lines
	}
	return rowKind{
		section: "containers", singular: "container", plural: "containers",
		names:  p.containers,
		taskID: containerTaskID,
		order:  teardownOrder,
		group: func(name string) (string, bool) {
			state := p.containerPreRun[name]
			if word := verbLeavesAlone(p.command, state, state != ""); word != "" {
				return word, false
			}
			if p.tasks[containerTaskID(name)] != nil || p.finished {
				return "", false
			}
			return containerStateWord(state), true
		},
		rowStatus: status,
		rowDetail: detail,
		rowLines: func(name string, termW, level int) ([]string, []timerEntry) {
			icon, tag, text := status(name)
			timer := timerEntry{}
			if t := p.tasks[containerTaskID(name)]; t != nil {
				timer = timerEntry{task: t, style: timerService}
			}
			lines, timers := []string{kindRowLine(icon, tag, text, name)}, []timerEntry{timer}
			for _, line := range detail(name, termW, level) {
				lines, timers = append(lines, line), append(timers, timerEntry{})
			}
			return lines, timers
		},
	}
}
