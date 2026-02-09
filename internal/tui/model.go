package tui

import (
	"context"

	"DockSTARTer2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
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
	}
}

// Init implements tea.Model
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.backdrop.Init(),
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
	case tea.KeyMsg:
		// If dialog is open, send keys to dialog
		if m.dialog != nil {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
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

		// Update backdrop size FIRST
		backdropModel, _ := m.backdrop.Update(msg)
		m.backdrop = backdropModel.(BackdropModel)

		// Update active screen size with full dimensions
		if m.activeScreen != nil {
			m.activeScreen.SetSize(m.width, m.height)
		}

		// Update dialog size if present
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, m.height)
			}
		}

		return m, nil

	case NavigateMsg:
		// Push current screen to stack and switch to new screen
		if m.activeScreen != nil {
			m.screenStack = append(m.screenStack, m.activeScreen)
		}
		m.activeScreen = msg.Screen
		if m.activeScreen != nil {
			m.activeScreen.SetSize(m.width, m.height)
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

// View implements tea.Model
// Uses backdrop + overlay pattern (same as dialogs)
func (m AppModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Get backdrop view
	backdropView := m.backdrop.View()

	// Get content view (dialog or screen)
	var contentView string
	if m.dialog != nil {
		contentView = m.dialog.View()
	} else if m.activeScreen != nil {
		contentView = m.activeScreen.View()
	}

	// If no content, just show backdrop
	if contentView == "" {
		return backdropView
	}

	// Use overlay to composite content over backdrop
	// Since we scan zones AFTER compositing (at root level), we can safely center
	// overlay.Composite(foreground, background, xPos, yPos, xOffset, yOffset)
	output := overlay.Composite(
		contentView,    // foreground (content to overlay)
		backdropView,   // background (backdrop base)
		overlay.Center, // xPos: center horizontally
		overlay.Center, // yPos: center vertically
		0,              // xOffset: no offset
		0,              // yOffset: no offset
	)

	// Scan zones at the root level (required for BubbleZones to work correctly)
	// Zones are positioned correctly because scanning happens AFTER compositing
	return zone.Scan(output)
}
