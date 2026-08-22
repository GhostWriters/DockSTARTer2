package enveditor

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Position is a location in the textarea's logical buffer: Row indexes a
// logical (unwrapped) line, Col indexes a rune within that line. Col may
// equal the line length, meaning "just past the last rune".
//
// Adapted from charm.land/bubbles/v2's textarea/selection.go (see
// _upstream/selection.go for the original) -- ported the read/track side
// as-is, but NOT deleteSelection/CopySelection: DS2's parallel lineMeta
// array and line-oriented KEY=VALUE format need different write-path
// semantics (see textarea_ds2.go's deleteSelectionDS2).
type Position struct {
	Row int
	Col int
}

// before reports whether p sorts before q in the buffer.
func (p Position) before(q Position) bool {
	if p.Row != q.Row {
		return p.Row < q.Row
	}
	return p.Col < q.Col
}

// PositionAt maps coordinates within the textarea's rendered area to a buffer
// position. Coordinates are relative to the textarea itself: (0, 0) is its
// top-left cell, including the prompt/line-number gutter. Callers that render
// the textarea at an offset must subtract that offset first.
//
// Coordinates outside the content resolve to the nearest position: above the
// first line yields the start of the buffer, below the last yields its end,
// and a column past a line's text yields the end of that line.
func (m Model) PositionAt(x, y int) Position {
	if len(m.value) == 0 {
		return Position{}
	}

	targetLine := y + m.viewport.YOffset()
	if targetLine < 0 {
		return Position{}
	}

	// DS2's gutter (prompt + optional line-number column) can vary in
	// width per theme/setting, unlike upstream's fixed prompt-width
	// assumption -- reuse the same rendered-width calculation already used
	// by handleMouseMotion for drag-to-reorder.
	gutterWidth := lipgloss.Width(m.promptView(0, -1)) + lipgloss.Width(m.lineNumberView(0, false, -1))
	contentX := x - gutterWidth
	if contentX < 0 {
		contentX = 0
	}

	display := 0
	for row, line := range m.value {
		wrapped := m.memoizedWrap(line, m.width)
		if len(wrapped) == 0 {
			wrapped = [][]rune{{}}
		}
		base := 0
		for _, wrappedLine := range wrapped {
			if display == targetLine {
				col := base + runeIndexForColumn(wrappedLine, contentX)
				return Position{Row: row, Col: clamp(col, 0, len(line))}
			}
			base += len(wrappedLine)
			display++
		}
	}

	lastRow := len(m.value) - 1
	return Position{Row: lastRow, Col: len(m.value[lastRow])}
}

// runeIndexForColumn returns the index of the rune occupying the given display
// column within runes, accounting for double-width runes. A column past the
// end of the line yields len(runes).
func runeIndexForColumn(runes []rune, col int) int {
	if col <= 0 {
		return 0
	}
	w := 0
	for i, r := range runes {
		rWidth := ansi.StringWidth(string(r))
		if w+rWidth > col {
			return i
		}
		w += rWidth
	}
	return len(runes)
}

// BeginSelection starts a selection at the given textarea-relative
// coordinates, discarding any previous selection, and moves the cursor there.
// Pair it with [Model.ExtendSelection] as the pointer moves and
// [Model.EndSelection] when the drag finishes.
//
// See [Model.PositionAt] for the coordinate convention.
func (m *Model) BeginSelection(x, y int) {
	pos := m.PositionAt(x, y)
	m.selectFrom(pos, pos)
	m.selecting = true
	m.moveCursorTo(pos)
}

// ExtendSelection extends an in-progress selection to the given
// textarea-relative coordinates and moves the cursor there. It is a no-op
// unless [Model.BeginSelection] started a drag.
//
// A selection spanning more than one row is snapped to whole-line
// boundaries (Col 0 on the earlier row, end-of-line on the later row) --
// DS2's .env format is line-oriented, so a selection is never allowed to
// hold just part of a KEY=VALUE line once it crosses into another one. See
// selectFrom, which every selection-setting path (this, BeginSelection,
// SelectAll, keyboard selection) funnels through, so the snap applies
// uniformly.
func (m *Model) ExtendSelection(x, y int) {
	if !m.selecting {
		return
	}
	pos := m.PositionAt(x, y)
	m.selHead = pos
	m.hasSelection = true
	m.snapSelectionToLines()
	m.moveCursorTo(m.selHead)
}

