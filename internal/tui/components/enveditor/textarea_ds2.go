// DS2-added extensions to the textarea component.
// This file contains types, methods, and functions that are new additions
// to the forked charmbracelet/bubbles textarea — not modifications of the
// original source. Functions that were modified from the original remain in
// textarea.go.
package enveditor

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"DockSTARTer2/internal/tui/components/enveditor/memoization"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// undoSnapshot captures editor state before a modifying operation.
type undoSnapshot struct {
	value    [][]rune
	lineMeta []Line
	row, col int
}

// Line represents a single line in our environment variable editor.
type Line struct {
	Text             string
	ReadOnly         bool
	IsVariable       bool // specific formatting for KEY=VALUE
	IsUserDefined    bool // can be reordered
	EditableStartCol int
	DefaultValue     string
	PendingDelete    bool   // marked for deletion on next save; shown with strikethrough
	InitialLine      string // full line text at load time, used for changed (C) gutter marker
	IsNewLine        bool   // added by the user after load; shows + in gutter
	IsInvalid        bool   // in user-defined section but key is in readOnlyVars; shows ! in gutter
	IsComment        bool   // line is a comment (# or ***)
}

// IsOverwrite returns true when the textarea is in overwrite (replace) mode.
func (m Model) IsOverwrite() bool { return m.Overwrite }

// ToggleOverwrite switches between insert and overwrite mode without moving the cursor.
func (m *Model) ToggleOverwrite() { m.Overwrite = !m.Overwrite }

// IsEditableAtCursor returns true when the cursor is on an editable position.
func (m Model) IsEditableAtCursor() bool { return m.isEditableAtCursor() }

// SetLineMeta updates the metadata for a specific row.
func (m *Model) SetLineMeta(row int, l Line) {
	if row >= 0 && row < len(m.lineMeta) {
		m.lineMeta[row] = l
		m.cacheValid = false
	}
}

// CurrentVariableName returns the variable key on the current cursor line, or "" if none.
func (m Model) CurrentVariableName() string {
	if m.row >= len(m.value) || m.row >= len(m.lineMeta) {
		return ""
	}
	if !m.lineMeta[m.row].IsVariable {
		return ""
	}
	line := string(m.value[m.row])
	eqIdx := strings.Index(line, "=")
	if eqIdx <= 0 {
		return ""
	}
	return strings.TrimSpace(line[:eqIdx])
}

// LineAt returns the raw string content of the given row, or "" if out of range.
func (m Model) LineAt(row int) string {
	if row < 0 || row >= len(m.value) {
		return ""
	}
	return string(m.value[row])
}

// GotoFirstEditable moves the cursor to the first editable position in the buffer.
func (m *Model) GotoFirstEditable() {
	for row, meta := range m.lineMeta {
		if meta.ReadOnly || meta.PendingDelete {
			continue
		}
		m.row = row
		m.col = meta.EditableStartCol
		m.repositionView()
		return
	}
	m.row = 0
	m.col = 0
}

// GetContent returns the reconstituted .env file content, excluding any lines
// marked as PendingDelete (those will be removed when the file is saved).
func (m *Model) GetContent() string {
	var sb strings.Builder
	needNewline := false
	for i, l := range m.value {
		if i < len(m.lineMeta) && m.lineMeta[i].PendingDelete {
			continue
		}
		if needNewline {
			sb.WriteString("\n")
		}
		sb.WriteString(string(l))
		needNewline = true
	}
	// Always end with a trailing newline.
	if needNewline {
		sb.WriteString("\n")
	}
	return sb.String()
}

// ActiveLines returns the buffer as a []string with PendingDelete lines excluded.
func (m *Model) ActiveLines() []string {
	out := make([]string, 0, len(m.value))
	for i, l := range m.value {
		if i < len(m.lineMeta) && m.lineMeta[i].PendingDelete {
			continue
		}
		out = append(out, string(l))
	}
	return out
}

// CurrentLineMeta returns the meta information of the current line
func (m *Model) CurrentLineMeta() (Line, bool) {
	if m.row < len(m.lineMeta) {
		return m.lineMeta[m.row], true
	}
	return Line{}, false
}

// AddVariable appends a new variable line to the editor
func (m *Model) AddVariable(key string, value string) {
	m.diffCache = make(map[int][]bool)
	m.insertVariableAt(len(m.value), key, value)
}

func (m *Model) insertVariableAt(row int, key string, value string) {
	m.diffCache = make(map[int][]bool)
	newLine := key + "=" + value
	if key == "" && value == "" {
		newLine = ""
	}
	l := Line{
		Text:             newLine,
		IsVariable:       key != "",
		IsUserDefined:    true,
		EditableStartCol: 0,
		IsNewLine:        true, // added by user after load
	}

	if key != "" && m.ValidateFunc != nil {
		vType := m.ValidationType
		if vType == "APPNAME" {
			vType = m.ValidationAppName
		}
		vKey := key
		if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
			if strings.HasSuffix(vType, ":") {
				vKey = vType + key
			} else {
				vKey = vType + ":" + key
			}
		}
		l.IsInvalid = !m.ValidateFunc(vKey, vType)
	}

	if key != "" {
		l.EditableStartCol = len(key) + 1
	}

	if row >= len(m.value) {
		m.value = append(m.value, []rune(newLine))
		m.lineMeta = append(m.lineMeta, l)
		m.row = len(m.value) - 1
	} else {
		m.value = slices.Insert(m.value, row, []rune(newLine))
		m.lineMeta = slices.Insert(m.lineMeta, row, l)
		m.row = row
	}
	m.col = 0
	if key != "" {
		m.col = len(newLine)
	}
	m.repositionView()
}

