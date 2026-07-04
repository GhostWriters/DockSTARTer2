package classic

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PromptTextHook displays a blocking text prompt dialog and returns the
// entered value. Panel's sudo-password interception (see panel_update.go)
// needs this real app-integrated prompt (routes through the running
// tea.Program or a standalone dialog); app wiring (internal/tui) overrides
// this at startup. Defaults to an immediate cancel so classic still builds
// standalone.
var PromptTextHook func(title, question string, sensitive bool) (string, error) = func(title, question string, sensitive bool) (string, error) {
	return "", nil
}

// ConfirmExitFallback is the tea.Cmd invoked when Esc/exit-button handling
// falls back to a generic "exit the program" action because the menu has no
// explicit btn-back/btn-cancel/btn-exit. Defaults to a plain tea.Quit; app
// wiring (internal/tui) overrides this at startup to show a real "confirm
// exit?" dialog instead, keeping classic decoupled from app-navigation
// message types.
var ConfirmExitFallback func() tea.Cmd = func() tea.Cmd {
	return tea.Quit
}

// MenuDeferredActionMsg carries an action to execute after one render cycle,
// allowing the spinner to appear before any synchronous work in the action blocks.
type MenuDeferredActionMsg struct {
	id     string
	action tea.Cmd
}

// deferAction returns a cmd that waits a short fixed delay before delivering
// a MenuDeferredActionMsg, giving Bubble Tea time to render the active button
// state before any synchronous work in the action blocks the loop.
func (m *MenuModel) deferAction(action tea.Cmd) tea.Cmd {
	iid := m.instanceID
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return MenuDeferredActionMsg{id: iid, action: action}
	})
}

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Spinner tick: advance frame and schedule next tick while loading.
	if deferred, ok := msg.(MenuDeferredActionMsg); ok && deferred.id == m.instanceID {
		// Keep processingItemIdx set so the spinner keeps ticking while the
		// action runs (actions like NavigateMsg can be slow to build the
		// new screen). ClearProcessingState() is called when the screen is popped
		// back from the stack, or explicitly by the action if it doesn't navigate.
		return m, deferred.action
	}
	// Button deferred-action messages are scoped to btnRow's own instanceID,
	// separate from MenuDeferredActionMsg above (list-item actions).
	if cmd, ok := m.btnRow.Update(msg); ok {
		return m, cmd
	}

	// Block all user input while an action is in flight (spinner visible).
	// Still allow system messages (size, lock state, etc.) to pass through.
	if m.processingItemIdx >= 0 || m.btnRow.IsProcessing() {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, LayerHitMsg, LayerWheelMsg, tea.MouseWheelMsg, ToggleFocusedMsg:
			return m, nil
		}
	}

	// 1. Centralized scrollbar processing (Throttling, Clicks, Dragging)
	// For cursor-driven lists, don't let the scrollbar handle wheel — the cursor
	// code below calls scrollLineUp/Down which also update viewStartY via the list
	// component's own pagination. Column-scroll mode (variableHeight + flow columns)
	// drives viewStartY directly so wheel is allowed there.
	isColumnScroll := m.FlowColumns >= 2 && m.MaxFlowRows > 0
	skipScrollbarWheel := false
	switch msg.(type) {
	case tea.MouseWheelMsg, LayerWheelMsg:
		skipScrollbarWheel = !isColumnScroll
	}
	if !skipScrollbarWheel {
		if newOff, cmd, changed := m.Scroll.Update(msg, m.ViewStartY, m.ScrollTotal(), m.Layout.ViewportHeight); changed {
			m.ViewStartY = newOff
			m.syncSelectionToViewport()
			m.InvalidateCache()
			return m, cmd
		}
	}

	// Any other incoming message (keypress, mouse event, window size) potentially
	// changes the state of the menu, so we must invalidate the render cache.
	m.InvalidateCache()

	// If a custom interceptor is defined, give it first right of refusal
	if m.Interceptor != nil {
		if cmd, handled := m.Interceptor(msg, m); handled {
			return m, cmd
		}
	}

	// For standard lists, ensure viewStartY follows the cursor.
	// Column scroll mode skips this — viewStartY is driven by wheel/explicit nav
	// in the interceptor, not by cursor position (which would pull viewStartY back).
	if !m.variableHeight && (m.FlowColumns < 2 || m.MaxFlowRows == 0) {
		visible := m.Layout.ViewportHeight
		if visible > 0 {
			cursorRow := m.list.Index()
			if cursorRow < m.ViewStartY {
				m.ViewStartY = cursorRow
			} else if cursorRow >= m.ViewStartY+visible {
				m.ViewStartY = cursorRow - visible + 1
			}
		}
	}

	if m.HandleWidgetClearPress(msg) {
		m.InvalidateCache()
		return m, nil
	}

	switch msg := msg.(type) {
	case LockStateChangedMsg:
		m.SetLockedByOthers(msg.LockedByOthers)
		return m, nil

	case tea.KeyPressMsg:
		// Title bar focus: intercept all keys before normal list handling.
		if handled, cmd := m.HandleTitleBarKey(msg, m.menuTitleBarCloseCmd()); handled {
			return m, cmd
		}
		switch {
		case key.Matches(msg, Keys.Enter), key.Matches(msg, Keys.Space):
			// Only block on a locked list item when the list (not a button) has focus.
			// Buttons like Exit and Back must remain responsive even when destructive
			// items are locked by another session.
			if m.focusedItem == FocusList || m.focusedItem == FocusSelectBtn {
				if sel := m.list.SelectedItem(); sel != nil {
					if item, ok := sel.(MenuItem); ok && item.Locked {
						return m, nil // Block interaction for locked items
					}
				}
			}
		}
	}

	// Route messages to content sections when present.
	if len(m.contentSections) > 0 {
		if updated, cmd, handled := m.updateSections(msg); handled {
			if mm, ok := updated.(*MenuModel); ok {
				*m = *mm
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {

	case ToggleFocusedMsg:
		// Middle click triggers toggle on the currently focused item
		return m.handleSpace()

	case tea.MouseClickMsg:

	case tea.MouseReleaseMsg:
		// Released is now handled by m.Scroll.Update above.
		return m, nil

	case LayerHitMsg:

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
			menuSelectedIndices[m.persistKey()] = idx
			m.focusedItem = FocusList

			// Handle column focus based on click region
			switch suffix {
			case "add":
				m.activeColumn = ColAdd
			case "enable":
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
					if m.Interceptor != nil {
						// We pass a ToggleFocusedMsg to the interceptor to represent a programmatic/mouse-driven toggle
						if cmd, handled := m.Interceptor(ToggleFocusedMsg{}, m); handled {
							return m, cmd
						}
					}
					return m.handleSpace()
				}
			}
			m.focusedItem = FocusList
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
		case IDTitleWidgetHelp, IDTitleWidgetClose:
			if handled, cmd := m.HandleTitleBarHit(msg, m.menuTitleBarCloseCmd()); handled {
				return m, cmd
			}
		case IDListPanel:
			// Hover moved back over the list — restore list focus so the wheel scrolls items.
			m.focusedItem = FocusList
			return m, nil
		case IDButtonPanel:
			// Hover landed on the button row background — focus the row without executing.
			// Keep whatever button is already highlighted; default to first button when coming from list.
			if m.focusedItem == FocusList {
				m.focusedItem = FocusBtn
				m.focusedBtnIndex = 0
			}
			return m, nil
		default:
			if msg.Button == tea.MouseLeft {
				for i, btn := range m.buttons {
					if btn.ZoneID == buttonID {
						m.focusedItem = FocusBtn
						m.focusedBtnIndex = i
						return m.handleEnter()
					}
				}
			}
		}

		return m, nil
	case LayerWheelMsg, tea.MouseWheelMsg:
		// Swallow wheel events while a previous scroll is still being processed.
		if m.ScrollPending() {
			return m, nil
		}

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
			// Empty wheelID means a raw MouseWheelMsg with no hit position (e.g. routed
			// from a parent's updateSections) — treat it as a list scroll.
			if wheelID == IDListPanel || wheelID == m.id || wheelID == "" {
				m.focusedItem = FocusList // Reclaim focus for the list so space/middle-click activates list items
				switch wheelBtn {
				case tea.MouseWheelUp:
					m.scrollLineUp()
				case tea.MouseWheelDown:
					m.scrollLineDown()
				}
				return m, nil
			}

			// When a button is focused (hover+scroll over button row), shift focus
			// left/right using the clamping helpers — no wrap at either end.
			// subMenuMode menus never render buttons, so always fall through to list scroll.
			isButtonFocused := m.focusedItem == FocusBtn
			if !m.subMenuMode && isButtonFocused {
				switch wheelBtn {
				case tea.MouseWheelUp:
					m.focusedItem = m.prevButtonFocus()
				case tea.MouseWheelDown:
					m.focusedItem = m.nextButtonFocus()
				}
				return m, nil
			}

			switch wheelBtn {
			case tea.MouseWheelUp:
				m.scrollLineUp()
			case tea.MouseWheelDown:
				m.scrollLineDown()
			}
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

		case key.Matches(keyMsg, Keys.PageDown):
			m.scrollPageDown()
			return m, nil

		case key.Matches(keyMsg, Keys.Home):
			m.list.Select(0)
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.persistKey()] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.End):
			m.list.Select(len(m.items) - 1)
			// Skip separators automatically (moving up)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.persistKey()] = m.cursor
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

		// Esc: back if available, else exit, else fall back to whatever
		// single button this dialog has (e.g. a ProgramBox's "OK" button,
		// which is neither a back/cancel nor an exit action).
		case key.Matches(keyMsg, Keys.Esc):
			for i, btn := range m.buttons {
				if (btn.ZoneID == "btn-back" || btn.ZoneID == "btn-cancel" || btn.ZoneID == IDBackButton) && btn.Action != nil {
					m.focusedItem = FocusBtn
					m.focusedBtnIndex = i
					action := btn.Action
					return m, m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return action() })
				}
			}
			for i, btn := range m.buttons {
				if (btn.ZoneID == "btn-exit" || btn.ZoneID == IDExitButton) && btn.Action != nil {
					m.focusedItem = FocusBtn
					m.focusedBtnIndex = i
					action := btn.Action
					return m, m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return action() })
				}
			}
			if len(m.buttons) == 1 && m.buttons[0].Action != nil {
				btn := m.buttons[0]
				m.focusedItem = FocusBtn
				m.focusedBtnIndex = 0
				return m, m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return btn.Action() })
			}
			return m, m.SetProcessingBtnDeferred(IDExitButton, ConfirmExitFallback())

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
								menuSelectedIndices[m.persistKey()] = idx
								m.focusedItem = FocusList
								m.updateDelegate()
								// NAVIGATION ONLY: Move cursor, do not execute Action.
								return m, nil
							}
						}
					}
				}

				// 2. Check Buttons (if no item matched)
				buttons := m.GetButtonSpecsForState()
				if idx, found := CheckButtonHotkeys(keyMsg, buttons); found {
					if idx < len(m.buttons) {
						m.focusedItem = FocusBtn
						m.focusedBtnIndex = idx
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

// menuTitleBarCloseCmd returns the action to run when the title bar's
// Close widget activates: fire the first back/cancel button if present,
// otherwise the first exit button, with the same spinner treatment as a
// normal button click. Returns nil if no matching button exists (Close
// then just blurs the title bar, same as TitleBarFocus's default nil-closeCmd
// behavior).
//
// This is passed as the closeCmd argument to HandleTitleBarKey/HandleTitleBarHit,
// which evaluate it eagerly as a plain tea.Cmd argument regardless of which key
// was pressed. The button search (and its focus/spinner side effects) must not
// run until the returned tea.Cmd is actually invoked by Bubble Tea, so the body
// is wrapped in a closure rather than executed directly here.
func (m *MenuModel) menuTitleBarCloseCmd() tea.Cmd {
	return func() tea.Msg {
		for i, btn := range m.buttons {
			if (btn.ZoneID == "btn-back" || btn.ZoneID == "btn-cancel" || btn.ZoneID == IDBackButton) && btn.Action != nil {
				m.focusedItem = FocusBtn
				m.focusedBtnIndex = i
				action := btn.Action
				cmd := m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return action() })
				return cmd()
			}
		}
		for i, btn := range m.buttons {
			if (btn.ZoneID == "btn-exit" || btn.ZoneID == IDExitButton) && btn.Action != nil {
				m.focusedItem = FocusBtn
				m.focusedBtnIndex = i
				action := btn.Action
				cmd := m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return action() })
				return cmd()
			}
		}
		// Fall back to whatever single button this dialog has (e.g. a
		// ProgramBox's "OK" button, which is neither a back/cancel nor an
		// exit action) so the title-bar [x] widget always has something to
		// close with.
		if len(m.buttons) == 1 && m.buttons[0].Action != nil {
			btn := m.buttons[0]
			m.focusedItem = FocusBtn
			m.focusedBtnIndex = 0
			cmd := m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return btn.Action() })
			return cmd()
		}
		return nil
	}
}

