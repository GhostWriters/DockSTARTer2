//go:build windows

package console

import "io"

// IsPuTTY always returns false on Windows — PuTTY runs as a local process
// with full VT support there, so no fixups are needed.
func IsPuTTY() bool { return false }

// ApplyPuTTYFixes is a no-op on Windows.
func ApplyPuTTYFixes(_ io.Writer) {}
