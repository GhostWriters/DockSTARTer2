package console

import (
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// ToProfile converts the colors in s, ANSI text bound for this process's own
// terminal, to the detected color profile (see GetPreferredProfile), the way
// a Bubble Tea program converts what it draws, so direct output and Bubble
// Tea's look the same. Only SGR sequences change. TrueColor and the no-color
// profiles leave s as is (RenderPolicy already keeps colors out of the
// latter), and so does a running TUI: its output reaches each session's
// screen through Bubble Tea, which converts for that session.
func ToProfile(s string) string {
	p := GetPreferredProfile()
	if p != colorprofile.ANSI && p != colorprofile.ANSI256 || IsTUIEnabled() || !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	w := colorprofile.Writer{Forward: &b, Profile: p}
	_, _ = w.WriteString(s)
	return b.String()
}

// ForTerminal readies ANSI text for this process's own terminal: its tint
// placeholders filled in (see ResolveTerminalTints), then its colors
// converted to the detected profile (see ToProfile).
func ForTerminal(s string) string {
	return ToProfile(ResolveTerminalTints(s))
}