// MoveVariableUp swaps the current row with the row above it if both are not read-only.
func (m *Model) MoveVariableUp() {
	if m.row <= 0 || m.row >= len(m.value) {
		return
	}
	// Allow pending-delete lines to move (they can be restored); block truly read-only lines.
	cur, prev := m.lineMeta[m.row], m.lineMeta[m.row-1]
	if (cur.ReadOnly && !cur.PendingDelete) || (prev.ReadOnly && !prev.PendingDelete) {
		return
	}
	if !cur.IsUserDefined || !prev.IsUserDefined {
		return
	}

	// Swap value
	m.value[m.row], m.value[m.row-1] = m.value[m.row-1], m.value[m.row]
	// Swap meta
	m.lineMeta[m.row], m.lineMeta[m.row-1] = m.lineMeta[m.row-1], m.lineMeta[m.row]

	m.row--
	m.repositionView()
	m.InvalidateCache()
}

// MoveVariableDown swaps the current row with the row below it if both are not read-only.
func (m *Model) MoveVariableDown() {
	if m.row >= len(m.value)-1 {
		return
	}
	// Allow pending-delete lines to move (they can be restored); block truly read-only lines.
	cur, next := m.lineMeta[m.row], m.lineMeta[m.row+1]
	if (cur.ReadOnly && !cur.PendingDelete) || (next.ReadOnly && !next.PendingDelete) {
		return
	}
	if !cur.IsUserDefined || !next.IsUserDefined {
		return
	}

	// Swap value
	m.value[m.row], m.value[m.row+1] = m.value[m.row+1], m.value[m.row]
	// Swap meta
	m.lineMeta[m.row], m.lineMeta[m.row+1] = m.lineMeta[m.row+1], m.lineMeta[m.row]

	m.row++
	m.repositionView()
	m.InvalidateCache()
}

// DeleteCurrentVariable marks the row under the cursor as pending deletion.
// The line stays visible with strikethrough styling until the file is saved.
// Ctrl+Z (undo) can restore it. Refresh from disk also clears all pending deletes.
func (m *Model) DeleteCurrentVariable() bool {
	if m.row >= len(m.lineMeta) || m.lineMeta[m.row].ReadOnly {
		return false
	}
	m.pushUndoSnapshot()
	m.lineMeta[m.row].PendingDelete = true
	m.lineMeta[m.row].ReadOnly = true // prevent editing while pending
	m.InvalidateCache()
	return true
}

// UndeleteCurrentVariable clears the PendingDelete flag on the current line,
// restoring it to its pre-deletion state.
func (m *Model) UndeleteCurrentVariable() bool {
	if m.row >= len(m.lineMeta) || !m.lineMeta[m.row].PendingDelete {
		return false
	}
	m.pushUndoSnapshot()
	m.lineMeta[m.row].PendingDelete = false
	m.lineMeta[m.row].ReadOnly = false
	m.InvalidateCache()
	return true
}

// UndeleteVariableByName finds the first PendingDelete row containing varName= and restores it.
func (m *Model) UndeleteVariableByName(varName string) bool {
	prefix := varName + "="
	for row, meta := range m.lineMeta {
		if !meta.PendingDelete {
			continue
		}
		line := strings.TrimSpace(string(m.value[row]))
		if strings.HasPrefix(line, prefix) {
			saved := m.row
			m.row = row
			ok := m.UndeleteCurrentVariable()
			if !ok {
				m.row = saved
			}
			return ok
		}
	}
	return false
}

// DeleteVariableByName finds the first row containing varName= and deletes it.
func (m *Model) DeleteVariableByName(varName string) bool {
	prefix := varName + "="
	for row, meta := range m.lineMeta {
		if meta.ReadOnly {
			continue
		}
		line := strings.TrimSpace(string(m.value[row]))
		if strings.HasPrefix(line, prefix) {
			saved := m.row
			m.row = row
			ok := m.DeleteCurrentVariable()
			if !ok {
				m.row = saved
			}
			return ok
		}
	}
	return false
}

// CursorVisualCol returns the visual (screen) column of the cursor, including
// the line number gutter and prompt widths — mirrors the xOffset calculation
// used for the hardware cursor in GetInputCursor.
func (m *Model) CursorVisualCol() int {
	lineInfo := m.LineInfo()
	w := lipgloss.Width
	baseStyle := m.activeStyle().Base
	return lineInfo.CharOffset +
		w(m.promptView(0, -1)) +
		w(m.lineNumberView(0, false, -1)) +
		baseStyle.GetMarginLeft() +
		baseStyle.GetPaddingLeft() +
		baseStyle.GetBorderLeftSize()
}

// CursorVisualRow returns the visual (screen) row index of the cursor, accounting
// for wrapped lines above it.
func (m *Model) CursorVisualRow() int {
	curr := 0
	for l, lineRunes := range m.value {
		if l == m.row {
			return curr
		}
		curr += len(m.memoizedWrap(lineRunes, m.width))
	}
	return curr
}

