// Package streamvp provides a streaming viewport component shared between the
// program box dialog and the console panel. It manages raw/rendered line history,
// an inline content-area spinner shown while a command is running, and viewport
// content rebuilding on resize.
package streamvp

import (
	"strings"
	"time"

	"DockSTARTer2/internal/console"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SpinnerTickMsg is sent on each spinner tick. Tag is an opaque identifier set
// by the owner so ticks from different instances don't cross-fire.
type SpinnerTickMsg struct{ Tag string }

const maxHistory = 5000

// Model is the shared streaming-viewport state. Embed it in program box and panel.
// Call RenderLine to convert a raw string to a rendered line before appending.
type Model struct {
	viewport viewport.Model

	rawLines []string // un-rendered originals (for resize re-wrap)
	lines    []string // rendered + width-clamped lines shown in viewport

	spinnerLine  string // transient inline spinner appended after last line; not in lines/rawLines
	spinnerFrame int

	// CommandRunning controls the inline spinner: true while a command is executing.
	CommandRunning bool

	tag string // unique per-instance identifier for SpinnerTickMsg
}

// New creates a Model with the given tag (used to scope spinner ticks).
func New(tag string) Model {
	return Model{
		viewport: viewport.New(),
		tag:      tag,
	}
}

// Viewport returns the underlying viewport (read-only access for callers that
// need Width/Height/ScrollPercent/etc.).
func (m *Model) Viewport() *viewport.Model { return &m.viewport }

// SetSize updates the viewport dimensions and re-wraps all lines.
func (m *Model) SetSize(width, height int) {
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
	m.reRenderLines()
}

// Width returns the current viewport width.
func (m *Model) Width() int { return m.viewport.Width() }

// Height returns the current viewport height.
func (m *Model) Height() int { return m.viewport.Height() }

// AtBottom reports whether the viewport is scrolled to the bottom.
func (m *Model) AtBottom() bool { return m.viewport.AtBottom() }

// YOffset returns the current scroll offset.
func (m *Model) YOffset() int { return m.viewport.YOffset() }

// SetYOffset sets the scroll offset directly (used after resize).
func (m *Model) SetYOffset(off int) { m.viewport.SetYOffset(off) }

// TotalLineCount returns the total rendered line count (excluding spinner line).
func (m *Model) TotalLineCount() int { return len(m.lines) }

// VisibleLineCount returns how many lines fit in the viewport.
func (m *Model) VisibleLineCount() int { return m.viewport.Height() }

// ScrollPercent returns 0.0–1.0 scroll position.
func (m *Model) ScrollPercent() float64 { return m.viewport.ScrollPercent() }

// GotoBottom scrolls to the bottom.
func (m *Model) GotoBottom() { m.viewport.GotoBottom() }

// GotoTop scrolls to the top.
func (m *Model) GotoTop() { m.viewport.GotoTop() }

// ScrollUp scrolls up by n lines.
func (m *Model) ScrollUp(n int) { m.viewport.ScrollUp(n) }

// ScrollDown scrolls down by n lines.
func (m *Model) ScrollDown(n int) { m.viewport.ScrollDown(n) }

// HalfPageUp scrolls up half a page.
func (m *Model) HalfPageUp() { m.viewport.HalfPageUp() }

// HalfPageDown scrolls down half a page.
func (m *Model) HalfPageDown() { m.viewport.HalfPageDown() }

// SetStyle applies a lipgloss style to the viewport background.
func (m *Model) SetStyle(s lipgloss.Style) { m.viewport.Style = s }

// View renders the viewport content (without spinner — caller composes as needed).
func (m *Model) View() string { return m.viewport.View() }

// ViewUpdate passes a message to the underlying viewport model and returns a cmd.
func (m *Model) ViewUpdate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return cmd
}

