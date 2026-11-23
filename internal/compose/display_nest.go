package compose

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// nestW is how far a group heading's rows sit further in, past the status column.
const nestW = sectionChildIndentW

// nestLine returns line with its content, everything past the status column (the
// row's name, an image or info line, a message), nestW further in, so it sits under a
// group heading. The status column stays put. Tags count no width.
func nestLine(line string) string {
	i := columnIndex(line, globalIndentW+iconW+spaceW+sectionStatusW)
	if i < 0 {
		return line // shorter than the status column: nothing to move
	}
	return line[:i] + strutil.Repeat(" ", nestW) + line[i:]
}

// columnIndex returns the byte index in line of its visible column col, tags counting
// no width, or -1 when the line is shorter.
func columnIndex(line string, col int) int {
	seen := 0
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "{{") {
			if end := strings.Index(line[i:], "}}"); end >= 0 {
				i += end + 2
				continue
			}
		}
		if seen == col {
			return i
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
		seen++
	}
	return -1
}

// indexAfterColumns returns the byte index in line just past its first n visible
// characters, before any tag that follows them, or -1 when it has fewer.
func indexAfterColumns(line string, n int) int {
	seen := 0
	for i := 0; i < len(line); {
		if seen == n {
			return i
		}
		if strings.HasPrefix(line[i:], "{{") {
			if end := strings.Index(line[i:], "}}"); end >= 0 {
				i += end + 2
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
		seen++
	}
	if seen == n {
		return len(line)
	}
	return -1
}

// labelRow returns a lone row's line (its name and a colon) with a heading's label,
// count noun:, before the name and no colon after, so it stands in for the heading
// over it; false when line isn't such a row.
func labelRow(line string, count int, singular, plural string) (string, bool) {
	const colon = "{{|DockerColon|}}:{{[-]}}"
	noun := singular
	if count != 1 {
		noun = plural
	}
	// Before the name's style tag, so the name keeps its style after the label's reset.
	i := indexAfterColumns(line, globalIndentW+iconW+spaceW+sectionStatusW+sectionChildIndentW)
	if i < 0 || !strings.HasSuffix(line, colon) {
		return line, false
	}
	return line[:i] + dockerlayout.Label(count, noun, true) + " " + strings.TrimSuffix(line[i:], colon), true
}

// headingLabel returns a kind heading's label, count noun and a colon, as a heading
// styles it.
func headingLabel(count int, noun string) string {
	return dockerlayout.Label(count, noun, false)
}

// visibleText returns line without its style tags.
func visibleText(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "{{") {
			if end := strings.Index(line[i:], "}}"); end >= 0 {
				i += end + 2
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// rowWord returns the status word a line shows in the status column, "" for a line
// with none (a detail line, a wrapped line of names).
func rowWord(line string) string {
	runes := []rune(visibleText(line))
	start := globalIndentW + iconW + spaceW
	if len(runes) <= start {
		return ""
	}
	return strings.TrimSpace(string(runes[start:min(start+sectionStatusW, len(runes))]))
}

// rowStatusTag returns the style tag of the status a line shows, "" for a line with none.
func rowStatusTag(line string) string {
	i := indexAfterColumns(line, globalIndentW+iconW+spaceW)
	if i < 0 || !strings.HasPrefix(line[i:], "{{|") {
		return ""
	}
	end := strings.Index(line[i:], "|}}")
	if end < 0 {
		return ""
	}
	return line[i : i+end+3]
}

// sharedStatus returns the status word, and its style tag, every row of lines shows,
// "" when they differ or there is none.
func sharedStatus(lines []string) (word, tag string) {
	for _, line := range lines {
		w := rowWord(line)
		if w == "" {
			continue
		}
		t := rowStatusTag(line)
		switch {
		case word == "":
			word, tag = w, t
		case word != w || tag != t:
			return "", ""
		}
	}
	return word, tag
}

// commonWord returns the word all of words are, "" when any is empty or they differ.
func commonWord(words []string) string {
	if len(words) == 0 || words[0] == "" {
		return ""
	}
	for _, w := range words[1:] {
		if w != words[0] {
			return ""
		}
	}
	return words[0]
}

// setWord returns the status word a heading over set shows (state being its settled
// state): the word all its rows show, so a heading over rows that are all Running says
// Running, or "" for the generic word (Pending, Processing, Complete, Warning, Error)
// when they differ. A warning, an error or a pending word is passed on only when the
// rows' own word is one, in that style, not a row that merely sits in a set that is.
func setWord(set groupSet, state sectionRollupState) string {
	word, tag := sharedStatus(set.lines)
	switch state {
	case rollupComplete, rollupProcessing:
		return word
	case rollupWarning:
		if tag == "{{|DockerStatusWarn|}}" {
			return word
		}
	case rollupError:
		if tag == "{{|DockerStatusFail|}}" {
			return word
		}
	case rollupPending:
		if tag == "{{|DockerStatusPending|}}" {
			return word
		}
	}
	return ""
}

// leadsWithLabel reports whether line is a folded group's line: it opens, past its
// status column, with the label (N services:) a heading over count rows of that noun
// would show.
func leadsWithLabel(line string, count int, singular, plural string) bool {
	noun := singular
	if count != 1 {
		noun = plural
	}
	runes := []rune(visibleText(line))
	start := globalIndentW + iconW + spaceW + sectionStatusW
	if len(runes) <= start {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(runes[start:])), strconv.Itoa(count)+" "+noun+":")
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
		sectionChildIndent + headingLabel(count, noun)
}

// stateLine returns a rollup state's icon, styled status and status text, as a
// section heading shows them: a spinner while processing.
func (p *consoleEventProcessor) stateLine(state sectionRollupState) (icon, tag, text string) {
	return p.stateLineWord(state, "")
}

// stateLineWord is stateLine with the text of a state replaced by word, the status all
// the rows it heads share, when there is one.
func (p *consoleEventProcessor) stateLineWord(state sectionRollupState, word string) (icon, tag, text string) {
	text, stTag, iconTag := sectionStatusText(state)
	if word != "" {
		text = word
	}
	icon = iconTag + p.sectionRollupIcon(state) + "{{[-]}}"
	if state == rollupProcessing {
		icon = p.activeSpinnerTag(p.icons().spinner)
	}
	return icon, stTag + text + "{{[-]}}", text
}

// groupSet is one kind's rows under changed: or unchanged:: how many, their rolled-up
// state and timer, and their lines: its rows, or for a folded unchanged set its
// group lines (one per state, or per set of possible port conflicts within one, each
// naming its kind).
type groupSet struct {
	count  int
	state  sectionRollupState
	timer  *consoleTask
	lines  []string
	timers []timerEntry
}

// kindGroups is one kind's (services, networks, volumes, models, jobs, orphans)
// changed and unchanged rows.
type kindGroups struct {
	singular, plural   string
	changed, unchanged groupSet
}

// groupedLines returns the changed: heading, then the unchanged: one, each with
// its kinds' rolled-up state and timer, leaving out one with no rows. Under each, a
// kind with rows gets a heading (N services:) and its rows, or folded group lines,
// nested under it, so a line keeps its indent as rows fold and unfold. A heading shows
// the status word all its rows share, and a folded group or a lone row that is all
// there is under it stands in for it.
func (p *consoleEventProcessor) groupedLines(kinds []kindGroups) ([]string, []timerEntry) {
	var lines []string
	var timers []timerEntry
	// After a failed run nothing is still pending: what never got going was cut short,
	// a warning. Nothing under unchanged: failed, so it is a warning at most.
	settle := func(adjective string, s sectionRollupState) sectionRollupState {
		if p.finished && !p.succeeded {
			if s == rollupPending || s == rollupProcessing || adjective == "unchanged" && s == rollupError {
				return rollupWarning
			}
		}
		return s
	}
	for _, adjective := range []string{"changed", "unchanged"} {
		var states []sectionRollupState
		var words []string
		var tasks []*consoleTask
		for _, k := range kinds {
			if set := k.set(adjective); set.count > 0 {
				state := settle(adjective, set.state)
				states = append(states, state)
				words = append(words, setWord(set, state))
				tasks = append(tasks, set.timer)
			}
		}
		if len(states) == 0 {
			continue
		}
		groupState := combineStates(states)
		icon, tag, text := p.stateLineWord(groupState, commonWord(words))
		if groupState == rollupComplete {
			tag = strings.Replace(tag, sectionFinalTag, "{{|DockerGroupStatus|}}", 1) // the group's own style, as its title
		}
		lines = append(lines, globalIndent+icon+" "+tag+strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text))+
			"{{|DockerGroupHeading|}}"+adjective+"{{[-]}}{{|DockerColon|}}:{{[-]}}")
		timers = append(timers, timerEntry{task: mergeTimers(tasks), style: timerGroup})
		for _, k := range kinds {
			set := k.set(adjective)
			if set.count == 0 {
				continue
			}
			state := settle(adjective, set.state)
			word := setWord(set, state)
			setTimers := set.timers
			setLines := set.lines
			// A folded group, or a lone row, that is all there is under the heading says
			// what the heading would (its word, its count), so it stands in for it: in the
			// heading's place, not nested, in its status style, taking its timer; lines
			// under it (an image, details) stay under it.
			merged := false
			if len(setLines) > 0 {
				// With lines under it (an image, details) the row stands in only for a heading
				// that is Complete, one with nothing to hide; its own word then replaces
				// Complete.
				alone := word != "" && len(setLines) == 1
				complete := state == rollupComplete
				switch {
				case (word != "" || complete) && leadsWithLabel(setLines[0], set.count, k.singular, k.plural):
					merged = true
				case set.count == 1 && (alone || complete):
					if line, ok := labelRow(setLines[0], set.count, k.singular, k.plural); ok {
						setLines = slices.Clone(setLines)
						setLines[0], merged = line, true
					}
				}
			}
			if merged {
				// Its timer, the row's own or the heading's, is in the heading's style.
				if len(setTimers) > 0 {
					setTimers = slices.Clone(setTimers)
					if setTimers[0].task == nil {
						setTimers[0].task = set.timer
					}
					setTimers[0].style = timerSection
				}
			} else {
				icon, tag, text := p.stateLineWord(state, word)
				lines = append(lines, groupHeading(icon, tag, text, set.count, k.singular, k.plural))
				timers = append(timers, timerEntry{task: set.timer, style: timerSection})
			}
			for _, line := range setLines {
				if !merged {
					line = nestLine(line)
				}
				lines = append(lines, line)
			}
			timers = append(timers, setTimers...)
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
