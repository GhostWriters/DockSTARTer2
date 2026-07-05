package console

import (
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

func TestRenderPolicyStripsWhenProfileHasNoColor(t *testing.T) {
	origProfile := GetPreferredProfile()
	defer SetPreferredProfile(origProfile)
	SetTUIEnabled(false)
	defer SetTUIEnabled(false)

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
	defer SetPreferredProfile(origProfile)
	SetTUIEnabled(false)
	defer SetTUIEnabled(false)

	SetPreferredProfile(colorprofile.TrueColor)
	got := semstyle.ToANSI("{{|Notice|}}hello{{[-]}}")
	if !strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI escapes for TrueColor profile, got %q", got)
	}
}

func TestRenderPolicyAlwaysRendersWhenTUIEnabled(t *testing.T) {
	origProfile := GetPreferredProfile()
	defer SetPreferredProfile(origProfile)
	defer SetTUIEnabled(false)

	SetPreferredProfile(colorprofile.NoTTY)
	SetTUIEnabled(true)
	got := semstyle.ToANSI("{{|Notice|}}hello{{[-]}}")
	if !strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI escapes when TUI is enabled regardless of profile, got %q", got)
	}
}
