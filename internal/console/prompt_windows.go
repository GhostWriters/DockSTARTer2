//go:build windows

package console

import "os"

// openTTY falls back to os.Stdin on Windows — the viewport is never active
// there, so this path is only reached in non-viewport codepaths.
// Returns false so the caller knows not to close the file.
func openTTY() (*os.File, bool) {
	return os.Stdin, false
}
