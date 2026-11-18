package compose

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/strutil"
)

// nestW is how far a group heading's rows sit further in, past the status column.
const nestW = sectionChildIndentW

// nestLine returns line with its content, everything past the status column (the
// row's name, an image or info line, a message), nestW further in, so it sits under a
// group heading. The status column stays put. Tags count no width.
func nestLine(line string) string {
	col := globalIndentW + iconW + spaceW + sectionStatusW
	seen := 0
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "{{") {
			if end := strings.Index(line[i:], "}}"); end >= 0 {
				i += end + 2
				continue
			}
		}
		if seen == col {
			return line[:i] + strutil.Repeat(" ", nestW) + line[i:]
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
		seen++
	}
	return line // shorter than the status column: nothing to move
}

// groupHeading returns a kind's heading under changed: or unchanged:: its rows'
// rolled-up state in the status column, then how many rows of noun it holds, both as
// a section heading's.
func groupHeading(icon, statusTag, statusText string, count int, singular, plural string) string {
	noun := singular
	if count != 1 {
		noun = plural
	}
	return globalIndent + icon + " " + statusTag + strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText)) +
		sectionChildIndent + "{{|DockerSection|}}" + strconv.Itoa(count) + " " + noun + "{{[-]}}{{|DockerColon|}}:{{[-]}}"
}

// stateLine returns a rollup state's icon, styled status and status text, as a
// section heading shows them: a spinner while processing.
func (p *consoleEventProcessor) stateLine(state sectionRollupState) (icon, tag, text string) {
	text, stTag, iconTag := sectionStatusText(state)
	icon = iconTag + p.sectionRollupIcon(state) + "{{[-]}}"
	if state == rollupProcessing {
		icon = p.activeSpinnerTag(p.icons().spinner)
	}
	return icon, stTag + text + "{{[-]}}", text
}

// groupSet is one kind's rows under changed: or unchanged:: how many, their rolled-up
// state and timer, and their lines: its rows, or for a folded unchanged set its
// group lines (one per state, each naming its kind).
type groupSet struct {
	count  int
	state  sectionRollupState
	timer  *consoleTask
	lines  []string
	timers []timerEntry
}

// kindGroups is one kind's (services, networks, volumes, jobs) changed and unchanged
// rows; folded marks its unchanged set as group lines rather than rows.
type kindGroups struct {
	singular, plural   string
	changed, unchanged groupSet
	folded             bool
}

// groupedLines returns the changed: heading, then the unchanged: one, each with
// its kinds' rolled-up state and timer, leaving out one with no rows. Under each, a
// kind with rows gets a heading (N services:) and its rows nested under it; folded
// unchanged group lines, which name their kind, go directly under unchanged:.
func (p *consoleEventProcessor) groupedLines(kinds []kindGroups) ([]string, []timerEntry) {
	var lines []string
	var timers []timerEntry
	for _, adjective := range []string{"changed", "unchanged"} {
		var states []sectionRollupState
		var tasks []*consoleTask
		for _, k := range kinds {
			if set := k.set(adjective); set.count > 0 {
				states = append(states, set.state)
				tasks = append(tasks, set.timer)
			}
		}
		if len(states) == 0 {
			continue
		}
		icon, tag, text := p.stateLine(combineStates(states))
		lines = append(lines, globalIndent+icon+" "+tag+strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text))+
			"{{|DockerGroupHeading|}}"+adjective+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
		timers = append(timers, timerEntry{task: mergeTimers(tasks), style: timerSection})
		for _, k := range kinds {
			set := k.set(adjective)
			if set.count == 0 {
				continue
			}
			if adjective == "unchanged" && k.folded {
				lines = append(lines, set.lines...)
				timers = append(timers, set.timers...)
				continue
			}
			icon, tag, text := p.stateLine(set.state)
			lines = append(lines, groupHeading(icon, tag, text, set.count, k.singular, k.plural))
			timers = append(timers, timerEntry{task: set.timer, style: timerSection})
			for _, line := range set.lines {
				lines = append(lines, nestLine(line))
			}
			timers = append(timers, set.timers...)
		}
	}
	return lines, timers
}

// set returns k's changed or unchanged set, by adjective.
func (k kindGroups) set(adjective string) groupSet {
	if adjective == "changed" {
		return k.changed
	}
	return k.unchanged
}

// combineStates returns the rolled-up state of several: an error or warning when any
// has one, processing while any is, or some are done and others still pending,
// pending while all are, complete once all are.
func combineStates(states []sectionRollupState) sectionRollupState {
	var errs, warns, procs, pends int
	for _, s := range states {
		switch s {
		case rollupError:
			errs++
		case rollupWarning:
			warns++
		case rollupProcessing:
			procs++
		case rollupPending:
			pends++
		}
	}
	switch {
	case errs > 0:
		return rollupError
	case warns > 0:
		return rollupWarning
	case procs > 0 || pends > 0 && pends < len(states):
		return rollupProcessing
	case pends > 0:
		return rollupPending
	}
	return rollupComplete
}

// mergeTimers returns a timer spanning several: from the earliest start, done at the
// latest end once all are done, or nil when none has started.
func mergeTimers(tasks []*consoleTask) *consoleTask {
	var start, end time.Time
	done := true
	for _, t := range tasks {
		if t == nil || t.startTime.IsZero() {
			continue
		}
		if start.IsZero() || t.startTime.Before(start) {
			start = t.startTime
		}
		if !t.completed() || t.endTime.IsZero() {
			done = false
		} else if t.endTime.After(end) {
			end = t.endTime
		}
	}
	if start.IsZero() {
		return nil
	}
	merged := &consoleTask{startTime: start}
	if done {
		merged.status = api.Done
		merged.endTime = end
	}
	return merged
}
