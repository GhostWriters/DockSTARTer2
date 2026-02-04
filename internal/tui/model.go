package tui

import (
	"context"
	"strings"

	"DockSTARTer2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Persistent components
	header   HeaderModel
	helpline HelplineModel

	// Modal dialog overlay (nil when no dialog)
	dialog tea.Model

	// Ready flag (set after first WindowSizeMsg)
	ready bool
}

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context, cfg config.AppConfig, startScreen ScreenModel) AppModel {
	return AppModel{
		ctx:          ctx,
		config:       cfg,
		activeScreen: startScreen,
		screenStack:  make([]ScreenModel, 0),
		header:       NewHeaderModel(),
		helpline:     NewHelplineModel(),
	}
}

// Init implements tea.Model
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.header.Init(),
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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update header width (reduced by 2 for invisible margins)
		m.header.SetWidth(m.width - 2)

		// Calculate content area height (header + sep + helpline)
		contentHeight := m.height - 3

		// Update active screen size
		if m.activeScreen != nil {
			m.activeScreen.SetSize(m.width, contentHeight)
		}

		// Update dialog size if present
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(m.width, contentHeight)
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
			contentHeight := m.height - 3
			m.activeScreen.SetSize(m.width, contentHeight)
			m.helpline.SetText(m.activeScreen.HelpText())
			cmds = append(cmds, m.activeScreen.Init())
		}
		return m, tea.Batch(cmds...)

	case NavigateBackMsg:
		// Pop from stack and return to previous screen
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			if m.activeScreen != nil {
				m.helpline.SetText(m.activeScreen.HelpText())
			}
		}
		return m, nil

	case ShowDialogMsg:
		m.dialog = msg.Dialog
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				contentHeight := m.height - 3
				sizable.SetSize(m.width, contentHeight)
			}
			cmds = append(cmds, m.dialog.Init())
		}
		return m, tea.Batch(cmds...)

	case CloseDialogMsg:
		m.dialog = nil
		return m, nil

	case UpdateHeaderMsg:
		m.header.Refresh()
		return m, nil

	case QuitMsg:
		return m, tea.Quit
	}

	// Update header
	var headerCmd tea.Cmd
	m.header, headerCmd = m.header.Update(msg)
	if headerCmd != nil {
		cmds = append(cmds, headerCmd)
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
		m.helpline.SetText(m.activeScreen.HelpText())
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m AppModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	styles := GetStyles()
	var b strings.Builder

	// Header with 1-char padding on left and right
	headerContent := m.header.View()
	headerStyle := lipgloss.NewStyle().
		Width(m.width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.Screen.GetBackground())
	b.WriteString(headerStyle.Render(headerContent))
	b.WriteString("\n")

	// Separator line with 1-char padding on left and right
	sep := strings.Repeat(styles.SepChar, m.width-2)
	sepStyle := lipgloss.NewStyle().
		Width(m.width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.HeaderBG.GetBackground())
	b.WriteString(sepStyle.Render(sep))
	b.WriteString("\n")

	// Content area
	contentHeight := m.height - 3
	var content string

	if m.dialog != nil {
		// Show dialog over screen
		content = m.dialog.View()
	} else if m.activeScreen != nil {
		content = m.activeScreen.View()
	}

	// Ensure content fills the height
	contentLines := strings.Count(content, "\n") + 1
	if contentLines < contentHeight {
		content += strings.Repeat("\n", contentHeight-contentLines)
	}

	// Apply screen background to content area
	contentStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(contentHeight).
		Background(styles.Screen.GetBackground())

	b.WriteString(contentStyle.Render(content))

	// Help line
	b.WriteString(m.helpline.View(m.width))

	return b.String()
}
