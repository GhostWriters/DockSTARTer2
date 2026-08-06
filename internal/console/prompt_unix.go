//go:build !windows

package console

import "os"

// openTTY opens the controlling terminal directly so prompts can read from it
// even when the viewport's discard goroutine owns os.Stdin.
// Returns the file and true on success; caller must close the file.
func openTTY() (*os.File, bool) {
	f, err := os.Open("/dev/tty")
	return f, err == nil
}
