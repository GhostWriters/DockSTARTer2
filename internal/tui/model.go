package tui

import (
	"context"

	"DockSTARTer2/internal/config"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// ScreenType identifies different screens in the TUI
type ScreenType int

const (
	ScreenMainMenu ScreenType = iota
	ScreenConfigMenu
	ScreenOptionsMenu
	ScreenAppSelect
	ScreenThemeSelect
)

// ScreenModel is the interface for all screen models
type ScreenModel interface {
	tea.Model
	Title() string
	HelpText() string
	SetSize(width, height int)
	IsMaximized() bool
	HasDialog() bool
	MenuName() string // Returns the name used for --menu or -M to return to this screen
}

// Navigation messages
type (
	// NavigateMsg requests navigation to a new screen
	NavigateMsg struct {
		Screen ScreenModel
	}

	// NavigateBackMsg requests navigation back to previous screen
	NavigateBackMsg struct{}

	// ShowDialogMsg shows a modal dialog
	ShowDialogMsg struct {
		Dialog tea.Model
	}

	// CloseDialogMsg closes the current dialog
	CloseDialogMsg struct {
		Result any
	}

	// UpdateHeaderMsg triggers a header refresh
	UpdateHeaderMsg struct{}

	// QuitMsg requests application exit
	QuitMsg struct{}

	// FinalizeSelectionMsg combines navigation and dialog display for atomic transitions
	FinalizeSelectionMsg struct {
		Dialog tea.Model
	}

	// ShowConfirmDialogMsg shows a confirmation dialog with a result channel
	ShowConfirmDialogMsg struct {
		Title      string
		Question   string
		DefaultYes bool
		ResultChan chan bool
	}
)

// AppModel is the root Bubble Tea model
type AppModel struct {
	ctx    context.Context
	config config.AppConfig

	// Terminal dimensions
	width  int
	height int

	// Screen management
	activeScreen ScreenModel
	screenStack  []ScreenModel

	// Persistent backdrop (header + separator + helpline)
	backdrop BackdropModel

	// Slide-up log panel (always present below helpline)
	logPanel        LogPanelModel
	logPanelFocused bool

	// Modal dialog overlay (nil when no dialog)
	dialog      tea.Model
	dialogStack []tea.Model

	// Channel for receiving confirmation result from a modal dialog
	pendingConfirm chan bool

	// Ready flag (set after first WindowSizeMsg)
	ready bool

	// Fatal indicates if the program should exit with a fatal error message
	Fatal bool
}

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context, cfg config.AppConfig, startScreen ScreenModel) AppModel {
	// Get initial help text from screen if available
	helpText := ""
	if startScreen != nil {
		helpText = startScreen.HelpText()
		CurrentPageName = startScreen.MenuName()
	}

	return AppModel{
		ctx:          ctx,
		config:       cfg,
		activeScreen: startScreen,
		screenStack:  make([]ScreenModel, 0),
		backdrop:     NewBackdropModel(helpText),
		logPanel:     NewLogPanelModel(),
	}
}

// NewAppModelStandalone creates a new application model that starts with a modal dialog only
func NewAppModelStandalone(ctx context.Context, cfg config.AppConfig, dialog tea.Model) AppModel {
	return AppModel{
		ctx:      ctx,
		config:   cfg,
		backdrop: NewBackdropModel(""),
		logPanel: NewLogPanelModel(),
		dialog:   dialog,
	}
}

