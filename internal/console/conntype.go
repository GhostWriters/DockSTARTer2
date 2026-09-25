package console

import "sync"

// ConnTypeDisplay is one connection type's runtime display settings.
type ConnTypeDisplay struct {
	LineCharacters bool
	Spinner        bool
	SpinnerSpeed   int    // milliseconds per spinner frame
	RefreshRate    int    // screen repaint interval in milliseconds
	Hyperlinks     string // "off", "inline", or "auto"
}

// defaultConnTypeDisplay is used for a connType with nothing recorded yet.
var defaultConnTypeDisplay = ConnTypeDisplay{SpinnerSpeed: 100, RefreshRate: 60, Hyperlinks: "inline"}

// connType-scoped display settings, and the connType whose session is
// rendering right now.
var (
	connTypeMu     sync.RWMutex
	activeConnType = "local"
	displayByType  = map[string]ConnTypeDisplay{}
)

// SetConnTypeDisplay records connType's display settings.
func SetConnTypeDisplay(connType string, d ConnTypeDisplay) {
	connTypeMu.Lock()
	defer connTypeMu.Unlock()
	displayByType[connType] = d
}

// DisplayFor returns connType's display settings.
func DisplayFor(connType string) ConnTypeDisplay {
	connTypeMu.RLock()
	defer connTypeMu.RUnlock()
	if d, ok := displayByType[connType]; ok {
		return d
	}
	return defaultConnTypeDisplay
}

// LineCharactersFor reports whether connType draws unicode line/box-drawing
// characters.
func LineCharactersFor(connType string) bool {
	return DisplayFor(connType).LineCharacters
}

// SpinnerEnabledFor reports whether connType shows spinners during tasks.
func SpinnerEnabledFor(connType string) bool {
	return DisplayFor(connType).Spinner
}

// SpinnerSpeedFor returns connType's milliseconds per spinner frame, aligned
// to its refresh rate (see AlignToRefreshRate).
func SpinnerSpeedFor(connType string) int {
	d := DisplayFor(connType)
	return AlignToRefreshRate(d.SpinnerSpeed, d.RefreshRate)
}

// RefreshRateFor returns connType's screen repaint interval in milliseconds.
func RefreshRateFor(connType string) int {
	return DisplayFor(connType).RefreshRate
}

// HyperlinksModeFor returns connType's hyperlinks mode.
func HyperlinksModeFor(connType string) string {
	return DisplayFor(connType).Hyperlinks
}

// LineCharacters reports whether the rendering connType (see ActiveConnType)
// draws unicode line/box-drawing characters.
func LineCharacters() bool {
	return LineCharactersFor(ActiveConnType())
}

// SpinnerEnabled reports whether the rendering connType (see ActiveConnType)
// shows spinners during tasks.
func SpinnerEnabled() bool {
	return SpinnerEnabledFor(ActiveConnType())
}

// SpinnerSpeed returns the rendering connType's (see ActiveConnType)
// milliseconds per spinner frame.
func SpinnerSpeed() int {
	return SpinnerSpeedFor(ActiveConnType())
}

// RefreshRate returns the rendering connType's (see ActiveConnType) screen
// repaint interval in milliseconds.
func RefreshRate() int {
	return RefreshRateFor(ActiveConnType())
}

// HyperlinksMode returns the rendering connType's (see ActiveConnType)
// hyperlinks mode.
func HyperlinksMode() string {
	return HyperlinksModeFor(ActiveConnType())
}

// ActiveConnType returns the connType whose session is rendering right now
// ("local" outside any session render pass).
func ActiveConnType() string {
	connTypeMu.RLock()
	defer connTypeMu.RUnlock()
	return activeConnType
}

// SetActiveConnType makes connType the rendering connType and returns a
// function restoring the previous one. Callers must hold semstyle's render
// scope lock (see ActivateTintFor) so concurrent sessions never interleave.
func SetActiveConnType(connType string) (restore func()) {
	connTypeMu.Lock()
	prev := activeConnType
	activeConnType = connType
	connTypeMu.Unlock()
	return func() {
		connTypeMu.Lock()
		activeConnType = prev
		connTypeMu.Unlock()
	}
}
