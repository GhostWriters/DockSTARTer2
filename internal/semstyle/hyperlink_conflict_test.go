package semstyle

import (
	"strings"
	"testing"
	"github.com/charmbracelet/colorprofile"
)

func TestHyperlinkRegisteredAndExplicit(t *testing.T) {
	SetPreferredProfile(colorprofile.TrueColor)
	st := New()
	st.RegisterConsoleTag("mylink", "cyan::U")
	st.RegisterHyperlinkTag("mylink")

	// Bare registered form — URL is display text
	r1 := st.ToANSI(`{{|mylink|}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(r1, "\x1b]8;;") {
		t.Errorf("registered bare: expected OSC8, got %q", r1)
	}
	if !strings.Contains(r1, "https://dockstarter.com") {
		t.Errorf("registered bare: expected URL as label, got %q", r1)
	}

	// Explicit label form — overrides display text even though tag is registered
	r2 := st.ToANSI(`{{|mylink::::DockSTARTer Website|}}https://dockstarter.com{{[-]}}`)
	if !strings.Contains(r2, "\x1b]8;;https://dockstarter.com\x1b\\") {
		t.Errorf("explicit label: expected URL as destination, got %q", r2)
	}
	if !strings.Contains(r2, "DockSTARTer Website") {
		t.Errorf("explicit label: expected label as display, got %q", r2)
	}
	if strings.Contains(r2, "https://dockstarter.com\x1b]8;;") {
		t.Errorf("explicit label: URL should not appear as display text, got %q", r2)
	}
}
