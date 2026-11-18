package compose

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// unchangedText is the status of the line a section's unchanged rows fold into.
const unchangedText = "Unchanged"

// foldsUnchanged reports whether the display folds the rows a command left as they
// were into one line per section: below -v, on the commands that bring services up,
// where most rows of a running stack are untouched. -v lists every row, and so does
// a run with a failure, where the rows around it are context.
func (p *consoleEventProcessor) foldsUnchanged(level int) bool {
	if level >= dockerlayout.VerboseDetails || p.finished && !p.succeeded || p.anyRowFailed() {
		return false
	}
	switch p.command {
	case "up", "update", "run":
		return true
	}
	return false
}

// serviceUnchanged reports whether svc is final and unchanged: its container
// untouched (Running), with no message, a healthy or no health check, no detail line
// still pending or with a problem, and its image, if any, unchanged.
func (p *consoleEventProcessor) serviceUnchanged(svc, imgName string, termW, level int) bool {
	t := p.tasks[svc]
	if t == nil || t.text != api.StatusRunning || t.status != api.Done || t.message != "" ||
		t.healthErr || t.conditionErr || t.interrupted {
		return false
	}
	if h := p.serviceHealth(svc); h != "" && h != healthHealthy {
		return false
	}
	if len(p.serviceDetailLines(svc, termW, level, false)) > 0 {
		return false
	}
	return imgName == "" || p.imageUnchanged(imgName, termW)
}

// imageUnchanged reports whether imgName is final and needed no download: for a
// pull of missing images only (up, run), one with no events is on disk already;
// update checks every image, so its image is unchanged once its check ends with
// every layer already there, and the version lines a changed image gets are absent.
func (p *consoleEventProcessor) imageUnchanged(imgName string, termW int) bool {
	t := p.tasks[imgName]
	switch {
	case t == nil:
		if p.command == "update" && !p.finished {
			return false // its check hasn't reported yet
		}
	case !t.completed() || p.worstImageStatus(imgName) != api.Done || !p.allLayersAlreadyExist(imgName):
		return false
	}
	return len(p.imageInfoLines(imgName, termW, false)) == 0
}

// foldLines returns the lines a section's unchanged rows fold into: Unchanged, how
// many there are of noun, and all their names, wrapped to termW under the first.
func (p *consoleEventProcessor) foldLines(names []string, singular, plural string, termW int) []string {
	noun := singular
	if len(names) != 1 {
		noun = plural
	}
	label := strconv.Itoa(len(names)) + " " + noun
	prefix := globalIndent + "{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}} " + serviceFinalTag + unchangedText + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(unchangedText)) + sectionChildIndent +
		"{{|DockerDetailLabel|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
	indentW := globalIndentW + iconW + spaceW + sectionStatusW + sectionChildIndentW + len(label) + 2
	room := max(termW-timerReserveW-summaryHeaderIndentW-indentW, minDetailW)
	styled := make([]string, len(names))
	for i, name := range names {
		styled[i] = dockerlayout.StyleServiceName(name) // names hold no spaces, so wrapping splits no tag
	}
	lines := wrapStyled(strings.Join(styled, "{{|DockerColon|}},{{[-]}} "), room)
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = strutil.Repeat(" ", indentW) + lines[i]
		}
	}
	return lines
}
