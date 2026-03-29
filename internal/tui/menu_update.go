package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Any incoming message (keypress, mouse event, window size) potentially changes
	// the state of the menu, so we must invalidate the render cache.
	m.InvalidateCache()

	// If a custom interceptor is defined, give it first right of refusal
	if m.interceptor != nil {
		if cmd, handled := m.interceptor(msg, m); handled {
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case ToggleFocusedMsg:
		// Middle click triggers toggle on the currently focused item
		return m.handleSpace()

	case tea.MouseClickMsg:
		// Raw left click routed by AppModel to start a scrollbar thumb drag.
		if msg.Button == tea.MouseLeft {
			m.sbDragging = true
			m.InvalidateCache()
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.sbDragging {
			if m.scrollbarDragTo(msg.Y) {
				m.InvalidateCache()
			}
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.sbDragging {
			m.sbDragging = false
			m.InvalidateCache()
		}
		return m, nil

	case LayerHitMsg:
		// Scrollbar region clicks
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if msg.Button != HoverButton {
				m.scrollLineUp()
				m.InvalidateCache()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if msg.Button != HoverButton {
				m.scrollLineDown()
				m.InvalidateCache()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if msg.Button != HoverButton {
				m.scrollPageUp()
				m.InvalidateCache()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if msg.Button != HoverButton {
				m.scrollPageDown()
				m.InvalidateCache()
			}
			return m, nil
		}

		// Handle specific item clicks
		id := msg.ID
		suffix := ""
		if strings.HasSuffix(id, "-add") {
			id = strings.TrimSuffix(id, "-add")
			suffix = "add"
		} else if strings.HasSuffix(id, "-enable") {
			id = strings.TrimSuffix(id, "-enable")
			suffix = "enable"
		} else if strings.HasSuffix(id, "-expand") {
			id = strings.TrimSuffix(id, "-expand")
			suffix = "expand"
		}

		if idx, ok := ParseMenuItemIndex(id, m.id); ok {
			// Move selection and column focus for any click
			m.list.Select(idx)
			m.cursor = idx
			menuSelectedIndices[m.id] = idx
			m.focusedItem = FocusList

			// Handle column focus based on click region
			if suffix == "add" {
				m.activeColumn = ColAdd
			} else if suffix == "enable" {
				m.activeColumn = ColEnable
			}

			// Right click on a menu item triggers its context menu
			if msg.Button == tea.MouseRight {
				return m, m.ShowContextMenu(idx, msg.X, msg.Y)
			}

			if idx >= 0 && idx < len(m.items) {
				item := m.items[idx]
				if suffix == "expand" && item.IsGroupHeader {
					// Expand/Collapse only
					return m, item.Action
				}

				// For checkboxes, radio buttons, or selectable items, we trigger a toggle.
				// We MUST check the interceptor first to ensure custom screen logic (like AppSelect) is honored.
				if item.IsCheckbox || item.IsRadioButton || item.Selectable {
					if m.interceptor != nil {
						// We pass a ToggleFocusedMsg to the interceptor to represent a programmatic/mouse-driven toggle
						if cmd, handled := m.interceptor(ToggleFocusedMsg{}, m); handled {
							return m, cmd
						}
					}
					return m.handleSpace()
				}
			}
			return m.handleEnter()
		}

		// Handle clicks/hovers on the menu's list background
		if msg.ID == m.id {
			m.focusedItem = FocusList
			return m, nil
		}

		// Handle button clicks (matches both direct and prefixed IDs e.g. "menuID.btn-back")
		buttonID := msg.ID
		if strings.Contains(buttonID, ".") {
			parts := strings.Split(buttonID, ".")
			if parts[0] == m.id {
				buttonID = parts[1]
			} else {
				// Click was for another menu's button
				return m, nil
			}
		}

		switch buttonID {
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
			if msg.Button == tea.MouseLeft {
				m.focusedItem = FocusSelectBtn
				return m.handleEnter()
			}
		case "btn-back":
			if msg.Button == tea.MouseLeft && m.backAction != nil {
				m.focusedItem = FocusBackBtn
				return m.handleEnter()
			}
		case "btn-exit":
			if msg.Button == tea.MouseLeft && m.showExit {
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
			// Normalize wheelID (handles "menuID.list_panel")
			if strings.Contains(wheelID, ".") {
				parts := strings.Split(wheelID, ".")
				if parts[0] == m.id {
					wheelID = parts[1]
				} else {
					// Wheel was over another menu
					return m, nil
				}
			}

			// IDListPanel: scroll the list regardless of button focus state.
			if wheelID == IDListPanel || wheelID == m.id {
				m.focusedItem = FocusList // Reclaim focus for the list so space/middle-click activates list items
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


		// Tab / ShiftTab: switch between screen-level elements
		// (e.g., menu dialog ↔ header version widget in the future)
		// A whole dialog/window is one screen element; buttons/list within it are not.
		// Does nothing until multi-element screens are implemented.
		case key.Matches(keyMsg, Keys.Tab), key.Matches(keyMsg, Keys.ShiftTab):
			return m, nil

		// Up / Down: navigate the list (independent of button focus)
		case key.Matches(keyMsg, Keys.Up):
			m.scrollLineUp()
			return m, nil

		case key.Matches(keyMsg, Keys.Down):
			m.scrollLineDown()
			return m, nil

		case key.Matches(keyMsg, Keys.PageUp):
			m.scrollPageUp()
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageUp):
			m.scrollHalfPageUp()
			return m, nil

		case key.Matches(keyMsg, Keys.PageDown):
			m.scrollPageDown()
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageDown):
			m.scrollHalfPageDown()
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
			if m.focusedItem == FocusList && (m.groupedMode || m.checkboxMode) {
				m.activeColumn = ColEnable
				return m, nil
			}
			m.focusedItem = m.nextButtonFocus()
			m.updateDelegate()
			return m, nil

		// Left: move to prev button (wraps within button row)
		case key.Matches(keyMsg, Keys.Left):
			if m.focusedItem == FocusList && (m.groupedMode || m.checkboxMode) {
				m.activeColumn = ColAdd
				return m, nil
			}
			m.focusedItem = m.prevButtonFocus()
			return m, nil

		// Ctrl+Right / Alt+Right: column navigation
		case key.Matches(keyMsg, Keys.EnvNextTab):
			m.activeColumn = ColEnable
			m.focusedItem = FocusList
			m.updateDelegate()
			return m, nil

		// Ctrl+Left / Alt+Left: column navigation
		case key.Matches(keyMsg, Keys.EnvPrevTab):
			m.activeColumn = ColAdd
			m.focusedItem = FocusList
			m.updateDelegate()
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
			if keyMsg.Text != "" && len(keyMsg.Text) == 1 {
				keyChar := strings.ToLower(keyMsg.Text)

				// 1. Check Menu Items first (priority)
				// Cyclical search: start from the item after the current selection
				items := m.items
				if len(items) > 0 {
					startIdx := (m.list.Index() + 1) % len(items)
					for i := 0; i < len(items); i++ {
						idx := (startIdx + i) % len(items)
						item := items[idx]
						if item.IsSeparator {
							continue
						}
						// Strip semantic tags and brackets to find the raw first letter
						displayTag := GetPlainText(item.Tag)
						tag := strings.TrimLeft(displayTag, " [({")
						if len(tag) > 0 {
							firstChar := strings.ToLower(string([]rune(tag)[0]))
							if firstChar == keyChar {
								m.list.Select(idx)
								m.cursor = idx
								menuSelectedIndices[m.id] = idx
								m.focusedItem = FocusList
								m.updateDelegate()
								// NAVIGATION ONLY: Move cursor, do not execute Action.
								return m, nil
							}
						}
					}
				}

				// 2. Check Buttons (if no item matched)
				buttons := m.getButtonSpecs()
				if idx, found := CheckButtonHotkeys(keyMsg, buttons); found {
					focusMap := []FocusItem{FocusSelectBtn}
					if m.backAction != nil {
						focusMap = append(focusMap, FocusBackBtn)
					}
					if m.showExit {
						focusMap = append(focusMap, FocusExitBtn)
					}

					if idx < len(focusMap) {
						m.focusedItem = focusMap[idx]
						m.updateDelegate()
					}
					// NAVIGATION ONLY: Move focus to button, do not execute Action automatically
					// for single-character letter keys.
					return m, nil
				}
			}
		}
	}

	return m, nil
}

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
		if (item.IsCheckbox || item.IsRadioButton) && item.Selectable {
			idx := m.list.Index()
			if m.groupedMode && m.activeColumn == ColEnable {
				item.Enabled = !item.Enabled
				if item.Enabled {
					item.Checked = true // Auto-add if user enables
					item.ShowEnabledGutter = true
				}
			} else {
				if item.IsRadioButton {
					item.Checked = true
				} else {
					item.Checked = !item.Checked
				}
				item.Selected = item.Checked
				if item.Checked {
					item.Enabled = true
					item.ShowEnabledGutter = true
				} else {
					item.Enabled = false
					item.ShowEnabledGutter = false
				}
			}
			// Update the item in our internal list too so state persists
			if idx >= 0 && idx < len(m.items) {
				m.items[idx] = item
				// Update list.Model internal items to reflect changes immediately
				m.list.SetItem(idx, item)
			}
			m.renderVersion++
			m.InvalidateCache()

			if item.SpaceAction != nil {
				return m, item.SpaceAction
			}
			return m, nil
		}
	}

	// Space acts on the current list item
	selectedItem = m.list.SelectedItem()
	if item, ok := selectedItem.(MenuItem); ok {
		if item.SpaceAction != nil {
			return m, item.SpaceAction
		}
		// Navigation items: Space falls through to Enter (executes the focused button action)
		// But for group headers, only fall through if we aren't specifically toggling a column.
		if item.Action != nil && !item.IsCheckbox && !item.IsRadioButton {
			if item.IsGroupHeader && (m.activeColumn == ColAdd || m.activeColumn == ColEnable) {
				return m, nil
			}
			return m.handleEnter()
		}
	}

	if m.spaceAction != nil {
		return m, m.spaceAction
	}
	return m, nil
}

// SetSize updates the menu dimensions and resizes the list
func (m *MenuModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.InvalidateCache()

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

	// Sections-based layout: delegate to the specialized calculator.
	if len(m.contentSections) > 0 {
		m.calculateSectionLayout()
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
		// Full dialog: outer border + inner list border + padding (2 sides)
		// Padding per side = 1 (fixed margin in ViewString)
		padding := 2
		maxListWidth = m.width - (layout.DialogBorder + layout.BorderWidth() + padding)
	}

	// Minimum width to avoid squished buttons
	const minWidth = 34
	if maxListWidth < minWidth {
		maxListWidth = minWidth
	}

	// Always reserve one column for the scrollbar gutter (space when off, track/thumb when on).
	// Maximized fills the maximum width; non-maximized is clamped between min and max.
	if m.maximized {
		listWidth = maxListWidth
	} else {
		if listWidth < minWidth {
			listWidth = minWidth
		}
		if listWidth > maxListWidth {
			listWidth = maxListWidth
		}
	}
	listWidth -= ScrollbarGutterWidth
	if listWidth < 1 {
		listWidth = 1
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
	// Button height is 3 with borders, or 1 if space is too tight for them.
	// innerBoxWidth mirrors the width passed to renderSimpleButtons in ViewString.
	innerBoxWidth := listWidth + GetLayout().BorderWidth()
	buttonHeight := ButtonRowHeight(innerBoxWidth, 0, m.getButtonSpecs()...)
	shadowHeight := 0
	hasShadow := currentConfig.UI.Shadow
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 4. Vertical Budgeting Logic
	var listHeight, overhead int
	var maxListHeight int

	// Measure title if in subMenuMode
	titleHeight := 0
	if m.subMenuMode && m.title != "" {
		titleHeight = 1
	}

	// Determine vertical spacing for buttons (only if defined)
	buttonBudget := 0
	if m.showButtons {
		buttonBudget = buttonHeight
	}

	if m.subMenuMode {
		// Sub-menu overhead: title + subtitle + own borders (2) + buttons
		overhead = titleHeight + subtitleHeight + layout.BorderHeight() + buttonBudget
		maxListHeight = m.height - overhead
	} else {
		// Full dialog overhead: borders, subtitle, buttons, shadow.
		// DialogContentHeight uses DialogButtonHeight (3) as the constant button budget;
		// if the width-based check dropped borders (buttonHeight = 1), add back the 2 freed lines.
		maxListHeight = layout.DialogContentHeight(m.height, subtitleHeight, m.showButtons, hasShadow)
		if m.showButtons && buttonHeight != DialogButtonHeight {
			maxListHeight += DialogButtonHeight - buttonHeight
		}
		// Account for inner border around the list (Top + Bottom = 2 lines)
		maxListHeight -= layout.BorderHeight()
		overhead = m.height - maxListHeight
	}

	// Height-based border fallback: drop bordered buttons when 2 or fewer list
	// rows remain — the 2 freed rows are more useful as list space, and this
	// threshold also prevents bordered buttons from showing just before clipping.
	if m.showButtons && buttonHeight == DialogButtonHeight && maxListHeight <= 2 {
		freed := DialogButtonHeight - 1 // reclaim 2 lines
		buttonHeight = 1
		buttonBudget = 1
		maxListHeight += freed
		overhead -= freed
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
		HeaderHeight:   overhead - layout.BorderHeight(), // Store the reserved overhead height
		ViewportHeight: listHeight,
		ButtonHeight:   buttonHeight,
		ShadowHeight:   shadowHeight,
		Overhead:       overhead,
	}

	m.list.SetSize(listWidth, listHeight)
}

// ---------------------------------------------------------------------------
// Scroll helpers — used by both key handlers and scrollbar hit handlers
// ---------------------------------------------------------------------------

func (m *MenuModel) scrollLineUp() {
	m.list.CursorUp()
	for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
		m.list.CursorUp()
		if m.list.Index() == 0 && m.items[0].IsSeparator {
			break
		}
	}
	m.cursor = m.list.Index()
	menuSelectedIndices[m.id] = m.cursor
}

func (m *MenuModel) scrollLineDown() {
	m.list.CursorDown()
	for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
		m.list.CursorDown()
		if m.list.Index() == len(m.items)-1 && m.items[len(m.items)-1].IsSeparator {
			break
		}
	}
	m.cursor = m.list.Index()
	menuSelectedIndices[m.id] = m.cursor
}

func (m *MenuModel) scrollPageUp() {
	// Keep cursor at the same visual row: advance one page back, same row within page.
	perPage := m.list.Paginator.PerPage
	if perPage < 1 {
		perPage = m.list.Height()
		if perPage < 1 {
			perPage = 5
		}
	}
	currentPage := m.list.Paginator.Page
	currentRow := m.list.Index() - currentPage*perPage
	if currentRow < 0 {
		currentRow = 0
	}
	newIndex := (currentPage-1)*perPage + currentRow
	if newIndex < 0 {
		newIndex = 0
	}
	m.list.Select(newIndex)
	for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
		m.list.CursorDown()
	}
	m.cursor = m.list.Index()
	menuSelectedIndices[m.id] = m.cursor
}

