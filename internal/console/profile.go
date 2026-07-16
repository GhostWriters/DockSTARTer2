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

// AlignToRefreshRate rounds spinnerMs to the nearest multiple of refreshMs,
// so the spinner's tick interval stays in sync with the screen's repaint
// cadence while remaining as close as possible to the configured speed.
// Exact multiples are left unchanged. Returns spinnerMs unmodified if
// refreshMs is not positive.
func AlignToRefreshRate(spinnerMs, refreshMs int) int {
	if refreshMs <= 0 {
		return spinnerMs
	}
	aligned := ((spinnerMs + refreshMs/2) / refreshMs) * refreshMs
	if aligned <= 0 {
		aligned = refreshMs
	}
	return aligned
}

func init() {
	if stat, err := os.Stderr.Stat(); err == nil {
		isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0
	}

	// semstyle auto-detects the color profile at its own init (respecting
	// NO_COLOR, TTY status, etc.), but ToANSI renders unconditionally unless
	// a RenderPolicy is set -- without this, tags would still emit raw
	// escape-sequence garbage when output is redirected to a file or the
	// terminal can't handle color. IsTUIEnabled is exempted since bubbletea
	// manages its own output stream/profile independent of stdout/stderr.
	semstyle.RenderPolicy = func() bool {
		return IsTUIEnabled() || GetPreferredProfile() > colorprofile.Ascii
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

// IsStdinTTY reports whether stdin is a real terminal. Interactive prompts
// that read a reply must check this: stdin can be redirected (e.g.
// `cmd < file`, cron, CI) while both output streams remain TTYs.
func IsStdinTTY() bool {
	if stat, err := os.Stdin.Stat(); err == nil {
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
