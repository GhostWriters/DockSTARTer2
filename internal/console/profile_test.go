package console

import (
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

func TestRenderPolicyStripsWhenProfileHasNoColor(t *testing.T) {
	origProfile := GetPreferredProfile()
	origTUIMode := TUIMode
	defer func() {
		SetPreferredProfile(origProfile)
		TUIMode = origTUIMode
	}()
	TUIMode = false

	SetPreferredProfile(colorprofile.NoTTY)
	got := semstyle.ToANSI("{{|Notice|}}hello{{[-]}}")
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected no ANSI escapes for NoTTY profile, got %q", got)
	}
	if got != "hello" {
		t.Errorf("expected plain text for NoTTY profile, got %q", got)
	}
}

func TestRenderPolicyRendersWhenProfileHasColor(t *testing.T) {
	origProfile := GetPreferredProfile()
	origTUIMode := TUIMode
	defer func() {
		SetPreferredProfile(origProfile)
		TUIMode = origTUIMode
	}()
	TUIMode = false

	SetPreferredProfile(colorprofile.TrueColor)
	got := semstyle.ToANSI("{{|Notice|}}hello{{[-]}}")
	if !strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI escapes for TrueColor profile, got %q", got)
	}
}

func TestRenderPolicyAlwaysRendersInTUIMode(t *testing.T) {
	origProfile := GetPreferredProfile()
	origTUIMode := TUIMode
	defer func() {
		SetPreferredProfile(origProfile)
		TUIMode = origTUIMode
	}()

	SetPreferredProfile(colorprofile.NoTTY)
	TUIMode = true
	got := semstyle.ToANSI("{{|Notice|}}hello{{[-]}}")
	if !strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI escapes when TUIMode is true regardless of profile, got %q", got)
	}
}
