package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update implements tea.Model.
func (m *ContextMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.screenW = msg.Width
		m.screenH = msg.Height
		m.recalculate()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, Keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, Keys.Enter):
			return m, m.executeSelected()
		case key.Matches(msg, Keys.Right):
			// Right arrow opens a submenu if the focused item has one.
			if m.cursor >= 0 && m.cursor < len(m.items) && len(m.items[m.cursor].SubItems) > 0 {
				return m, m.executeSelected()
			}
		case key.Matches(msg, Keys.Esc):
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}

	case LayerHitMsg:
		if msg.ID == "ctxmenu.bg" {
			// Click outside the item list — close
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		// Per-item hit: "ctxmenu.item-N"
		if strings.HasPrefix(msg.ID, "ctxmenu.item-") {
			idxStr := strings.TrimPrefix(msg.ID, "ctxmenu.item-")
			idx := parseIntSafe(idxStr)
			if idx >= 0 && idx < len(m.items) && !m.items[idx].IsSeparator && !m.items[idx].IsHeader && !m.items[idx].Disabled {
				m.cursor = idx
				if msg.Button == tea.MouseLeft {
					return m, m.executeSelected()
				}
			}
		}

	case LayerWheelMsg:
		if msg.Button == tea.MouseWheelDown {
			m.scrollBy(1)
		} else {
			m.scrollBy(-1)
		}

	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelDown {
			m.scrollBy(1)
		} else {
			m.scrollBy(-1)
		}

	case tea.MouseMotionMsg:
		// Update the highlighted item on mouse hover so HelpText() reflects the
		// item under the cursor. model_update.go calls h.HelpText() after every
		// dialog Update(), so the helpline updates automatically.
		idx := m.itemIndexAt(msg.X, msg.Y)
		if idx >= 0 && idx < len(m.items) && !m.items[idx].IsSeparator && !m.items[idx].IsHeader && !m.items[idx].Disabled {
			m.cursor = idx
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
			if m.cursor >= m.offset+m.maxVisible {
				m.offset = m.cursor - m.maxVisible + 1
			}
		}
	}

	return m, nil
}

// itemIndexAt returns the absolute item index at screen coordinates (x, y),
// or -1 if the coordinates are outside the menu content area.
func (m *ContextMenuModel) itemIndexAt(x, y int) int {
	// Content rows begin at menuY+1 (inside the top border).
	rowY := m.menuY + 1
	absIdx := m.offset
	for _, item := range m.visibleItems() {
		h := itemHeight(item)
		if y >= rowY && y < rowY+h && x >= m.menuX+1 && x < m.menuX+m.menuW+3 {
			return absIdx
		}
		rowY += h
		absIdx++
	}
	return -1
}
