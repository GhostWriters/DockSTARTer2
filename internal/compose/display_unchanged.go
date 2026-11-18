package compose

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// Group words for the services nothing has happened to yet, by their state before the
// run, and groupOrder the order the groups follow the changed rows in, a service's
// life from pulled to running. Running holds both those running before and those
// confirmed untouched (and Cached, a pull's).
const (
	pendingText    = "Pending"
	notCreatedText = "Not Created"
	stoppedText    = "Stopped"
	notPulledText  = "Not Pulled"
	cachedText     = "Cached"
)

var groupOrder = []string{pendingText, notPulledText, notCreatedText, stoppedText, api.StatusRunning, cachedText}

// rowGroup is where a row goes in its section: its own line for a change, or into
// the group of its state, nothing having happened to it yet or for good.
type rowGroup int

const (
	groupChanged rowGroup = iota // something happened to it: its own row, first
	groupPending                 // nothing has happened to it yet: the group of its state before the run
	groupSettled                 // confirmed left as it was: Running (Cached for a pull; Scheduled for a job)
)

// serviceGroupWord returns the group svc goes in, imgName being its image ("" for
// none), or "" for its own row: something happened to it, or the call has returned
// and it isn't settled. Until something happens to it, a service sits in the group of
// what's next for it: Not Pulled when its image isn't on disk, else for a pull Cached,
// otherwise its container's state before the run (Running, Stopped, Not Created;
// unhealthy is a problem, its own row).
func (p *consoleEventProcessor) serviceGroupWord(svc, imgName string, termW int) string {
	switch p.serviceGroup(svc, imgName, termW) {
	case groupSettled:
		return p.settledWord()
	case groupChanged:
		return ""
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
		return stoppedText
	case state == healthUnhealthy:
		return ""
	}
	return api.StatusRunning
}

// groupsRows reports whether the display groups its rows: on the commands where
// most rows of a running stack are left as they were, while nothing has failed. A
// failure shows every row in its place, the rows around it being context.
func (p *consoleEventProcessor) groupsRows() bool {
	if p.finished && !p.succeeded || p.anyRowFailed() {
		return false
	}
	switch p.command {
	case "up", "update", "run", "pull":
		return true
	}
	return false
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
// is untouched (Running) with no message, a healthy or no health check and no detail
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
	if len(p.serviceDetailLines(svc, termW, 0, false)) > 0 {
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
	if p.serviceHealth(svc) == healthUnhealthy || len(p.serviceDetailLines(svc, termW, 0, false)) > 0 {
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
// holds, and all their names, wrapped to termW under the first. pending styles it as
// a group of rows not running (yet).
func (p *consoleEventProcessor) groupLines(word string, pending bool, names []string, singular, plural string, termW int) []string {
	noun := singular
	if len(names) != 1 {
		noun = plural
	}
	label := strconv.Itoa(len(names)) + " " + noun
	icon, style := "{{|DockerMarkerDone|}}"+p.icons().done+"{{[-]}}", serviceFinalTag
	if pending {
		icon, style = "{{|DockerStatusPending|}}"+p.icons().pending+"{{[-]}}", "{{|DockerStatusPending|}}"
	}
	prefix := globalIndent + icon + " " + style + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) + sectionChildIndent +
		"{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}}"
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
