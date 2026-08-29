package console

import (
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

func TestAlignToRefreshRate(t *testing.T) {
	tests := []struct {
		name                     string
		speedMs, refreshMs, want int
	}{
		{"exact multiple is unchanged", 480, 60, 480},
		{"rounds up to nearest multiple", 520, 60, 540}, // 520 is closer to 540 than 480
		{"rounds down to nearest multiple", 490, 60, 480},
		{"non-positive refresh rate leaves speed unmodified", 100, 0, 100},
		{"negative refresh rate leaves speed unmodified", 100, -1, 100},
		{"result never rounds down to zero -- floors at refreshMs", 20, 60, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AlignToRefreshRate(tt.speedMs, tt.refreshMs); got != tt.want {
				t.Errorf("AlignToRefreshRate(%d, %d) = %d, want %d", tt.speedMs, tt.refreshMs, got, tt.want)
			}
		})
	}
}

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
