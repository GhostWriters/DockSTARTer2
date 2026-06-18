package console

import (
	"os"

	"github.com/charmbracelet/colorprofile"

	"DockSTARTer2/internal/semstyle"
)

var (
	isTTYGlobal bool

	// TUIMode indicates whether we're running in TUI mode (always render colors)
	TUIMode bool

	// LineCharacters indicates whether unicode line/box-drawing characters are enabled.
	LineCharacters bool

	// SpinnerEnabled controls whether the CLI spinner is shown during tasks.
	SpinnerEnabled bool

	// SpinnerSpeed is the milliseconds per CLI spinner frame (default 250).
	SpinnerSpeed int = 250
)

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
