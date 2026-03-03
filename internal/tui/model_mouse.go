package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// isButtonHitID returns true if the hit ID belongs to a button region.
// Button IDs from menus use the "btn-" prefix; button IDs from screens/dialogs
// use the "_button" suffix (e.g. "apply_button", "back_button", "exit_button").
func isButtonHitID(id string) bool {
	return strings.HasPrefix(id, "btn-") || strings.HasSuffix(id, "_button")
}

// hitIDToPanelID converts a hit ID to its parent panel ID for hover-based interactions.
// Returns the panel that should receive focus when hovering over the given region.
// For example: "item-theme_list-0" -> "theme_panel", "apply_button" -> "button_panel"
func hitIDToPanelID(hitID string) string {
	// Map menu item prefixes to their panel IDs
	if strings.HasPrefix(hitID, "item-theme_list-") {
		return IDThemePanel
	}
	if strings.HasPrefix(hitID, "item-options_list-") {
		return IDOptionsPanel
	}
	// Any other "item-" hit (regular menu list items) maps to the list panel so
	// hovering back over the list restores FocusList before the wheel is forwarded.
	if strings.HasPrefix(hitID, "item-") {
		return IDListPanel
	}
	// Map all button row IDs (both menu "btn-" buttons and display_options named buttons)
	// to the button panel so hover+scroll cycles buttons and middle-click activates focused button
	if strings.HasPrefix(hitID, "btn-") {
		return IDButtonPanel
	}
	if hitID == IDApplyButton || hitID == IDBackButton || hitID == IDExitButton || hitID == IDButtonPanel {
		return IDButtonPanel
	}
	// For panel IDs themselves, return as-is
	if hitID == IDThemePanel || hitID == IDOptionsPanel {
		return hitID
	}
	// All other IDs (regular menu items, etc.) don't map to a panel
	return ""
}

