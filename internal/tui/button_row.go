package tui

import (
	"fmt"
	"sync/atomic"
	"time"

	"DockSTARTer2/internal/console"

	tea "charm.land/bubbletea/v2"
)

var buttonRowCounter atomic.Uint64

// buttonRowDeferredActionMsg carries an action to execute after one render cycle,
// allowing the spinner to appear before any synchronous work in the action blocks.
type buttonRowDeferredActionMsg struct {
	id     string
	action tea.Cmd
}

// ButtonRow is a self-contained button row for single-row, no-list dialogs
// (confirm/choice/message/prompt/etc). It owns its own processing/spinner
// state and satisfies SpinnerAdvancer directly, so embedding types don't
// need their own forwarding method unless they have other spinner-bearing
// subcomponents.
//
// Usage:
//  1. Embed *ButtonRow (via NewButtonRow) in your model struct.
//  2. In Update, call m.buttons.Update(msg) first; if it returns a non-nil
//     cmd, return it.
//  3. When a button is clicked, call m.buttons.SetProcessing(zoneID, action)
//     and return the cmd it returns.
//  4. Before rendering, call m.buttons.Specs(focused, focusedIndex) to get []ButtonSpec.
type ButtonRow struct {
	instanceID      string
	buttons         []ButtonDef
	processingBtnID string
	spinnerFrame    int
	lastSpinner     time.Time
}

// NewButtonRow creates a ButtonRow with the given button definitions.
func NewButtonRow(buttons []ButtonDef) *ButtonRow {
	return &ButtonRow{
		instanceID: fmt.Sprintf("buttonrow#%d", buttonRowCounter.Add(1)),
		buttons:    buttons,
	}
}

// SetButtons replaces the button definitions.
func (b *ButtonRow) SetButtons(buttons []ButtonDef) {
	b.buttons = buttons
}

// ZoneIDAt returns the ZoneID of the button at idx, or "" if out of range.
func (b *ButtonRow) ZoneIDAt(idx int) string {
	if idx < 0 || idx >= len(b.buttons) {
		return ""
	}
	return b.buttons[idx].ZoneID
}

// SetProcessing marks the given button zone ID as spinning and defers the
// action by a short fixed delay so the button's active state renders before
// the action runs. Use this instead of running action directly for any
// action that is synchronous/blocking (e.g. opens a confirm dialog on the
// same goroutine).
func (b *ButtonRow) SetProcessing(zoneID string, action tea.Cmd) tea.Cmd {
	b.processingBtnID = zoneID
	b.spinnerFrame = 0
	id := b.instanceID
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return buttonRowDeferredActionMsg{id: id, action: action}
	})
}

// MarkProcessing marks the given button zone ID as spinning without
// scheduling any deferred action. Use this when the caller drives its own
// deferred dispatch separately (e.g. a list-item action that also visually
// spins a proxy "Select" button).
func (b *ButtonRow) MarkProcessing(zoneID string) {
	b.processingBtnID = zoneID
	b.spinnerFrame = 0
}

// Clear stops the spinner.
func (b *ButtonRow) Clear() {
	b.processingBtnID = ""
}

// IsProcessing reports whether a spinner is currently active.
func (b *ButtonRow) IsProcessing() bool {
	return b.processingBtnID != ""
}

// IsProcessingID reports whether the given zone ID is the one currently spinning.
func (b *ButtonRow) IsProcessingID(zoneID string) bool {
	return b.processingBtnID == zoneID
}

// Update handles the internal deferred-action message. Returns (cmd, true)
// when the message was consumed; the caller should return cmd directly.
func (b *ButtonRow) Update(msg tea.Msg) (tea.Cmd, bool) {
	if deferred, ok := msg.(buttonRowDeferredActionMsg); ok && deferred.id == b.instanceID {
		// Keep processingBtnID set so the spinner keeps ticking while the
		// action runs (e.g. opening a confirm dialog). Callers clear it
		// explicitly (Clear()) once the action has resolved.
		return deferred.action, true
	}
	return nil, false
}

// AdvanceSpinner advances the button spinner frame if its interval has
// elapsed. Returns true if the frame changed. Called by the global tick.
func (b *ButtonRow) AdvanceSpinner(now time.Time) bool {
	if b.processingBtnID == "" || !console.SpinnerEnabled {
		return false
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 || now.Sub(b.lastSpinner) < fps {
		return false
	}
	b.lastSpinner = now
	ctx := GetActiveContext()
	frames := console.SpinnerFramesTitleUnicode
	if !ctx.LineCharacters {
		frames = console.SpinnerFramesTitleASCII
	}
	b.spinnerFrame = (b.spinnerFrame + 1) % len(frames)
	return true
}

// AdvanceSpinners satisfies SpinnerAdvancer directly.
func (b *ButtonRow) AdvanceSpinners(now time.Time) bool {
	return b.AdvanceSpinner(now)
}

// Specs returns the current button configuration as []ButtonSpec, with
// Active/Spinning/SpinnerFrame populated from focus and processing state.
// focused indicates whether button-row focus (vs. some other widget in the
// dialog) is active; when false, no button renders as Active from keyboard
// focus (though a processing button still renders as Active). focusedIndex
// is the caller-owned index of the currently focused button (ButtonRow does
// not track focus itself).
func (b *ButtonRow) Specs(focused bool, focusedIndex int) []ButtonSpec {
	specs := make([]ButtonSpec, len(b.buttons))
	for i, btn := range b.buttons {
		active := (focused && focusedIndex == i) || b.processingBtnID == btn.ZoneID
		specs[i] = ButtonSpec{
			Text:         btn.Label,
			Active:       active,
			Locked:       btn.Locked,
			Spinning:     b.processingBtnID == btn.ZoneID,
			SpinnerFrame: b.spinnerFrame,
			ZoneID:       btn.ZoneID,
			Help:         btn.Help,
		}
	}
	return specs
}

// ApplySpinner returns a copy of specs with Spinning/SpinnerFrame set on the
// matching button. Use this when the caller builds []ButtonSpec directly
// (e.g. Active determined by dialog-specific state rather than a focused
// index) but still wants ButtonRow's spinner state applied.
func (b *ButtonRow) ApplySpinner(specs []ButtonSpec) []ButtonSpec {
	if b.processingBtnID == "" || !console.SpinnerEnabled {
		return specs
	}
	out := make([]ButtonSpec, len(specs))
	copy(out, specs)
	for i := range out {
		if out[i].ZoneID == b.processingBtnID {
			out[i].Spinning = true
			out[i].SpinnerFrame = b.spinnerFrame
		}
	}
	return out
}
