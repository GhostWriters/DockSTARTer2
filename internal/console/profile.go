package console

import (
	"os"

	"github.com/charmbracelet/colorprofile"

	"github.com/GhostWriters/semstyle"
)

var (
	isTTYGlobal bool

	// TUIMode indicates whether we're running in TUI mode (always render colors)
	TUIMode bool

	// LineCharacters indicates whether unicode line/box-drawing characters are enabled.
	LineCharacters bool

	// SpinnerEnabled controls whether the CLI spinner is shown during tasks.
	SpinnerEnabled bool

	// SpinnerSpeed is the milliseconds per CLI spinner frame (default 120;
	// overwritten from config before any real use).
	SpinnerSpeed int = 100
)

// AlignToRefreshRate rounds spinnerMs up to the nearest multiple of refreshMs,
// so the spinner's tick interval never falls out of sync with the screen's
// repaint cadence. Exact multiples are left unchanged. Returns spinnerMs
// unmodified if refreshMs is not positive.
func AlignToRefreshRate(spinnerMs, refreshMs int) int {
	if refreshMs <= 0 {
		return spinnerMs
	}
	return ((spinnerMs + refreshMs - 1) / refreshMs) * refreshMs
}

func init() {
	if stat, err := os.Stderr.Stat(); err == nil {
		isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0
	}
}

// GetPreferredProfile returns the detected or forced color profile.
// Re-exported from the styling engine (semstyle owns the profile state).
func GetPreferredProfile() colorprofile.Profile {
	return semstyle.GetPreferredProfile()
}

// SetPreferredProfile explicitly sets the color profile (useful for testing).
func SetPreferredProfile(p colorprofile.Profile) {
	semstyle.SetPreferredProfile(p)
}

// IsTTY reports whether stderr is a real terminal.
func IsTTY() bool {
	return isTTYGlobal
}

// IsStdoutTTY reports whether stdout is a real terminal. Distinct from IsTTY (stderr):
// stdout can be redirected (e.g. `cmd > file`) while stderr stays a TTY, so output
// destined for stdout must check this rather than IsTTY.
func IsStdoutTTY() bool {
	if stat, err := os.Stdout.Stat(); err == nil {
		return (stat.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// SetTTY allows forcing the TTY status.
// Returns the previous value so it can be restored.
func SetTTY(isTTY bool) bool {
	old := isTTYGlobal
	isTTYGlobal = isTTY
	return old
}
