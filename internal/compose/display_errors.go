package compose

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/strutil"
)

// unhealthyHealthErr reports whether t's error is compose's report that its own
// service is unhealthy ("dependency <svc> failed to start"): its healthcheck line and
// its dependents' needs lines already show that, so its row shows no message.
func (p *consoleEventProcessor) unhealthyHealthErr(t *consoleTask) bool {
	return t.healthErr && p.serviceHealth(t.id) == healthUnhealthy
}

// settleHealthErr makes a row whose only error is its own failed health check a
// warning once its container is known unhealthy: being unhealthy is a warning on its
// own, and the error belongs to the services that needed it healthy. Until then (it may
// have exited instead) it stays an error. Callers hold p.mtx.
func (p *consoleEventProcessor) settleHealthErr(t *consoleTask) {
	switch {
	case !t.healthErr || t.status != api.Error && t.status != api.Warning:
	case p.serviceHealth(t.id) == healthUnhealthy:
		t.status = api.Warning
	default:
		t.status = api.Error
	}
}

// conditionRE matches compose's message for a service it didn't start because a
// condition on one of its dependencies wasn't met.
var conditionRE = regexp.MustCompile(`^not started: condition "([^"]+)" of service "([^"]+)" not met$`)

// ownConditionUnmet reports whether msg says svc wasn't started because a condition on
// one of its own required dependencies wasn't met, rather than because another row's
// failure stopped the run.
func (p *consoleEventProcessor) ownConditionUnmet(svc, msg string) bool {
	m := conditionRE.FindStringSubmatch(msg)
	if m == nil {
		return false
	}
	for _, d := range p.dependsOn[svc] {
		if d.name == m[2] && d.required {
			return true
		}
	}
	return false
}

// shortMessage returns an error message on one line, without Docker's "Error response
// from daemon: " prefix: the row or name it's shown with already says it failed.
func shortMessage(msg string) string {
	return strings.TrimPrefix(strings.Join(strings.Fields(msg), " "), "Error response from daemon: ")
}

// requiredDependencyFailed reports whether one of svc's required dependencies failed or
// went down, on the commands that list them.
func (p *consoleEventProcessor) requiredDependencyFailed(svc string) bool {
	if !commandWatchesHealth(p.command) {
		return false
	}
	for _, d := range p.dependsOn[svc] {
		if state, _ := p.depStatus(d); d.required && (state == depFailed || p.serviceDown(d.name)) {
			return true
		}
	}
	return false
}

// maxErrorLines caps the lines a wrapped error message takes; the log summary has it
// whole.
const maxErrorLines = 6

// errorWrapW is how wide a wrapped error message's lines get, room permitting in
// termW, when the table is narrower: a small table (a -c run of one job, or a run
// failed before any layer bars) would otherwise wrap it a few words a line. It's
// capped so that one long message doesn't push every timer to a wide terminal's
// edge.
const errorWrapW = 100

