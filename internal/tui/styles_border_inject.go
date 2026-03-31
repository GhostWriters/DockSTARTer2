package tui

import (
	"regexp"
	"strings"

	"DockSTARTer2/internal/theme"
)

// ansiSeqRe matches ANSI SGR escape sequences (e.g. \x1b[1m, \x1b[38;5;15m).
var ansiSeqRe = regexp.MustCompile(`\x1b\[[\d;]*m`)

// ansiCodeFromFlags converts a StyleFlags struct into a single ANSI escape sequence string.
// Returns "" if no flags are set.
func ansiCodeFromFlags(f theme.StyleFlags) string {
	var codes []string
	if f.Bold          { codes = append(codes, "1") }
	if f.Dim           { codes = append(codes, "2") }
	if f.Italic        { codes = append(codes, "3") }
	if f.Underline     { codes = append(codes, "4") }
	if f.Blink         { codes = append(codes, "5") }
	if f.Reverse       { codes = append(codes, "7") }
	if f.Strikethrough { codes = append(codes, "9") }
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// injectBeforeFirstAnsi inserts code immediately before the first ANSI escape in line.
// If no ANSI escape exists, the code is prepended at the start.
func injectBeforeFirstAnsi(line, code string) string {
	if code == "" {
		return line
	}
	loc := ansiSeqRe.FindStringIndex(line)
	if loc == nil {
		return code + line
	}
	return line[:loc[0]] + code + line[loc[0]:]
}

// injectBeforeLastColorAnsi inserts code immediately before the last non-reset ANSI sequence in line.
// Non-reset means any sequence other than \x1b[m or \x1b[0m (which clear attributes).
// Injecting directly before (not before any preceding resets) ensures the attribute
// is not cleared before it reaches the right border character.
func injectBeforeLastColorAnsi(line, code string) string {
	if code == "" {
		return line
	}
	locs := ansiSeqRe.FindAllStringIndex(line, -1)
	if len(locs) == 0 {
		return line
	}
	for i := len(locs) - 1; i >= 0; i-- {
		seq := line[locs[i][0]:locs[i][1]]
		if seq != "\x1b[m" && seq != "\x1b[0m" {
			pos := locs[i][0]
			return line[:pos] + code + line[pos:]
		}
	}
	return line
}

// InjectBorderFlags applies ANSI attribute codes derived from flags to the border characters
// of an already-rendered lipgloss bordered string, without affecting the content area.
//
// flags  — applied to the top border row and the left border character on each middle row.
// flags2 — applied to the right border character on each middle row.
//           If hasBottom is true, also applied to the bottom border row.
//
// Set hasBottom=false when the bottom border was removed (e.g. via BorderBottom(false))
// and will be appended separately; the bottom builders apply flags2 themselves.
func InjectBorderFlags(rendered string, flags, flags2 theme.StyleFlags, hasBottom bool) string {
	flagCode := ansiCodeFromFlags(flags)
	flag2Code := ansiCodeFromFlags(flags2)
	if flagCode == "" && flag2Code == "" {
		return rendered
	}

	lines := strings.Split(rendered, "\n")
	n := len(lines)
	// split may produce a trailing empty string; find the real last index
	end := n
	if end > 0 && lines[end-1] == "" {
		end--
	}
	if end == 0 {
		return rendered
	}

	for i := 0; i < end; i++ {
		switch {
		case i == 0:
			// Top border — inject flags at the start of the line
			lines[i] = injectBeforeFirstAnsi(lines[i], flagCode)
		case hasBottom && i == end-1:
			// Bottom border — inject flags2 at the start of the line
			lines[i] = injectBeforeFirstAnsi(lines[i], flag2Code)
		default:
			// Middle line — left border gets flagCode, right border gets flag2Code.
			// Apply left injection first, then right (order matters for index positions).
			lines[i] = injectBeforeLastColorAnsi(injectBeforeFirstAnsi(lines[i], flagCode), flag2Code)
		}
	}
	return strings.Join(lines, "\n")
}
