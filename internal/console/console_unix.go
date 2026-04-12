//go:build !windows

package console

// EnableVirtualTerminalProcessing is a no-op on non-Windows systems.
func EnableVirtualTerminalProcessing() {}
