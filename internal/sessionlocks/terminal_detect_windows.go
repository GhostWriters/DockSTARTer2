//go:build windows

package sessionlocks

import "os"

// DetectTerminal returns a terminal identifier string.
// XTVERSION detection is not supported on Windows; falls back to TERM_PROGRAM/TERM.
func DetectTerminal() string {
	term := os.Getenv("TERM")
	if termProgram := os.Getenv("TERM_PROGRAM"); termProgram != "" {
		if term != "" {
			return termProgram + "/" + term
		}
		return termProgram
	}
	return term
}
