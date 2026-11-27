package compose

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
)

// tableTestLines returns a table's lines as plain text, failing unless every line is
// one width and each row has its borders in place.
func tableTestLines(t *testing.T, p *consoleEventProcessor, summary string, lines []string, timers []timerEntry) []string {
	t.Helper()
	if p.buildW == 0 {
		p.buildW = logWidth // the width buildLines would build at
	}
	c := p.tableChars()
	var out []string
	width := -1
	for i, l := range p.tableLines(summary, lines, timers) {
		plain := semstyle.ToPlain(l)
		out = append(out, plain)
		w := utf8.RuneCountInString(plain)
		if width < 0 {
			width = w
		} else if w != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, w, width, plain)
		}
		if r := []rune(plain); i > 0 && i < len(out)-1 && string(r[0]) != c.v && string(r[0]) != c.leftJoin {
			t.Errorf("line %d has no left border: %q", i, plain)
		}
	}
	return out
}

func TestTableLines(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		p := NewConsoleEventProcessor(context.Background(), nil, "up", map[string][]string{}, nil, map[string]string{}, "proj", ascii, 0, true, nil, nil).(*consoleEventProcessor)
		task := &consoleTask{startTime: time.Now().Add(-2 * time.Second), endTime: time.Now(), status: api.Done}
		summary := " ✓ docker compose up: a fairly long summary line naming everything"
		lines := []string{
			" ✓ Complete     ─ changed",
			" ✓ Started        1 service: app",
			" i Info              version: 1.0",
			"",
		}
		timers := []timerEntry{{task: task, style: timerGroup, rule: true}, {task: task, style: timerService}, {}, {}}
		out := tableTestLines(t, p, summary, lines, timers)
		if len(out) != len(lines)+4 {
			t.Fatalf("ascii=%v: %d lines, want %d", ascii, len(out), len(lines)+4)
		}
		c := p.tableChars()
		if !strings.Contains(out[1], "docker compose up") {
			t.Errorf("ascii=%v: heading row = %q", ascii, out[1])
		}
		if !strings.HasPrefix(out[2], c.leftJoin) || !strings.Contains(out[2], c.cross) {
			t.Errorf("ascii=%v: separator = %q", ascii, out[2])
		}
		if r := []rune(out[3]); strings.Count(out[3], c.v) != 4 || string(r[tableStatusW+1]) != c.v || string(r[1]) == " " || string(r[tableStatusW+2]) == " " {
			t.Errorf("ascii=%v: row %q, want status and detail cells with no padding beside their borders", ascii, out[3])
		}
		if want := c.v + "✓ Started     " + c.v + "  1 service: app"; !strings.HasPrefix(out[4], want) {
			t.Errorf("ascii=%v: row %q, want it to start %q", ascii, out[4], want)
		}
	}
}

func TestWrapHeading(t *testing.T) {
	s := "✓ {{[yellow::B]}}status{{[-]}}{{|DockerColon|}}:{{[-]}} {{|DockerSection|}}18 services{{[-]}} ({{|DockerImage|}}17 images{{[-]}}, 162 layers), 1 model, 1 job"
	if got := wrapHeading(s, 100); len(got) != 1 || got[0] != s {
		t.Errorf("wrapHeading at 100 = %q, want it whole", got)
	}
	var got []string
	for _, l := range wrapHeading(s, 46) {
		got = append(got, semstyle.ToPlain(l))
	}
	want := []string{"✓ status: 18 services (17 images, 162 layers),", "          1 model, 1 job"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrapHeading at 46 = %q, want %q (never inside the brackets)", got, want)
	}
	if got := plainLines(wrapHeading(s, 20)); got[0] != "✓ status: 18 services (17 images, 162 layers)," {
		t.Errorf("wrapHeading at 20 starts %q, want a count wider than a row whole", got[0])
	}
}

func TestNarrowTableWrapsItsHeading(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.table = true
	for _, l := range plainLines(p.buildLines(60, 0)) {
		if w := utf8.RuneCountInString(l); w > 60 {
			t.Errorf("line %d wide at 60: %q", w, l)
		}
	}
}

func TestTrimTrailingSpaces(t *testing.T) {
	for in, want := range map[string]string{
		"image: x   ":                      "image: x",
		"{{|App|}}image{{[-]}}  {{[-]}}  ": "{{|App|}}image{{[-]}}{{[-]}}",
		"   ":                              "",
		"a b":                              "a b",
	} {
		if got := trimTrailingSpaces(in); got != want {
			t.Errorf("trimTrailingSpaces(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitAtColumn(t *testing.T) {
	line := "{{|DockerMarkerDone|}}✓{{[-]}} {{|DockerStatusDone|}}Started{{[-]}}     detail{{[-]}}"
	head, tail := splitAtColumn(line, tableStatusW)
	if got := semstyle.ToPlain(head); got != "✓ Started     " {
		t.Errorf("head = %q", got)
	}
	if got := semstyle.ToPlain(tail); got != "detail" {
		t.Errorf("tail = %q", got)
	}
	if head, tail := splitAtColumn("short", tableStatusW); head != "short" || tail != "" {
		t.Errorf("short line split into %q, %q", head, tail)
	}
}

func TestLogIsATable(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.table = true
	p.On(containerEvent("db", "Recreated", api.Done))
	p.tableLineW = 2 * logWidth // a wide terminal's table
	p.asciiMode = true          // the session's setting doesn't choose the log's copies
	built, builtASCII := p.logLines()
	lines, ascii := plainLines(built), plainLines(builtASCII)
	if !strings.HasPrefix(lines[0], "┌") {
		t.Fatalf("log starts %q, want a table with line characters", lines[0])
	}
	if !strings.HasPrefix(ascii[0], "+-") || len(ascii) != len(lines) || strings.ContainsAny(strings.Join(ascii, ""), "─│┌✓") {
		t.Errorf("ASCII copy =\n%s\nwant the same %d lines without line characters", strings.Join(ascii, "\n"), len(lines))
	}
	if !p.asciiMode {
		t.Error("asciiMode not restored after the log")
	}
	for i, l := range lines {
		if w := utf8.RuneCountInString(l); w > logWidth || w != utf8.RuneCountInString(lines[0]) {
			t.Errorf("log line %d is %d wide, want one width, at most %d: %q", i, w, logWidth, l)
		}
	}
	if p.tableLineW != 2*logWidth {
		t.Errorf("tableLineW = %d after the log, want the live table's %d", p.tableLineW, 2*logWidth)
	}
}

func TestTableOnlyWidens(t *testing.T) {
	p := NewConsoleEventProcessor(context.Background(), nil, "up", map[string][]string{}, nil, map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	wide := tableTestLines(t, p, " ✓ heading", []string{" ✓ Started        a much longer line than the next"}, []timerEntry{{}})
	narrow := tableTestLines(t, p, " ✓ heading", []string{" ✓ Started        short"}, []timerEntry{{}})
	if w, n := utf8.RuneCountInString(wide[0]), utf8.RuneCountInString(narrow[0]); n != w {
		t.Errorf("table narrowed from %d to %d when its widest line went", w, n)
	}
}
