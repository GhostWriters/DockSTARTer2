package console

import (
	"os"

	"golang.org/x/term"
)

// GetTerminalSize returns the current width and height of the terminal window.
// It prioritizes Stdout, then Stderr. If neither are terminals, it returns 0, 0.
func GetTerminalSize() (int, int, error) {
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return width, height, nil
	}
	if width, height, err := term.GetSize(int(os.Stderr.Fd())); err == nil {
		return width, height, nil
	}
	return 0, 0, nil
}
