// Package sinput wraps charm.land/bubbles/v2/textinput with mouse-driven text
// selection (click-to-position, double-click word, triple-click all, drag).
//
// Usage in a dialog:
//
//  1. Replace `textinput.Model` fields with `sinput.Model`.
//  2. In GetHitRegions, register a 1-row hit region for the input area and
//     call m.input.SetScreenTextX(offsetX + borderCols + promptW) to store
//     the absolute screen X of the first text character.
//  3. In Update, handle LayerHitMsg for the input region by calling
//     m.input.HandleClick(msg.X).
//  4. In Update, forward tea.MouseMotionMsg / tea.MouseReleaseMsg to sinput
//     when m.input.IsSelecting().
package sinput

import (
	"strings"

	"DockSTARTer2/internal/tui/components/selection"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Blink re-exports textinput.Blink so callers don't need to import both packages.
var Blink = textinput.Blink

// Model wraps textinput.Model and adds mouse-driven text selection.
type Model struct {
	textinput.Model

	// Selection state (all in logical rune positions)
	selActive  bool
	selStart   int // inclusive
	selEnd     int // exclusive
	selAnchor  int
	isSelecting bool // left button held, extending selection

	// Click tracking for multi-click
	clickTracker selection.ClickTracker

	// Viewport maintained independently so we can compute visible slice for
	// selection rendering (the textinput's internal offset is private).
	viewOffset int

	// Selection highlight style. Defaults to inverted text colours.
	SelStyle lipgloss.Style

	// screenTextX is the absolute screen X of the first text character
	// (after border, padding, prompt). Set by SetScreenTextX().
	screenTextX int
}

// New wraps an existing textinput.Model.  Configure the textinput first
// (CharLimit, Placeholder, EchoMode, Focus, Styles, etc.) then wrap it.
func New(ti textinput.Model) Model {
	return Model{
		Model:    ti,
		SelStyle: lipgloss.NewStyle().Reverse(true),
	}
}

// SetScreenTextX records the absolute screen X coordinate of the first text
// character (i.e. after the outer border, inner border/padding, and prompt).
// Call this from GetHitRegions() each frame.
func (m *Model) SetScreenTextX(x int) { m.screenTextX = x }

// PromptWidth returns the visual width of the prompt string.
func (m Model) PromptWidth() int { return lipgloss.Width(m.Model.Prompt) }

// IsSelecting reports whether a drag-selection is in progress.
func (m Model) IsSelecting() bool { return m.isSelecting }

// SelectedText returns the currently selected text, or "" if no selection.
func (m Model) SelectedText() string {
	if !m.selActive {
		return ""
	}
	value := []rune(m.Model.Value())
	s := clamp(m.selStart, 0, len(value))
	e := clamp(m.selEnd, 0, len(value))
	if s >= e {
		return ""
	}
	return string(value[s:e])
}

// HandleClick processes a left-click at absolute screen X (absX).
// Implements multi-click: 1=move cursor, 2=word, 3+=select all.
func (m *Model) HandleClick(absX int) {
	value := []rune(m.Model.Value())
	relX := absX - m.screenTextX
	col := clamp(relX+m.viewOffset, 0, len(value))

	count := m.clickTracker.Track(col)
	switch {
	case count >= 3:
		// Triple-click: select entire value
		m.selStart = 0
		m.selEnd = len(value)
		m.selAnchor = 0
		m.selActive = len(value) > 0
		m.isSelecting = false
	case count == 2:
		// Double-click: select word
		s, e := selection.WordBoundsAt(value, col)
		m.selStart = s
		m.selEnd = e
		m.selAnchor = s
		m.selActive = s < e
		m.isSelecting = false
	default:
		// Single click: move cursor, start potential drag
		m.Model.SetCursor(col)
		m.clearSelection()
		m.isSelecting = true
		m.selAnchor = col
	}
	m.syncViewport()
}

// HandleDragTo extends the selection to absX during a left-button drag.
func (m *Model) HandleDragTo(absX int) {
	if !m.isSelecting {
		return
	}
	value := []rune(m.Model.Value())
	relX := absX - m.screenTextX
	col := clamp(relX+m.viewOffset, 0, len(value))

	anchor := m.selAnchor
	if col < anchor {
		m.selStart = col
		m.selEnd = anchor
	} else {
		m.selStart = anchor
		m.selEnd = col
	}
	m.selActive = m.selStart < m.selEnd
	m.Model.SetCursor(col)
	m.syncViewport()
}

// EndDrag finalises a drag-selection.
func (m *Model) EndDrag() { m.isSelecting = false }

// Update handles messages. Key events are forwarded to the embedded
// textinput (which manages cursor movement, typing, paste, etc.) after
// clearing or acting on any active selection.  Mouse events are handled here.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseReleaseMsg:
		m.isSelecting = false
		return m, nil

	case tea.KeyPressMsg:
		// When a selection is active, intercept destructive keys to delete the
		// selected region, and printable keys to replace it.
		if m.selActive {
			km := m.Model.KeyMap
			isDelete := false
			for _, b := range []key.Binding{
				km.DeleteCharacterBackward, km.DeleteCharacterForward,
				km.DeleteWordBackward, km.DeleteWordForward,
				km.DeleteAfterCursor, km.DeleteBeforeCursor,
			} {
				if key.Matches(msg, b) {
					isDelete = true
					break
				}
			}
			if isDelete {
				m.deleteSelection()
				m.clearSelection()
				m.syncViewport()
				return m, nil
			}
			// Printable input: delete selection then insert
			if msg.Text != "" {
				m.deleteSelection()
				m.clearSelection()
				// Fall through: textinput will insert the typed rune at cursor
			} else {
				// Navigation / other modifier keys: just clear selection
				m.clearSelection()
			}
		}
	}

	// Delegate to the embedded textinput for all other messages.
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	m.syncViewport()
	return m, cmd
}

