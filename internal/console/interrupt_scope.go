package console

import "sync"

var (
	interruptScopeMu sync.Mutex
	interruptScope   func()
)

// SetInterruptScope registers a handler that intercepts the next Ctrl+C
// instead of main's default "abort the whole process" behavior -- for
// commands like --logs -F where Ctrl+C should stop just that command's own
// long-running operation (a live-following log stream), not exit ds2
// entirely. Pass nil to clear it once the operation finishes, so a later
// Ctrl+C goes back to the default process-abort behavior.
func SetInterruptScope(handler func()) {
	interruptScopeMu.Lock()
	defer interruptScopeMu.Unlock()
	interruptScope = handler
}

// HandleScopedInterrupt calls the currently registered interrupt scope
// handler, if any, and reports whether one was active. main's SIGINT
// handler checks this first each time and skips its own default handling
// when it returns true.
func HandleScopedInterrupt() bool {
	interruptScopeMu.Lock()
	handler := interruptScope
	interruptScopeMu.Unlock()
	if handler == nil {
		return false
	}
	handler()
	return true
}
