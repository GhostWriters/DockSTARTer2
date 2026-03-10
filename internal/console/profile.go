package console

import (
	"os"

	"github.com/charmbracelet/colorprofile"
)

var (
	isTTYGlobal bool

	// preferredProfile stores the detected or forced color profile
	preferredProfile colorprofile.Profile

	// TUIMode indicates whether we're running in TUI mode (always render colors)
	TUIMode bool
)

func init() {
	if stat, err := os.Stderr.Stat(); err == nil {
		isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0
	}
	preferredProfile = colorprofile.Detect(os.Stderr, os.Environ())
}

// GetPreferredProfile returns the detected or forced color profile.
func GetPreferredProfile() colorprofile.Profile {
	return preferredProfile
}

// SetPreferredProfile explicitly sets the color profile (useful for testing).
func SetPreferredProfile(p colorprofile.Profile) {
	preferredProfile = p
}

// SetTTY allows forcing the TTY status.
// Returns the previous value so it can be restored.
func SetTTY(isTTY bool) bool {
	old := isTTYGlobal
	isTTYGlobal = isTTY
	return old
}