// Init implements tea.Model
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.backdrop.Init(),
		m.logPanel.Init(),
	}
	if m.activeScreen != nil {
		cmds = append(cmds, m.activeScreen.Init())
	}
	if m.dialog != nil {
		cmds = append(cmds, m.dialog.Init())
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case toggleLogPanelMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		// Return focus to screen when panel collapses
		if !m.logPanel.expanded {
			m.setLogPanelFocus(false)
		}
		// Resize backdrop, screen, and dialog to match new panel height
		backdropHeight := m.height - m.logPanel.Height()
		backdropMsg := tea.WindowSizeMsg{Width: m.width, Height: backdropHeight}
		backdropModel, _ := m.backdrop.Update(backdropMsg)
		m.backdrop = backdropModel.(BackdropModel)
		if m.activeScreen != nil {
			m.activeScreen.SetSize(m.width, backdropHeight)
		}
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, backdropHeight)
			}
		}
		return m, cmd

	case logLineMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		return m, cmd

	case tea.KeyMsg:
		// Specialized Help Blockade
		// If help is open, ANY key closes it and we return immediately to prevent leaks.
		if m.dialog != nil {
			if _, ok := m.dialog.(*helpDialogModel); ok {
				var cmd tea.Cmd
				m.dialog, cmd = m.dialog.Update(msg)
				return m, cmd
			}
		}

		// Global Priority Actions (always work, regardless of focus)
		if key.Matches(msg, Keys.ToggleLog) {
			return m, func() tea.Msg { return toggleLogPanelMsg{} }
		}
		if key.Matches(msg, Keys.Help) {
			return m, func() tea.Msg { return ShowDialogMsg{Dialog: newHelpDialogModel()} }
		}
		if key.Matches(msg, Keys.ForceQuit) {
			m.Fatal = true
			return m, tea.Quit
		}

		// Screen Navigation / Element Cycling
		// Cycle: Screen -> LogPanel -> Header(App) -> Header(Tmpl) -> Screen
		if key.Matches(msg, Keys.Tab) {
			if m.logPanelFocused {
				m.setLogPanelFocus(false)
				// setLogPanelFocus(false) refocuses screen/dialog. We need to unfocus them for Header focus.
				if m.dialog != nil {
					if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(false)
					}
				} else if m.activeScreen != nil {
					if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(false)
					}
				}
				m.backdrop.header.SetFocus(HeaderFocusApp)
				return m, nil
			} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
				m.backdrop.header.SetFocus(HeaderFocusTmpl)
				return m, nil
			} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
				m.backdrop.header.SetFocus(HeaderFocusNone)
				// Focus returns to screen/dialog
				if m.dialog != nil {
					if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(true)
					}
				} else if m.activeScreen != nil {
					if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(true)
					}
				}
				return m, nil
			} else {
				// From screen to log panel
				m.setLogPanelFocus(true)
				return m, nil
			}
		}

		if key.Matches(msg, Keys.ShiftTab) {
			if m.logPanelFocused {
				m.setLogPanelFocus(false)
				// Focus returns to screen/dialog (reverse cycle)
				// setLogPanelFocus(false) already restores focus, so we are good.
				return m, nil
			} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
				m.backdrop.header.SetFocus(HeaderFocusNone)
				m.setLogPanelFocus(true)
				return m, nil
			} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
				m.backdrop.header.SetFocus(HeaderFocusApp)
				return m, nil
			} else {
				// From screen to header (tmpl)
				if m.dialog != nil {
					if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(false)
					}
				} else if m.activeScreen != nil {
					if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(false)
					}
				}
				m.backdrop.header.SetFocus(HeaderFocusTmpl)
				return m, nil
			}
		}

		// Arrow Key Navigation within Header
		if m.backdrop.header.GetFocus() != HeaderFocusNone {
			if key.Matches(msg, Keys.Right) {
				if m.backdrop.header.GetFocus() == HeaderFocusApp {
					m.backdrop.header.SetFocus(HeaderFocusTmpl)
				}
				// Consume the key event even if already on last item
				return m, nil
			}
			if key.Matches(msg, Keys.Left) {
				if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
					m.backdrop.header.SetFocus(HeaderFocusApp)
				}
				// Consume the key event even if already on first item
				return m, nil
			}
			// Escape to return to screen
			if key.Matches(msg, Keys.Esc) {
				m.backdrop.header.SetFocus(HeaderFocusNone)
				if m.dialog != nil {
					if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(true)
					}
				} else if m.activeScreen != nil {
					if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
						focusable.SetFocused(true)
					}
				}
				return m, nil
			}
		}

		// Handle Enter on focused header items
		if key.Matches(msg, Keys.Enter) {
			switch m.backdrop.header.GetFocus() {
			case HeaderFocusApp:
				return m, TriggerAppUpdate()
			case HeaderFocusTmpl:
				return m, TriggerTemplateUpdate()
			}
		}

		// Focused Log Panel Actions
		// When log panel is focused, it gets all scroll/navigation keys exclusively
		// We handle this AFTER global cycling (Tab/ShiftTab) so we don't trap those keys.
		if m.logPanelFocused {
			// Esc unfocuses the panel and returns focus to the screen/dialog
			if key.Matches(msg, Keys.Esc) {
				m.setLogPanelFocus(false)
				return m, nil
			}
			// Enter or Space toggles the panel open/closed
			if key.Matches(msg, Keys.Enter) || msg.String() == " " {
				return m, func() tea.Msg { return toggleLogPanelMsg{} }
			}
			// All other keys go to the panel viewport
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd
		}

		// Modal Dialog Support
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, cmd
		}

		// Active Screen Support (fallback)
		if m.activeScreen != nil {
			updated, cmd := m.activeScreen.Update(msg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			return m, cmd
		}

	case tea.MouseClickMsg:
		// Specialized Help Blockade
		// If help is open, ANY click closes it and we return immediately to prevent leaks.
		if m.dialog != nil {
			if _, ok := m.dialog.(*helpDialogModel); ok {
				var cmd tea.Cmd
				m.dialog, cmd = m.dialog.Update(msg)
				return m, cmd
			}
		}

		// Check for header clicks (triggers update when clicking version numbers)
		if msg.Button == tea.MouseLeft {
			if handled, cmd := m.backdrop.header.HandleMouse(msg); handled {
				// If header took focus, ensure we unfocus screen/logpanel
				if m.backdrop.header.GetFocus() != HeaderFocusNone {
					m.setLogPanelFocus(false)
					// setLogPanelFocus(false) refocuses screen/dialog. We need to unfocus them for Header focus.
					if m.dialog != nil {
						if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
							focusable.SetFocused(false)
						}
					} else if m.activeScreen != nil {
						if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
							focusable.SetFocused(false)
						}
					}
				}

				// Trigger update on click
				if zi := zone.Get(ZoneAppVersion); zi != nil && zi.InBounds(msg) {
					return m, TriggerAppUpdate()
				}
				if zi := zone.Get(ZoneTmplVersion); zi != nil && zi.InBounds(msg) {
					return m, TriggerTemplateUpdate()
				}

				return m, cmd
			}
		}

		// Check log panel clicks (left button only)
		if msg.Button == tea.MouseLeft {
			if zi := zone.Get(logPanelZoneID); zi != nil && zi.InBounds(msg) {
				return m, func() tea.Msg { return toggleLogPanelMsg{} }
			}
			if zi := zone.Get(logViewportZoneID); zi != nil && zi.InBounds(msg) {
				m.setLogPanelFocus(true)
				return m, nil
			}
			// Click outside log panel — return focus to screen/dialog without
			// swallowing the click (falls through so buttons/items still fire)
			if m.logPanelFocused {
				m.setLogPanelFocus(false)
			}
		}
		// Forward mouse events to dialog or active screen
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, cmd
		}
		// If no dialog, let it fall through to active screen handling below

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update log panel with full dimensions first (so Height() is correct)
		m.logPanel.SetSize(m.width, m.height)

		// All other components use backdropHeight (terminal minus log panel strip)
		backdropHeight := m.height - m.logPanel.Height()
		sizeMsg := tea.WindowSizeMsg{Width: m.width, Height: backdropHeight}

		// Update backdrop
		backdropModel, _ := m.backdrop.Update(sizeMsg)
		m.backdrop = backdropModel.(BackdropModel)

		// Update dialog or active screen with the reduced height message
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, backdropHeight)
			}
			var dialogCmd tea.Cmd
			m.dialog, dialogCmd = m.dialog.Update(sizeMsg)
			if dialogCmd != nil {
				cmds = append(cmds, dialogCmd)
			}
		} else if m.activeScreen != nil {
			m.activeScreen.SetSize(m.width, backdropHeight)
			updated, screenCmd := m.activeScreen.Update(sizeMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			if screenCmd != nil {
				cmds = append(cmds, screenCmd)
			}
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
		}
		return m, tea.Batch(cmds...)

	case NavigateMsg:
		// Push current screen to stack and switch to new screen
		if m.activeScreen != nil {
			m.screenStack = append(m.screenStack, m.activeScreen)
		}
		m.activeScreen = msg.Screen
		if m.activeScreen != nil {
			CurrentPageName = m.activeScreen.MenuName()
			m.activeScreen.SetSize(m.width, m.height-m.logPanel.Height())
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
			cmds = append(cmds, m.activeScreen.Init())
		}
		return m, tea.Batch(cmds...)

	case NavigateBackMsg:
		// Pop from stack and return to previous screen
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			if m.activeScreen != nil {
				CurrentPageName = m.activeScreen.MenuName()
				m.activeScreen.SetSize(m.width, m.height-m.logPanel.Height())
				m.backdrop.SetHelpText(m.activeScreen.HelpText())
			}
		} else {
			// If stack is empty, we "go back" to nothing (which triggers quit at the bottom)
			m.activeScreen = nil
			CurrentPageName = ""
		}

		// Check for application exit immediately if we just cleared the last screen and have no dialog.
		// This avoids the "No active screen" splotch and ensures ESC works for standalone tools.
		if m.ready && m.activeScreen == nil && m.dialog == nil {
			return m, tea.Quit
		}
		return m, nil

	case ShowDialogMsg:
		// Push current dialog to stack if one exists
		if m.dialog != nil {
			m.dialogStack = append(m.dialogStack, m.dialog)
		}

		m.dialog = msg.Dialog
		if m.dialog != nil {
			backdropHeight := m.height - m.logPanel.Height()
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, backdropHeight)
			}
			// Explicitly focus the new dialog
			if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
				focusable.SetFocused(true)
			}
			cmds = append(cmds, m.dialog.Init())
		}
		return m, tea.Batch(cmds...)

	case ShowConfirmDialogMsg:
		// If a dialog is already open, push it to stack and show the confirm dialog as the new top
		if m.dialog != nil {
			m.dialogStack = append(m.dialogStack, m.dialog)
		}

		// Show it as the main dialog (top of stack)
		dialog := newConfirmDialog(msg.Title, msg.Question, msg.DefaultYes)
		backdropHeight := m.height - m.logPanel.Height()
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(m.width, backdropHeight)
		}
		m.dialog = dialog
		// Explicitly focus the new confirmation dialog
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(true)
		}
		m.pendingConfirm = msg.ResultChan
		return m, m.dialog.Init()

	case FinalizeSelectionMsg:
		// Atomically clear/navigate and show dialog to avoid race conditions in batches
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
		} else {
			m.activeScreen = nil
		}

		// Show the new dialog
		m.dialog = msg.Dialog
		if m.dialog != nil {
			backdropHeight := m.height - m.logPanel.Height()
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, backdropHeight)
			}
			if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
				focusable.SetFocused(true)
			}
			return m, m.dialog.Init()
		}
		return m, nil

	case CloseDialogMsg:
		// If we (AppModel) have a pending confirmation, we are the direct parent of the closing dialog.
		if m.pendingConfirm != nil {
			if result, ok := msg.Result.(bool); ok {
				m.pendingConfirm <- result
			} else {
				m.pendingConfirm <- false // Default to false if result invalid
			}
			m.pendingConfirm = nil
		}

		// Clear current dialog and try to pop from stack
		m.dialog = nil
		if len(m.dialogStack) > 0 {
			// Pop from stack
			m.dialog = m.dialogStack[len(m.dialogStack)-1]
			m.dialogStack = m.dialogStack[:len(m.dialogStack)-1]

			// Re-focus and re-size the popped dialog
			if m.dialog != nil {
				backdropHeight := m.height - m.logPanel.Height()
				if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
					sizable.SetSize(m.width, backdropHeight)
				}
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			}
			return m, nil
		}

		// Fallback: if stack is empty and no screen, we quit.
		// Reaching here usually means the main dialog that started the program is closing itself.
		// This handles NewAppModelStandalone cases and standalone screens after NavigateBack is handled.
		if m.activeScreen == nil {
			return m, tea.Quit
		}
		return m, nil

	case UpdateHeaderMsg:
		m.backdrop.header.Refresh()
		return m, nil

	case QuitMsg:
		return m, tea.Quit
	}

	// Update backdrop
	var backdropCmd tea.Cmd
	backdropModel, backdropCmd := m.backdrop.Update(msg)
	m.backdrop = backdropModel.(BackdropModel)
	if backdropCmd != nil {
		cmds = append(cmds, backdropCmd)
	}

	// Update dialog if present, otherwise update active screen
	if m.dialog != nil {
		var dialogCmd tea.Cmd
		m.dialog, dialogCmd = m.dialog.Update(msg)
		if dialogCmd != nil {
			cmds = append(cmds, dialogCmd)
		}
		// If dialog supports help text, update backdrop
		if h, ok := m.dialog.(interface{ HelpText() string }); ok {
			m.backdrop.SetHelpText(h.HelpText())
		}
	} else if m.activeScreen != nil {
		var screenCmd tea.Cmd
		updated, screenCmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		if screenCmd != nil {
			cmds = append(cmds, screenCmd)
		}
		// Update helpline from screen
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
	}

	// Check for application exit when both activeScreen and dialog are nil
	// This happens when NavigateBack is used on the "root" screen.
	// We wait until the end of Update to handle batches (e.g. ShowDialog + NavigateBack)
	if m.ready && m.activeScreen == nil && m.dialog == nil {
		return m, tea.Batch(tea.Quit, tea.Batch(cmds...))
	}

	return m, tea.Batch(cmds...)
}

