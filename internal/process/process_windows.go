//go:build windows

package process

import (
	"golang.org/x/sys/windows"
)

// Exists reports whether a process with the given PID is running.
func Exists(pid int) bool {
	if pid <= 0 {
		return false
	}
	const access = windows.PROCESS_QUERY_LIMITED_INFORMATION
	handle, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return false
		}
		// If we get an access denied error, it means the process exists but we can't query it.
		// Any other error besides "invalid parameter" (which means PID is gone) implies existence.
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer windows.CloseHandle(handle)
	return true
}
