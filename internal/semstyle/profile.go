package semstyle

import (
	"os"

	"github.com/charmbracelet/colorprofile"
)

// preferredProfile stores the detected or forced color profile used when rendering
// extended/hex colors. Detected from stderr at init; can be overridden via SetPreferredProfile.
var preferredProfile colorprofile.Profile

func init() {
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
