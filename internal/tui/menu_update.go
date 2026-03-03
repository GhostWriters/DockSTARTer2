package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ToggleFocusedMsg:
		// Middle click triggers toggle on the currently focused item
		return m.handleSpace()

	case LayerHitMsg:
		// Handle specific item clicks
		if strings.HasPrefix(msg.ID, "item-"+m.id+"-") {
			indexStr := strings.TrimPrefix(msg.ID, "item-"+m.id+"-")
			if idx, err := strconv.Atoi(indexStr); err == nil {
				m.list.Select(idx)
				m.cursor = idx
				menuSelectedIndices[m.id] = idx
				m.focusedItem = FocusList

				// Middle click is handled by AppModel (global Space mapping)
				// We just handle the selection here.
				if msg.Button == tea.MouseMiddle {
					return m, nil
				}

				// For checkboxes/radio buttons, clicking toggles (Space action)
				// For regular items, clicking executes (Enter action)
				if idx >= 0 && idx < len(m.items) {
					item := m.items[idx]
					if item.IsCheckbox || item.IsRadioButton || item.Selectable {
						return m.handleSpace()
					}
				}
				return m.handleEnter()
			}
		}

		// Handle clicks on the menu itself (not a specific item/button)
		if msg.ID == m.id {
			return m, nil
		}

		// Handle button clicks
		switch msg.ID {
		case IDListPanel:
			// Hover moved back over the list — restore list focus so the wheel scrolls items.
			m.focusedItem = FocusList
			return m, nil
		case IDButtonPanel:
			// Hover landed on the button row background — focus the row without executing.
			// Keep whatever button is already highlighted; default to Select when coming from list.
			if m.focusedItem == FocusList {
				m.focusedItem = FocusSelectBtn
			}
			return m, nil
		case "btn-select":
			m.focusedItem = FocusSelectBtn
			return m.handleEnter()
		case "btn-back":
			if m.backAction != nil {
				m.focusedItem = FocusBackBtn
				return m.handleEnter()
			}
		case "btn-exit":
			if m.showExit {
				m.focusedItem = FocusExitBtn
				return m.handleEnter()
			}
		}

		return m, nil

	case LayerWheelMsg, tea.MouseWheelMsg:
		// Handle mouse wheel scrolling (raw or semantic)
		var wheelBtn tea.MouseButton
		var wheelID string
		if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
			wheelBtn = mwMsg.Button
		} else if lwMsg, ok := msg.(LayerWheelMsg); ok {
			wheelBtn = lwMsg.Button
			wheelID = lwMsg.ID
		}

		if wheelBtn != 0 {
			// IDListPanel: scroll the list regardless of button focus state.
			// Mirrors keyboard up/down — button highlight is preserved independently.
			if wheelID == IDListPanel {
				if wheelBtn == tea.MouseWheelUp {
					m.list.CursorUp()
					for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
						m.list.CursorUp()
					}
				} else if wheelBtn == tea.MouseWheelDown {
					m.list.CursorDown()
					for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
						m.list.CursorDown()
					}
				}
				m.cursor = m.list.Index()
				menuSelectedIndices[m.id] = m.cursor
				return m, nil
			}

			// When a button is focused (hover+scroll over button row), shift focus
			// left/right using the clamping helpers — no wrap at either end.
			// subMenuMode menus never render buttons, so always fall through to list scroll.
			if !m.subMenuMode && (m.focusedItem == FocusSelectBtn || m.focusedItem == FocusBackBtn || m.focusedItem == FocusExitBtn) {
				if wheelBtn == tea.MouseWheelUp {
					m.focusedItem = m.prevButtonFocus()
				} else if wheelBtn == tea.MouseWheelDown {
					m.focusedItem = m.nextButtonFocus()
				}
				return m, nil
			}

			if wheelBtn == tea.MouseWheelUp {
				m.list.CursorUp()
				// Skip separators automatically
				for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
					m.list.CursorUp()
				}
			} else if wheelBtn == tea.MouseWheelDown {
				m.list.CursorDown()
				// Skip separators automatically
				for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
					m.list.CursorDown()
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil
		}

	case tea.KeyPressMsg:
		keyMsg := msg
		switch {
		case key.Matches(keyMsg, Keys.Help):
			return m, func() tea.Msg { return ShowDialogMsg{Dialog: NewHelpDialogModel()} }

		// Tab / ShiftTab: switch between screen-level elements
		// (e.g., menu dialog ↔ header version widget in the future)
		// A whole dialog/window is one screen element; buttons/list within it are not.
		// Does nothing until multi-element screens are implemented.
		case key.Matches(keyMsg, Keys.Tab), key.Matches(keyMsg, Keys.ShiftTab):
			return m, nil

		// Up / Down: navigate the list (independent of button focus)
		case key.Matches(keyMsg, Keys.Up):
			m.list.CursorUp()
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
				m.list.CursorUp()
				// Safety: if we hit top and it's a separator (unlikely with header), stop or wrap?
				// Simple safety: if index is 0 and it's a separator, try going down instead?
				// For now, assume top item isn't a separator or just let bubbles handle bounds.
				if m.list.Index() == 0 && m.items[0].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.Down):
			m.list.CursorDown()
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
				m.list.CursorDown()
				if m.list.Index() == len(m.items)-1 && m.items[len(m.items)-1].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.PageUp):
			pageHeight := m.list.Height()
			if pageHeight < 1 {
				pageHeight = 5 // Fallback
			}
			newIndex := m.list.Index() - pageHeight
			if newIndex < 0 {
				newIndex = 0
			}
			m.list.Select(newIndex)
			// Skip separators automatically (moving down to find first selectable)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageUp):
			pageHeight := m.list.Height() / 2
			if pageHeight < 1 {
				pageHeight = 1
			}
			newIndex := m.list.Index() - pageHeight
			if newIndex < 0 {
				newIndex = 0
			}
			m.list.Select(newIndex)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.PageDown):
			pageHeight := m.list.Height()
			if pageHeight < 1 {
				pageHeight = 5 // Fallback
			}
			newIndex := m.list.Index() + pageHeight
			if newIndex >= len(m.items) {
				newIndex = len(m.items) - 1
			}
			m.list.Select(newIndex)
			// Skip separators automatically (moving up to find first selectable)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageDown):
			pageHeight := m.list.Height() / 2
			if pageHeight < 1 {
				pageHeight = 1
			}
			newIndex := m.list.Index() + pageHeight
			if newIndex >= len(m.items) {
				newIndex = len(m.items) - 1
			}
			m.list.Select(newIndex)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.Home):
			m.list.Select(0)
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.End):
			m.list.Select(len(m.items) - 1)
			// Skip separators automatically (moving up)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		// Right: move to next button (wraps within button row)
		case key.Matches(keyMsg, Keys.Right):
			m.focusedItem = m.nextButtonFocus()
			return m, nil

		// Left: move to prev button (wraps within button row)
		case key.Matches(keyMsg, Keys.Left):
			m.focusedItem = m.prevButtonFocus()
			return m, nil

		// Enter: select/confirm current focused element
		case key.Matches(keyMsg, Keys.Enter):
			return m.handleEnter()

		// Space: select/toggle current focused element
		case key.Matches(keyMsg, Keys.Space):
			return m.handleSpace()

		// Esc: back if available, else exit
		case key.Matches(keyMsg, Keys.Esc):
			if m.backAction != nil {
				return m, m.backAction
			}
			return m, ConfirmExitAction()

		// Dynamic Hotkeys
		default:
			// In v2, KeyPressMsg has Text field directly
			if keyMsg.Text != "" {
				keyRune := strings.ToLower(keyMsg.Text)

				// 1. Check Menu Items first (priority)
				for i, item := range m.items {
					// Handle tagged tags like [F] properly
					tag := strings.Trim(item.Tag, "[]")
					if len(tag) > 0 {
						firstChar := strings.ToLower(string(tag[0]))
						if firstChar == keyRune {
							m.list.Select(i)
							m.cursor = i
							menuSelectedIndices[m.id] = i
							m.focusedItem = FocusList
							return m.handleEnter()
						}
					}
				}

				// 2. Check Buttons (if no item matched)
				// Determine available buttons using shared helper
				buttons := m.getButtonSpecs()

				if idx, found := CheckButtonHotkeys(keyMsg, buttons); found {
					// Map index back to FocusItem
					// Use zone IDs or index to map, assuming standard order
					// But since we use dynamic buttons, we should map based on the button's intended action
					// Or just map index if we know the order: Select, [Back], [Exit]
					// Helper: map button text/zone to FocusItem?
					// Simpler: iterate known types and check if active in specs?
					// Because getButtonSpecs builds in order: Select, Back, Exit.

					// Re-derive focus based on index loop
					focusMap := []FocusItem{FocusSelectBtn}
					if m.backAction != nil {
						focusMap = append(focusMap, FocusBackBtn)
					}
					if m.showExit {
						focusMap = append(focusMap, FocusExitBtn)
					}

					if idx < len(focusMap) {
						m.focusedItem = focusMap[idx]
					}
					return m.handleEnter()
				}
			}
		}
	}

	return m, nil
}
func (m *MenuModel) View() tea.View { return tea.View{Content: m.ViewString()} }

