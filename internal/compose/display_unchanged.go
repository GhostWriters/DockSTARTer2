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
	if p.portConflict != nil && p.portConflict.involves(svc) || p.hasConditionLines(svc, termW) {
		return false
	}
	for _, m := range p.serviceModels[svc] {
		if t := p.tasks[modelTaskID(m.name)]; t != nil && t.status == api.Error {
			return false // its model line shows the failure
		}
	}
	return imgName == "" || p.imageUnchanged(imgName)
}

// groupsRows reports whether the display groups its rows under changed: and
// unchanged:: on the commands that bring services up or pull them, and on teardown
// commands. After a failure only the commands that bring services up, and docker
// start, stop or restart by container name, still group, and only below
// dockerlayout.VerboseDetails (buildLevel): rows showing the same thing fold (an
// interrupted service with nothing to show), and -v, like the log, shows each row in
// its place, rows that show the same thing still folded as everywhere. A failed
// compose teardown shows every row.
func (p *consoleEventProcessor) groupsRows() bool {
	if p.finished && !p.succeeded || p.anyRowFailed() {
		switch {
		case p.command == "up", p.command == "update", p.command == "run", p.containerVerb != "":
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
// its state (teardownSkip, verbLeavesAlone: stop of one already stopped is Not Running
// though compose still reports stopping it); until compose reports on the rest, they
// sit in the group of their state before the run, Running, Paused or Not Running.
func (p *consoleEventProcessor) teardownGroupWord(svc string) (word string, pending bool) {
	t := p.tasks[svc]
	state, seen := p.preRun[svc]
	if word := teardownSkip(p.command, state, seen); word != "" && t == nil {
		return word, false
	}
	if word := verbLeavesAlone(p.command, state, seen); word != "" {
		return word, false
	}
	switch {
	case !seen && t == nil:
		return notCreatedText, false
	case t != nil || p.finished:
		return "", false
	case state == "stopped":
		return notRunningText, true
	case state == preRunPaused:
		return pausedText, true
	}
	return api.StatusRunning, true
}

// verbLeavesAlone returns the group of a container command leaves as it was, by its
// state before the run (seen: it has a container), though docker accepts the command:
// stop of one stopped is Not Running, and start (docker start, by container name)
// of one running is Running. "" when the command changes it.
func verbLeavesAlone(command, state string, seen bool) string {
	switch {
	case !seen:
		return ""
	case command == "stop" && state == "stopped":
		return notRunningText
	case command == "start" && state != "stopped" && state != preRunPaused:
		return api.StatusRunning
	}
	return ""
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
// layers being downloaded are. Once the call has returned, nothing is pending. A
// service the command is going to start that others depend on is a change at once
// (willStart).
func (p *consoleEventProcessor) serviceGroup(svc, imgName string, termW int) rowGroup {
	switch {
	case p.serviceSettled(svc, imgName, termW):
		return groupSettled
	case p.serviceChanged(svc, imgName, termW) || p.finished || p.willStart(svc):
		return groupChanged
	}
	return groupPending
}

// willStart reports whether a command that brings services up is going to start svc's
// container (it is stopped or doesn't exist yet) and something depends on it. Such a
// service has its own row from the first frame, with the services waiting on it in its
// needed-by line, instead of sitting in the group of its state until compose reports.
func (p *consoleEventProcessor) willStart(svc string) bool {
	switch p.command {
	case "up", "update":
		state, seen := p.preRun[svc]
		return (!seen || state == "stopped") && len(p.neededBy(svc)) > 0
	}
	return false
}

// serviceSettled reports whether svc is final and left as it was: for a pull, its
// image needed no download and no container was touched; otherwise its container
// is untouched (Running) with no message, a healthy or no health check and no condition
// line pending or with a problem, and its image, if any, unchanged.
func (p *consoleEventProcessor) serviceSettled(svc, imgName string, termW int) bool {
	t := p.tasks[svc]
	if p.command == "pull" {
		return t == nil && imgName != "" && p.imageUnchanged(imgName)
	}
	if t == nil || t.text != api.StatusRunning || t.status != api.Done || t.message != "" ||
		t.healthErr || t.conditionErr || t.interrupted {
		return false
	}
	if h := p.serviceHealth(svc); h != "" && h != healthHealthy {
		return false
	}
	if p.hasConditionLines(svc, termW) {
		return false
	}
	return imgName == "" || p.imageUnchanged(imgName)
}

// serviceChanged reports whether something happened to svc: a container action, an
// error or warning, a problem shown under it, or its image downloading or changed.
func (p *consoleEventProcessor) serviceChanged(svc, imgName string, termW int) bool {
	if t := p.tasks[svc]; t != nil && (t.text != api.StatusRunning || t.status == api.Error || t.status == api.Warning ||
		t.message != "" || t.healthErr || t.conditionErr || t.interrupted) {
		return true
	}
	if p.serviceHealth(svc) == healthUnhealthy || p.hasConditionLines(svc, termW) {
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
	return p.imagePullChanged(imgName)
}

// imagePullChanged reports whether this run's pull changed imgName: a first pull, or a
// new digest or version. A service of the image brought up, which shows the version it
// now runs under the image, doesn't count: that is the service's change, not its
// siblings'.
func (p *consoleEventProcessor) imagePullChanged(imgName string) bool {
	before, hadBefore := p.imageMetaBefore[imgName]
	after, pulled := p.imageMetaAfter[imgName]
	switch {
	case !pulled:
		return false
	case !hadBefore:
		return true
	}
	differs := func(a, b string) bool { return a != "" && b != "" && a != b }
	return differs(before.digest, after.digest) || differs(before.version, after.version)
}

// imageUnchanged reports whether imgName is final and needed no download: for a
// pull of missing images only (up, run), one with no events is on disk already;
// update and pull check every image, so theirs is unchanged once its check ends with
// every layer already there and its pull didn't change it (imagePullChanged).
func (p *consoleEventProcessor) imageUnchanged(imgName string) bool {
	t := p.tasks[imgName]
	switch {
	case t == nil:
		if (p.command == "update" || p.command == "pull") && !p.finished {
			return false // its check hasn't reported yet
		}
	case !t.completed() || p.worstImageStatus(imgName) != api.Done || !p.allLayersAlreadyExist(imgName):
		return false
	}
	return !p.imagePullChanged(imgName)
}

// groupLines returns a folded group's lines: its state word, how many rows of noun it
// holds, underlined, and all their names, wrapped to termW under the
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

// foldedRowLines returns the line of rows of noun (services, containers) that show the
// same thing, folded: their marker and status, how many, and their names.
func (p *consoleEventProcessor) foldedRowLines(icon, statusTag, statusText string, names []string, noun string, termW int) []string {
	return p.namesLines(icon, statusTag, statusText, len(names), noun, names, termW)
}

// namesLines returns a line of icon, statusTag (whose visible text is word), count noun
// underlined, and names, wrapped to termW under the first.
func (p *consoleEventProcessor) namesLines(icon, statusTag, word string, count int, noun string, names []string, termW int) []string {
	label := strconv.Itoa(count) + " " + noun
	prefix := globalIndent + icon + " " + statusTag +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) + sectionChildIndent +
		dockerlayout.Label(count, noun, true)
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

// hasConditionLines reports whether something is shown under svc's row that makes it a
// change: its healthcheck while not healthy, a dependency with a problem, the state and
// log of one that went down, or a ports conflict. A dependency still pending is only a
// check on what is already running, so it doesn't: the row stays in its group until the
// condition fails.
func (p *consoleEventProcessor) hasConditionLines(svc string, termW int) bool {
	if line := p.healthLine(svc, termW); line != "" && p.serviceHealth(svc) != healthHealthy {
		return true
	}
	return p.dependsHaveProblem(svc) || len(p.serviceLogLines(svc, termW)) > 0 || len(p.portsLines(svc, termW)) > 0
}

// dependsHaveProblem reports whether any of svc's dependencies has failed its condition
// or gives a warning (a pending one has not).
func (p *consoleEventProcessor) dependsHaveProblem(svc string) bool {
	if !commandWatchesHealth(p.command) {
		return false
	}
	for _, d := range p.dependsOn[svc] {
		if state, _ := p.depStatus(d); state == depFailed || state == depWarning {
			return true
		}
	}
	return false
}