// SetVariableValue finds the row for varName, replaces its value, and invalidates the cache.
// The new value is written as-is after the '=' sign (include quoting if needed by the caller).
// Returns true if the variable was found and updated. Read-only rows are skipped.
func (m *Model) SetVariableValue(varName, newValue string) bool {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable || meta.ReadOnly {
			continue
		}
		// The editable start column is len(varName)+1 (position after '=').
		// Verify this row's key matches varName.
		startCol := meta.EditableStartCol
		if startCol < 1 || startCol > len(m.value[row]) {
			continue
		}
		rowKey := string(m.value[row][:startCol-1]) // everything before '='
		if rowKey != varName {
			continue
		}
		m.pushUndoSnapshot()
		// Update the value portion of the line.
		prefix := string(m.value[row][:startCol]) // includes '='
		m.value[row] = []rune(prefix + newValue)
		m.lineMeta[row].Text = string(m.value[row])
		m.InvalidateCache()
		return true
	}
	return false
}

// GetSelectedText returns the currently selected text, or "" if no selection is active.
func (m *Model) GetSelectedText() string {
	if !m.selActive || m.selRow < 0 || m.selRow >= len(m.value) {
		return ""
	}
	line := m.value[m.selRow]
	s := clamp(m.selStartCol, 0, len(line))
	e := clamp(m.selEndCol, 0, len(line))
	if s >= e {
		return ""
	}
	return string(line[s:e])
}

// GetVariableValue returns everything after '=' for varName (raw, including any surrounding quotes), or "" if not found.
func (m *Model) GetVariableValue(varName string) string {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return lineStr[eqIdx+1:]
			}
		}
	}
	return ""
}

// GetVariableMeta returns the Line metadata for varName, or false if not found.
func (m *Model) GetVariableMeta(varName string) (Line, bool) {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return meta, true
			}
		}
	}
	return Line{}, false
}

// GetVariableInitialValue returns everything after '=' in InitialLine for varName, or "" if not found.
func (m *Model) GetVariableInitialValue(varName string) string {
	for _, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		if eqIdx := strings.Index(meta.InitialLine, "="); eqIdx > 0 {
			if strings.TrimSpace(meta.InitialLine[:eqIdx]) == varName {
				return meta.InitialLine[eqIdx+1:]
			}
		}
	}
	return ""
}

// HasVariable returns true if varName exists in the editor (regardless of its value).
func (m *Model) HasVariable(varName string) bool {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return true
			}
		}
	}
	return false
}

// snapshot returns a deep copy of the current editor state.
func (m *Model) snapshot() undoSnapshot {
	valueCopy := make([][]rune, len(m.value))
	for i, line := range m.value {
		lc := make([]rune, len(line))
		copy(lc, line)
		valueCopy[i] = lc
	}
	metaCopy := make([]Line, len(m.lineMeta))
	copy(metaCopy, m.lineMeta)
	return undoSnapshot{value: valueCopy, lineMeta: metaCopy, row: m.row, col: m.col}
}

// restoreSnapshot applies a snapshot and refreshes caches.
func (m *Model) restoreSnapshot(s undoSnapshot) {
	m.value = s.value
	m.lineMeta = s.lineMeta
	m.row = clamp(s.row, 0, max(0, len(s.value)-1))
	m.col = clamp(s.col, 0, len(m.value[m.row]))
	m.cache = memoization.NewMemoCache[line, [][]rune](m.cache.Capacity())
	m.InvalidateCache()
}

// pushUndoSnapshot saves a deep copy of the current editor state onto the undo stack.
// Any new edit clears the redo stack. The stack is capped at 100 entries.
func (m *Model) pushUndoSnapshot() {
	m.redoStack = nil // new edit invalidates redo history
	m.undoStack = append(m.undoStack, m.snapshot())
	const maxUndoDepth = 100
	if len(m.undoStack) > maxUndoDepth {
		m.undoStack = m.undoStack[1:]
	}
}

// Undo restores the most recent snapshot from the undo stack.
// The current state is pushed onto the redo stack so it can be redone.
// Returns true if a snapshot was available and restored.
func (m *Model) Undo() bool {
	if len(m.undoStack) == 0 {
		return false
	}
	m.redoStack = append(m.redoStack, m.snapshot())
	entry := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.restoreSnapshot(entry)
	return true
}

// Redo reapplies the most recently undone edit.
// Returns true if a redo snapshot was available and restored.
func (m *Model) Redo() bool {
	if len(m.redoStack) == 0 {
		return false
	}
	m.undoStack = append(m.undoStack, m.snapshot())
	entry := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.restoreSnapshot(entry)
	return true
}

// ClearUndo discards all undo and redo history. Call this after a full content
// reload (e.g. refresh) to prevent undoing across the reload boundary.
func (m *Model) ClearUndo() {
	m.undoStack = nil
	m.redoStack = nil
}

