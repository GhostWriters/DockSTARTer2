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
	"github.com/atotto/clipboard"
)

// ─── Message types for context-menu clipboard operations ────────────────────

// PasteMsg inserts Text at the cursor position, replacing any active selection.
type PasteMsg struct{ Text string }

// CutMsg deletes the active selection (or the entire value) after the caller
// has already written the text to the OS clipboard.
type CutMsg struct{}

// SelectAllMsg selects all text in the field.
type SelectAllMsg struct{}

// ─── KeyMap ──────────────────────────────────────────────────────────────────

// KeyMap holds the keyboard shortcuts used by sinput.
type KeyMap struct {
	SelectLeft  key.Binding // shift+left  — extend selection left
	SelectRight key.Binding // shift+right — extend selection right
	SelectHome  key.Binding // shift+home  — extend selection to start
	SelectEnd   key.Binding // shift+end   — extend selection to end
	SelectAll   key.Binding // ctrl+a      — select all
	Copy        key.Binding // ctrl+c      — copy to clipboard
	Cut         key.Binding // ctrl+x      — cut to clipboard
	Insert      key.Binding // insert      — toggle insert/overwrite mode
}

// DefaultKeyMap returns the standard key bindings for sinput.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		SelectLeft:  key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("shift+left", "select left")),
		SelectRight: key.NewBinding(key.WithKeys("shift+right"), key.WithHelp("shift+right", "select right")),
		SelectHome:  key.NewBinding(key.WithKeys("shift+home"), key.WithHelp("shift+home", "select to start")),
		SelectEnd:   key.NewBinding(key.WithKeys("shift+end"), key.WithHelp("shift+end", "select to end")),
		SelectAll:   key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "select all")),
		Copy:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "copy")),
		Cut:         key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl+x", "cut")),
		Insert:      key.NewBinding(key.WithKeys("insert"), key.WithHelp("insert", "toggle insert/overwrite")),
	}
}

// Blink re-exports textinput.Blink so callers don't need to import both packages.
var Blink = textinput.Blink

// Model wraps textinput.Model and adds mouse-driven text selection.
type Model struct {
	textinput.Model

	// Selection state (all in logical rune positions)
	selActive   bool
	selStart    int // inclusive
	selEnd      int // exclusive
	selAnchor   int
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

	// KeyMap holds keyboard shortcut bindings.
	KeyMap KeyMap

	// Overwrite indicates insert (false) vs overwrite (true) mode.
	Overwrite bool
}

// New wraps an existing textinput.Model.  Configure the textinput first
// (CharLimit, Placeholder, EchoMode, Focus, Styles, etc.) then wrap it.
func New(ti textinput.Model) Model {
	m := Model{
		Model:    ti,
		SelStyle: lipgloss.NewStyle().Reverse(true),
		KeyMap:   DefaultKeyMap(),
	}
	// Use the hardware cursor (rendered by the terminal) instead of the
	// virtual cursor (a character rendered inside the view string).  The
	// hardware cursor supports shape changes (bar vs block) and doesn't
	// require a Blink command loop.
	m.Model.SetVirtualCursor(false)
	return m
}

// SetScreenTextX records the absolute screen X coordinate of the first text
// character (i.e. after the outer border, inner border/padding, and prompt).
// Call this from GetHitRegions() each frame.
func (m *Model) SetScreenTextX(x int) { m.screenTextX = x }

// PromptWidth returns the visual width of the prompt string.
func (m Model) PromptWidth() int { return lipgloss.Width(m.Model.Prompt) }

// IsSelecting reports whether a drag-selection is in progress.
func (m Model) IsSelecting() bool { return m.isSelecting }

// IsOverwrite reports whether the input is in overwrite (OVR) mode.
func (m Model) IsOverwrite() bool { return m.Overwrite }

