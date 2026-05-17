package tui

import (
	keybind "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *HelpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalDocLines, visibleDocRows := m.docInfo()
	if newOff, cmd, changed := m.Scroll.Update(msg, m.contextOffset, totalDocLines, visibleDocRows); changed {
		m.contextOffset = newOff
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Help key (? / F1) cycles pages when paged, otherwise closes.
		if keybind.Matches(msg, Keys.Help) {
			if m.paged {
				n := m.numPages
				if n < 2 {
					n = 2
				}
				m.page = (m.page + 1) % n
				m.contextOffset = 0
				return m, nil
			}
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		switch {
		case keybind.Matches(msg, Keys.Up):
			m.contextOffset--
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
			return m, nil
		case keybind.Matches(msg, Keys.Down):
			m.contextOffset++
			return m, nil
		case keybind.Matches(msg, Keys.PageUp):
			m.contextOffset -= 5
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
			return m, nil
		case keybind.Matches(msg, Keys.PageDown):
			m.contextOffset += 5
			return m, nil
		case keybind.Matches(msg, Keys.Home):
			m.contextOffset = 0
			return m, nil
		}
		// Any other key closes the help dialog (Esc also works)
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.MouseWheelMsg:
		// On a bindings-only page (context overflowed to its own page), scrolling does nothing.
		if m.paged && m.contextPaged && m.page != 0 {
			return m, nil
		}
		// Logic previously here is now in HandleScrollbarUpdate above.
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMotionMsg:
		// Logic previously here is now in HandleScrollbarUpdate above.
		return m, nil

	case tea.MouseClickMsg:
		// Logic handled in m.Scroll.Update at top of function.
		return m, nil

	case tea.MouseReleaseMsg:
		// Logic handled in m.Scroll.Update at top of function.
		return m, nil

	case LayerHitMsg:
		// Non-scrollbar click: cycle pages when paged, otherwise close.
		// Only handle this if the click was actually on the help dialog background.
		if msg.ID != "help_dialog" {
			return m, nil
		}
		if m.paged {
			n := m.numPages
			if n < 2 {
				n = 2
			}
			m.page = (m.page + 1) % n
			m.contextOffset = 0
			return m, nil
		}
		return m, func() tea.Msg { return CloseDialogMsg{} }

	}
	return m, nil
}