func (m *Model) handleMouseClick(msg tea.MouseClickMsg) {
	styles := m.activeStyle()
	msg.X -= styles.Base.GetMarginLeft() + styles.Base.GetPaddingLeft() + styles.Base.GetBorderLeftSize()
	msg.Y -= styles.Base.GetMarginTop() + styles.Base.GetPaddingTop() + styles.Base.GetBorderTopSize()

	// Every left-click clears any prior text selection.
	m.selActive = false
	m.isSelecting = false

	// Gutter width (prompts + line numbers)
	gutterWidth := lipgloss.Width(m.promptView(0, -1)) + lipgloss.Width(m.lineNumberView(0, false, -1))

	total := m.totalDisplayLines()
	visible := m.height
	scrollbarX := m.width + gutterWidth

	// Check if click is on the scrollbar (last column of the viewport area)
	if total > visible && msg.X >= scrollbarX {
		if visible >= 3 {
			// Check up arrow
			if msg.Y == 0 {
				m.viewport.ScrollUp(1)
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
				return
			}
			// Check down arrow
			if msg.Y == visible-1 {
				m.viewport.ScrollDown(1)
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
				return
			}

			trackH := visible - 2
			maxOff := total - visible
			thumbH := max(1, trackH*visible/total)
			offset := m.viewport.YOffset()

			thumbStart := 0
			if maxOff > 0 {
				thumbStart = (trackH - thumbH) * offset / maxOff
			}
			thumbEnd := thumbStart + thumbH

			trackRelY := msg.Y - 1
			if trackRelY >= thumbStart && trackRelY < thumbEnd {
				m.isScrollbarDragging = true
				m.sbDragMouseOffsetY = trackRelY - thumbStart
			} else {
				if trackH > 1 {
					targetPct := float64(trackRelY) / float64(trackH-1)
					targetOffset := int(targetPct * float64(maxOff))
					m.viewport.SetYOffset(clamp(targetOffset, 0, maxOff))
				}
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
			}
		} else {
			trackH := visible
			thumbH := max(1, trackH*visible/total)
			maxOff := total - visible

			offset := m.viewport.YOffset()
			thumbTrackStart := 0
			if maxOff > 0 {
				thumbTrackStart = (trackH - thumbH) * offset / maxOff
			}
			thumbEnd := thumbTrackStart + thumbH

			if msg.Y >= thumbTrackStart && msg.Y < thumbEnd {
				// Clicked on the thumb
				m.isScrollbarDragging = true
				m.sbDragMouseOffsetY = msg.Y - thumbTrackStart
			} else {
				if trackH > 1 {
					targetPct := float64(msg.Y) / float64(trackH-1)
					targetOffset := int(targetPct * float64(maxOff))
					m.viewport.SetYOffset(clamp(targetOffset, 0, maxOff))
				}
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
			}
		}
		return
	}

	// Adjust for viewport scroll
	targetViewLine := msg.Y + m.viewport.YOffset()
	targetColX := msg.X - gutterWidth

	// Check if click is in the gutter area (line numbers)
	if msg.X < gutterWidth {
		// Find which logical row was clicked
		currViewLine := 0
		for l, lineRunes := range m.value {
			wrappedLines := m.memoizedWrap(lineRunes, m.width)
			numWrapped := len(wrappedLines)
			if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
				lm := m.lineMeta[l]
				if lm.IsUserDefined && (!lm.ReadOnly || lm.PendingDelete) {
					m.isDragging = true
					m.draggedRow = l
					m.row = l
					m.CursorStart()
				}
				return
			}
			currViewLine += numWrapped
		}
	}

	// Find logical row and column by iterating through m.value and wrapped lines
	currViewLine := 0
	for l, lineRunes := range m.value {
		wrappedLines := m.memoizedWrap(lineRunes, m.width)
		numWrapped := len(wrappedLines)

		if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
			// Click is on this logical line
			m.row = l

			// Find which wrapped line it is
			wrappedLineIdx := targetViewLine - currViewLine

			// Find the character index in the logical line
			charIdx := 0
			for i := 0; i < wrappedLineIdx; i++ {
				charIdx += len(wrappedLines[i])
			}

			// Find the column within the wrapped line
			clickedWrappedCol := clamp(targetColX, 0, len(wrappedLines[wrappedLineIdx]))
			m.col = charIdx + clickedWrappedCol

			// Clamp to actual line length
			if m.col > len(lineRunes) {
				m.col = len(lineRunes)
			}

			// Multi-click detection: same row, same col, within 400 ms.
			const multiClickWindow = 400 * time.Millisecond
			now := time.Now()
			if m.row == m.lastClickRow && m.col == m.lastClickCol &&
				now.Sub(m.lastClickTime) <= multiClickWindow {
				m.clickCount++
			} else {
				m.clickCount = 1
			}
			m.lastClickTime = now
			m.lastClickRow = m.row
			m.lastClickCol = m.col

			switch m.clickCount {
			case 2:
				// Double-click: select the word at cursor ('=' is a boundary).
				s, e := wordBoundsAt(lineRunes, m.col)
				if s < e {
					m.selRow = m.row
					m.selStartCol = s
					m.selEndCol = e
					m.selAnchorCol = s
					m.selActive = true
					m.isSelecting = false // selection complete
				}
			case 3:
				// Triple-click on value side: select entire value (after '=').
				// On key side: no change — keep the word selection from double-click.
				eqIdx := -1
				for i, r := range lineRunes {
					if r == '=' {
						eqIdx = i
						break
					}
				}
				if eqIdx >= 0 && m.col > eqIdx {
					m.selRow = m.row
					m.selStartCol = eqIdx + 1
					m.selEndCol = len(lineRunes)
					m.selAnchorCol = eqIdx + 1
					m.selActive = m.selStartCol < m.selEndCol
					m.isSelecting = false
				}
				// On key side: selection stays as-is from double-click.
			default: // 1 or 4+
				if m.clickCount >= 4 {
					// Four clicks: select entire line.
					m.selRow = m.row
					m.selStartCol = 0
					m.selEndCol = len(lineRunes)
					m.selAnchorCol = 0
					m.selActive = m.selStartCol < m.selEndCol
					m.isSelecting = false
				} else {
					// Single click: just set anchor for potential drag.
					m.isSelecting = true
					m.selRow = m.row
					m.selAnchorCol = m.col
					m.selStartCol = m.col
					m.selEndCol = m.col
				}
			}

			return
		}
		currViewLine += numWrapped
	}
}