// CursorColumn returns the visual X offset of the cursor within the
// rendered textinput view (i.e. after prompt, within the text width).
// Returns 0 when the input is not focused.
func (m Model) CursorColumn() int {
	c := m.Model.Cursor()
	if c == nil {
		return 0
	}
	return c.Position.X
}

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
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseReleaseMsg:
		m.isSelecting = false
		return m, nil

	// ─── Context-menu clipboard messages ─────────────────────────────────────
	case PasteMsg:
		if m.selActive {
			m.deleteSelection()
			m.clearSelection()
		}
		val := []rune(m.Model.Value())
		pos := m.Model.Position()
		ins := []rune(msg.Text)
		newVal := string(append(append(val[:pos:pos], ins...), val[pos:]...))
		m.Model.SetValue(newVal)
		m.Model.SetCursor(pos + len(ins))
		m.syncViewport()
		return m, nil

	case CutMsg:
		text := m.SelectedText()
		if text == "" {
			text = m.Model.Value()
			m.Model.SetValue("")
			m.Model.SetCursor(0)
		} else {
			m.deleteSelection()
			m.clearSelection()
		}
		_ = clipboard.WriteAll(text)
		m.syncViewport()
		return m, nil

	case SelectAllMsg:
		val := []rune(m.Model.Value())
		m.selStart = 0
		m.selEnd = len(val)
		m.selAnchor = 0
		m.selActive = len(val) > 0
		m.Model.SetCursor(len(val))
		m.syncViewport()
		return m, nil

	case tea.KeyPressMsg:
		// ─── Insert key: toggle insert/overwrite mode ─────────────────────────
		if key.Matches(msg, m.KeyMap.Insert) {
			m.Overwrite = !m.Overwrite
			return m, nil
		}

		// ─── Keyboard selection (shift+arrow/home/end) ────────────────────────
		if key.Matches(msg, m.KeyMap.SelectLeft, m.KeyMap.SelectRight,
			m.KeyMap.SelectHome, m.KeyMap.SelectEnd) {
			if !m.selActive {
				m.selAnchor = m.Model.Position()
			}
			// Strip Shift so textinput moves the cursor without its own logic.
			stripped := msg
			stripped.Mod &^= tea.ModShift
			m.Model, cmd = m.Model.Update(stripped)
			m.updateSelectionFromAnchor(m.Model.Position())
			m.syncViewport()
			return m, cmd
		}

		// ─── Select all ───────────────────────────────────────────────────────
		if key.Matches(msg, m.KeyMap.SelectAll) {
			val := []rune(m.Model.Value())
			m.selStart = 0
			m.selEnd = len(val)
			m.selAnchor = 0
			m.selActive = len(val) > 0
			m.Model.SetCursor(len(val))
			m.syncViewport()
			return m, nil
		}

		// ─── Copy ─────────────────────────────────────────────────────────────
		if key.Matches(msg, m.KeyMap.Copy) {
			text := m.SelectedText()
			if text == "" {
				text = m.Model.Value()
			}
			_ = clipboard.WriteAll(text)
			return m, nil
		}

		// ─── Cut ──────────────────────────────────────────────────────────────
		if key.Matches(msg, m.KeyMap.Cut) {
			text := m.SelectedText()
			if text == "" {
				text = m.Model.Value()
				m.Model.SetValue("")
				m.Model.SetCursor(0)
			} else {
				m.deleteSelection()
				m.clearSelection()
			}
			_ = clipboard.WriteAll(text)
			m.syncViewport()
			return m, nil
		}

		// ─── Selection-aware delete / printable-replace ───────────────────────
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

	// ─── Overwrite mode: replace char at cursor instead of inserting ──────
	if kp, ok := msg.(tea.KeyPressMsg); ok && m.Overwrite && kp.Text != "" {
		val := []rune(m.Model.Value())
		pos := m.Model.Position()
		ins := []rune(kp.Text)
		var newVal []rune
		if pos < len(val) {
			// Replace chars starting at cursor (overwrite)
			end := pos + len(ins)
			if end > len(val) {
				end = len(val)
			}
			newVal = append(append([]rune{}, val[:pos]...), ins...)
			newVal = append(newVal, val[end:]...)
		} else {
			// Cursor is at end — just append
			newVal = append(val, ins...)
		}
		m.Model.SetValue(string(newVal))
		m.Model.SetCursor(pos + len(ins))
		m.syncViewport()
		return m, nil
	}

	// Delegate to the embedded textinput for all other messages.
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

// updateSelectionFromAnchor sets selStart/selEnd based on the relationship
// between newPos and the stored selAnchor, then updates selActive.
func (m *Model) updateSelectionFromAnchor(newPos int) {
	if newPos <= m.selAnchor {
		m.selStart = newPos
		m.selEnd = m.selAnchor
	} else {
		m.selStart = m.selAnchor
		m.selEnd = newPos
	}
	m.selActive = m.selStart < m.selEnd
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
