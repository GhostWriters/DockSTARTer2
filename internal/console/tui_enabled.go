package console

import "sync/atomic"

var tuiEnabled atomic.Bool

// IsTUIEnabled returns true if the application is currently running in TUI mode.
func IsTUIEnabled() bool {
	return tuiEnabled.Load()
}

// SetTUIEnabled sets whether the application is running in TUI mode.
func SetTUIEnabled(enabled bool) {
	tuiEnabled.Store(enabled)
}
