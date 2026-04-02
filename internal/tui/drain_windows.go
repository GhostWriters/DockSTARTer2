//go:build windows

package tui

// drainStdin is a no-op on Windows; the TUI runs on Linux targets only.
func drainStdin() {}