// View renders the input. When a selection is active, renders a custom view
// with the selection highlighted. Otherwise delegates to textinput.View().
func (m Model) View() string {
	if !m.selActive {
		return m.Model.View()
	}

	value := []rune(m.Model.Value())
	n := len(value)
	w := m.Model.Width()

	// Compute visible window
	off := m.viewOffset
	offRight := n
	if w > 0 {
		offRight = min(n, off+w)
	}
	visible := value[off:offRight]

	// Selection bounds within the visible window
	selS := clamp(m.selStart-off, 0, len(visible))
	selE := clamp(m.selEnd-off, 0, len(visible))

	styles := m.Model.Styles()
	var st textinput.StyleState
	if m.Model.Focused() {
		st = styles.Focused
	} else {
		st = styles.Blurred
	}
	textSt := st.Text.Inline(true)
	promptSt := st.Prompt.Inline(true)

	var sb strings.Builder
	sb.WriteString(promptSt.Render(m.Model.Prompt))
	if selS > 0 {
		sb.WriteString(textSt.Render(string(visible[:selS])))
	}
	if selE > selS {
		sb.WriteString(m.SelStyle.Render(string(visible[selS:selE])))
	}
	if selE < len(visible) {
		sb.WriteString(textSt.Render(string(visible[selE:])))
	}
	return sb.String()
}

// ─── helpers ────────────────────────────────────────────────────────────────

func (m *Model) clearSelection() {
	m.selActive = false
	m.isSelecting = false
}

func (m *Model) deleteSelection() {
	if !m.selActive {
		return
	}
	value := []rune(m.Model.Value())
	s := clamp(m.selStart, 0, len(value))
	e := clamp(m.selEnd, 0, len(value))
	if s >= e {
		return
	}
	newVal := string(value[:s]) + string(value[e:])
	m.Model.SetValue(newVal)
	m.Model.SetCursor(s)
}

// syncViewport keeps m.viewOffset consistent with the textinput cursor so that
// the cursor stays within the visible window for our custom rendering path.
func (m *Model) syncViewport() {
	pos := m.Model.Position()
	n := len([]rune(m.Model.Value()))
	w := m.Model.Width()
	if w <= 0 || n <= w {
		m.viewOffset = 0
		return
	}
	if pos < m.viewOffset {
		m.viewOffset = pos
	} else if pos >= m.viewOffset+w {
		m.viewOffset = pos - w + 1
	}
	m.viewOffset = clamp(m.viewOffset, 0, max(0, n-w))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
