package classic

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
		if m.subMenu != nil {
			m.subMenu.screenW = msg.Width
			m.subMenu.screenH = msg.Height
			m.subMenu.recalculate()
		}

	case tea.KeyPressMsg:
		// Route keyboard to submenu when open
		if m.subMenu != nil {
			updated, cmd := m.subMenu.Update(msg)
			if sub, ok := updated.(*ContextMenuModel); ok {
				if sub.isClosed {
					m.subMenu = nil
					return m, nil
				}
				m.subMenu = sub
			}
			return m, cmd
		}
		switch {
		case key.Matches(msg, Keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, Keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, Keys.Enter), key.Matches(msg, Keys.Space):
			return m, m.executeSelected()
		case key.Matches(msg, Keys.Right):
			// Right arrow opens a submenu if the focused item has one.
			if m.cursor >= 0 && m.cursor < len(m.items) && len(m.items[m.cursor].SubItems) > 0 {
				return m, m.executeSelected()
			}
		case key.Matches(msg, Keys.Esc), key.Matches(msg, Keys.Left):
			m.isClosed = true
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}

	case LayerHitMsg:
		// Route prefixed hits to submenu
		if m.subMenu != nil && strings.HasPrefix(msg.ID, "ctxmenu.sub.") {
			subMsg := msg
			subMsg.ID = strings.TrimPrefix(msg.ID, "ctxmenu.sub.")
			updated, cmd := m.subMenu.Update(subMsg)
			if sub, ok := updated.(*ContextMenuModel); ok {
				if sub.isClosed {
					m.subMenu = nil
					return m, nil
				}
				m.subMenu = sub
			}
			return m, cmd
		}
		// Middle click selects the focused item (cursor), not the item under the mouse.
		if msg.Button == tea.MouseMiddle {
			if m.subMenu != nil {
				subMsg := msg
				subMsg.Button = tea.MouseMiddle
				updated, cmd := m.subMenu.Update(subMsg)
				if sub, ok := updated.(*ContextMenuModel); ok {
					if sub.isClosed {
						m.subMenu = nil
						return m, nil
					}
					m.subMenu = sub
				}
				return m, cmd
			}
			if m.cursor >= 0 && m.cursor < len(m.items) && !m.items[m.cursor].IsSeparator && !m.items[m.cursor].IsHeader && !m.items[m.cursor].Disabled {
				return m, m.executeSelected()
			}
			return m, nil
		}
		if msg.ID == "ctxmenu.bg" {
			if m.subMenu != nil {
				// Click on parent area while submenu open — close submenu only
				m.subMenu = nil
				return m, nil
			}
			// Click outside — close everything
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		// Per-item hit: "ctxmenu.item-N"
		if strings.HasPrefix(msg.ID, "ctxmenu.item-") {
			idxStr := strings.TrimPrefix(msg.ID, "ctxmenu.item-")
			idx := parseIntSafe(idxStr)
			if idx >= 0 && idx < len(m.items) && !m.items[idx].IsSeparator && !m.items[idx].IsHeader && !m.items[idx].Disabled {
				m.subMenu = nil // clicking a parent item closes any open submenu
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
		}
	}

	return m, nil
}

// itemIndexAt returns the absolute item index at screen coordinates (x, y),
// or -1 if the coordinates are outside the menu content area.
func (m *ContextMenuModel) itemIndexAt(x, y int) int {
	// Content rows begin at menuY+1 (inside the top border).
	rowY := m.menuY + 1
	pinned := m.pinnedCount()
	for vi, item := range m.visibleItems() {
		var absIdx int
		if vi < pinned {
			absIdx = vi
		} else {
			absIdx = pinned + m.offset + (vi - pinned)
		}
		h := itemHeight(item)
		if y >= rowY && y < rowY+h && x >= m.menuX+1 && x < m.menuX+m.menuW+3 {
			return absIdx
		}
		rowY += h
	}
	return -1
}
