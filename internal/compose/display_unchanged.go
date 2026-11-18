package compose

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// Group words for the services nothing has happened to yet, by their state before the
// run, and groupOrder the order the groups follow the changed rows in, a service's
// life from pulled to running. Processing holds those whose image compose is checking
// (whether it needs pulling); Running both those running before and those confirmed
// untouched (and Cached, a pull's).
const (
	pendingText    = "Pending"
	notCreatedText = "Not Created"
	notRunningText = "Not Running"
	pausedText     = "Paused"
	processingText = "Processing"
	notPulledText  = "Not Pulled"
	cachedText     = "Cached"
)

var groupOrder = []string{pendingText, processingText, interruptedText, notPulledText, notCreatedText, notRunningText, pausedText, api.StatusRunning, cachedText}

// rowGroup is where a row goes: its own line under changed: for a change, or into
// the group of its state under unchanged:, nothing having happened to it yet or for
// good.
type rowGroup int

const (
	groupChanged     rowGroup = iota // something happened to it: its own row, under changed:
	groupPending                     // nothing has happened to it yet: the group of its state before the run
	groupSettled                     // confirmed left as it was: Running (Cached for a pull; Scheduled for a job)
	groupInterrupted                 // a failed run never got to it, with nothing else to show
)

// serviceGroupWord returns the group svc goes in, imgName being its image ("" for
// none), or "" for its own row: something happened to it, or the call has returned
// and it isn't settled. Until something happens to it, a service sits in the group of
// what's next for it: Processing while compose checks its image, Not Pulled when its
// image isn't on disk, else for a pull Cached, otherwise its container's state before
// the run (Running, Paused, Not Running, Not Created; unhealthy is a problem, its own
// row). A failed run's services with nothing to show are Interrupted.
func (p *consoleEventProcessor) serviceGroupWord(svc, imgName string, termW int) string {
	if p.interruptedOnly(svc, imgName, termW) {
		return interruptedText
	}
	switch p.serviceGroup(svc, imgName, termW) {
	case groupSettled:
		return p.settledWord()
	case groupChanged:
		return ""
	}
	if img := p.tasks[imgName]; imgName != "" && img != nil && !img.completed() {
		return processingText // compose is seeing whether its image needs pulling
	}
	if imgName != "" {
		if _, onDisk := p.imageMetaBefore[imgName]; !onDisk {
			return notPulledText
		}
	}
	if p.command == "pull" {
		if imgName == "" {
			return pendingText
		}
		return cachedText
	}
	switch state, seen := p.preRun[svc]; {
	case !seen:
		return notCreatedText
	case state == "stopped":
		return notRunningText
	case state == preRunPaused:
		return pausedText
	case state == healthUnhealthy:
		return ""
	}
	return api.StatusRunning
}

// interruptedOnly reports whether svc, after a failed run, has nothing to show but
// being interrupted: compose stopped before reaching it, or gave it up, with no line
// under its row (no confirmed port conflict, health or dependency line, failed model)
// and its image, if any, unchanged. Such services fold into the Interrupted group; one
// with a possible port conflict folds apart with its line, as any group's.
func (p *consoleEventProcessor) interruptedOnly(svc, imgName string, termW int) bool {
	if !p.finished || p.succeeded {
		return false
	}
	if t := p.tasks[svc]; t != nil && !t.interrupted {
		return false
	}
	if p.portConflict != nil && p.portConflict.involves(svc) || len(p.serviceConditionLines(svc, termW, 0, false)) > 0 {
		return false
	}
	for _, m := range p.serviceModels[svc] {
		if t := p.tasks[modelTaskID(m.name)]; t != nil && t.status == api.Error {
			return false // its model line shows the failure
		}
	}
	return imgName == "" || p.imageUnchanged(imgName, termW)
}

// groupsRows reports whether the display groups its rows under changed: and
// unchanged:: on the commands that bring services up or pull them, and on teardown
// commands. After a failure only the commands that bring services up still group,
// and only below dockerlayout.VerboseDetails (buildLevel): rows showing the same
// thing fold (an interrupted service with nothing to show), and -v, like the log,
// shows every row in its place. A failed teardown shows every row.
func (p *consoleEventProcessor) groupsRows() bool {
	if p.finished && !p.succeeded || p.anyRowFailed() {
		switch p.command {
		case "up", "update", "run":
			return p.buildLevel < dockerlayout.VerboseDetails
		}
		return false
	}
	switch p.command {
	case "up", "update", "run", "pull":
		return true
	}
	return p.isTeardownCommand()
}

