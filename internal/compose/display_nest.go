package compose

import (
	"regexp"
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

// headLook is the status word, style tag and icon a heading takes from its rows when
// they all show the same; the zero value is the generic look of the heading's state.
type headLook struct{ word, tag, icon string }

// rowIcon returns the styled icon a line shows before its status, "" for a line with none.
func rowIcon(line string) string {
	i := indexAfterColumns(line, globalIndentW)
	j := indexAfterColumns(line, globalIndentW+iconW+spaceW)
	if i < 0 || j < i {
		return ""
	}
	return strings.TrimSpace(line[i:j])
}

// groupLead matches how a folded group's line opens past its status column: N noun:.
var groupLead = regexp.MustCompile(`^\d+ [^\s:]+:`)

// isRowLine reports whether line is a row of its set (a named row, or a folded group
// of them) and not a line under one (an image, an info, a ports or a health line).
func isRowLine(line string) bool {
	if strings.HasSuffix(line, "{{|DockerColon|}}:{{[-]}}") {
		return true
	}
	runes := []rune(visibleText(line))
	start := globalIndentW + iconW + spaceW + sectionStatusW
	return len(runes) > start && groupLead.MatchString(strings.TrimSpace(string(runes[start:])))
}

// sharedStatus returns the look every row of lines shows, the zero look when they
// differ or there is none. Only the rows count: the lines under a row never change what
// its heading says, a failure under one reaches the heading through its state.
func sharedStatus(lines []string) headLook {
	var look headLook
	for _, line := range lines {
		if !isRowLine(line) {
			continue
		}
		w := rowWord(line)
		if w == "" {
			continue
		}
		l := headLook{word: w, tag: rowStatusTag(line), icon: rowIcon(line)}
		switch {
		case look.word == "":
			look = l
		case look != l:
			return headLook{}
		}
	}
	return look
}

// commonLook returns the look all of looks are, the zero look when any is generic or
// they differ.
func commonLook(looks []headLook) headLook {
	if len(looks) == 0 || looks[0].word == "" {
		return headLook{}
	}
	for _, l := range looks[1:] {
		if l != looks[0] {
			return headLook{}
		}
	}
	return looks[0]
}

// setLook returns the look a heading over set shows (state being its settled state):
// the one all its rows show, so a heading over rows that are all Running says Running,
// or the zero look for the generic word (Pending, Processing, Complete, Warning,
// Error) when they differ. A warning, an error or a pending word is passed on only
// when the rows' own word is one, in that style, not a row that merely sits in a set
// that is.
func setLook(set groupSet, state sectionRollupState) headLook {
	look := sharedStatus(set.lines)
	switch state {
	case rollupComplete, rollupProcessing:
		return look
	case rollupWarning:
		if look.tag == "{{|DockerStatusWarn|}}" {
			return look
		}
	case rollupError:
		if look.tag == "{{|DockerStatusFail|}}" {
			return look
		}
	case rollupPending:
		if look.tag == "{{|DockerStatusPending|}}" {
			return look
		}
	}
	return headLook{}
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
	return p.stateLineWord(state, headLook{})
}

// stateLineWord is stateLine with the text of a state replaced by the word of look, the
// status all the rows it heads share, when there is one. Unless it is processing, when
// their icon is not the state's (an info marker, as a manual job's) the heading takes
// their icon and style too.
func (p *consoleEventProcessor) stateLineWord(state sectionRollupState, look headLook) (icon, tag, text string) {
	text, stTag, iconTag := sectionStatusText(state)
	icon = iconTag + p.sectionRollupIcon(state) + "{{[-]}}"
	if state == rollupProcessing {
		icon = p.activeSpinnerTag(p.icons().spinner)
	}
	if look.word != "" {
		text = look.word
		if state != rollupProcessing && look.icon != "" && look.icon != icon {
			icon, stTag = look.icon, look.tag
		}
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

// namesLead matches a line's text past its status column when a label (N noun:) leads
// names on it: the indent, then the label and the space after it.
var namesLead = regexp.MustCompile(`^( *)(\d+ [^\s:]+: )\S`)

// namesColumn returns the visible column where the names a label (N noun:) leads start
// on line, -1 when line is not a row with such names.
func namesColumn(line string) int {
	if rowWord(line) == "" {
		return -1
	}
	runes := []rune(visibleText(line))
	start := globalIndentW + iconW + spaceW + sectionStatusW
	if len(runes) <= start {
		return -1
	}
	m := namesLead.FindStringSubmatch(string(runes[start:]))
	if m == nil {
		return -1
	}
	return start + utf8.RuneCountInString(m[1]) + utf8.RuneCountInString(m[2])
}

// continuesNames reports whether line is a wrapped line of names starting at col.
func continuesNames(line string, col int) bool {
	runes := []rune(visibleText(line))
	return len(runes) > col && strings.TrimLeft(string(runes[:col]), " ") == "" && runes[col] != ' '
}

// alignNames returns lines with the names of every label (N noun:) starting at one
// column, the furthest any reaches; a folded group's names are rewrapped to termW from
// there. timers stay one per line.
func alignNames(lines []string, timers []timerEntry, termW int) ([]string, []timerEntry) {
	type namesRun struct{ first, end, col int } // lines [first, end), names at col
	var runs []namesRun
	target := 0
	for i := 0; i < len(lines); i++ {
		col := namesColumn(lines[i])
		if col < 0 {
			continue
		}
		r := namesRun{first: i, end: i + 1, col: col}
		for r.end < len(lines) && strings.HasSuffix(visibleText(lines[r.end-1]), ",") && continuesNames(lines[r.end], col) {
			r.end++
		}
		runs = append(runs, r)
		target = max(target, col)
		i = r.end - 1
	}
	var outLines []string
	var outTimers []timerEntry
	next := 0
	for _, r := range runs {
		if r.col == target {
			continue
		}
		outLines = append(outLines, lines[next:r.first]...)
		outTimers = append(outTimers, timers[next:r.first]...)
		next = r.end
		k := indexAfterColumns(lines[r.first], r.col)
		names := []string{lines[r.first][k:]}
		for _, line := range lines[r.first+1 : r.end] {
			names = append(names, strings.TrimLeft(line, " "))
		}
		room := max(termW-timerReserveW-summaryHeaderIndentW-target, minDetailW)
		for w, part := range wrapStyled(strings.Join(names, " "), room) {
			if w == 0 {
				outLines = append(outLines, lines[r.first][:k]+strutil.Repeat(" ", target-r.col)+part)
			} else {
				outLines = append(outLines, strutil.Repeat(" ", target)+part)
			}
			t := timerEntry{}
			if r.first+w < r.end {
				t = timers[r.first+w]
			}
			outTimers = append(outTimers, t)
		}
	}
	if next == 0 {
		return lines, timers
	}
	return append(outLines, lines[next:]...), append(outTimers, timers[next:]...)
}

// groupedLines returns the changed: heading, then the unchanged: one, each with
// its kinds' rolled-up state and timer, leaving out one with no rows. Under each, a
// kind with rows gets a heading (N services:) and its rows, or folded group lines,
// nested under it, so a line keeps its indent as rows fold and unfold. A heading shows
// the status word all its rows share, and a folded group or a lone row that is all
// there is under it stands in for it. Names after a label (N noun:) all start at one
// column (alignNames), wrapped to termW.
func (p *consoleEventProcessor) groupedLines(kinds []kindGroups, termW int) ([]string, []timerEntry) {
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
		var looks []headLook
		var tasks []*consoleTask
		for _, k := range kinds {
			if set := k.set(adjective); set.count > 0 {
				state := settle(adjective, set.state)
				states = append(states, state)
				looks = append(looks, setLook(set, state))
				tasks = append(tasks, set.timer)
			}
		}
		if len(states) == 0 {
			continue
		}
		groupState := combineStates(states)
		icon, tag, text := p.stateLineWord(groupState, commonLook(looks))
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
			look := setLook(set, state)
			setTimers := set.timers
			setLines := set.lines
			// A folded group, or a lone row, that is all there is under the heading says
			// what the heading would (its word, its count), so it stands in for it: in the
			// heading's place, not nested, in its status style, taking its timer; lines
			// under it (an image, details) stay under it.
			merged := false
			if len(setLines) > 0 {
				// With lines under it (an image, details) the row stands in only for a heading
				// that is Complete, one with nothing to hide.
				alone := look.word != "" && len(setLines) == 1
				complete := state == rollupComplete
				switch {
				case (look.word != "" || complete) && leadsWithLabel(setLines[0], set.count, k.singular, k.plural):
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
				icon, tag, text := p.stateLineWord(state, look)
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
	return alignNames(lines, timers, termW)
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
