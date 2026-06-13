package console

import (
	"fmt"
	"os"
	"strings"
	"sync"
	tea "charm.land/bubbletea/v2"
)

const historyMax = 10000 // lines kept for dump to main screen on exit

// Viewport manages output history and an optional full-screen Bubble Tea
// viewport that activates lazily when compose starts.
//
// Lifecycle:
//
//	StartViewport() — begins buffering: output goes to stderr normally AND
//	                  is stored in history. No alternate screen yet.
//	Activate()      — called when compose starts. Launches a minimal Bubble Tea
//	                  program pre-filled with recent history. pgup/pgdn work.
//	Deactivate()    — called when compose finishes. Stops the tea program, dumps
//	                  history to main screen, returns to normal output.
//	stop()          — called on program exit; handles both states.
//
// Non-TTY / TUI mode: viewport is never started; all methods are no-ops.
// Thread safety: all exported methods are safe for concurrent use.
type Viewport struct {
	mu        sync.Mutex
	buffering bool // capturing history but not yet on alt screen
	active    bool // Bubble Tea program is running

	// full history — every line ever appended
	history  []string
	dumpFrom int // history index at Activate() time; stop() dumps history[dumpFrom:]

	// lastComposeLines holds the most recent UpdateLines snapshot so stop() can
	// dump the live compose rows (which are never appended to history).
	lastComposeLines []string

	// tea program handle — non-nil while active
	teaProg *tea.Program
}

// GlobalViewport is the process-wide viewport instance.
var GlobalViewport *Viewport

// viewportWriter is an io.Writer that feeds logger output into the viewport.
type viewportWriter struct{}

func (viewportWriter) Write(p []byte) (int, error) {
	if GlobalViewport != nil {
		vp := GlobalViewport
		lines := splitLines(string(p))
		if vp.IsActive() {
			// Send to tea program; also capture to history.
			vp.mu.Lock()
			vp.appendToHistoryLocked(lines)
			prog := vp.teaProg
			vp.mu.Unlock()
			if prog != nil {
				prog.Send(cliAppendMsg{lines: lines})
			}
			return len(p), nil
		}
		if vp.IsBuffering() {
			// Write through to stderr and capture to history.
			vp.mu.Lock()
			vp.appendToHistoryLocked(lines)
			vp.mu.Unlock()
			// fall through to os.Stderr write below
		}
	}
	return os.Stderr.Write(p)
}

// ViewportWriter returns the io.Writer that routes into the active viewport.
func ViewportWriter() *viewportWriter { return &viewportWriter{} }

// StartViewport initialises the global viewport and begins buffering history.
// Output still flows to stderr normally until Activate() is called.
// Returns a stop function that must be called (or deferred) before exit.
func StartViewport() func() {
	if !isTTYGlobal || TUIMode {
		return func() {}
	}
	if w, h, err := GetTerminalSize(); err != nil || w <= 0 || h <= 0 {
		return func() {}
	}

	vp := &Viewport{buffering: true}
	GlobalViewport = vp

	return func() {
		vp.stop()
		GlobalViewport = nil
	}
}

// Activate enters the full-screen Bubble Tea viewport, pre-filled with recent
// history. Called by compose just before its first UpdateLines().
// Safe to call multiple times — subsequent calls are no-ops.
func (vp *Viewport) Activate() {
	vp.mu.Lock()
	if vp.active {
		vp.mu.Unlock()
		return
	}

	// Snapshot history to pre-fill; record where to start the forced-stop dump.
	snap := make([]string, len(vp.history))
	copy(snap, vp.history)
	vp.dumpFrom = len(vp.history)
	vp.active = true
	vp.buffering = false
	vp.mu.Unlock()

	// Use ?47h (alternate screen, no save/restore) so exiting with ?47l
	// returns to a blank main screen rather than restoring saved content.
	// Bubble Tea never sees AltScreen:true so it won't issue its own sequences.
	fmt.Fprint(os.Stderr, "\033[?47h\033[H\033[2J")

	// Pass history into the model before Run() so the very first frame
	// already has content — no blank-screen flash on alt screen entry.
	model := newCLIViewportModel(snap)
	prog := tea.NewProgram(model,
		tea.WithOutput(os.Stderr),
		tea.WithoutCatchPanics(),
	)

	vp.mu.Lock()
	vp.teaProg = prog
	vp.mu.Unlock()

	// Run the program in a goroutine — it blocks until quit.
	go func() {
		_, _ = prog.Run()
	}()

	// Wait until the tea program has processed its first WindowSizeMsg so it
	// is fully initialised before compose starts sending UpdateLines.
	<-model.readyCh
}

