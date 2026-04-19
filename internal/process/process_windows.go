//go:build windows

package process

import (
	"os"
)

// Exists reports whether a process with the given PID is running.
func Exists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess only finds the process if it exists.
	_ = p
	return true
}
