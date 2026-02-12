package tui

import (
	"context"

	"DockSTARTer2/internal/config"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone"
	overlay "github.com/rmhubbert/bubbletea-overlay"
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
	dialog tea.Model

	// Ready flag (set after first WindowSizeMsg)
	ready bool
}

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context, cfg config.AppConfig, startScreen ScreenModel) AppModel {
	// Get initial help text from screen if available
	helpText := ""
	if startScreen != nil {
		helpText = startScreen.HelpText()
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

// Init implements tea.Model
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.backdrop.Init(),
		m.logPanel.Init(),
	}
	if m.activeScreen != nil {
		cmds = append(cmds, m.activeScreen.Init())
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
		// Toggle log panel (always works, even when focused)
		if key.Matches(msg, Keys.ToggleLog) {
			return m, func() tea.Msg { return toggleLogPanelMsg{} }
		}

		// When log panel is focused, it gets all scroll/navigation keys
		if m.logPanelFocused {
			// Esc or Tab unfocuses the panel and returns focus to the screen
			if key.Matches(msg, Keys.Esc) || key.Matches(msg, Keys.Tab) {
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

		// Tab focuses the log panel strip (works whether collapsed or expanded)
		if key.Matches(msg, Keys.Tab) && m.dialog == nil {
			m.setLogPanelFocus(true)
			return m, nil
		}

		// If dialog is open, send keys to dialog
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
		// Check log panel clicks
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
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
				m.backdrop.SetHelpText(m.activeScreen.HelpText())
			}
		}
		return m, nil

	case ShowDialogMsg:
		m.dialog = msg.Dialog
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, m.height)
			}
			cmds = append(cmds, m.dialog.Init())
		}
		return m, tea.Batch(cmds...)

	case CloseDialogMsg:
		m.dialog = nil
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
}

// View implements tea.Model
// Uses backdrop + overlay pattern (same as dialogs)
func (m AppModel) View() tea.View {
	if !m.ready {
		return "Initializing..."
	}

	// Get backdrop view
	output := m.backdrop.View()

	// Layer 1: Active Screen
	if m.activeScreen != nil {
		screenView := m.activeScreen.View()
		if screenView != "" {
			output = overlay.Composite(
				screenView,
				output,
				overlay.Center,
				overlay.Center,
				0,
				0,
			)
		}
	}

	// Layer 2: Modal Dialog
	if m.dialog != nil {
		dialogView := m.dialog.View()
		if dialogView != "" {
			output = overlay.Composite(
				dialogView,
				output,
				overlay.Center,
				overlay.Center,
				0,
				0,
			)
		}
	}

	// Layer 3: Log panel — appended below the backdrop (below helpline)
	output = lipgloss.JoinVertical(lipgloss.Left, output, m.logPanel.View())

	// Scan zones at the root level
	return zone.Scan(output)
}