// Deactivate stops the Bubble Tea program and dumps compose output to the main
// screen so it appears in the terminal scrollback.
func (vp *Viewport) Deactivate() {
	vp.mu.Lock()
	if !vp.active {
		vp.mu.Unlock()
		return
	}
	prog := vp.teaProg
	history := vp.history
	dumpFrom := vp.dumpFrom
	composeLines := vp.lastComposeLines
	vp.active = false
	vp.buffering = true
	vp.teaProg = nil
	vp.mu.Unlock()

	if prog != nil {
		prog.Send(cliQuitMsg{})
		prog.Wait()
		termMu.Lock()
		fmt.Fprint(os.Stderr, "\033[?47l") // exit alt screen, main screen is blank
		for _, line := range history[dumpFrom:] {
			fmt.Fprint(os.Stderr, line+"\r\n")
		}
		for _, line := range composeLines {
			fmt.Fprint(os.Stderr, line+"\r\n")
		}
		termMu.Unlock()
	}
}

// ForceStop stops the viewport immediately — used by signal handlers.
func (vp *Viewport) ForceStop() {
	vp.stop()
}

func (vp *Viewport) stop() {
	vp.mu.Lock()
	wasActive := vp.active
	prog := vp.teaProg
	history := vp.history
	dumpFrom := vp.dumpFrom
	composeLines := vp.lastComposeLines
	vp.active = false
	vp.buffering = false
	vp.teaProg = nil
	vp.mu.Unlock()

	if wasActive && prog != nil {
		prog.Send(cliQuitMsg{})
		prog.Wait()
		termMu.Lock()
		fmt.Fprint(os.Stderr, "\033[?47l")
		for _, line := range history[dumpFrom:] {
			fmt.Fprint(os.Stderr, line+"\r\n")
		}
		for _, line := range composeLines {
			fmt.Fprint(os.Stderr, line+"\r\n")
		}
		termMu.Unlock()
	}
}

// Append adds a line to history and the visible viewport.
// Acquires termMu — do not call from a path that already holds termMu.
func (vp *Viewport) Append(line string) {
	vp.mu.Lock()
	lines := []string{line}
	vp.appendToHistoryLocked(lines)
	prog := vp.teaProg
	active := vp.active
	vp.mu.Unlock()

	if active && prog != nil {
		prog.Send(cliAppendMsg{lines: lines})
	} else {
		termMu.Lock()
		fmt.Fprintln(os.Stderr, line)
		termMu.Unlock()
	}
}

// SetHeader sets a persistent header line shown above all viewport content.
func (vp *Viewport) SetHeader(line string) {
	vp.mu.Lock()
	prog := vp.teaProg
	active := vp.active
	vp.mu.Unlock()
	if active && prog != nil {
		prog.Send(cliSetHeaderMsg{line: line})
	}
}

// UpdateLines replaces the visible viewport content (used by compose display).
// Not added to history — compose's logSummary writes the final state via Append.
// A snapshot is kept in lastComposeLines so stop() can dump them on forced exit.
func (vp *Viewport) UpdateLines(lines []string) {
	vp.mu.Lock()
	prog := vp.teaProg
	active := vp.active
	snap := make([]string, len(lines))
	copy(snap, lines)
	vp.lastComposeLines = snap
	vp.mu.Unlock()

	if active && prog != nil {
		prog.Send(cliSetLinesMsg{lines: lines})
	}
}

// SetSpinnerLine — no-op in tea mode; streamvp handles its own spinner.
func (vp *Viewport) SetSpinnerLine(_ string) {}

// ClearSpinnerLine — no-op in tea mode.
func (vp *Viewport) ClearSpinnerLine() {}

// AppendToHistory adds a line to history without affecting the visible viewport.
func (vp *Viewport) AppendToHistory(line string) {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	vp.appendToHistoryLocked([]string{line})
}

// History returns a snapshot of all lines written.
func (vp *Viewport) History() []string {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	out := make([]string, len(vp.history))
	copy(out, vp.history)
	return out
}

// IsActive reports whether the Bubble Tea viewport is currently running.
func (vp *Viewport) IsActive() bool {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	return vp.active
}

// IsBuffering reports whether the viewport is capturing history without the alt screen.
func (vp *Viewport) IsBuffering() bool {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	return vp.buffering && !vp.active
}

// IsRawMode reports whether the viewport owns raw mode.
// In tea mode, Bubble Tea owns raw mode while active.
func (vp *Viewport) IsRawMode() bool {
	return vp.IsActive()
}

// Height returns 0 when not active (tea program owns sizing).
func (vp *Viewport) Height() int { return 0 }

// appendToHistoryLocked appends lines to history. Caller must hold vp.mu.
func (vp *Viewport) appendToHistoryLocked(lines []string) {
	for _, l := range lines {
		if l == "" {
			continue
		}
		vp.history = append(vp.history, l)
	}
	if len(vp.history) > historyMax {
		vp.history = vp.history[len(vp.history)-historyMax:]
	}
}

// splitLines splits a byte payload on newlines, discarding blank entries.
func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}


