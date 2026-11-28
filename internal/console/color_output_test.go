package console

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestToProfile(t *testing.T) {
	prev := GetPreferredProfile()
	defer SetPreferredProfile(prev)

	const truecolor = "\x1b[38;2;255;95;215mpink\x1b[m \x1b]8;;https://x\x07link\x1b]8;;\x07"
	tests := []struct {
		profile colorprofile.Profile
		want    string
	}{
		{colorprofile.TrueColor, truecolor},
		{colorprofile.ANSI256, "\x1b[38;5;206mpink\x1b[m \x1b]8;;https://x\x07link\x1b]8;;\x07"},
		{colorprofile.NoTTY, truecolor},
	}
	for _, tt := range tests {
		SetPreferredProfile(tt.profile)
		if got := ToProfile(truecolor); got != tt.want {
			t.Errorf("ToProfile under %s = %q, want %q", tt.profile, got, tt.want)
		}
	}

	SetPreferredProfile(colorprofile.ANSI256)
	if got := ToProfile("plain"); got != "plain" {
		t.Errorf("ToProfile(plain) = %q", got)
	}

	SetTUIEnabled(true)
	defer SetTUIEnabled(false)
	if got := ToProfile(truecolor); got != truecolor {
		t.Errorf("ToProfile with the TUI running = %q, want it unchanged", got)
	}
}
