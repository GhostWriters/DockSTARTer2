package console

import (
	"os"
	"strings"

	"github.com/muesli/termenv"
)

var (
	isTTYGlobal bool

	// preferredProfile stores the detected or forced color profile
	preferredProfile termenv.Profile

	// TUIMode indicates whether we're running in TUI mode (always render colors)
	TUIMode bool
)

func init() {
	// Initialize TTY and Profile
	isTTYGlobal = false
	if stat, err := os.Stdout.Stat(); err == nil {
		isTTYGlobal = (stat.Mode() & os.ModeCharDevice) != 0
	}
	preferredProfile = detectProfile()
}

// GetPreferredProfile returns the detected or forced color profile
func GetPreferredProfile() termenv.Profile {
	return preferredProfile
}

// SetPreferredProfile explicitly sets the color profile (useful for testing)
func SetPreferredProfile(p termenv.Profile) {
	preferredProfile = p
}

// SetTTY allows forcing the TTY status (useful for testing ANSI output in non-interactive tests).
// Returns the previous value so it can be restored.
func SetTTY(isTTY bool) bool {
	old := isTTYGlobal
	isTTYGlobal = isTTY
	return old
}

// detectProfile determines the appropriate color profile based on environment variables.
// Priority: COLORTERM > TERM > automatic detection
func detectProfile() termenv.Profile {
	// 1. Check COLORTERM for explicit overrides
	colorTerm := strings.ToLower(os.Getenv("COLORTERM"))
	switch colorTerm {
	case "truecolor", "24bit":
		return termenv.TrueColor
	case "8bit", "256color":
		return termenv.ANSI256
	case "4bit", "16color", "8color", "3bit":
		return termenv.ANSI
	case "1bit", "2color", "mono", "false", "0":
		return termenv.Ascii
	}

	// 2. Check TERM for well-known color-capable terms
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "direct") {
		return termenv.TrueColor
	}
	if strings.Contains(term, "256color") {
		return termenv.ANSI256
	}
	if strings.Contains(term, "16color") {
		return termenv.ANSI
	}
	if term == "dumb" {
		return termenv.Ascii
	}

	// 3. Fallback to automatic detection
	return termenv.ColorProfile()
}