// handleMouseMsg processes mouse input.
// Returns (model, cmd, handled) where handled indicates if the event was consumed.
func (m *AppModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	// 1. RESIZE DRAG PRIORITY: If log panel is dragging, it intercepts EVERYTHING
	if m.logPanel.isDragging {
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)

		// If height changed, we need to resize other components
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
		}
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(caW, caH)
			}
		}
		return m, cmd, true
	}

	// 2. FOCUS PRIORITY: If log panel has keyboard focus, it owns the scroll wheel and middle click.
	// We do this BEFORE dialog checks so that if a user tabs to logs and a dialog is behind it,
	// the wheel still scrolls the logs.
	if m.logPanelFocused {
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseMiddle {
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}
	}

	// 3. HELP BLOCKADE: If help is open, ANY click closes it for convenience.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			if _, ok := msg.(tea.MouseClickMsg); ok {
				var cmd tea.Cmd
				m.dialog, cmd = m.dialog.Update(msg)
				return m, cmd, true
			}
		}
	}

	// 4. HOVER-AWARE WHEEL AND MIDDLE-CLICK
	// When wheel scrolling or middle-clicking, first focus the panel under the mouse,
	// then perform the action. This allows hover+scroll/click without needing to click first.
	// If not hovering over a scrollable area, do nothing.
	if wheelMsg, isWheel := msg.(tea.MouseWheelMsg); isWheel {
		// Hit test to find what's under the mouse
		hitID := m.hitRegions.FindHit(wheelMsg.X, wheelMsg.Y)

		// No hit = not over a scrollable area, ignore the wheel
		if hitID == "" {
			return m, nil, true
		}

		// Status bar: route wheel to the header for version cycling
		if hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion {
			if m.dialog != nil {
				return m, nil, true // Block interaction if dialog is open
			}
			var cmd tea.Cmd
			if m.backdrop != nil {
				updated, bCmd := m.backdrop.Update(LayerWheelMsg{ID: IDStatusBar, Button: wheelMsg.Button})
				if backdrop, ok := updated.(*BackdropModel); ok {
					m.backdrop = backdrop
				}
				cmd = bCmd
			}
			return m, cmd, true
		}

		// Check if hovering over log panel - if so, focus and scroll it
		if hitID == IDLogPanel || hitID == IDLogViewport {
			m.setLogPanelFocus(true)
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}

		// Unfocus log panel since we're over something else
		m.setLogPanelFocus(false)

		// Clear header focus — wheel moved away from the status bar
		m.setHeaderFocus(HeaderFocusNone)

		panelID := hitIDToPanelID(hitID)

		// List panel: send a semantic LayerWheelMsg so screens can scroll the list
		// without changing button focus — mirrors keyboard up/down arrow behaviour.
		if panelID == IDListPanel {
			listWheel := LayerWheelMsg{ID: IDListPanel, Button: wheelMsg.Button}
			var cmd tea.Cmd
			if m.dialog != nil {
				m.dialog, cmd = m.dialog.Update(listWheel)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(listWheel)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				cmd = sCmd
			}
			return m, cmd, true
		}

		// For other panels (submenus, button row), switch focus to the hovered panel first
		if panelID != "" {
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft}
			if m.dialog != nil {
				m.dialog, _ = m.dialog.Update(focusMsg)
			} else if m.activeScreen != nil {
				updated, _ := m.activeScreen.Update(focusMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
			}
		}

		// Forward the wheel event to scroll
		var cmd tea.Cmd
		if m.dialog != nil {
			m.dialog, cmd = m.dialog.Update(msg)
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(msg)
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			cmd = sCmd
		}
		return m, cmd, true
	}

	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseMiddle {
		// Hit test to find what's under the mouse
		hitID := m.hitRegions.FindHit(click.X, click.Y)

		// Status bar: middle-click activates the currently focused version item
		if hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion || hitID == IDHeaderFlags {
			if m.dialog != nil {
				return m, nil, true // Block interaction if dialog is open
			}
			var cmd tea.Cmd
			if m.backdrop != nil {
				updated, bCmd := m.backdrop.Update(ToggleFocusedMsg{})
				if backdrop, ok := updated.(*BackdropModel); ok {
					m.backdrop = backdrop
				}
				cmd = bCmd
			}
			return m, cmd, true
		}

		// Check if hovering over log panel - focus it and send toggle
		if hitID == IDLogPanel || hitID == IDLogViewport {
			m.setLogPanelFocus(true)
			updated, cmd := m.logPanel.Update(ToggleFocusedMsg{})
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}

		// Unfocus log panel
		m.setLogPanelFocus(false)

		// Clear header focus — middle-click landed away from the status bar
		m.setHeaderFocus(HeaderFocusNone)

		// Check if the hit ID maps to a panel (submenu or button row).
		// Panel-mapped IDs use the hover model: focus the panel, then activate the
		// currently focused item in that panel via ToggleFocusedMsg.
		// This covers display_options submenus (theme/options) and button row.
		panelID := hitIDToPanelID(hitID)
		if panelID != "" {
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft}
			if m.dialog != nil {
				m.dialog, _ = m.dialog.Update(focusMsg)
			} else if m.activeScreen != nil {
				updated, _ := m.activeScreen.Update(focusMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
			}
			var toggleCmd tea.Cmd
			if m.dialog != nil {
				m.dialog, toggleCmd = m.dialog.Update(ToggleFocusedMsg{})
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(ToggleFocusedMsg{})
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				toggleCmd = sCmd
			}
			return m, toggleCmd, true
		}

		// For buttons not mapped to a panel (regular menu/dialog buttons like btn-select,
		// Yes/No confirm buttons, OK dismiss buttons), dispatch as a left click so the
		// button action fires normally.
		if isButtonHitID(hitID) {
			layerMsg := LayerHitMsg{ID: hitID, Button: tea.MouseLeft}
			var btnCmd tea.Cmd
			if m.dialog != nil {
				m.dialog, btnCmd = m.dialog.Update(layerMsg)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(layerMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				btnCmd = sCmd
			}
			return m, btnCmd, true
		}

		// For anything else with no panel and no button mapping, send ToggleFocusedMsg
		// generically (e.g., middle-clicking empty screen space).
		var mainCmd tea.Cmd
		if m.dialog != nil {
			m.dialog, mainCmd = m.dialog.Update(ToggleFocusedMsg{})
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(ToggleFocusedMsg{})
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			mainCmd = sCmd
		}
		return m, mainCmd, true
	}

	// 5. HIT TESTING & SEMANTIC DISPATCH (for other buttons/wheel)
	var hitID string
	var hitButton tea.MouseButton

	switch me := msg.(type) {
	case tea.MouseClickMsg:
		hitID = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
	case tea.MouseWheelMsg:
		hitID = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
	}

	if hitID != "" {
		// Create semantic message with button info
		var semanticMsg tea.Msg
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			semanticMsg = LayerWheelMsg{ID: hitID, Button: hitButton}
		} else {
			semanticMsg = LayerHitMsg{ID: hitID, Button: hitButton}
		}

		// A. AppModel Internal IDs (handled globally)
		switch hitID {
		case IDLogToggle:
			if me, ok := msg.(tea.MouseClickMsg); ok && me.Button == tea.MouseLeft {
				return m, func() tea.Msg { return toggleLogPanelMsg{} }, true
			}
		case IDLogResize:
			if me, ok := msg.(tea.MouseClickMsg); ok && me.Button == tea.MouseLeft {
				// Correctly deliver raw msg to start dragging
				updated, cmd := m.logPanel.Update(msg)
				m.logPanel = updated.(LogPanelModel)
				m.setLogPanelFocus(true)
				return m, cmd, true
			}
		case IDLogPanel, IDLogViewport:
			m.setLogPanelFocus(true)
			if _, ok := msg.(tea.MouseWheelMsg); ok {
				updated, cmd := m.logPanel.Update(msg)
				m.logPanel = updated.(LogPanelModel)
				return m, cmd, true
			}
			return m, nil, true
		default:
			// If we hit anything else (dialog, screen, header), ensure logs and header are unfocused
			m.setLogPanelFocus(false)
			m.setHeaderFocus(HeaderFocusNone)
		}

		// B. Component-specific Dispatch (Semantic Pre-pass)
		var semanticCmd tea.Cmd
		if m.dialog != nil {
			m.dialog, semanticCmd = m.dialog.Update(semanticMsg)
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(semanticMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			semanticCmd = sCmd
		}

		// C. Global Backdrop (Header etc)
		var backdropCmd tea.Cmd
		if m.backdrop != nil {
			// Do NOT forward semantic messages natively to backdrop if a dialog is open (prevents status bar hits)
			if m.dialog != nil && (hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion || hitID == IDHeaderFlags) {
				// Block background hits
			} else {
				updated, bCmd := m.backdrop.Update(semanticMsg)
				if backdrop, ok := updated.(*BackdropModel); ok {
					m.backdrop = backdrop
				}
				backdropCmd = bCmd
			}
		}

		// RETURN FALSE: Allow raw message to fall through for full compatibility
		m.updateComponentFocus()
		return m, tea.Batch(semanticCmd, backdropCmd), false
	}

	// 6. MODAL FALLBACK (No hit, but dialog is open)
	if m.dialog != nil {
		m.setLogPanelFocus(false)
		return m, nil, false // Let raw msg fall through to dialog in standard loop
	}

	// 7. DROP UNHANDLED HOVER: Stop unhandled MouseMotion events from falling
	// through and triggering full-frame UI redrawing up to 120 times a second
	if _, ok := msg.(tea.MouseMotionMsg); ok {
		return m, nil, true
	}

	// 8. DEFAULT: No hits, no modal.
	m.updateComponentFocus()
	return m, nil, false
}
