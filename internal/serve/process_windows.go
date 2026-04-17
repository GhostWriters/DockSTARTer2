//go:build windows

package serve

import (
	"os"
)

// processExists reports whether a process with the given PID is running.
func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess only finds the process if it exists.
	// A non-nil result with no error means the process is running.
	_ = p
	return true
}