func (m *MenuModel) scrollHalfPageUp() {
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
}

func (m *MenuModel) scrollPageDown() {
	// Keep cursor at the same visual row: advance one page forward, same row within page.
	perPage := m.list.Paginator.PerPage
	if perPage < 1 {
		perPage = m.list.Height()
		if perPage < 1 {
			perPage = 5
		}
	}
	currentPage := m.list.Paginator.Page
	currentRow := m.list.Index() - currentPage*perPage
	if currentRow < 0 {
		currentRow = 0
	}
	newIndex := (currentPage+1)*perPage + currentRow
	if newIndex >= len(m.items) {
		newIndex = len(m.items) - 1
	}
	m.list.Select(newIndex)
	for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
		m.list.CursorUp()
	}
	m.cursor = m.list.Index()
	menuSelectedIndices[m.id] = m.cursor
}

func (m *MenuModel) scrollHalfPageDown() {
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
}

// scrollbarDragTo updates the scroll position based on an absolute mouse Y coordinate
// during a scrollbar thumb drag. It maps the track-relative position to an item offset.
// Returns true if the scroll position changed (cache should be invalidated).
func (m *MenuModel) scrollbarDragTo(mouseY int) bool {
	trackH := m.sbInfo.Height - 2 // subtract top and bottom arrows
	if trackH < 1 {
		return false
	}

	// mouseY relative to the start of the track (row 1, just after the up arrow)
	trackRelY := mouseY - (m.sbAbsTopY + 1)
	if trackRelY < 0 {
		trackRelY = 0
	}
	if trackRelY > trackH {
		trackRelY = trackH
	}

	total := len(m.items)
	visible := m.layout.ViewportHeight

	if m.variableHeight {
		// For variable-height items, use line-based total/offset for accurate thumb positioning.
		// Also move the cursor to approximately the dragged line so renderVariableHeightList
		// does not snap viewStartY back to the old cursor position.
		lineTotal := m.lastScrollTotal
		maxOff := lineTotal - visible
		if maxOff <= 0 {
			return false
		}
		newOff := trackRelY * maxOff / trackH
		if newOff < 0 {
			newOff = 0
		}
		if newOff > maxOff {
			newOff = maxOff
		}
		if newOff == m.viewStartY {
			return false
		}
		m.viewStartY = newOff
		// Move cursor proportionally so the render auto-scroll aligns with newOff.
		if total > 0 && lineTotal > 0 {
			approxIdx := newOff * total / lineTotal
			if approxIdx >= total {
				approxIdx = total - 1
			}
			m.list.Select(approxIdx)
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
		}
		return true
	}

	maxOff := total - visible
	if maxOff <= 0 {
		return false
	}
	newOff := trackRelY * maxOff / trackH
	if newOff < 0 {
		newOff = 0
	}
	if newOff > maxOff {
		newOff = maxOff
	}
	if newOff == m.list.Index() {
		return false
	}
	m.list.Select(newOff)
	m.cursor = m.list.Index()
	menuSelectedIndices[m.id] = m.cursor
	return true
}