// AppendLines renders each raw line, appends to history, clears the spinner line,
// and rebuilds viewport content scrolling to the bottom.
// renderFn converts a raw string to a display-ready string (theme colors, ANSI, etc.).
func (m *Model) AppendLines(rawLines []string, renderFn func(string) string) {
	w := m.viewport.Width()
	for _, raw := range rawLines {
		m.rawLines = append(m.rawLines, raw)
		rendered := renderFn(raw)
		if w > 0 {
			rendered = lipgloss.NewStyle().MaxWidth(w).Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}
	m.trimHistory()
	m.spinnerLine = ""
	m.setViewportContent(true)
}

// ReplaceLines replaces all current content with newLines, re-rendering each through renderFn.
// Viewport position is preserved (scrollToBottom=false) so live updates don't jump the view.
func (m *Model) ReplaceLines(rawLines []string, renderFn func(string) string) {
	w := m.viewport.Width()
	m.rawLines = make([]string, 0, len(rawLines))
	m.lines = make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		m.rawLines = append(m.rawLines, raw)
		rendered := renderFn(raw)
		if w > 0 {
			rendered = lipgloss.NewStyle().MaxWidth(w).Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}
	m.spinnerLine = ""
	m.setViewportContent(false)
}

// ReplaceTailLines keeps the first headerCount lines intact and replaces everything
// after them with newLines. Used by the program box to preserve pre-compose notice
// lines while updating the live compose display below them.
func (m *Model) ReplaceTailLines(headerCount int, rawLines []string, renderFn func(string) string) {
	w := m.viewport.Width()
	// Trim back to the header lines.
	if headerCount < len(m.rawLines) {
		m.rawLines = m.rawLines[:headerCount]
		m.lines = m.lines[:headerCount]
	}
	for _, raw := range rawLines {
		m.rawLines = append(m.rawLines, raw)
		rendered := renderFn(raw)
		if w > 0 {
			rendered = lipgloss.NewStyle().MaxWidth(w).Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}
	m.spinnerLine = ""
	m.setViewportContent(false)
}

// ClearSpinner removes the inline spinner line and rebuilds content.
func (m *Model) ClearSpinner() {
	if m.spinnerLine == "" {
		return
	}
	m.spinnerLine = ""
	m.setViewportContent(false)
}

// SpinnerTickCmd returns the Bubble Tea command that fires the next SpinnerTickMsg
// after the configured spinner speed. Returns nil if spinners are disabled.
func (m *Model) SpinnerTickCmd() tea.Cmd {
	if !console.SpinnerEnabled {
		return nil
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 {
		fps = 100 * time.Millisecond
	}
	tag := m.tag
	return tea.Tick(fps, func(time.Time) tea.Msg {
		return SpinnerTickMsg{Tag: tag}
	})
}

// HandleSpinnerTick processes a SpinnerTickMsg if it belongs to this instance.
// Returns (advanced, cmd): advanced=true if the frame was updated.
// Callers should call SpinnerTickCmd() themselves if they want to reschedule —
// this keeps the decision (continue vs stop) with the caller.
func (m *Model) HandleSpinnerTick(msg SpinnerTickMsg) (advanced bool) {
	if msg.Tag != m.tag || !m.CommandRunning || !console.SpinnerEnabled {
		return false
	}
	lineChars := getLineCharacters()
	frames := console.SpinnerFramesUnicode
	if !lineChars {
		frames = console.SpinnerFramesASCII
	}
	m.spinnerFrame = (m.spinnerFrame + 1) % len(frames)
	frame := lipgloss.NewStyle().Foreground(console.SpinnerColor).Render(frames[m.spinnerFrame])
	// Wrap in console background so it blends with the viewport
	m.spinnerLine = frame
	m.setViewportContent(false)
	return true
}

// CurrentFrame returns the current spinner frame character, or "" if spinners
// are disabled or CommandRunning is false.
func (m *Model) CurrentFrame() string {
	if !m.CommandRunning || !console.SpinnerEnabled {
		return ""
	}
	lineChars := getLineCharacters()
	frames := console.SpinnerFramesUnicode
	if !lineChars {
		frames = console.SpinnerFramesASCII
	}
	return frames[m.spinnerFrame%len(frames)]
}

// Clear wipes all lines and resets spinner state.
func (m *Model) Clear() {
	m.rawLines = nil
	m.lines = nil
	m.spinnerLine = ""
	m.CommandRunning = false
	m.viewport.SetContent("")
}

// ─── internal ────────────────────────────────────────────────────────────────

func (m *Model) setViewportContent(scrollToBottom bool) {
	content := strings.Join(m.lines, "\n")
	if m.spinnerLine != "" {
		content += "\n" + m.spinnerLine
	}
	atBottom := m.viewport.AtBottom()
	savedOffset := m.viewport.YOffset()
	m.viewport.SetContent(content)
	if scrollToBottom || atBottom {
		if m.viewport.Height() > 0 {
			m.viewport.GotoBottom()
		}
	} else {
		// Restore scroll position — SetContent resets the offset to 0.
		m.viewport.SetYOffset(savedOffset)
	}
}

func (m *Model) reRenderLines() {
	if len(m.rawLines) == 0 {
		m.setViewportContent(true)
		return
	}
	// Re-render is caller-driven via a provided renderFn — but we don't store
	// renderFn on the struct to keep it stateless. Instead callers call
	// ReRenderWith when they have a renderFn available (e.g. on resize).
	// setViewportContent still rebuilds from the already-rendered m.lines.
	m.setViewportContent(true)
}

// ReRenderWith re-wraps all raw lines using renderFn (call on resize).
func (m *Model) ReRenderWith(renderFn func(string) string) {
	w := m.viewport.Width()
	m.lines = make([]string, 0, len(m.rawLines))
	style := lipgloss.NewStyle().MaxWidth(w)
	for _, raw := range m.rawLines {
		rendered := renderFn(raw)
		if w > 0 {
			rendered = style.Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}
	m.trimHistory()
	m.setViewportContent(true)
}

func (m *Model) trimHistory() {
	if len(m.rawLines) > maxHistory {
		m.rawLines = m.rawLines[len(m.rawLines)-maxHistory:]
		m.lines = m.lines[len(m.lines)-maxHistory:]
	}
}

// getLineCharacters reads the active context's LineCharacters setting.
// Defined as a variable so tests can override it.
var getLineCharacters = func() bool {
	// Late-bound import avoids an import cycle: streamvp → tui.
	// We read the global via the console package which has no cycle.
	return console.LineCharacters
}
