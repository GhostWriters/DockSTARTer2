//go:build !windows

package sessionlocks

import (
	"os"
	"syscall"

	"DockSTARTer2/internal/process"
)

// ProcessExists reports whether a process with the given PID is running.
func ProcessExists(pid int) bool {
	return process.Exists(pid)
}

// signalProcess on Unix sends SIGTERM for a graceful stop of the session TUI.
func signalProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
