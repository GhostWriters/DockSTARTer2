package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// isButtonHitID returns true if the hit ID belongs to a button region.
// Button IDs from menus use the "btn-" prefix; button IDs from screens/dialogs
// use the "_button" suffix (e.g. "apply_button", "back_button", "exit_button").
func isButtonHitID(id string) bool {
	return strings.HasPrefix(id, "btn-") ||
		strings.HasSuffix(id, "_button") ||
		strings.Contains(id, ".") // Standard pattern for dialog buttons (dialogID.ZoneID)
}

// hitIDToPanelID converts a hit ID to its parent panel ID for hover-based interactions.
func hitIDToPanelID(hitID string) string {
	// 1. Map menu item IDs ("item-<panelID>-<index>") to their parent panel IDs.
	if strings.HasPrefix(hitID, "item-") {
		parts := strings.Split(hitID, "-")
		if len(parts) >= 3 {
			return strings.Join(parts[1:len(parts)-1], "-")
		}
		return IDListPanel
	}

	// 2. Normalize prefixed IDs (e.g. "menuID.list_panel" -> "menuID")
	// For multi-panel focus, we need the parent component ID (e.g. "options_panel")
	// rather than the generic internal zone ID ("list_panel").
	effectiveID := hitID
	if strings.Contains(hitID, ".") {
		parts := strings.Split(hitID, ".")
		effectiveID = parts[0]
	}

	// 3. Map button IDs to the button panel ONLY IF they are panel-level buttons in a sub-menu.
	// We want global buttons like Apply/Back/Exit to fall through to normal `MouseLeft` routing,
	// so they don't get caught in the panel-hover -> ToggleFocusedMsg auto-activation branch.
	if strings.HasPrefix(effectiveID, "btn-") || effectiveID == IDApplyButton || effectiveID == IDBackButton || effectiveID == IDExitButton {
		return IDButtonPanel
	}

	// 4. Panel IDs themselves
	if effectiveID == IDThemePanel || effectiveID == IDOptionsPanel || effectiveID == IDListPanel ||
		effectiveID == IDLogViewport || effectiveID == IDButtonPanel {
		return effectiveID
	}

	// 4b. Scrollable list regions in dialogs: map to IDListPanel so wheel uses
	// hover+LayerWheelMsg routing instead of the focus-snap generic path.
	if effectiveID == "setvalue_list" || effectiveID == "addvar_list" {
		return IDListPanel
	}

	// 5. Base Menu IDs (the background region of a MenuModel).
	// If the user hovers the background of a menu, we still want the wheel to scroll the list.
	// Common screen IDs are "main_menu", "config_menu", "options_menu", "app_selection", "global_flags"
	// Rather than hardcoding every ID, if it's not a known panel/button but has a hit, it's likely a menu background.
	if hitID != "" && hitID != IDStatusBar && hitID != IDAppVersion && hitID != IDTmplVersion && hitID != IDHeaderFlags && hitID != IDLogPanel && hitID != IDLogToggle && hitID != IDLogResize {
		return effectiveID
	}

	return ""
}