// calculateSectionLayout distributes available height among content sections.
// Fixed sections (flowMode) get their intrinsic height; the remaining height goes
// to expandable sections.  Called by calculateLayout when contentSections is set.
func (m *MenuModel) calculateSectionLayout() {
	layout := GetLayout()
	contentWidth := m.width - layout.BorderWidth()
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Button height — start with the width-based decision.
	buttonHeight := DialogButtonHeight
	buttonBudget := 0
	if m.showButtons {
		buttonHeight = ButtonRowHeight(contentWidth, 0, m.getButtonSpecs()...)
		buttonBudget = buttonHeight
	}

	// Available height inside the outer border.
	innerHeight := m.height - layout.BorderHeight()

	// Pass 1: measure fixed sections (flow mode = intrinsic height).
	sectionHeights := make([]int, len(m.contentSections))
	fixedTotal := 0
	expandableCount := 0
	for i, sec := range m.contentSections {
		if sec.flowMode {
			flowH := sec.GetFlowHeight(contentWidth)
			sectionH := flowH + layout.BorderHeight()
			sectionHeights[i] = sectionH
			fixedTotal += sectionH
		} else {
			expandableCount++
		}
	}

	// Remaining height for expandable sections.
	const minExpandable = 4
	remaining := innerHeight - fixedTotal - buttonBudget

	// Height-based button border fallback: drop to flat only when expandable
	// sections would have no room at all.
	if m.showButtons && buttonHeight == DialogButtonHeight && remaining < minExpandable {
		buttonHeight = 1
		buttonBudget = 1
		remaining = innerHeight - fixedTotal - buttonBudget
	}
	if remaining < minExpandable {
		remaining = minExpandable
	}

	expandableH := remaining
	if expandableCount > 1 {
		expandableH = remaining / expandableCount
	}

	// Pass 2: size each section.
	for i, sec := range m.contentSections {
		h := sectionHeights[i]
		if h == 0 {
			h = expandableH
		}
		sec.SetSize(contentWidth, h)
	}

	shadowHeight := 0
	if currentConfig.UI.Shadow {
		shadowHeight = DialogShadowHeight
	}

	m.layout = DialogLayout{
		Width:        m.width,
		Height:       m.height,
		ButtonHeight: buttonHeight,
		ShadowHeight: shadowHeight,
	}
}

// SetFlowMode toggles horizontal flow layout
func (m *MenuModel) SetFlowMode(flow bool) {
	m.flowMode = flow
}

// SetHeaderVisibility toggles background/title for sub-menus
func (m *MenuModel) SetHeaderVisibility(visible bool) {
	m.list.SetShowTitle(visible)
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