func (m *Model) handleMouseMotion(msg tea.MouseMotionMsg) {
	styles := m.activeStyle()
	msg.X -= styles.Base.GetMarginLeft() + styles.Base.GetPaddingLeft() + styles.Base.GetBorderLeftSize()
	msg.Y -= styles.Base.GetMarginTop() + styles.Base.GetPaddingTop() + styles.Base.GetBorderTopSize()

	if m.isScrollbarDragging {
		total := m.totalDisplayLines()
		visible := m.height
		if total > visible {
			if visible >= 3 {
				trackH := visible - 2
				maxOff := total - visible
				thumbH := max(1, trackH*visible/total)
				thumbTravel := trackH - thumbH
				if thumbTravel < 1 {
					thumbTravel = 1
				}

				trackRelY := msg.Y - 1
				thumbTrackStart := trackRelY - m.sbDragMouseOffsetY
				if thumbTrackStart < 0 {
					thumbTrackStart = 0
				}
				if thumbTrackStart > thumbTravel {
					thumbTrackStart = thumbTravel
				}

				newOff := thumbTrackStart * maxOff / thumbTravel
				m.viewport.SetYOffset(clamp(newOff, 0, maxOff))
			} else {
				trackH := visible
				maxOff := total - visible
				thumbH := max(1, trackH*visible/total)
				thumbTravel := trackH - thumbH
				if thumbTravel < 1 {
					thumbTravel = 1
				}

				thumbTrackStart := msg.Y - m.sbDragMouseOffsetY
				if thumbTrackStart < 0 {
					thumbTrackStart = 0
				}
				if thumbTrackStart > thumbTravel {
					thumbTrackStart = thumbTravel
				}

				newOff := thumbTrackStart * maxOff / thumbTravel
				m.viewport.SetYOffset(clamp(newOff, 0, maxOff))
			}
			m.constrainCursorToView()
			m.sbScrolled = true
		}
		return
	}

	if m.isSelecting {
		gutterWidth := lipgloss.Width(m.promptView(0, -1)) + lipgloss.Width(m.lineNumberView(0, false, -1))
		targetViewLine := msg.Y + m.viewport.YOffset()
		targetColX := msg.X - gutterWidth
		currViewLine := 0
		for l, lineRunes := range m.value {
			wrappedLines := m.memoizedWrap(lineRunes, m.width)
			numWrapped := len(wrappedLines)
			if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
				if l == m.selRow { // single-row selection only
					wrappedLineIdx := targetViewLine - currViewLine
					charIdx := 0
					for i := 0; i < wrappedLineIdx; i++ {
						charIdx += len(wrappedLines[i])
					}
					curCol := charIdx + clamp(targetColX, 0, len(wrappedLines[wrappedLineIdx]))
					if curCol > len(lineRunes) {
						curCol = len(lineRunes)
					}
					start, end := m.selAnchorCol, curCol
					if start > end {
						start, end = end, start
					}
					m.selStartCol = start
					m.selEndCol = end
					m.selActive = start < end
				}
				return
			}
			currViewLine += numWrapped
		}
		return
	}

	if !m.isDragging {
		return
	}

	targetViewLine := msg.Y + m.viewport.YOffset()

	// Find which logical row the mouse is over
	currViewLine := 0
	for l, lineRunes := range m.value {
		wrappedLines := m.memoizedWrap(lineRunes, m.width)
		numWrapped := len(wrappedLines)

		if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
			targetRow := l
			if targetRow == m.draggedRow {
				currViewLine += numWrapped
				continue
			}

			// Only allow moving to adjacent rows
			if targetRow == m.draggedRow-1 || targetRow == m.draggedRow+1 {
				// Check both rows are user-defined
				dragMeta := m.lineMeta[m.draggedRow]
				targetMeta := m.lineMeta[targetRow]
				if dragMeta.IsUserDefined && targetMeta.IsUserDefined {
					// Swap the rows
					m.value[m.draggedRow], m.value[targetRow] = m.value[targetRow], m.value[m.draggedRow]
					m.lineMeta[m.draggedRow], m.lineMeta[targetRow] = m.lineMeta[targetRow], m.lineMeta[m.draggedRow]
					m.row = targetRow
					m.draggedRow = targetRow
					m.InvalidateCache()
				}
			}
			return
		}
		currViewLine += numWrapped
	}
}

func (m *Model) handleMouseRelease(_ tea.MouseReleaseMsg) {
	m.isDragging = false
	m.isScrollbarDragging = false
	m.isSelecting = false
	// selActive persists — selection remains visible until the next left-click.
}

// IsDragging returns true if the user is currently dragging a line, scrollbar, or text selection.
func (m Model) IsDragging() bool {
	return m.isDragging || m.isScrollbarDragging || m.isSelecting
}

// IsScrollbarDragging reports whether the scrollbar thumb is currently being dragged.
func (m Model) IsScrollbarDragging() bool {
	return m.isScrollbarDragging
}

// invalidateDiffCache removes cached diff data for a single row and marks the view cache stale.
func (m *Model) invalidateDiffCache(row int) {
	if m.diffCache != nil {
		delete(m.diffCache, row)
	}
	m.cacheValid = false
}

