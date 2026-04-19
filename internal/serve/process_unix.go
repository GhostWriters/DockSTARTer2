//go:build !windows

package serve

import "DockSTARTer2/internal/process"

// ProcessExists reports whether a process with the given PID is running.
func ProcessExists(pid int) bool {
	return process.Exists(pid)
}
