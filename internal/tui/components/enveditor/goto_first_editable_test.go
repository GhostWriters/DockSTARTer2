package enveditor

import "testing"

// TestGotoFirstEditable_SkipsSpacerBlankLine verifies that a plain spacer
// blank line between comment blocks (not IsVariable, not IsUserDefined) is
// skipped in favor of the first real variable line, rather than landing the
// cursor on the spacer just because it isn't ReadOnly.
func TestGotoFirstEditable_SkipsSpacerBlankLine(t *testing.T) {
	m := New()
	m.ParseEnv("### File: /path/.env\n\n###\n### DoplarrRS\n###\nDOPLARRRS__ENABLED='true'\n", func(string) string { return "" }, nil)

	if got := string(m.value[m.row]); got != "DOPLARRRS__ENABLED='true'" {
		t.Errorf("expected cursor on the variable line, got row %d: %q", m.row, got)
	}
}

// TestGotoFirstEditable_FallsBackToLastLine verifies that when nothing in
// the buffer is a real editing target, the cursor lands on the last line
// rather than the first.
func TestGotoFirstEditable_FallsBackToLastLine(t *testing.T) {
	m := New()
	m.ParseEnv("### File: /path/.env\n\n###\n### DoplarrRS\n###\n", func(string) string { return "" }, nil)

	if want := len(m.value) - 1; m.row != want {
		t.Errorf("expected cursor on the last line (row %d), got row %d", want, m.row)
	}
}