func (m *Model) getDiffMask(row int) []bool {
	if m.diffCache == nil {
		m.diffCache = make(map[int][]bool)
	}
	if mask, ok := m.diffCache[row]; ok {
		return mask
	}

	if row >= len(m.lineMeta) || row >= len(m.value) {
		return nil
	}
	meta := m.lineMeta[row]
	if meta.DefaultValue == "" {
		return nil
	}

	lineRunes := m.value[row]
	if meta.EditableStartCol >= len(lineRunes) {
		return nil
	}

	valuePartRunes := lineRunes[meta.EditableStartCol:]
	// Filter out trailing newlines for diff purposes
	for len(valuePartRunes) > 0 && (valuePartRunes[len(valuePartRunes)-1] == '\n' || valuePartRunes[len(valuePartRunes)-1] == '\r') {
		valuePartRunes = valuePartRunes[:len(valuePartRunes)-1]
	}

	valuePart := string(valuePartRunes)
	defValue := meta.DefaultValue

	diffs := m.dmp.DiffMain(defValue, valuePart, false)

	mask := make([]bool, len(valuePartRunes))
	cursor := 0
	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			cursor += len([]rune(d.Text))
		case diffmatchpatch.DiffInsert:
			runes := []rune(d.Text)
			for i := 0; i < len(runes); i++ {
				if cursor+i < len(mask) {
					mask[cursor+i] = true
				}
			}
			cursor += len(runes)
		case diffmatchpatch.DiffDelete:
			// Deletions don't occupy space in the buffer
		}
	}
	m.diffCache[row] = mask
	return mask
}

// renderRunes formats runes with partial highlighting.
func (m *Model) renderRunes(runes []rune, l int, startIdx int, baseStyle lipgloss.Style) string {
	// Strip trailing newline so it is never included in a styled Render call —
	// styled newlines appear as a trailing coloured/reversed block in the terminal.
	for len(runes) > 0 && (runes[len(runes)-1] == '\n' || runes[len(runes)-1] == '\r') {
		runes = runes[:len(runes)-1]
	}
	if l >= len(m.lineMeta) {
		return baseStyle.Render(string(runes))
	}
	meta := &m.lineMeta[l]
	if meta.PendingDelete {
		return m.activeStyle().PendingDeleteText.Inherit(baseStyle).Render(string(runes))
	}
	if meta.ReadOnly {
		if meta.IsComment {
			return m.activeStyle().CommentText.Inherit(baseStyle).Render(string(runes))
		}
		return m.activeStyle().ReadOnlyText.Inherit(baseStyle).Render(string(runes))
	}
	if !meta.IsVariable && !meta.IsUserDefined {
		return baseStyle.Render(string(runes))
	}

	// Determine if the current variable name is valid
	keyIsValid := true
	keyIsDuplicate := false
	eqIdx := -1
	if (meta.IsVariable || meta.IsUserDefined) && m.ValidateFunc != nil && m.ValidationType != "" {
		lineRunes := m.value[l]
		for i, r := range lineRunes {
			if r == '=' {
				eqIdx = i
				break
			}
		}
		if eqIdx > 0 {
			key := strings.TrimSpace(string(lineRunes[:eqIdx]))
			vType := m.ValidationType
			if vType == "APPNAME" {
				vType = m.ValidationAppName
			}
			// Internally prepend app prefix for validation if we're in an app-specific tab (bare names)
			vKey := key
			if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
				if strings.HasSuffix(vType, ":") {
					vKey = vType + key
				} else {
					vKey = vType + ":" + key
				}
			}
			keyIsValid = m.ValidateFunc(vKey, vType)

			// Duplicate check using the pre-calculated map
			if m.duplicateKeys != nil && m.duplicateKeys[key] > 1 {
				keyIsDuplicate = true
			}
		} else if len(lineRunes) > 0 {
			// Early validation: check if the partial key is valid in this context
			key := strings.TrimSpace(string(lineRunes))
			vType := m.ValidationType
			if vType == "APPNAME" {
				vType = m.ValidationAppName
			}
			vKey := key
			if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
				if strings.HasSuffix(vType, ":") {
					vKey = vType + key
				} else {
					vKey = vType + ":" + key
				}
			}
			keyIsValid = m.ValidateFunc(vKey, vType)
		}
	}

	var b strings.Builder
	styles := m.activeStyle()
	modStyle := styles.ModifiedText.Inherit(baseStyle)
	invalidStyle := styles.InvalidText.Inherit(baseStyle)
	duplicateStyle := styles.DuplicateText.Inherit(baseStyle)
	builtinKeyStyle := styles.BuiltinText.Inherit(baseStyle).Inline(true)

	for i, r := range runes {
		fullIdx := startIdx + i

		// Selection highlight takes priority over other styles.
		if m.selActive && l == m.selRow && fullIdx >= m.selStartCol && fullIdx < m.selEndCol {
			b.WriteString(m.activeStyle().SelectionText.Inherit(baseStyle).Render(string(r)))
			continue
		}

		// Key part highlighting
		if (eqIdx >= 0 && fullIdx < meta.EditableStartCol-1) || (eqIdx == -1 && (meta.IsVariable || meta.IsUserDefined)) {
			if !keyIsValid {
				b.WriteString(invalidStyle.Render(string(r)))
				continue
			}
			if keyIsDuplicate {
				b.WriteString(duplicateStyle.Render(string(r)))
				continue
			}
			if meta.IsUserDefined {
				b.WriteString(modStyle.Render(string(r)))
			} else {
				b.WriteString(builtinKeyStyle.Render(string(r)))
			}
			continue
		}

		// '=' or following content
		if fullIdx < meta.EditableStartCol {
			b.WriteString(baseStyle.Render(string(r)))
		} else {
			valIdx := fullIdx - meta.EditableStartCol
			mask := m.getDiffMask(l)

			isModifiedChar := false
			if mask != nil && valIdx >= 0 && valIdx < len(mask) {
				isModifiedChar = mask[valIdx]
			}

			// Note: We don't use meta.DefaultValue == "" || meta.IsUserDefined || meta.IsNewLine logic here
			// because localized diffing only applies when we have a default value to compare against.
			// Variable headers/key/equals are handled above or in caller.
			if r == '\n' || r == '\r' || !isModifiedChar {
				b.WriteString(baseStyle.Render(string(r)))
			} else {
				b.WriteString(modStyle.Render(string(r)))
			}
		}
	}
	return b.String()
}

