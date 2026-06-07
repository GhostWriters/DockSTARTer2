package tui

import (
	"fmt"
	"sync/atomic"
	"time"

	"DockSTARTer2/internal/console"

	tea "charm.land/bubbletea/v2"
)

var buttonSpinnerCounter atomic.Uint64

// btnSpinnerTickMsg is the internal tick message for ButtonSpinner.
type btnSpinnerTickMsg struct{ id string }

// ButtonSpinner is an embeddable helper that drives a spinner on a button while
// an action is in flight. Embed it in any screen/dialog that has its own buttons.
//
// Usage:
//  1. Embed ButtonSpinner in your model struct.
//  2. In Init/constructor, call s.btnSpinner.Init().
//  3. In Update, call s.btnSpinner.Update(msg) first; if it returns a cmd, batch it.
//  4. When a button is clicked, call s.btnSpinner.SetProcessing(zoneID) and batch
//     the returned tick cmd with your action cmd.
//  5. When the action resolves (or on screen restore), call s.btnSpinner.Clear().
//  6. Before rendering, call s.btnSpinner.ApplyToSpecs(specs) to inject Spinning state.
type ButtonSpinner struct {
	instanceID   string
	processingID string
	frame        int
}

// Init must be called once (in the model constructor or Init()) to assign a unique ID.
func (b *ButtonSpinner) Init() {
	b.instanceID = fmt.Sprintf("btnspinner#%d", buttonSpinnerCounter.Add(1))
	b.processingID = ""
	b.frame = 0
}

// SetProcessing marks the given button zone ID as spinning and returns the first tick cmd.
func (b *ButtonSpinner) SetProcessing(zoneID string) tea.Cmd {
	b.processingID = zoneID
	b.frame = 0
	return b.tickCmd()
}

// Clear stops the spinner.
func (b *ButtonSpinner) Clear() {
	b.processingID = ""
}

// IsProcessing reports whether a spinner is currently active.
func (b *ButtonSpinner) IsProcessing() bool {
	return b.processingID != ""
}

// Update handles the internal tick message. Returns (cmd, true) when the tick was
// consumed; the caller should batch cmd with other cmds.
func (b *ButtonSpinner) Update(msg tea.Msg) (tea.Cmd, bool) {
	tick, ok := msg.(btnSpinnerTickMsg)
	if !ok || tick.id != b.instanceID {
		return nil, false
	}
	if b.processingID == "" {
		return nil, true
	}
	ctx := GetActiveContext()
	frames := console.SpinnerFramesTitleUnicode
	if !ctx.LineCharacters {
		frames = console.SpinnerFramesTitleASCII
	}
	b.frame = (b.frame + 1) % len(frames)
	return b.tickCmd(), true
}

// ApplyToSpecs returns a copy of specs with Spinning/SpinnerFrame set on the
// matching button. Pass the result directly to RenderCenteredButtons*.
func (b *ButtonSpinner) ApplyToSpecs(specs []ButtonSpec) []ButtonSpec {
	if b.processingID == "" || !console.SpinnerEnabled {
		return specs
	}
	out := make([]ButtonSpec, len(specs))
	copy(out, specs)
	for i := range out {
		if out[i].ZoneID == b.processingID {
			out[i].Spinning = true
			out[i].SpinnerFrame = b.frame
		}
	}
	return out
}

func (b *ButtonSpinner) tickCmd() tea.Cmd {
	if !console.SpinnerEnabled {
		return nil
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 {
		fps = 100 * time.Millisecond
	}
	id := b.instanceID
	return tea.Tick(fps, func(time.Time) tea.Msg {
		return btnSpinnerTickMsg{id: id}
	})
}
