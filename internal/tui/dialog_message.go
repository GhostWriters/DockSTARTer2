package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MessageType represents the type of message dialog
type MessageType int

const (
	MessageInfo MessageType = iota
	MessageSuccess
	MessageWarning
	MessageError
)

// messageDialogModel represents a message dialog
type messageDialogModel struct {
	title       string
	message     string
	messageType MessageType
	width       int
	height      int
}

// newMessageDialog creates a new message dialog
func newMessageDialog(title, message string, msgType MessageType) messageDialogModel {
	return messageDialogModel{
		title:       title,
		message:     message,
		messageType: msgType,
	}
}

func (m messageDialogModel) Init() tea.Cmd {
	return nil
}

func (m messageDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Any key press closes the dialog
		return m, tea.Quit
	}

	return m, nil
}

func (m messageDialogModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	styles := GetStyles()

	// Message text style based on type
	var messageStyle lipgloss.Style
	var titlePrefix string

	switch m.messageType {
	case MessageSuccess:
		messageStyle = lipgloss.NewStyle().
			Foreground(styles.ItemSelected).
			Padding(1, 2)
		titlePrefix = "✓ "

	case MessageWarning:
		messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("yellow")).
			Padding(1, 2)
		titlePrefix = "⚠ "

	case MessageError:
		messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("red")).
			Bold(true).
			Padding(1, 2)
		titlePrefix = "✗ "

	default: // MessageInfo
		messageStyle = lipgloss.NewStyle().
			Foreground(styles.ItemNormal).
			Padding(1, 2)
		titlePrefix = "ℹ "
	}

	// Build dialog content
	content := messageStyle.Render(m.message) + "\n\n" +
		lipgloss.NewStyle().
			Foreground(styles.ItemHelp).
			Italic(true).
			Align(lipgloss.Center).
			Render("Press any key to continue")

	// Wrap in dialog box
	dialogStyle := styles.Dialog.
		Padding(0, 1)
	dialogStyle = ApplyStraightBorder(dialogStyle, styles.LineCharacters)

	dialog := dialogStyle.Render(content)

	// Add title with prefix
	fullTitle := titlePrefix + m.title
	dialogWithTitle := renderBorderWithTitleStatic(fullTitle, dialog)

	// Center on screen
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogWithTitle,
	)
}

// ShowMessageDialog displays a message dialog
func ShowMessageDialog(title, message string, msgType MessageType) {
	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.Theme); err == nil {
		InitStyles(cfg)
	}

	model := newMessageDialog(title, message, msgType)
	p := tea.NewProgram(model)

	p.Run()
}

// ShowInfoDialog displays an info message dialog
func ShowInfoDialog(title, message string) {
	ShowMessageDialog(title, message, MessageInfo)
}

// ShowSuccessDialog displays a success message dialog
func ShowSuccessDialog(title, message string) {
	ShowMessageDialog(title, message, MessageSuccess)
}

// ShowWarningDialog displays a warning message dialog
func ShowWarningDialog(title, message string) {
	ShowMessageDialog(title, message, MessageWarning)
}

// ShowErrorDialog displays an error message dialog
func ShowErrorDialog(title, message string) {
	ShowMessageDialog(title, message, MessageError)
}
