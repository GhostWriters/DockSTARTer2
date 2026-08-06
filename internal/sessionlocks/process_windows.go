//go:build windows

package sessionlocks

import (
	"os"

	"DockSTARTer2/internal/process"
)

// ProcessExists reports whether a process with the given PID is running.
func ProcessExists(pid int) bool {
	return process.Exists(pid)
}

// signalProcess on Windows uses a forceful Kill since there is no native SIGTERM.
func signalProcess(proc *os.Process) error {
	return proc.Kill()
}
