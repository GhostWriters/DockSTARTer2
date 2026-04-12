package console

import (
	"sync/atomic"
)

var tuiEnabled atomic.Bool
var tuiDying atomic.Bool

// IsTUIEnabled returns true if the TUI is currently active.
func IsTUIEnabled() bool {
	return tuiEnabled.Load()
}

// SetTUIEnabled updates the atomic TUI status.
func SetTUIEnabled(enabled bool) {
	tuiEnabled.Store(enabled)
}

// IsTUIDying returns true if the TUI has entered its emergency shutdown phase.
// This is used by the renderer goroutine to freeze output and prevent remnants.
func IsTUIDying() bool {
	return tuiDying.Load()
}

// SetTUIDying sets the atomic dying flag to freeze the TUI renderer.
func SetTUIDying(dying bool) {
	tuiDying.Store(dying)
}
