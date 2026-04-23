package graphics

import (
	"os"
	"runtime"
)

// CanDisplayGraphics returns true if the current environment is known to support 
// high-fidelity terminal graphics (Kitty or Sixel).
func CanDisplayGraphics() bool {
	// Check common terminal indicators
	term := os.Getenv("TERM")
	termProgram := os.Getenv("TERM_PROGRAM")

	// WezTerm supports Sixel and Kitty on all platforms
	if termProgram == "WezTerm" {
		return true
	}

	// Kitty is the gold standard for Kitty graphics
	if term == "xterm-kitty" {
		return true
	}

	// iTerm2 supports Sixel and its own protocol
	if termProgram == "iTerm.app" {
		return true
	}

	// On Windows, if we're not in a known high-end terminal, we default to false
	// to avoid sending raw sequences to cmd.exe/powershell.exe which usually fail.
	if runtime.GOOS == "windows" {
		return false
	}

	// On Linux/Unix, many modern terminals support Sixel even if not explicitly marked.
	// We'll be optimistic here as Sixel is generally safer than Kitty.
	return true
}