// isReadOnlyRow returns true if the current row shouldn't be edited at all
func (m *Model) isReadOnlyRow() bool {
	if m.row >= len(m.lineMeta) {
		return false
	}
	return m.lineMeta[m.row].ReadOnly
}

// isEditableAtCursor returns true if the cursor is at or after EditableStartCol
func (m *Model) isEditableAtCursor() bool {
	if m.row >= len(m.lineMeta) {
		return true
	}
	if m.lineMeta[m.row].ReadOnly {
		return false
	}
	if m.lineMeta[m.row].IsUserDefined {
		return true
	}
	return m.col >= m.lineMeta[m.row].EditableStartCol
}

// isBackspaceEditable returns true if a backward deletion is allowed
func (m *Model) isBackspaceEditable() bool {
	if m.row >= len(m.lineMeta) {
		return true
	}
	if m.lineMeta[m.row].ReadOnly {
		return false
	}
	if m.col == 0 {
		// Never join lines on Backspace — use Delete at end of the previous line instead.
		return false
	}
	if m.lineMeta[m.row].IsUserDefined {
		return true
	}
	return m.col > m.lineMeta[m.row].EditableStartCol
}

// HasValidationErrors returns true if any variable name in the editor is invalid
// or if any user-defined line has content but no '=' separator.
func (m *Model) HasValidationErrors() bool {
	if m.ValidateFunc == nil || m.ValidationType == "" {
		return false
	}

	for i, lineRunes := range m.value {
		if i >= len(m.lineMeta) {
			continue
		}
		meta := &m.lineMeta[i]

		// Skip comments and truly read-only lines
		if meta.ReadOnly || meta.IsComment {
			continue
		}

		// Check for the pre-calculated IsInvalid flag derived from reclassifyCurrentLine
		if meta.IsInvalid {
			return true
		}

		// Check for "incomplete" lines: user-defined content that is NOT yet a variable (no '=')
		if meta.IsUserDefined && len(lineRunes) > 0 && !meta.IsVariable {
			return true
		}

		// Validation check for variables
		if meta.IsVariable {
			// Find '=' index
			eqIdx := -1
			for j, r := range lineRunes {
				if r == '=' {
					eqIdx = j
					break
				}
			}

			if eqIdx > 0 {
				// Validate key
				key := string(lineRunes[:eqIdx])
				vType := m.ValidationType
				if vType == "APPNAME" {
					vType = m.ValidationAppName
				}
				// Internally prepend app prefix for validation if we're in an app-specific tab (bare names)
				vKey := key
				if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
					if strings.HasSuffix(vType, ":") {
						vKey = vType + key
					} else {
						vKey = vType + ":" + key
					}
				}
				if !m.ValidateFunc(vKey, vType) {
					return true
				}
			}
		}
	}
	return false
}

// InvalidateCache clears the rendered view cache.
func (m *Model) InvalidateCache() {
	m.cacheValid = false
	m.diffCache = make(map[int][]bool)
}

// CheckCache returns the cached rendered screen if it's still valid.
func (m *Model) CheckCache() (string, bool) {
	if m.cacheValid && m.lastView != "" {
		return m.lastView, true
	}
	return "", false
}

// SaveCache saves the newly generated screen string to the cache and marks it as valid.
func (m *Model) SaveCache(view string) string {
	m.lastView = view
	m.cacheValid = true
	return view
}

// SetLineCharacters sets whether the textarea should use stylized line-art
// characters for its scrollbar.
func (m *Model) SetLineCharacters(v bool) {
	m.LineCharacters = v
	m.InvalidateCache()
}

// constrainCursorToView moves the cursor so it is within the visible viewport.
// It is used during free scrolling to prevent snap-back.
func (m *Model) constrainCursorToView() {
	minimum := m.viewport.YOffset()
	maximum := minimum + m.viewport.Height() - 1
	if row := m.cursorLineNumber(); row < minimum {
		m.setCursorLineRelative(minimum - row)
	} else if row > maximum {
		m.setCursorLineRelative(maximum - row)
	}
}

// totalDisplayLines returns the total number of lines including soft wraps.
func (m Model) totalDisplayLines() int {
	lines := 0
	for i := range m.value {
		lines += len(m.memoizedWrap(m.value[i], m.width))
	}
	return lines
}

// gutterStyleFor returns the lipgloss style that should be used for the gutter
// of the given data line, and whether the line has a non-default marker.
// Used by both promptView (renders the marker char) and promptContinuationView
// (renders a blank space in the same style).
func (m Model) gutterStyleFor(dataLine int) (style lipgloss.Style, hasMarker bool) {
	if dataLine < 0 || dataLine >= len(m.lineMeta) {
		return m.activeStyle().computedPrompt(), false
	}
	styles := m.activeStyle()
	meta := m.lineMeta[dataLine]
	if meta.PendingDelete {
		return styles.GutterDeleted, true
	}
	if meta.IsInvalid {
		return styles.GutterInvalid, true
	}
	if meta.IsVariable {
		lineContent := string(m.value[dataLine])
		if meta.IsNewLine || meta.InitialLine == "" {
			return styles.GutterAdded, true
		}
		if !meta.ReadOnly && meta.InitialLine != "" && lineContent != meta.InitialLine {
			return styles.GutterModified, true
		}
	}
	return m.activeStyle().computedPrompt(), false
}

