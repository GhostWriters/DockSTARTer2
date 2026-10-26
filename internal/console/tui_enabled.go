package console

import (
	"sync/atomic"

	"github.com/GhostWriters/semstyle"
)

var tuiEnabled atomic.Bool
var tuiDying atomic.Bool

// IsTUIEnabled returns true if the TUI is currently active.
func IsTUIEnabled() bool {
	return tuiEnabled.Load()
}

// DeferTintsChangedHook is set by the display engine to clear its style
// caches when tints switch between deferred and direct (see SetTUIEnabled),
// since a cached style holds whichever form was current when it was built.
var DeferTintsChangedHook func()

// SetTUIEnabled updates the atomic TUI status. Tints are deferred while the
// TUI runs (see semstyle.SetDeferTints), so anything rendered for it, in any
// goroutine, carries tint placeholders that each frame resolves.
func SetTUIEnabled(enabled bool) {
	tuiEnabled.Store(enabled)
	if semstyle.DeferTints() == enabled {
		return
	}
	semstyle.SetDeferTints(enabled)
	if DeferTintsChangedHook != nil {
		DeferTintsChangedHook()
	}
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