// teardownGroupWord returns the group a teardown leaves svc in, or "" for its own row
// under changed:, and whether the group's rows are still to be acted on. A service
// with no container is Not Created, one the command leaves alone is in the group of
// its state (teardownSkip), and for stop one already stopped is Not Running, though
// compose still reports stopping it; until compose reports on the rest, they sit in
// the group of their state before the run, Running, Paused or Not Running.
func (p *consoleEventProcessor) teardownGroupWord(svc string) (word string, pending bool) {
	t := p.tasks[svc]
	state, seen := p.preRun[svc]
	if word := teardownSkip(p.command, state, seen); word != "" && t == nil {
		return word, false
	}
	switch {
	case !seen && t == nil:
		return notCreatedText, false
	case p.command == "stop" && seen && state == "stopped":
		return notRunningText, false
	case t != nil || p.finished:
		return "", false
	case state == "stopped":
		return notRunningText, true
	case state == preRunPaused:
		return pausedText, true
	}
	return api.StatusRunning, true
}

// teardownTargets returns the services of names that command acts on, sorted, by
// their containers' states before the run (preRun), and true; false for a command
// that takes every service it's given.
func teardownTargets(command string, names []string, preRun map[string]string) ([]string, bool) {
	if !skipsByState(command) {
		return nil, false
	}
	out := []string{}
	for _, name := range names {
		if state, seen := preRun[name]; teardownSkip(command, state, seen) == "" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, true
}

// skipsByState reports whether command leaves out services by their state: pause,
// unpause and kill.
func skipsByState(command string) bool {
	switch command {
	case "pause", "unpause", "kill":
		return true
	}
	return false
}

// teardownSkip returns the group of a service command leaves alone, by its container's
// state before the run (seen: it has one), or "" when compose acts on it: pause takes
// running containers, unpause paused ones, and kill either. Compose fails a whole run
// on a container already paused (pause) or not paused (unpause), and kill on finding
// none to kill, so DS2 passes it only the others.
func teardownSkip(command, state string, seen bool) string {
	if !skipsByState(command) {
		return ""
	}
	switch {
	case !seen:
		return notCreatedText
	case state == "stopped":
		return notRunningText
	case command == "pause" && state == preRunPaused:
		return pausedText
	case command == "unpause" && state != preRunPaused:
		return api.StatusRunning
	}
	return ""
}

// settledWord returns the state the command leaves a service it didn't change in,
// which names that group: an untouched container, or for a pull an image needing
// no download.
func (p *consoleEventProcessor) settledWord() string {
	if p.command == "pull" {
		return cachedText
	}
	return api.StatusRunning
}

// serviceGroup returns svc's group, imgName being its image ("" for none). A
// service stays pending until something happens to it or it's confirmed settled;
// an image check alone (update checks every image) isn't something happening,
// layers being downloaded are. Once the call has returned, nothing is pending.
func (p *consoleEventProcessor) serviceGroup(svc, imgName string, termW int) rowGroup {
	switch {
	case p.serviceSettled(svc, imgName, termW):
		return groupSettled
	case p.serviceChanged(svc, imgName, termW) || p.finished:
		return groupChanged
	}
	return groupPending
}

// serviceSettled reports whether svc is final and left as it was: for a pull, its
// image needed no download and no container was touched; otherwise its container
// is untouched (Running) with no message, a healthy or no health check and no condition
// line pending or with a problem, and its image, if any, unchanged.
func (p *consoleEventProcessor) serviceSettled(svc, imgName string, termW int) bool {
	t := p.tasks[svc]
	if p.command == "pull" {
		return t == nil && imgName != "" && p.imageUnchanged(imgName, termW)
	}
	if t == nil || t.text != api.StatusRunning || t.status != api.Done || t.message != "" ||
		t.healthErr || t.conditionErr || t.interrupted {
		return false
	}
	if h := p.serviceHealth(svc); h != "" && h != healthHealthy {
		return false
	}
	if len(p.serviceConditionLines(svc, termW, 0, false)) > 0 {
		return false
	}
	return imgName == "" || p.imageUnchanged(imgName, termW)
}

// serviceChanged reports whether something happened to svc: a container action, an
// error or warning, a problem shown under it, or its image downloading or changed.
func (p *consoleEventProcessor) serviceChanged(svc, imgName string, termW int) bool {
	if t := p.tasks[svc]; t != nil && (t.text != api.StatusRunning || t.status == api.Error || t.status == api.Warning ||
		t.message != "" || t.healthErr || t.conditionErr || t.interrupted) {
		return true
	}
	if p.serviceHealth(svc) == healthUnhealthy || len(p.serviceConditionLines(svc, termW, 0, false)) > 0 {
		return true
	}
	if imgName == "" {
		return false
	}
	if img := p.tasks[imgName]; img != nil && (img.status == api.Error || img.status == api.Warning) {
		return true
	}
	for _, l := range p.layersForImage(imgName) {
		if l.text != "Already exists" {
			return true // a layer being or been downloaded
		}
	}
	return len(p.imageInfoLines(imgName, termW, false)) > 0
}

// imageUnchanged reports whether imgName is final and needed no download: for a
// pull of missing images only (up, run), one with no events is on disk already;
// update and pull check every image, so theirs is unchanged once its check ends with
// every layer already there, and the version lines a changed image gets are absent.
func (p *consoleEventProcessor) imageUnchanged(imgName string, termW int) bool {
	t := p.tasks[imgName]
	switch {
	case t == nil:
		if (p.command == "update" || p.command == "pull") && !p.finished {
			return false // its check hasn't reported yet
		}
	case !t.completed() || p.worstImageStatus(imgName) != api.Done || !p.allLayersAlreadyExist(imgName):
		return false
	}
	return len(p.imageInfoLines(imgName, termW, false)) == 0
}

// groupLines returns a folded group's lines: its state word, how many rows of noun it
// holds, in DockerNamesLabel, and all their names, wrapped to termW under the
// first. pending styles it as a group of rows not running (yet); Processing, rows
// whose image compose is checking, with a spinner; Interrupted, the rows a failed run
// never got to, as a warning.
func (p *consoleEventProcessor) groupLines(word string, pending bool, names []string, singular, plural string, termW int) []string {
	noun := singular
	if len(names) != 1 {
		noun = plural
	}
	icon, style := "{{|DockerMarkerDone|}}"+p.icons().done+"{{[-]}}", serviceFinalTag
	switch {
	case word == processingText:
		icon, style = p.activeSpinnerTag(p.icons().spinner), "{{|DockerStatusActive|}}"
	case word == interruptedText:
		icon, style = "{{|DockerMarkerWarn|}}"+p.icons().warn+"{{[-]}}", "{{|DockerStatusWarn|}}"
	case pending:
		icon, style = "{{|DockerStatusPending|}}"+p.icons().pending+"{{[-]}}", "{{|DockerStatusPending|}}"
	}
	return p.namesLines(icon, style+word+"{{[-]}}", word, len(names), noun, names, termW)
}

// foldedRowLines returns the line of service rows that show the same thing, folded:
// their marker and status, how many, and their names.
func (p *consoleEventProcessor) foldedRowLines(icon, statusTag, statusText string, names []string, termW int) []string {
	return p.namesLines(icon, statusTag, statusText, len(names), "services", names, termW)
}

// namesLines returns a line of icon, statusTag (whose visible text is word), count noun
// in DockerNamesLabel, and names, wrapped to termW under the first.
func (p *consoleEventProcessor) namesLines(icon, statusTag, word string, count int, noun string, names []string, termW int) []string {
	label := strconv.Itoa(count) + " " + noun
	prefix := globalIndent + icon + " " + statusTag +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) + sectionChildIndent +
		"{{|DockerNamesLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}}"
	if len(names) == 0 {
		return []string{prefix}
	}
	indentW := globalIndentW + iconW + spaceW + sectionStatusW + sectionChildIndentW + len(label) + 2
	room := max(termW-timerReserveW-summaryHeaderIndentW-indentW, minDetailW)
	styled := make([]string, len(names))
	for i, name := range names {
		styled[i] = dockerlayout.StyleServiceName(name) // names hold no spaces, so wrapping splits no tag
	}
	lines := wrapStyled(strings.Join(styled, "{{|DockerColon|}},{{[-]}} "), room)
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + " " + lines[i]
		} else {
			lines[i] = strutil.Repeat(" ", indentW) + lines[i]
		}
	}
	return lines
}