// promptContinuationView renders a blank gutter cell for soft-wrapped continuation
// rows, styled to match the marker style of the logical line's first row.
func (m Model) promptContinuationView(dataLine int) string {
	style, _ := m.gutterStyleFor(dataLine)
	return style.Render(" ")
}

// reclassifyCurrentLine updates IsVariable, EditableStartCol, and IsUserDefined
// for the current row as the user types. Keeps rendering and key-lock correct
// without a full ReclassifyEnv pass.
func (m *Model) reclassifyCurrentLine() {
	if m.row >= len(m.lineMeta) || m.row >= len(m.value) {
		return
	}
	meta := &m.lineMeta[m.row]
	// Skip pre-existing built-in variables — their key is locked.
	if meta.IsVariable && !meta.IsUserDefined && !meta.IsNewLine {
		return
	}
	line := m.value[m.row]
	eqIdx := -1
	for i, r := range line {
		if r == '=' {
			eqIdx = i
			break
		}
	}
	if eqIdx >= 0 {
		meta.IsVariable = true
		meta.EditableStartCol = eqIdx + 1
		meta.IsUserDefined = true

		key := strings.TrimSpace(string(line[:eqIdx]))
		vType := m.ValidationType
		if vType == "APPNAME" {
			vType = m.ValidationAppName
		}
		vKey := key
		if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
			if strings.HasSuffix(vType, ":") {
				vKey = vType + key
			} else {
				vKey = vType + ":" + key
			}
		}
		if m.ValidateFunc != nil {
			meta.IsInvalid = !m.ValidateFunc(vKey, vType)
		}

		// Resolve default for this user-defined key so value diffs work normally.
		if m.defaultFunc != nil && meta.DefaultValue == "" {
			if key != "" {
				meta.DefaultValue = strings.TrimSpace(m.defaultFunc(key))
				if meta.DefaultValue != "" {
					delete(m.diffCache, m.row)
				}
			}
		}
	} else {
		meta.IsVariable = false
		meta.EditableStartCol = 0

		if len(line) > 0 {
			meta.IsUserDefined = true
			if m.ValidateFunc != nil {
				// Incomplete lines (no '=') are always considered invalid for the gutter marker and saving
				meta.IsInvalid = true
			}
		} else {
			meta.IsUserDefined = false
			meta.IsInvalid = false
		}
	}
}

// wordBoundsAt returns the start (inclusive) and end (exclusive) column indices
// of the word at col within line. '=' is treated as a word separator.
func wordBoundsAt(line []rune, col int) (start, end int) {
	n := len(line)
	if n == 0 || col >= n {
		return col, col
	}
	isSep := func(r rune) bool { return r == '=' || unicode.IsSpace(r) }
	if isSep(line[col]) {
		return col, col
	}
	start = col
	for start > 0 && !isSep(line[start-1]) {
		start--
	}
	end = col
	for end < n && !isSep(line[end]) {
		end++
	}
	return start, end
}

// mergeLineBelow merges the current line the cursor is on with the line below.
func (m *Model) mergeLineBelow(row int) {
	m.diffCache = make(map[int][]bool)
	if row >= len(m.value)-1 {
		return
	}

	// To perform a merge, we will need to combine the two lines and then
	m.value[row] = append(m.value[row], m.value[row+1]...)

	// Shift all lines up by one
	for i := row + 1; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if row+1 < len(m.lineMeta) {
		m.lineMeta = append(m.lineMeta[:row+1], m.lineMeta[row+2:]...)
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

// mergeLineAbove merges the current line the cursor is on with the line above.
func (m *Model) mergeLineAbove(row int) {
	m.diffCache = make(map[int][]bool)
	if row <= 0 {
		return
	}

	m.col = len(m.value[row-1])
	m.row = m.row - 1

	// To perform a merge, we will need to combine the two lines and then
	m.value[row-1] = append(m.value[row-1], m.value[row]...)

	// Shift all lines up by one
	for i := row; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if row < len(m.lineMeta) {
		m.lineMeta = append(m.lineMeta[:row], m.lineMeta[row+1:]...)
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

// splitLine splits the line at row at col, creating a new line below.
func (m *Model) splitLine(row, col int) {
	m.diffCache = make(map[int][]bool)
	// To perform a split, take the current line and keep the content before
	// the cursor, take the content after the cursor and make it the content of
	// the line underneath, and shift the remaining lines down by one
	head, tailSrc := m.value[row][:col], m.value[row][col:]
	tail := make([]rune, len(tailSrc))
	copy(tail, tailSrc)

	m.value = append(m.value[:row+1], m.value[row:]...)

	m.value[row] = head
	m.value[row+1] = tail

	// Duplicate meta if it exists
	if row < len(m.lineMeta) {
		oldMeta := m.lineMeta[row]
		newMeta := oldMeta
		if oldMeta.IsUserDefined {
			// New user-defined line starts fresh for a new key
			newMeta.EditableStartCol = 0
			newMeta.IsVariable = true
			newMeta.DefaultValue = ""
		}
		m.lineMeta = append(m.lineMeta[:row+1], append([]Line{newMeta}, m.lineMeta[row+1:]...)...)
	}

	m.col = 0
	m.row++
}