// nextButtonFocus, prevButtonFocus → menu_buttons.go
// ViewString, Layers, renderBorderWithTitle, viewSubMenu → menu_render.go
// getButtonSpecs, renderSimpleButtons, renderButtons, renderButtonBox → menu_buttons.go
// GetHitRegions → menu_hit_regions.go
// renderFlow, GetFlowHeight → menu_render_flow.go
// renderVariableHeightList → menu_render_list.go

func (m *MenuModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		// 1. Try list item action first (for navigation menus)
		// This is the primary function for navigation menus, and also applies
		// if "Select" is used as a proxy for Enter on the list.
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.Action != nil {
				// Update cursor for persistence
				m.cursor = m.list.Index()
				menuSelectedIndices[m.id] = m.cursor
				return m, item.Action
			}

			// In checkbox mode, Enter on a list item toggles its state.
			// Enter on the "Select" (Done) button should NOT toggle; it should fall through to enterAction.
			if m.checkboxMode && m.focusedItem == FocusList {
				m.ToggleSelectedItem()
				return m, nil
			}
		}

		// 2. Fall back to model-level enter action (for "Done" buttons on selection screens)
		if m.enterAction != nil {
			return m, m.enterAction
		}

	case FocusBackBtn:
		if m.backAction != nil {
			return m, m.backAction
		}
	case FocusExitBtn:
		return m, ConfirmExitAction()
	}

	return m, nil
}
func (m *MenuModel) handleSpace() (tea.Model, tea.Cmd) {
	if m.checkboxMode {
		m.ToggleSelectedItem()
		return m, nil
	}

	// Always prioritize checkbox toggle if item is one
	selectedItem := m.list.SelectedItem()
	if item, ok := selectedItem.(MenuItem); ok {
		if item.IsCheckbox && item.Selectable {
			item.Checked = !item.Checked
			item.Selected = item.Checked
			// Update the item in our internal list too so state persists
			idx := m.list.Index()
			if idx >= 0 && idx < len(m.items) {
				m.items[idx].Checked = item.Checked
				m.items[idx].Selected = item.Selected
				// Update list.Model internal items to reflect changes immediately
				m.list.SetItem(idx, item)
			}
			m.lastView = "" // Invalidate cache

			if item.SpaceAction != nil {
				return m, item.SpaceAction
			}
			return m, nil
		}
	}

	if m.focusedItem == FocusList {
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.SpaceAction != nil {
				return m, item.SpaceAction
			}
			// Items with only Action (e.g. dropdown selectors) have no SpaceAction.
			// Fall through to Enter so middle-clicking over the panel activates them.
			if item.Action != nil {
				return m.handleEnter()
			}
		}
	}

	if m.spaceAction != nil {
		return m, m.spaceAction
	}
	// Fallback: activate whichever button is currently focused
	if m.focusedItem == FocusSelectBtn || m.focusedItem == FocusBackBtn || m.focusedItem == FocusExitBtn {
		return m.handleEnter()
	}
	return m, nil
}