// placeErrorLines fits each error line to the table (the widest other line),
// widening it, and the timer column with it, to wrap a message at errorWrapW.
// Messages that fit on the row above go there, all starting at one shared column
// two spaces past the longest of those rows; the column is the one that fits the
// most messages. The others wrap at word boundaries onto lines under their row, at
// most maxErrorLines, the last ending in … when more is cut. timers stays parallel
// to lines.
func (p *consoleEventProcessor) placeErrorLines(lines []string, timers []timerEntry, termW int) ([]string, []timerEntry) {
	// An error line is its style tag after at least the error indent's spaces: more
	// for a row nested under a group heading (nestLine).
	prefixes := p.errorLinePrefixes()
	errorPrefix := func(line string) string {
		rest := strings.TrimLeft(line, " ")
		indent := len(line) - len(rest)
		if indent < p.errorIndentW() {
			return ""
		}
		for _, prefix := range prefixes {
			if style := strings.TrimLeft(prefix, " "); strings.HasPrefix(rest, style) {
				return line[:indent] + style
			}
		}
		return ""
	}
	plainW := func(s string) int { return utf8.RuneCountInString(semstyle.ToPlain(s)) }
	width := 0
	for _, line := range lines {
		if errorPrefix(line) == "" {
			width = max(width, plainW(line))
		}
	}

	// Each error line directly under a row could go on it: pick the column (one of
	// those rows' widths) that fits the most messages, the narrowest on a tie.
	type candidate struct{ rowW, msgW int }
	candidates := make(map[int]candidate) // error line index → its row's and message's widths
	for i := 1; i < len(lines); i++ {
		if prefix := errorPrefix(lines[i]); prefix != "" && errorPrefix(lines[i-1]) == "" {
			msg := strings.TrimSuffix(strings.TrimPrefix(lines[i], prefix), "{{[-]}}")
			candidates[i] = candidate{plainW(lines[i-1]), utf8.RuneCountInString(msg)}
		}
	}
	col, best := 0, 0
	for _, c := range candidates {
		n := 0
		for _, o := range candidates {
			if o.rowW <= c.rowW && c.rowW+2+o.msgW <= width {
				n++
			}
		}
		if n > best || n == best && c.rowW < col {
			col, best = c.rowW, n
		}
	}

	termRoom := termW - timerReserveW - summaryHeaderIndentW - p.errorIndentW()
	room := max(width-p.errorIndentW(), min(errorWrapW, termRoom), minDetailW)
	outLines := make([]string, 0, len(lines))
	outTimers := make([]timerEntry, 0, len(timers))
	for i, line := range lines {
		prefix := errorPrefix(line)
		if prefix == "" {
			outLines = append(outLines, line)
			outTimers = append(outTimers, timers[i])
			continue
		}
		style := strings.TrimLeft(prefix, " ") // the message's style tag
		msg := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "{{[-]}}")
		if c, ok := candidates[i]; ok && best > 0 && c.rowW <= col && col+2+c.msgW <= width {
			row := outLines[len(outLines)-1]
			outLines[len(outLines)-1] = row + strutil.Repeat(" ", col-c.rowW+2) + style + msg + "{{[-]}}"
			continue
		}
		nested := len(prefix) - len(style) - p.errorIndentW() // a nested row's message starts that much further in
		for _, part := range wrapWords(msg, room-nested, maxErrorLines) {
			outLines = append(outLines, prefix+part+"{{[-]}}")
			outTimers = append(outTimers, timerEntry{})
		}
	}
	return outLines, outTimers
}

// wrapWords wraps s at spaces into lines of at most width runes, breaking a word longer
// than width, and returns at most maxLines of them, the last ending in … when more is cut.
func wrapWords(s string, width, maxLines int) []string {
	width = max(width, 2)
	var out []string
	line := ""
	flush := func() {
		out = append(out, line)
		line = ""
	}
	for _, word := range strings.Fields(s) {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				flush()
			}
			r := []rune(word)
			out = append(out, string(r[:width]))
			word = string(r[width:])
		}
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			flush()
			line = word
		}
	}
	if line != "" {
		flush()
	}
	if len(out) > maxLines {
		out = out[:maxLines]
		last := []rune(out[maxLines-1])
		if len(last)+2 <= width {
			out[maxLines-1] = string(last) + " …"
		} else {
			out[maxLines-1] = string(last[:width-1]) + "…"
		}
	}
	return out
}

// wrapStyled wraps semstyle-tagged text at spaces into lines of at most width visible
// runes, measuring each word without its tags (tags hold no spaces). A word longer than
// width breaks where the line runs out when it is plain text, and stays whole when it
// carries tags.
func wrapStyled(s string, width int) []string {
	width = max(width, 2)
	var out []string
	line, lineW := "", 0
	for _, word := range strings.Fields(s) {
		w := utf8.RuneCountInString(semstyle.ToPlain(word))
		for w > width && !strings.Contains(word, "{{") {
			if line != "" {
				out = append(out, line)
				line, lineW = "", 0
			}
			r := []rune(word)
			out = append(out, string(r[:width]))
			word = string(r[width:])
			w = len(r) - width
		}
		switch {
		case line == "":
			line, lineW = word, w
		case lineW+1+w <= width:
			line, lineW = line+" "+word, lineW+1+w
		default:
			out = append(out, line)
			line, lineW = word, w
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// hasConditionErr reports whether a row wasn't started because one of its own required
// dependencies' conditions wasn't met.
func (p *consoleEventProcessor) hasConditionErr() bool {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, t := range p.tasks {
		if t.conditionErr {
			return true
		}
	}
	return false
}

// causeIcon returns the warning icon in the cause marker's style (causeMarker), for a
// row whose own state is a warning but caused another row's failure, or "" for any other
// row.
func (p *consoleEventProcessor) causeIcon(t *consoleTask) string {
	if t == nil || !p.unhealthyHealthErr(t) {
		return ""
	}
	return p.causeMarker()
}

// causeMarker returns the warning icon in DockerMarkerCause (red), marking what caused
// another row's failure; under status, which reports and fails nothing, a plain warning.
func (p *consoleEventProcessor) causeMarker() string {
	if p.command == statusCommand {
		return "{{|DockerMarkerWarn|}}" + p.icons().warn + "{{[-]}}"
	}
	return "{{|DockerMarkerCause|}}" + p.icons().warn + "{{[-]}}"
}
