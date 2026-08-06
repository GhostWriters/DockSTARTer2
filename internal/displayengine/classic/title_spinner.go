package classic

import (
	"time"

	"DockSTARTer2/internal/console"
)

// TitleSpinner is a self-contained title-style spinner (the two-character
// flanking indicator rendered via console.TitleSpinnerFrames). Embed it in
// any model that shows a spinner in a title bar or inline list-item marker.
// Callers control activation via Start/Stop; TitleSpinner only owns frame
// advancement and rendering.
type TitleSpinner struct {
	active      bool
	frame       int
	lastSpinner time.Time
}

// Start marks the spinner active. If it was already active, the frame is
// left as-is so a burst of Start calls (e.g. one per incoming log line)
// doesn't visually restart the animation; otherwise the frame resets to 0.
func (s *TitleSpinner) Start() {
	if !s.active {
		s.frame = 0
	}
	s.active = true
}

// Stop marks the spinner inactive. Indicators() returns "" while inactive.
func (s *TitleSpinner) Stop() {
	s.active = false
}

// IsActive reports whether the spinner is currently running.
func (s *TitleSpinner) IsActive() bool { return s.active }

// AdvanceSpinner advances the frame if active and enough time has elapsed.
// Returns true if the frame changed. Called by the global tick.
func (s *TitleSpinner) AdvanceSpinner(now time.Time) bool {
	if !s.active {
		return false
	}
	newFrame, newLastSpinner, advanced := console.AdvanceTitleSpinnerFrame(s.frame, s.lastSpinner, now, GetActiveContext().LineCharacters)
	if !advanced {
		return false
	}
	s.frame = newFrame
	s.lastSpinner = newLastSpinner
	return true
}

// Indicators returns the left/right flanking characters, or "" when
// inactive or spinners are globally disabled.
func (s *TitleSpinner) Indicators() (left, right string) {
	if !s.active || !console.SpinnerEnabled {
		return "", ""
	}
	ctx := GetActiveContext()
	return console.TitleSpinnerFrames(s.frame, ctx.LineCharacters)
}