// SetSize updates the menu dimensions and resizes the list
func (m *MenuModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// If in flow mode, calculate height based on content
	if m.flowMode {
		flowLines := m.GetFlowHeight(width)
		// +2 for top/bottom borders
		m.layout.ViewportHeight = flowLines
		m.layout.Height = flowLines + 2
	} else {
		m.calculateLayout()
	}
}

// Width returns the menu's width
func (m *MenuModel) Width() int {
	return m.width
}

// Height returns the menu's height
func (m *MenuModel) Height() int {
	return m.height
}

func (m *MenuModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// 1. Calculate list width first — subtitle height measurement depends on it.
	layout := GetLayout()
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(m.items)
	// Width = tag + spacing(2) + desc + margins(2) + buffer(4)
	listWidth := maxTagLen + 2 + maxDescLen + 2 + 4

	// Constrain width to fit within terminal dialog area using Layout helpers
	var maxListWidth int
	if m.subMenuMode {
		// Submenu: just has its own border, content fills the rest
		maxListWidth, _ = layout.InnerContentSize(m.width, m.height)
	} else {
		// Full dialog: outer border + inner list border + padding
		// Total overhead = outer border (2) + inner border (2) + padding (2) = 6
		maxListWidth = m.width - 6
	}
	if maxListWidth < 34 {
		maxListWidth = 34
	}

	// If maximized, fill the space. Otherwise, ensure minimum for buttons.
	if m.maximized {
		listWidth = maxListWidth
	} else {
		if listWidth < 34 {
			listWidth = 34
		}
		if listWidth > maxListWidth {
			listWidth = maxListWidth
		}
	}

	// 2. Subtitle Height — measured at the actual render width (listWidth + 4) so
	// word-wrap lines match what ViewString() produces. Using lipgloss.Height(m.subtitle)
	// only counts explicit '\n' and underestimates at narrow terminal widths.
	subtitleHeight := 0
	if m.subtitle != "" {
		styles := GetStyles()
		outerContentWidth := listWidth + 4
		subtitleStyle := styles.Dialog.Width(outerContentWidth).Padding(0, 1)
		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		subtitleHeight = lipgloss.Height(subtitleStyle.Render(subStr))
	}

	// 3. Button and Shadow Heights
	buttonHeight := DialogButtonHeight
	shadowHeight := 0
	hasShadow := currentConfig.UI.Shadow
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 4. Vertical Budgeting Logic
	var listHeight, overhead int
	var maxListHeight int
	if m.subMenuMode {
		// Sub-menu overhead is just the subtitle and its own borders (2)
		overhead = subtitleHeight + layout.BorderHeight()
		maxListHeight = m.height - overhead
	} else {
		// Full dialog overhead: borders, subtitle, buttons, shadow
		// Vertical budgeting uses DialogContentHeight which handles gaps
		maxListHeight = layout.DialogContentHeight(m.height, subtitleHeight, true, hasShadow)
		// Account for inner border around the list (Top + Bottom = 2 lines)
		maxListHeight -= layout.BorderHeight()
		overhead = m.height - maxListHeight
	}

	if maxListHeight < 3 {
		maxListHeight = 3
	}

	// 5. Calculate intrinsic list height based on items
	itemHeight := 1
	spacing := 0
	totalItemHeight := len(m.items) * itemHeight
	if len(m.items) > 1 && spacing > 0 {
		totalItemHeight += (len(m.items) - 1) * spacing
	}

	// Final list height is whichever is smaller: intrinsic or maximum
	listHeight = totalItemHeight
	if m.maximized || listHeight > maxListHeight {
		listHeight = maxListHeight
	}

	m.layout = DialogLayout{
		Width:          m.width,
		Height:         m.height,
		HeaderHeight:   subtitleHeight,
		ViewportHeight: listHeight,
		ButtonHeight:   buttonHeight,
		ShadowHeight:   shadowHeight,
		Overhead:       overhead,
	}

	m.list.SetSize(listWidth, listHeight)
}

// SetFlowMode toggles horizontal flow layout
func (m *MenuModel) SetFlowMode(flow bool) {
	m.flowMode = flow
}

// SetHeaderVisibility toggles background/title for sub-menus
func (m *MenuModel) SetHeaderVisibility(visible bool) {
	m.list.SetShowTitle(visible)
}

// Title returns the menu title
func (m *MenuModel) Title() string {
	return m.title
}

// HelpText returns the current item's help text
func (m *MenuModel) HelpText() string {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].Help
	}
	return ""
}

// Cursor returns the current selection index
func (m *MenuModel) Cursor() int {
	return m.cursor
}
