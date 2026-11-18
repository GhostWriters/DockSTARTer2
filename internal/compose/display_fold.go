package compose

import (
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// serviceRow is a service's row as the display draws it: its marker and status, and
// the lines under it (its error, its detail lines).
type serviceRow struct {
	svc, icon, statusTag, statusText string
	lines                            []string
}

// appendServiceRows draws rows, applying the display's folding rule: rows that show the
// same thing (marker, status and every line under them) fold into one line naming
// them, with their lines once and their timers merged. With -v (level at
// dockerlayout.VerboseDetails) every row shows on its own.
func (p *consoleEventProcessor) appendServiceRows(rows []serviceRow, level, termW int, appendLine func(string, *consoleTask, timerStyle)) {
	key := func(r serviceRow) string { return r.icon + r.statusTag + "\x00" + strings.Join(r.lines, "\n") }
	folded := map[string][]string{}
	if level < dockerlayout.VerboseDetails {
		for _, r := range rows {
			folded[key(r)] = append(folded[key(r)], r.svc)
		}
	}
	drawn := map[string]bool{}
	for _, r := range rows {
		if names := folded[key(r)]; len(names) > 1 {
			if drawn[key(r)] {
				continue
			}
			drawn[key(r)] = true
			for i, line := range p.foldedRowLines(r.icon, r.statusTag+"{{[-]}}", r.statusText, names, termW) {
				var timer *consoleTask
				if i == 0 {
					timer = p.foldedTimer(names)
				}
				appendLine(line, timer, timerService)
			}
		} else {
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(r.statusText))
			appendLine(globalIndent+r.icon+" "+r.statusTag+"{{[-]}}"+statusPad+sectionChildIndent+dockerlayout.StyleServiceName(r.svc)+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(r.svc), timerService)
		}
		for _, line := range r.lines {
			appendLine(line, nil, timerLayer)
		}
	}
}

// foldedTimer returns the timer of a folded line of services names: theirs merged,
// from the earliest start to the last end.
func (p *consoleEventProcessor) foldedTimer(names []string) *consoleTask {
	tasks := make([]*consoleTask, 0, len(names))
	for _, name := range names {
		if t := p.serviceTimerTask(name); t != nil {
			tasks = append(tasks, t)
		}
	}
	return mergeTimers(tasks)
}