// handleMouseMsg processes mouse input.
// Returns (model, cmd, handled) where handled indicates if the event was consumed.
func (m *AppModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	// 1. RESIZE DRAG PRIORITY: If log panel is dragging, it intercepts EVERYTHING
	if m.logPanel.isDragging {
		prevHeight := m.logPanel.height
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)

		// Only resize downstream components when the panel height actually changed.
		// Mouse motion events fire at pixel resolution but terminal rows span many
		// pixels, so many consecutive events map to the same row — skipping the
		// resize avoids invalidating the menu render cache and recomputing shadows
		// on those no-op frames.
		if m.logPanel.height != prevHeight {
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
		}
		return m, cmd, true
	}

	// 1b. LOG PANEL SCROLLBAR DRAG PRIORITY: If the log-panel scrollbar thumb is being dragged,
	// intercept all mouse events for proportional scrolling.
	if m.logPanelSbDragging {
		if _, ok := msg.(tea.MouseReleaseMsg); ok {
			m.logPanelSbDragging = false
			return m, nil, true
		}
		if motion, ok := msg.(tea.MouseMotionMsg); ok {
			vpH := m.logPanel.Height() - 1
			updated, changed := m.logPanel.DragScrollbar(motion.Y, m.logPanelSbAbsTopY, vpH)
			if changed {
				m.logPanel = updated
			}
			return m, nil, true
		}
		return m, nil, true
	}

	// 2. SCROLLBAR DRAG PRIORITY: If the active screen or dialog is dragging a scrollbar thumb
	// it intercepts all mouse events (motion, release) until the drag ends.
	type sbDragger interface{ IsScrollbarDragging() bool }
	if m.activeScreen != nil {
		if d, ok := m.activeScreen.(sbDragger); ok && d.IsScrollbarDragging() {
			updated, cmd := m.activeScreen.Update(msg)
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			return m, cmd, true
		}
	}
	if m.dialog != nil {
		if d, ok := m.dialog.(sbDragger); ok && d.IsScrollbarDragging() {
			updated, cmd := m.dialog.Update(msg)
			m.dialog = updated
			return m, cmd, true
		}
	}

	// 3. FOCUS PRIORITY: If log panel has keyboard focus, it owns the scroll wheel and middle click.
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

	// 4. HELP BLOCKADE: If help is open, ANY click closes it for convenience.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			if click, ok := msg.(tea.MouseClickMsg); ok {
				var cmd tea.Cmd
				m.dialog, cmd = m.dialog.Update(msg)
				// If right-click, we handled closing it — stop here
				if click.Button == tea.MouseRight {
					return m, cmd, true
				}
				return m, cmd, true
			}
		}
	}

	// 4b. GLOBAL RIGHT-CLICK: Intercept right-click on background or anywhere
	// if it wasn't intercepted by a modal blockade above.
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseRight {
		// Don't allow bringing up a new context menu if one is already open.
		// Standard behavior is to click outside to close the current one.
		if _, ok := m.dialog.(*ContextMenuModel); ok {
			return m, nil, true
		}

		hit := m.hitRegions.FindHit(click.X, click.Y)
		hitID := ""
		if hit != nil {
			hitID = hit.ID
		}

		// 1. If hitting a context menu already open, let the hit region dispatch it (usually closes)
		if strings.HasPrefix(hitID, "ctxmenu.") {
			// Fall through to normal hit dispatch
		} else if hit == nil || hitID == IDStatusBar || hitID == IDLogPanel || hitID == IDLogViewport || hitID == IDLogToggle || hitID == IDLogResize ||
			hitID == IDAppVersion || hitID == IDTmplVersion || hitID == IDHeaderFlags {
			// 2. If hitting background or a global element that doesn't usually have a context menu,
			// show the global context menu.
			return m, m.showGlobalContextMenu(click.X, click.Y, hit), true
		}
		// 3. Otherwise, fall through to hit region dispatch (to allow sinput/vars-editor right-click menus)
	}

	// 4. HOVER-AWARE WHEEL AND MIDDLE-CLICK
	// When wheel scrolling or middle-clicking, first focus the panel under the mouse,
	// then perform the action. This allows hover+scroll/click without needing to click first.
	// If not hovering over a scrollable area, do nothing.
	if wheelMsg, isWheel := msg.(tea.MouseWheelMsg); isWheel {
		// Hit test to find what's under the mouse
		hit := m.hitRegions.FindHit(wheelMsg.X, wheelMsg.Y)
		hitID := ""
		if hit != nil {
			hitID = hit.ID
		}

		// No hit = not over a scrollable area, ignore the wheel
		if hit == nil {
			return m, nil, true
		}

		// Status bar: route wheel to the header for version cycling
		if hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion {
			if m.dialog != nil {
				return m, nil, true // Block interaction if dialog is open
			}
			var cmd tea.Cmd
			if m.backdrop != nil {
			updated, bCmd := m.backdrop.Update(LayerWheelMsg{ID: IDStatusBar, Button: wheelMsg.Button, Hit: hit})
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
			// Trigger focus shift FIRST so the border changes visually
			focusMsg := LayerHitMsg{ID: IDListPanel, Button: HoverButton, X: wheelMsg.X, Y: wheelMsg.Y, Hit: hit} // Use custom HoverButton
			if m.dialog != nil {
				m.dialog, _ = m.dialog.Update(focusMsg)
			} else if m.activeScreen != nil {
				updated, _ := m.activeScreen.Update(focusMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
			}

			listWheel := LayerWheelMsg{ID: IDListPanel, Button: wheelMsg.Button, Hit: hit}
			var cmd tea.Cmd
			if m.dialog != nil {
				m.dialog, cmd = m.dialog.Update(listWheel)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(listWheel)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				cmd = sCmd
				return m, cmd, true
			}
		}

		// For other panels (submenus, button row), switch focus to the hovered panel first
		if panelID != "" {
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft, X: wheelMsg.X, Y: wheelMsg.Y, Hit: hit}
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
		hit := m.hitRegions.FindHit(click.X, click.Y)
		hitID := ""
		if hit != nil {
			hitID = hit.ID
		}

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
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft, X: click.X, Y: click.Y, Hit: hit}
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
			layerMsg := LayerHitMsg{ID: hitID, Button: tea.MouseLeft, X: click.X, Y: click.Y, Hit: hit}
			var btnCmd tea.Cmd
			if m.dialog != nil {
				m.dialog, btnCmd = m.dialog.Update(layerMsg)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(layerMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				btnCmd = sCmd
				return m, btnCmd, true
			}
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
	var hitX, hitY int

	var hit *HitRegion
	switch me := msg.(type) {
	case tea.MouseClickMsg:
		hit = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
		hitX, hitY = me.X, me.Y
	case tea.MouseWheelMsg:
		hit = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
		hitX, hitY = me.X, me.Y
	}

	if hit != nil {
		hitID = hit.ID
	}

	if hitID != "" {
		// Create semantic message with button info
		var semanticMsg tea.Msg
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			semanticMsg = LayerWheelMsg{ID: hitID, Button: hitButton, Hit: hit}
		} else {
			semanticMsg = LayerHitMsg{ID: hitID, Button: hitButton, X: hitX, Y: hitY, Hit: hit}
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
			// Log panel scrollbar hits (IDs are "log_panel.sb.*")
			if strings.HasPrefix(hitID, IDLogPanel+".sb.") {
				m.setLogPanelFocus(true)
				if !strings.HasSuffix(hitID, ".sb.thumb") {
					// Arrow/track clicks: route LayerHitMsg to log panel and return
					updated, cmd := m.logPanel.Update(LayerHitMsg{ID: hitID, Button: hitButton, Hit: hit})
					m.logPanel = updated.(LogPanelModel)
					return m, cmd, true
				}
				// .sb.thumb: fall through to B0 to start the drag
			} else {
				// If we hit anything else (dialog, screen, header), ensure logs and header are unfocused
				m.setLogPanelFocus(false)
				m.setHeaderFocus(HeaderFocusNone)
			}
		}

		// B0. Scrollbar thumb drag start — route raw click so the drag-target gets the Y coordinate.
		// Other .sb.* IDs (up/down/above/below) are handled normally via LayerHitMsg.
		if strings.HasSuffix(hitID, ".sb.thumb") {
			if me, ok := msg.(tea.MouseClickMsg); ok && me.Button == tea.MouseLeft {
				// Log panel scrollbar thumb
				if strings.HasPrefix(hitID, IDLogPanel+".sb.") {
					m.setLogPanelFocus(true)
					m.logPanelSbDragging = true
					m.logPanelSbAbsTopY = m.height - m.logPanel.Height() + 1
					return m, nil, true
				}
				// Dialog scrollbar thumb
				if m.dialog != nil {
					updated, cmd := m.dialog.Update(msg)
					m.dialog = updated
					return m, cmd, true
				}
				// Active screen scrollbar thumb
				if m.activeScreen != nil {
					updated, cmd := m.activeScreen.Update(msg)
					if s, ok := updated.(ScreenModel); ok {
						m.activeScreen = s
					}
					return m, cmd, true
				}
			}
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

		// Return handled=true if semantic messages were dispatched to stop raw click fall-through
		m.updateComponentFocus()

		// Final Fallback for unhandled right-clicks: trigger global context menu
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseRight {
			// If we didn't show a context menu or handle the click semantically, show global menu.
			if semanticCmd == nil && backdropCmd == nil {
				return m, m.showGlobalContextMenu(click.X, click.Y, hit), true
			}
		}

		return m, tea.Batch(semanticCmd, backdropCmd), true
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