func (m *MenuModel) handleEnter() (tea.Model, tea.Cmd) {
	// FocusBtn dispatches to the focused button's Action
	if m.focusedItem == FocusBtn {
		if m.focusedBtnIndex >= 0 && m.focusedBtnIndex < len(m.buttons) {
			btn := m.buttons[m.focusedBtnIndex]
			if btn.Action != nil {
				action := btn.Action
				return m, m.SetProcessingBtnDeferred(btn.ZoneID, func() tea.Msg { return action() })
			}
		}
		// Button has no action (inert) — also check if it's the first button (Select-role)
		// and try item action / enterAction as a fallback.
		if m.focusedBtnIndex == 0 {
			selectedItem := m.list.SelectedItem()
			if item, ok := selectedItem.(MenuItem); ok {
				if item.Action != nil && !item.Locked {
					m.cursor = m.list.Index()
					menuSelectedIndices[m.persistKey()] = m.cursor
					m.processingItemIdx = m.cursor
					m.titleSpinner.Start()
					if len(m.buttons) > 0 {
						m.btnRow.MarkProcessing(m.buttons[0].ZoneID)
					}
					m.InvalidateCache()
					return m, m.deferAction(item.Action)
				}
			}
			if m.enterAction != nil {
				if len(m.buttons) > 0 {
					m.btnRow.MarkProcessing(m.buttons[0].ZoneID)
				}
				m.InvalidateCache()
				return m, m.deferAction(m.enterAction)
			}
		}
		return m, nil
	}

	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		// 1. Try list item action first (for navigation menus)
		// This is the primary function for navigation menus, and also applies
		// if "Select" is used as a proxy for Enter on the list.
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.Action != nil {
				if item.Locked {
					return m, nil
				}
				// Update cursor for persistence
				m.cursor = m.list.Index()
				menuSelectedIndices[m.persistKey()] = m.cursor
				m.processingItemIdx = m.cursor
				m.titleSpinner.Start()
				m.btnRow.MarkProcessing("btn-select")
				m.InvalidateCache()
				return m, m.deferAction(item.Action)
			}
		}

		// 2. Fall back to model-level enter action (for "Done" buttons on selection screens)
		if m.enterAction != nil {
			m.btnRow.MarkProcessing("btn-select")
			m.InvalidateCache()
			return m, m.deferAction(m.enterAction)
		}

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
		// Group headers (IsCheckbox: false) have no Checked/Enabled of their own
		// to toggle, but still need to reach the Interceptor below so app_selection's
		// toggleItem can handle Space on the Expand/Name columns for them.
		if (item.IsCheckbox || item.IsRadioButton || (m.groupedMode && item.IsGroupHeader)) && item.Selectable {
			if item.Locked {
				return m, nil
			}
			idx := m.list.Index()
			switch {
			case m.groupedMode && item.IsGroupHeader:
				// Read-only/aggregate state -- the Interceptor owns all Space
				// behavior for a group header row.
			case m.groupedMode && m.activeColumn == ColEnable:
				item.Enabled = !item.Enabled
				if item.Enabled {
					item.Checked = true // Auto-add if user enables
					item.ShowEnabledGutter = true
				}
			case m.groupedMode && (m.activeColumn == ColExpand || m.activeColumn == ColName):
				// Neither the Expand nor Name column has Checked/Enabled state of
				// its own -- leave item untouched and let the Interceptor
				// (app_selection's toggleItem) fully own what Space does here.
			default:
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

			if m.Interceptor != nil {
				if cmd, handled := m.Interceptor(ToggleFocusedMsg{}, m); handled {
					return m, cmd
				}
			}

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
		layout := GetLayout()
		flowMaxW, _ := layout.InnerContentSize(width, height)
		if flowMaxW > 2 {
			flowMaxW -= 2
		}
		flowLines := m.GetFlowHeight(flowMaxW)
		// +2 for top/bottom borders
		m.Layout.ViewportHeight = flowLines
		m.Layout.Height = flowLines + 2
	} else {
		m.calculateLayout()
	}

	// After layout recalculation, clamp viewStartY so the scrollbar thumb
	// renders at the correct position immediately on resize (before the next
	// Update call would otherwise correct it). Variable-height lists clamp
	// inside renderVariableHeightList, so only standard lists need this here.
	if !m.variableHeight && m.Layout.ViewportHeight > 0 {
		maxOff := m.ScrollTotal() - m.Layout.ViewportHeight
		if maxOff < 0 {
			maxOff = 0
		}
		if m.ViewStartY > maxOff {
			m.ViewStartY = maxOff
		}
		if m.ViewStartY < 0 {
			m.ViewStartY = 0
		}
	}

	if m.OnResize != nil {
		m.OnResize(width, height)
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
		// Submenu: scrollbar sits flush against the right border (no right margin),
		// so only subtract the left margin, not both -- unless this section
		// has no left margin of its own (noLeftMargin), e.g. a section nested
		// inside an outer sectioned dialog's viewWithSections, which already
		// applies its own margin around every section -- subtracting a
		// second column here would leave the list (and its trailing
		// scrollbar) one column narrower than the space it's actually
		// rendered into, shifting the scrollbar left of the border.
		maxListWidth, _ = layout.InnerContentSize(m.width, m.height)
		if !m.noLeftMargin {
			maxListWidth -= layout.ContentSideMargin
		}
	} else {
		// Full dialog: Outer Border (2) + Margins (2) + Inner Border (2)
		maxListWidth = m.width - (layout.BorderWidth() + layout.ContentMarginWidth() + layout.BorderWidth())
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
	buttonHeight := ButtonRowHeight(innerBoxWidth, 0, m.GetButtonSpecsForState()...)
	shadowHeight := 0
	hasShadow := currentConfig.UI.Shadow
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 4. Vertical Budgeting Logic
	var listHeight, overhead int
	var maxListHeight int

	// Determine vertical spacing for buttons (only if defined)
	buttonBudget := 0
	if m.showButtons {
		buttonBudget = buttonHeight
	}

	if m.subMenuMode {
		// Sub-menu overhead: subtitle + own borders (2) + buttons.
		// Title is embedded in the top border line by RenderBorderedBoxCtx, so it does
		// not consume a content row and is NOT counted here.
		overhead = subtitleHeight + layout.BorderHeight() + buttonBudget
		maxListHeight = m.height - overhead
	} else {
		// Full dialog overhead: borders, subtitle, buttons, shadow.
		// DialogContentHeight uses DialogButtonHeight (3) as the constant button budget;
		// if the width-based check dropped borders (buttonHeight = 1), add back the 2 freed lines.
		maxListHeight = layout.DialogContentHeight(m.height, subtitleHeight, m.showButtons, false) // Do not subtract shadow from inner box
		if m.showButtons && buttonHeight != DialogButtonHeight {
			maxListHeight += DialogButtonHeight - buttonHeight
		}
		// Account for inner border around the list (Top + Bottom = 2 lines)
		maxListHeight -= layout.BorderHeight()
		overhead = m.height - maxListHeight
	}

	// Large titlebar: deduct from list budget; drop titlebar first when space is tight.
	// Submenus always use small titlebar regardless of config.
	enabled := !m.subMenuMode && m.title != "" && currentConfig.UI.LargeTitleBars
	useLargeTitleBar, maxListHeight := DecideLargeTitleBar(enabled, maxListHeight, 3)
	if useLargeTitleBar {
		overhead += LargeTitleBarOverhead
	}

	// Height-based border fallback: drop bordered buttons when 2 or fewer list
	// rows remain — the 2 freed rows are more useful as list space, and this
	// threshold also prevents bordered buttons from showing just before clipping.
	if m.showButtons && buttonHeight == DialogButtonHeight && maxListHeight <= 2 {
		freed := DialogButtonHeight - 1 // reclaim 2 lines
		buttonHeight = 1
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

	// Final list height is whichever is smaller: intrinsic or maximum.
	// When maximized is true, we force the full height.
	listHeight = totalItemHeight
	if listHeight > maxListHeight || m.maximized {
		listHeight = maxListHeight
	}

	m.Layout = DialogLayout{
		Width:          m.width,
		Height:         m.height,
		HeaderHeight:   overhead - layout.BorderHeight(), // Store the reserved overhead height
		ViewportHeight: listHeight,
		ButtonHeight:   buttonHeight,
		ShadowHeight:   shadowHeight,
		Overhead:       overhead,
		SubtitleHeight: subtitleHeight,
		LargeTitleBar:  useLargeTitleBar,
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
	menuSelectedIndices[m.persistKey()] = m.cursor
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
	menuSelectedIndices[m.persistKey()] = m.cursor
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
	menuSelectedIndices[m.persistKey()] = m.cursor
}

func (m *MenuModel) scrollHalfPageUp() { //nolint:unused
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
	menuSelectedIndices[m.persistKey()] = m.cursor
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
	menuSelectedIndices[m.persistKey()] = m.cursor
}

func (m *MenuModel) scrollHalfPageDown() { //nolint:unused
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
	menuSelectedIndices[m.persistKey()] = m.cursor
}

// EscapeAction implements EscapeActioner: runs back/cancel action if present, otherwise prompts to exit.
func (m *MenuModel) EscapeAction() tea.Cmd {
	for _, btn := range m.buttons {
		if (btn.ZoneID == "btn-back" || btn.ZoneID == "btn-cancel" || btn.ZoneID == IDBackButton) && btn.Action != nil {
			action := btn.Action
			return func() tea.Msg { return action() }
		}
	}
	for _, btn := range m.buttons {
		if (btn.ZoneID == "btn-exit" || btn.ZoneID == IDExitButton) && btn.Action != nil {
			action := btn.Action
			return func() tea.Msg { return action() }
		}
	}
	return ConfirmExitFallback()
}

// SetHeaderVisibility toggles background/title for sub-menus
func (m *MenuModel) SetHeaderVisibility(visible bool) {
	m.list.SetShowTitle(visible)
}

// HelpText returns the current item's help text. When content sections are
// in use, the focused section's own HelpText (if any) takes priority --
// this is how sinput-only sections (no MenuItem list of their own) surface
// a SectionHelp string.
func (m *MenuModel) HelpText() string {
	if len(m.contentSections) > 0 && m.focusedSection >= 0 && m.focusedSection < len(m.contentSections) {
		if h, ok := m.contentSections[m.focusedSection].(interface{ HelpText() string }); ok {
			if t := h.HelpText(); t != "" {
				return t
			}
		}
	}
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].Help
	}
	return m.SectionHelp
}

// Cursor returns the current selection index
func (m *MenuModel) Cursor() int {
	return m.cursor
}

// syncSelectionToViewport ensures the current selection index (m.cursor) is within
// the visible range of the viewport [m.ViewStartY, m.ViewStartY + visible - 1].
// It is called after manual scroll events (scrollbar drag, mouse wheel) to
// satisfy the "Selection follows scroll" requirement.
func (m *MenuModel) syncSelectionToViewport() {
	// Column scroll mode manages cursor position in the interceptor; skip here.
	if m.FlowColumns >= 2 && m.MaxFlowRows > 0 {
		return
	}
	visible := m.Layout.ViewportHeight
	if visible <= 0 || len(m.items) == 0 {
		return
	}

	maxIdx := len(m.items) - 1

	// Range [low, high]
	low := m.ViewStartY
	high := m.ViewStartY + visible - 1
	if high > maxIdx {
		high = maxIdx
	}

	if m.list.Index() < low {
		m.list.Select(low)
		// Skip separators (moving down)
		for m.list.Index() < maxIdx && m.items[m.list.Index()].IsSeparator {
			m.list.CursorDown()
		}
	} else if m.list.Index() > high {
		m.list.Select(high)
		// Skip separators (moving up)
		for m.list.Index() > 0 && m.items[m.list.Index()].IsSeparator {
			m.list.CursorUp()
		}
	}

	m.cursor = m.list.Index()
	menuSelectedIndices[m.persistKey()] = m.cursor
}
