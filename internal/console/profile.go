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

	// RefreshRate is the screen repaint interval in milliseconds (default
	// 60; overwritten from config before any real use) -- the baseline
	// AlignToRefreshRate aligns other periodic UI speeds against, so their
	// state changes always land on an actual repaint instead of getting
	// stranded between two out-of-sync clocks until the next unrelated one.
	RefreshRate int = 60

	// HyperlinksMode caches ui.hyperlinks ("off"/"inline"/"auto"; default
	// "inline", overwritten from config before any real use). Consulted by
	// semstyle.HyperlinkModeFunc (wired below) -- must stay a cheap cached
	// read, not a live config.LoadAppConfig() call: that hook fires once per
	// hyperlink tag rendered, including tags emitted from inside
	// LoadAppConfig()'s own no-config-file bootstrap path (FormatFolderPath
	// et al.), so calling LoadAppConfig() from the hook would recursively
	// re-enter it every time a bootstrap message renders.
	HyperlinksMode string = "inline"
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

	// semstyle's FormatFilePath/FormatFolderPath/etc. (the app-agnostic path-segmenting
	// logic, moved there since it has nothing DS2-specific in it) consult this hook to
	// decide whether including a file:// URL is safe -- only DS2 knows about its own
	// SSH/web server and same-machine detection, so that gating logic (blocksHyperlink)
	// stays local and is wired in here rather than moving too.
	semstyle.HyperlinkEligibleFunc = func() bool {
		return !blocksHyperlink()
	}

	// Reads the cached HyperlinksMode var, not config.LoadAppConfig() -- see
	// HyperlinksMode's doc comment for why a live config read here is unsafe.
	semstyle.HyperlinkModeFunc = func() semstyle.HyperlinkMode {
		switch HyperlinksMode {
		case "off":
			return semstyle.HyperlinkModeOff
		case "auto":
			return semstyle.HyperlinkModeAuto
		default:
			return semstyle.HyperlinkModeInline
		}
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
