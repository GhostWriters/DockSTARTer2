//go:build !windows

package process

import (
	"os"
	"syscall"
)

// Exists reports whether a process with the given PID is running.
func Exists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; signal 0 checks existence.
	err = p.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
