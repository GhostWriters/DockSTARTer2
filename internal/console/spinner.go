package console

import (
	"fmt"
	"os"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
)

var (
	SpinnerFramesASCII   = []string{"|", "/", "-", "\\"}
	SpinnerFramesUnicode = []string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"}
	cliSpinnerFPS = time.Second / 4 // fallback; StartSpinner uses SpinnerSpeed if set
	cliSpinnerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
)

// termMu serialises all terminal writes (spinner goroutine + log output path).
// Every caller that writes to stderr must hold this while doing clear+write+show
// as a single atomic unit, preventing the goroutine from interleaving.
var termMu sync.Mutex

var activeSpinner struct {
	mu      sync.Mutex
	paused  bool
	visible bool // true if a spinner frame is currently on the line
	active  bool // true while the spinner is running
	frame   int
}

// spinnerFrames returns the correct frame set based on config.
func spinnerFrames() []string {
	if LineCharacters {
		return SpinnerFramesUnicode
	}
	return SpinnerFramesASCII
}

// LockTerminal acquires the terminal write lock. The caller must write to
// stderr and then call UnlockTerminal. This prevents the spinner goroutine
// from interleaving with log output.
func LockTerminal() {
	termMu.Lock()
}

// UnlockTerminal releases the terminal write lock.
func UnlockTerminal() {
	termMu.Unlock()
}

// PauseSpinner stops the spinner from drawing and clears the current line.
// Call before writing output or prompting for input. Resume with ResumeSpinner.
func PauseSpinner() {
	termMu.Lock()
	defer termMu.Unlock()
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
}

// RestoreCursor unconditionally shows the terminal cursor. Call on exit or
// signal to ensure the cursor is never left hidden.
func RestoreCursor() {
	fmt.Fprintf(os.Stderr, "\033[?25h")
}

// ClearSpinnerLine erases the spinner character from the current line if one
// is visible. Must be called while holding termMu (via LockTerminal).
func ClearSpinnerLine() {
	activeSpinner.mu.Lock()
	defer activeSpinner.mu.Unlock()
	if activeSpinner.visible {
		fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
		activeSpinner.visible = false
	}
}

// ShowSpinnerFrame draws the current spinner frame. Must be called while
// holding termMu (via LockTerminal), after the log line has been written.
func ShowSpinnerFrame() {
	if !isTTYGlobal || TUIMode || IsTUIEnabled() || !SpinnerEnabled {
		return
	}
	activeSpinner.mu.Lock()
	defer activeSpinner.mu.Unlock()
	if !activeSpinner.active || activeSpinner.paused {
		return
	}
	frames := spinnerFrames()
	fmt.Fprintf(os.Stderr, "\033[?25l\r%s", cliSpinnerStyle.Render(frames[activeSpinner.frame%len(frames)]))
	activeSpinner.visible = true
	activeSpinner.frame++
}

// StartSpinner marks the spinner as active and starts the background tick.
// The goroutine holds termMu for each frame write, so it cannot interleave
// with log output that also holds termMu across clear+write+show.
// Returns a stop function that clears the spinner.
func StartSpinner() func() {
	if !isTTYGlobal || TUIMode || IsTUIEnabled() || !SpinnerEnabled {
		return func() {}
	}

	activeSpinner.mu.Lock()
	activeSpinner.active = true
	activeSpinner.frame = 0
	activeSpinner.paused = false
	activeSpinner.visible = false
	activeSpinner.mu.Unlock()

	fps := cliSpinnerFPS
	if SpinnerSpeed > 0 {
		fps = time.Duration(SpinnerSpeed) * time.Millisecond
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(fps):
				termMu.Lock()
				activeSpinner.mu.Lock()
				if activeSpinner.active && !activeSpinner.paused {
					frames := spinnerFrames()
					fmt.Fprintf(os.Stderr, "\033[?25l\r%s", cliSpinnerStyle.Render(frames[activeSpinner.frame%len(frames)]))
					activeSpinner.visible = true
					activeSpinner.frame++
				}
				activeSpinner.mu.Unlock()
				termMu.Unlock()
			}
		}
	}()

	return func() {
		close(done)
		termMu.Lock()
		activeSpinner.mu.Lock()
		activeSpinner.active = false
		activeSpinner.visible = false
		activeSpinner.paused = false
		fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
		activeSpinner.mu.Unlock()
		termMu.Unlock()
	}
}