// EndSelection completes an in-progress drag. The selection itself is
// retained so it can be read with [Model.SelectedText]; a zero-width
// selection (a plain click) is discarded.
func (m *Model) EndSelection() {
	m.selecting = false
	if m.selAnchor == m.selHead {
		m.ClearSelection()
	}
}

// SelectAll selects the entire buffer.
func (m *Model) SelectAll() {
	if len(m.value) == 0 {
		return
	}
	lastRow := len(m.value) - 1
	m.selectFrom(
		Position{Row: 0, Col: 0},
		Position{Row: lastRow, Col: len(m.value[lastRow])},
	)
	m.selecting = false
}

// ClearSelection removes the current selection, if any.
func (m *Model) ClearSelection() {
	m.hasSelection = false
	m.selecting = false
	m.selAnchor = Position{}
	m.selHead = Position{}
}

// HasSelection reports whether a non-empty selection is active.
func (m Model) HasSelection() bool {
	return m.hasSelection && m.selAnchor != m.selHead
}

// Selection returns the selected range, normalized so start sorts before end,
// and whether a non-empty selection is active.
func (m Model) Selection() (start, end Position, ok bool) {
	if !m.HasSelection() {
		return Position{}, Position{}, false
	}
	start, end = m.selAnchor, m.selHead
	if end.before(start) {
		start, end = end, start
	}
	return start, end, true
}

// SelectedText returns the selected text, with logical lines joined by "\n".
// It returns the empty string when nothing is selected. Read-only lines are
// included -- read-only only restricts editing, not copying (see
// deleteSelectionDS2 for the write-path policy, which does exclude them).
func (m Model) SelectedText() string {
	start, end, ok := m.Selection()
	if !ok {
		return ""
	}

	if start.Row == end.Row {
		line := m.value[start.Row]
		return string(line[clamp(start.Col, 0, len(line)):clamp(end.Col, 0, len(line))])
	}

	var b strings.Builder
	for row := start.Row; row <= end.Row; row++ {
		line := m.value[row]
		switch row {
		case start.Row:
			b.WriteString(string(line[clamp(start.Col, 0, len(line)):]))
		case end.Row:
			b.WriteString(string(line[:clamp(end.Col, 0, len(line))]))
		default:
			b.WriteString(string(line))
		}
		if row < end.Row {
			b.WriteRune('\n')
		}
	}
	return b.String()
}

// selectFrom sets the selection anchor and head, then applies the
// whole-line snap for cross-row selections (see ExtendSelection).
func (m *Model) selectFrom(anchor, head Position) {
	m.selAnchor = anchor
	m.selHead = head
	m.hasSelection = true
	m.snapSelectionToLines()
}

// snapSelectionToLines expands a cross-row selection to whole-line
// boundaries in place: Col 0 on whichever of selAnchor/selHead sorts
// first, end-of-line on whichever sorts last. No-op for a single-row
// selection, where partial-line content is fine.
func (m *Model) snapSelectionToLines() {
	if m.selAnchor.Row == m.selHead.Row {
		return
	}
	first, second := &m.selAnchor, &m.selHead
	if second.before(*first) {
		first, second = second, first
	}
	first.Col = 0
	if second.Row >= 0 && second.Row < len(m.value) {
		second.Col = len(m.value[second.Row])
	}
}

// moveCursorTo places the cursor at the given buffer position, clamped to the
// buffer's bounds.
func (m *Model) moveCursorTo(pos Position) {
	if len(m.value) == 0 {
		return
	}
	row := clamp(pos.Row, 0, len(m.value)-1)
	m.row = row
	m.col = clamp(pos.Col, 0, len(m.value[row]))
}


// startKeyboardSelection begins (or continues) a shift-modified keyboard
// selection at the cursor's current position.
func (m *Model) startKeyboardSelection() {
	if !m.hasSelection {
		m.selAnchor = Position{Row: m.row, Col: m.col}
	}
	m.hasSelection = true
	m.selHead = Position{Row: m.row, Col: m.col}
}

// updateKeyboardSelection moves the selection head to the cursor's current
// position (call after moving the cursor with shift held) and applies the
// whole-line snap for cross-row selections.
func (m *Model) updateKeyboardSelection() {
	m.selHead = Position{Row: m.row, Col: m.col}
	m.snapSelectionToLines()
	m.moveCursorTo(m.selHead)
}
