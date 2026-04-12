package console

import "sync/atomic"

var tuiEnabled atomic.Bool
var tuiDying atomic.Bool

// IsTUIEnabled returns true if the application is currently running in TUI mode.
func IsTUIEnabled() bool {
	return tuiEnabled.Load()
}

// SetTUIEnabled sets whether the application is running in TUI mode.
func SetTUIEnabled(enabled bool) {
	tuiEnabled.Store(enabled)
}

// IsTUIDying returns true if the TUI is currently shutting down due to a panic.
func IsTUIDying() bool {
	return tuiDying.Load()
}

// SetTUIDying sets whether the TUI is currently shutting down.
func SetTUIDying(dying bool) {
	tuiDying.Store(dying)
}
