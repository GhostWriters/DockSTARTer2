package enveditor

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestRenderRunes_SyntheticWrapSpaceNotStyled verifies that wrap()'s
// synthetic trailing space (appended to the last wrapped row of every
// logical line as cursor-navigation bookkeeping, not real buffer content)
// never picks up content-specific styling like CommentText -- visible as a
// stray highlighted box past the actual text whenever a theme gives that
// style a distinct background (e.g. a Reverse-flagged comment style).
func TestRenderRunes_SyntheticWrapSpaceNotStyled(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.ParseEnv("### Immich\nFOO=bar\n", func(string) string { return "" }, nil)

	// A Reverse-flagged style makes the bug visible in the raw ANSI output:
	// SGR 7 (reverse) turns on, and if the synthetic space were still
	// included it would appear inside that same styled span before reset.
	reverse := lipgloss.NewStyle().Reverse(true)
	m.styles.Focused.CommentText = reverse
	m.styles.Blurred.CommentText = reverse

	out := m.View()

	// "### Immich" is exactly 10 runes; the real content ends there. The
	// reverse-styled span (opened by SGR 7, "\x1b[7m") must close with a
	// reset ("\x1b[m") immediately after "Immich", not after a trailing
	// space.
	idx := strings.Index(out, "### Immich")
	if idx == -1 {
		t.Fatalf("expected rendered output to contain the comment text, got: %q", out)
	}
	afterText := out[idx+len("### Immich"):]
	if !strings.HasPrefix(afterText, "\x1b[m") && !strings.HasPrefix(afterText, "\x1b[0m") {
		t.Errorf("expected a style reset immediately after \"### Immich\" (no styled trailing space), got: %q", afterText[:min(20, len(afterText))])
	}
}

// TestRenderRunes_BlankUserDefinedLineNotStyled covers the same invariant
// for a second case: wrap() appends its synthetic space even to a
// completely empty line, so a blank line's single wrapped row is one
// synthetic space, not truly empty. Inside a "(User Defined Variables)"
// section such a line is classified IsUserDefined (see ParseEnv), which
// would otherwise treat that space as unclosed "key" content (no "=" yet)
// and style it as modified -- a stray highlighted character on every blank
// line in that section, regardless of cursor position.
func TestRenderRunes_BlankUserDefinedLineNotStyled(t *testing.T) {
	m := New()
	m.SetWidth(40)
	// No actual variable line -- isolates the blank-line bug from the
	// legitimate reverse-highlighting a real typed key would also get.
	m.ParseEnv("### MyApp (User Defined Variables)\n###\n\n\n", func(string) string { return "" }, nil)

	reverse := lipgloss.NewStyle().Reverse(true)
	m.styles.Focused.ModifiedText = reverse
	m.styles.Blurred.ModifiedText = reverse

	out := m.View()

	if strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected no reverse-styled (modified) span on any blank line, got: %q", out)
	}
}
