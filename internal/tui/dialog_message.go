package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	overlay "github.com/rmhubbert/bubbletea-overlay"
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
		return ""
	}

	styles := GetStyles()

	// Message text style based on type
	var messageStyle lipgloss.Style
	var titlePrefix string

	switch m.messageType {
	case MessageSuccess:
		// Use green for success (approximating theme's TitleSuccess color)
		messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Padding(1, 2)
		titlePrefix = "✓ "

	case MessageWarning:
		// Use yellow for warning (approximating theme's TitleWarning color)
		messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Padding(1, 2)
		titlePrefix = "⚠ "

	case MessageError:
		// Use red for error (from TagKey which is red in theme)
		messageStyle = lipgloss.NewStyle().
			Foreground(styles.TagKey.GetForeground()).
			Bold(true).
			Padding(1, 2)
		titlePrefix = "✗ "

	default: // MessageInfo
		messageStyle = lipgloss.NewStyle().
			Foreground(styles.ItemNormal.GetForeground()).
			Padding(1, 2)
		titlePrefix = "ℹ "
	}

	// Build dialog content
	content := messageStyle.Render(m.message)

	// Add padding to content (border will be added by RenderDialogWithTitle)
	paddedContent := styles.Dialog.
		Padding(0, 1).
		Render(content)

	// Add title with prefix and wrap in border with title embedded (matching menu style)
	fullTitle := titlePrefix + m.title
	dialogWithTitle := RenderDialogWithTitle(fullTitle, paddedContent)

	// Add shadow (matching menu style)
	dialogWithTitle = AddShadow(dialogWithTitle)

	// Just return the dialog content - backdrop will be handled by overlay
	return dialogWithTitle
}

// messageWithBackdrop wraps a message dialog with backdrop using overlay
type messageWithBackdrop struct {
	backdrop BackdropModel
	dialog   messageDialogModel
}

func (m messageWithBackdrop) Init() tea.Cmd {
	return tea.Batch(m.backdrop.Init(), m.dialog.Init())
}

func (m messageWithBackdrop) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Update backdrop
	backdropModel, cmd := m.backdrop.Update(msg)
	m.backdrop = backdropModel.(BackdropModel)
	cmds = append(cmds, cmd)

	// Update dialog
	dialogModel, cmd := m.dialog.Update(msg)
	m.dialog = dialogModel.(messageDialogModel)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m messageWithBackdrop) View() string {
	// Use overlay to composite dialog over backdrop
	// overlay.Composite(foreground, background, xPos, yPos, xOffset, yOffset)
	return overlay.Composite(
		m.dialog.View(),    // foreground (dialog content)
		m.backdrop.View(),  // background (backdrop base)
		overlay.Center,
		overlay.Center,
		0,
		0,
	)
}

// ShowMessageDialog displays a message dialog
func ShowMessageDialog(title, message string, msgType MessageType) {
	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.Theme); err == nil {
		InitStyles(cfg)
	}

	helpText := "Press any key to continue"
	model := messageWithBackdrop{
		backdrop: NewBackdropModel(helpText),
		dialog:   newMessageDialog(title, message, msgType),
	}

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
