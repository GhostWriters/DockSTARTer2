//go:build !windows

package serve

import (
	"os"
	"syscall"
)

// processExists reports whether a process with the given PID is running.
func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; signal 0 checks existence.
	return p.Signal(syscall.Signal(0)) == nil
}
