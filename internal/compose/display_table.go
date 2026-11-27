package compose

import (
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"

	"DockSTARTer2/internal/strutil"
)

const (
	// tableStatusW is the status cell's width: a line's marker and status, without
	// its indent or the gutter after the status.
	tableStatusW = sectionHeaderIndent - globalIndentW - sectionStatusGutterW
	// tableExtraW is the columns a table adds over the borderless layout, which
	// buildLines takes off the width its lines fit: -1, as the table drops the
	// summary indent, margin and gutter (6) and its borders and timer padding add 5.
	tableExtraW = -1
	// tableFrameW is a row's columns besides its detail and timer: the status cell,
	// the borders and the timer's padding.
	tableFrameW = tableStatusW + 6
)

// tableChars is the box-drawing characters of the line characters setting.
type tableChars struct {
	h, v, topLeft, topRight, bottomLeft, bottomRight, topJoin, bottomJoin, leftJoin, rightJoin, cross string
}

func (p *consoleEventProcessor) tableChars() tableChars {
	if p.asciiMode {
		return tableChars{"-", "|", "+", "+", "+", "+", "+", "+", "+", "+", "+"}
	}
	return tableChars{"─", "│", "┌", "┐", "└", "┘", "┬", "┴", "├", "┤", "┼"}
}

// tableLines returns the summary and lines in a table: the summary as heading rows
// spanning the status and detail cells (wrapHeading), then each line's status, detail and timer
// cells, closed by a bottom border. The status and detail cells have no padding
// beside their borders. The detail cell only widens during a run, so the borders
// don't move as lines come and go, unless the output narrows past it.
func (p *consoleEventProcessor) tableLines(summary string, lines []string, timers []timerEntry) []string {
	c := p.tableChars()
	plainW := func(s string) int { return utf8.RuneCountInString(semstyle.ToPlain(s)) }
	border := func(s string) string { return "{{|DockerBorder|}}" + s + "{{[-]}}" }
	rule := func(n int) string { return strutil.Repeat(c.h, n) }
	pad := func(s string, w int) string { return s + strutil.Repeat(" ", max(w-plainW(s), 0)) }
	unindent := func(s string) string { _, rest := splitAtColumn(s, globalIndentW); return rest }

	timerStrs, timerW := p.timerStrings(timers)
	statuses := make([]string, len(lines))
	details := make([]string, len(lines))
	detailW := 0
	for i, line := range lines {
		var rest string
		statuses[i], rest = splitAtColumn(unindent(line), tableStatusW)
		// Drop the gutter, keeping any tag beside it.
		gutter, detail := splitAtColumn(rest, sectionStatusGutterW)
		// Trailing spaces, padding a line out to a column it has nothing in, would
		// widen the cell.
		details[i] = trimTrailingSpaces(strings.Replace(gutter, " ", "", 1) + detail)
		w := plainW(details[i])
		if timers[i].rule {
			w += 1 + minRuleW
		}
		detailW = max(detailW, w)
	}
	room := p.buildW - timerW - tableFrameW
	// The heading spans both cells and the border between, wrapping to the wider of
	// the room and the lines.
	var heading []string
	if summary != "" {
		heading = wrapHeading(unindent(summary), max(detailW, room)+tableStatusW+1)
	}
	for _, h := range heading {
		detailW = max(detailW, plainW(h)-tableStatusW-1)
	}
	detailW = max(detailW, min(p.tableLineW, room))
	p.tableLineW = detailW
	p.maxLineWidth = tableStatusW + detailW
	spanW := tableStatusW + 1 + detailW
	timerRule := rule(timerW + 2)

	var out []string
	if len(heading) > 0 {
		out = append(out, border(c.topLeft+rule(spanW)+c.topJoin+timerRule+c.topRight))
		for i, h := range heading {
			timer := strutil.Repeat(" ", timerW)
			if i == 0 && !p.startTime.IsZero() {
				timer = p.summaryTimer()
			}
			out = append(out, border(c.v)+pad(h, spanW)+border(c.v)+" "+timer+" "+border(c.v))
		}
		if len(lines) == 0 {
			return append(out, border(c.bottomLeft+rule(spanW)+c.bottomJoin+timerRule+c.bottomRight))
		}
		out = append(out, border(c.leftJoin+rule(tableStatusW)+c.topJoin+rule(detailW)+c.cross+timerRule+c.rightJoin))
	} else {
		out = append(out, border(c.topLeft+rule(tableStatusW)+c.topJoin+rule(detailW)+c.topJoin+timerRule+c.topRight))
	}
	for i := range lines {
		detail := details[i]
		if timers[i].rule {
			detail += p.headingRule(detailW - plainW(detail))
		}
		timer := strutil.Repeat(" ", timerW)
		if timers[i].task != nil {
			s := timerStrs[i]
			timer = timerStyleTag(timers[i].style) + strutil.Repeat(" ", timerW-len(s)) + s + "{{[-]}}"
		}
		out = append(out, border(c.v)+pad(statuses[i], tableStatusW)+border(c.v)+pad(detail, detailW)+border(c.v)+" "+timer+" "+border(c.v))
	}
	return append(out, border(c.bottomLeft+rule(tableStatusW)+c.bottomJoin+rule(detailW)+c.bottomJoin+timerRule+c.bottomRight))
}

// wrapHeading wraps the summary into rows of at most width columns, breaking after
// the commas between its counts (never inside brackets), each later row starting
// under the first count. A count wider than a row stays whole.
func wrapHeading(s string, width int) []string {
	if utf8.RuneCountInString(semstyle.ToPlain(s)) <= width {
		return []string{s}
	}
	// The pieces end after each top-level ", "; indent is the column after ": ".
	var pieces []string
	indent, col, depth, start := -1, 0, 0, 0
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "{{") {
			if j := strings.Index(s[i+2:], "}}"); j >= 0 {
				i += j + 4
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ' ' && i > 0 && depth == 0:
			if prev := semstyle.ToPlain(s[:i]); strings.HasSuffix(prev, ",") {
				pieces = append(pieces, s[start:i])
				start = i + size
			} else if indent < 0 && strings.HasSuffix(prev, ":") {
				indent = col + 1
			}
		}
		i += size
		col++
	}
	pieces = append(pieces, s[start:])
	indent = max(indent, 0)

	var rows []string
	row := pieces[0]
	for _, piece := range pieces[1:] {
		if utf8.RuneCountInString(semstyle.ToPlain(row))+1+utf8.RuneCountInString(semstyle.ToPlain(piece)) <= width {
			row += " " + piece
			continue
		}
		rows = append(rows, row)
		row = strutil.Repeat(" ", indent) + piece
	}
	return append(rows, row)
}

// trimTrailingSpaces returns a tag string without its trailing spaces, keeping the
// tags among them.
func trimTrailingSpaces(s string) string {
	tags := ""
	for {
		if t := strings.TrimRight(s, " "); t != s {
			s = t
			continue
		}
		if strings.HasSuffix(s, "}}") {
			if i := strings.LastIndex(s, "{{"); i >= 0 {
				s, tags = s[:i], s[i:]+tags
				continue
			}
		}
		return s + tags
	}
}

// splitAtColumn splits a tag string at visible column col, a tag at the split going
// to the second part.
func splitAtColumn(s string, col int) (string, string) {
	n := 0
	for i := 0; i < len(s); {
		if n == col {
			return s[:i], s[i:]
		}
		if strings.HasPrefix(s[i:], "{{") {
			if j := strings.Index(s[i+2:], "}}"); j >= 0 {
				i += j + 4
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return s, ""
}
