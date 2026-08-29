package enveditor

import "testing"

func TestIsNewLineKey(t *testing.T) {
	m := New()
	m.ParseEnv("###\n### MyApp (User Defined)\n###\nMYAPP__ENABLED='true'\n", func(string) string { return "" }, nil)

	// Simulate the user navigating to the end and typing a brand-new var.
	m.row = len(m.value) - 1
	m.col = 0
	for _, r := range "TEST_VAR='hello'\n" {
		m.InsertRune(r)
	}

	if !m.IsNewLineKey("TEST_VAR") {
		t.Error("expected TEST_VAR (just typed by the user) to report IsNewLine")
	}
	if m.IsNewLineKey("MYAPP__ENABLED") {
		t.Error("expected MYAPP__ENABLED (pre-existing, loaded from the file) to not report IsNewLine")
	}
	if m.IsNewLineKey("NONEXISTENT_KEY") {
		t.Error("expected a key with no matching line to report false")
	}
}