// setLogPanelFocus updates logPanelFocused and tells the active screen to
// unfocus/refocus its border accordingly (if it supports the interface).
func (m *AppModel) setLogPanelFocus(focused bool) {
	m.logPanelFocused = focused
	m.logPanel.focused = focused
	if m.activeScreen != nil {
		if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(!focused)
		}
	}
	if m.dialog != nil {
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(!focused)
		}
	}
}

// ViewStringer is an interface for models that provide string content for compositing
type ViewStringer interface {
	ViewString() string
}

// View implements tea.Model
// Uses backdrop + overlay pattern (same as dialogs)
func (m AppModel) View() tea.View {
	if !m.ready {
		return tea.NewView("Initializing...")
	}

	// Get backdrop view as string
	output := m.backdrop.ViewString()

	// Layer 1: Active Screen
	if m.activeScreen != nil {
		var screenContent string
		if vs, ok := m.activeScreen.(ViewStringer); ok {
			screenContent = vs.ViewString()
		}
		if screenContent != "" {
			output = Overlay(
				screenContent,
				output,
				OverlayCenter,
				OverlayCenter,
				0,
				0,
			)
		}
	}

	// Layer 2: Modal Dialog
	if m.dialog != nil {
		var dialogContent string
		if vs, ok := m.dialog.(ViewStringer); ok {
			dialogContent = vs.ViewString()
		}
		if dialogContent != "" {
			output = Overlay(
				dialogContent,
				output,
				OverlayCenter,
				OverlayCenter,
				0,
				0,
			)
		}
	}

	// Layer 3: Log panel — appended below the backdrop (below helpline)
	var logPanelContent string
	if vs, ok := interface{}(m.logPanel).(ViewStringer); ok {
		logPanelContent = vs.ViewString()
	}
	output = lipgloss.JoinVertical(lipgloss.Left, output, logPanelContent)

	// Scan zones at the root level
	v := tea.NewView(zone.Scan(output))
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}
