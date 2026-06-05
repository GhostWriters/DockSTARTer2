//go:build windows

package tui

import "time"

// drainStdin on Windows sleeps briefly to let ConPTY process and discard any
// buffered mouse events after we've sent the mouse-disable escape sequences.
// Without this, a mouse button release from clicking Exit can arrive in the
// shell's stdin as a raw SGR sequence (e.g. "[<32;49;17M") before the terminal
// has acted on the disable request.
func drainStdin() {
	time.Sleep(50 * time.Millisecond)
}
