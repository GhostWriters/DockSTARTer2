package compose

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
)

// namesStart returns the visible column where name starts on line, -1 when absent.
func namesStart(line, name string) int {
	plain := semstyle.ToPlain(line)
	i := strings.Index(plain, name)
	if i < 0 {
		return -1
	}
	return utf8.RuneCountInString(plain[:i])
}

func TestAlignNamesLinesUpEveryLabelsNames(t *testing.T) {
	p := newGroupsTestProcessor("up")
	lines := p.groupLines(api.StatusRunning, false, []string{"web"}, "service", "services", 160)
	lines = append(lines, p.groupLines("Existing", false, []string{"compose_default"}, "network", "networks", 160)...)
	lines = append(lines, p.groupLines("Existing", false, []string{"ds2-llm"}, "model", "models", 160)...)
	lines = append(lines, nestLine(p.groupLines("Scheduled", false, []string{"ds2-tick"}, "job", "jobs", 160)[0]))
	got, timers := alignNames(lines, make([]timerEntry, len(lines)), 160)
	if len(got) != len(lines) || len(timers) != len(got) {
		t.Fatalf("got %d lines, %d timers, want %d each", len(got), len(timers), len(lines))
	}
	want := namesStart(got[1], "compose_default")
	for i, name := range []string{"web", "compose_default", "ds2-llm", "ds2-tick"} {
		if col := namesStart(got[i], name); col != want {
			t.Errorf("%s starts at column %d, want %d:\n%s", name, col, want, strings.Join(plainLines(got), "\n"))
		}
	}
}

func TestAlignNamesRewrapsAFoldedGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	var names []string
	for _, n := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett", "kilo", "lima"} {
		names = append(names, n+"-service")
	}
	const termW = 80
	lines := p.groupLines(api.StatusRunning, false, []string{"web"}, "service", "services", termW)
	lines = append(lines, nestLine(p.groupLines("Existing", false, []string{"compose_default"}, "network", "networks", termW)[0]))
	lines = append(lines, p.groupLines(api.StatusRunning, false, names, "service", "services", termW)...)
	got, timers := alignNames(lines, make([]timerEntry, len(lines)), termW)
	if len(timers) != len(got) {
		t.Fatalf("got %d timers for %d lines", len(timers), len(got))
	}
	want := namesStart(got[1], "compose_default")
	if col := namesStart(got[0], "web"); col != want {
		t.Errorf("web starts at column %d, want %d", col, want)
	}
	room := termW - timerReserveW - summaryHeaderIndentW
	seen := 0
	for _, line := range got[2:] {
		plain := semstyle.ToPlain(line)
		if w := utf8.RuneCountInString(plain); w > room {
			t.Errorf("line is %d wide, past %d: %q", w, room, plain)
		}
		runes := []rune(plain)
		if len(runes) <= want || runes[want-1] != ' ' || runes[want] == ' ' {
			t.Errorf("names don't start at column %d: %q", want, plain)
		}
		seen += strings.Count(plain, "-service")
	}
	if seen != len(names) {
		t.Errorf("rewrapped lines hold %d names, want %d", seen, len(names))
	}
}
