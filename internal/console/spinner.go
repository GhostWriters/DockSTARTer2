package console

import (
	semstyle "github.com/GhostWriters/semstyle/lg"
	"fmt"
	"os"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
)

var (
	// SpinnerColor is the ANSI color used for spinner frames in both CLI and TUI contexts.
	SpinnerColor = lipgloss.Color("2") // green

	SpinnerFramesASCII        = []string{"|", "/", "-", "\\"}
	SpinnerFramesTitleASCII   = []string{"|", "/", "-", "\\"}
	SpinnerFramesUnicode      = []string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"}
	SpinnerFramesTitleUnicode = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	cliSpinnerFPS             = time.Second / 4 // fallback; StartSpinner uses SpinnerSpeed if set
	cliSpinnerStyle           = lipgloss.NewStyle().Foreground(SpinnerColor)
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

// TitleSpinnerFrames returns the left (clockwise) and right (counter-clockwise) frame
// characters for the current spinner position, using the title spinner frame set.
// The right frame mirrors the left so both appear to spin toward the center.
func TitleSpinnerFrames(frame int, lineCharacters bool) (left, right string) {
	if lineCharacters {
		frames := SpinnerFramesTitleUnicode
		n := len(frames)
		left = frames[frame%n]
		right = frames[(n-frame%n)%n]
	} else {
		frames := SpinnerFramesTitleASCII
		n := len(frames)
		left = frames[frame%n]
		right = frames[(n-frame%n)%n]
	}
	return
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

// Println renders semantic/direct tags in a and prints the line, routing to the active
// viewport when present and otherwise writing to the terminal around the spinner. This is
// the app-level I/O wrapper over the styling engine (semstyle owns the rendering).
func Println(a ...any) {
	msg := semstyle.ToANSI(fmt.Sprint(a...))
	if GlobalViewport != nil && GlobalViewport.active {
		GlobalViewport.Append(msg)
		return
	}
	LockTerminal()
	ClearSpinnerLine()
	fmt.Println(msg)
	ShowSpinnerFrame()
	UnlockTerminal()
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
// When the viewport is active the spinner renders inside the viewport's
// reserved region (bottom row) using the same spinnerLine pattern as streamvp.
// Otherwise it writes directly to stderr on the current line.
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
				activeSpinner.mu.Lock()
				if !activeSpinner.active || activeSpinner.paused {
					activeSpinner.mu.Unlock()
					continue
				}
				frames := spinnerFrames()
				frame := cliSpinnerStyle.Render(frames[activeSpinner.frame%len(frames)])
				activeSpinner.frame++
				activeSpinner.mu.Unlock()

				if vp := GlobalViewport; vp != nil && vp.IsActive() {
					vp.SetSpinnerLine(frame)
				} else if !TUIMode && !IsTUIEnabled() {
					termMu.Lock()
					fmt.Fprintf(os.Stderr, "\033[?25l\r%s", frame)
					activeSpinner.mu.Lock()
					activeSpinner.visible = true
					activeSpinner.mu.Unlock()
					termMu.Unlock()
				}
			}
		}
	}()

	return func() {
		close(done)
		activeSpinner.mu.Lock()
		activeSpinner.active = false
		activeSpinner.visible = false
		activeSpinner.paused = false
		activeSpinner.mu.Unlock()

		if vp := GlobalViewport; vp != nil && vp.IsActive() {
			vp.ClearSpinnerLine()
		} else if !TUIMode {
			termMu.Lock()
			fmt.Fprintf(os.Stderr, "\r\033[K\033[?25h") // clear line and restore cursor
			termMu.Unlock()
		}
	}
}
