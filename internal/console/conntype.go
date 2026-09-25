package console

import "sync"

// connType-scoped display flags, and the connType whose session is
// rendering right now.
var (
	connTypeMu      sync.RWMutex
	activeConnType  = "local"
	lineCharsByType = map[string]bool{}
	spinnerOnByType = map[string]bool{}
)

// SetConnTypeDisplay records connType's line_characters and spinner settings.
func SetConnTypeDisplay(connType string, lineCharacters, spinner bool) {
	connTypeMu.Lock()
	defer connTypeMu.Unlock()
	lineCharsByType[connType] = lineCharacters
	spinnerOnByType[connType] = spinner
}

// LineCharactersFor reports whether connType draws unicode line/box-drawing
// characters.
func LineCharactersFor(connType string) bool {
	connTypeMu.RLock()
	defer connTypeMu.RUnlock()
	return lineCharsByType[connType]
}

// SpinnerEnabledFor reports whether connType shows spinners during tasks.
func SpinnerEnabledFor(connType string) bool {
	connTypeMu.RLock()
	defer connTypeMu.RUnlock()
	return spinnerOnByType[connType]
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
