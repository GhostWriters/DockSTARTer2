//go:build !windows

package tui

import (
	"os"
	"syscall"
)

// drainStdin discards any bytes already buffered in stdin.
// After a mouse-driven exit, SGR-encoded mouse events may still be queued in
// the OS pipe before the terminal processes our mouse-disable sequences.
// Draining here prevents those sequences from appearing as raw text in the shell.
func drainStdin() {
	fd := int(os.Stdin.Fd())
	// Temporarily make stdin non-blocking so we can read without hanging.
	if err := syscall.SetNonblock(fd, true); err != nil {
		return
	}
	defer syscall.SetNonblock(fd, false) //nolint:errcheck
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n == 0 || err != nil {
			break
		}
	}
}
