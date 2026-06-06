package console

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	cliSpinnerFramesASCII   = []string{"|", "/", "-", "\\"}
	cliSpinnerFramesUnicode = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	cliSpinnerFPS           = time.Second / 4
)

var activeSpinner struct {
	mu      sync.Mutex
	paused  bool
	visible bool // true if a spinner frame is currently on the line
}

// PauseSpinner stops the spinner from drawing and clears the current line.
// Call before writing output or prompting for input. Resume with ResumeSpinner.
func PauseSpinner() {
	activeSpinner.mu.Lock()
	defer activeSpinner.mu.Unlock()
	activeSpinner.paused = true
	if activeSpinner.visible {
		fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
		activeSpinner.visible = false
	}
}

// ResumeSpinner re-enables spinner drawing after a PauseSpinner call.
func ResumeSpinner() {
	activeSpinner.mu.Lock()
	defer activeSpinner.mu.Unlock()
	activeSpinner.paused = false
	if SpinnerEnabled {
		fmt.Fprintf(os.Stderr, "\033[?25l") // re-hide cursor as spinner resumes
	}
}

// ClearSpinnerLine erases the spinner character from the current line if one is visible.
// Call before writing any output to stderr/stdout so the spinner doesn't interleave with text.
func ClearSpinnerLine() {
	activeSpinner.mu.Lock()
	defer activeSpinner.mu.Unlock()
	if activeSpinner.visible {
		fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
		activeSpinner.visible = false
	}
}

// StartSpinner starts an animated spinner on stderr while a task is running.
// Returns a stop function that clears the spinner line. No-op if not a TTY or TUI is active.
func StartSpinner() func() {
	if !isTTYGlobal || TUIMode || IsTUIEnabled() || !SpinnerEnabled {
		return func() {}
	}

	frames := cliSpinnerFramesASCII
	if LineCharacters {
		frames = cliSpinnerFramesUnicode
	}

	fmt.Fprintf(os.Stderr, "\033[?25l") // hide cursor for the duration of the spinner

	done := make(chan struct{})
	go func() {
		frame := 0
		for {
			select {
			case <-done:
				return
			case <-time.After(cliSpinnerFPS):
				activeSpinner.mu.Lock()
				if !activeSpinner.paused {
					fmt.Fprintf(os.Stderr, "\r%s", frames[frame%len(frames)])
					activeSpinner.visible = true
				}
				activeSpinner.mu.Unlock()
				frame++
			}
		}
	}()

	return func() {
		close(done)
		activeSpinner.mu.Lock()
		fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
		activeSpinner.visible = false
		activeSpinner.paused = false
		activeSpinner.mu.Unlock()
	}
}
