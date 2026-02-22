package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2" // used for zone.Get in Update()
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

	// Unified layout (deterministic sizing)
	layout DialogLayout
}

// newMessageDialog creates a new message dialog
func newMessageDialog(title, message string, msgType MessageType) *messageDialogModel {
	return &messageDialogModel{
		title:       title,
		message:     message,
		messageType: msgType,
	}
}

func (m *messageDialogModel) Init() tea.Cmd {
	return nil
}

func (m *messageDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		// Any key press closes the dialog
		// Use CloseDialogMsg so AppModel can handle it when running within existing TUI
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.MouseClickMsg:
		// Check if OK button was clicked (auto-generated zone ID: "Button.OK")
		if zoneInfo := zone.Get("Button.OK"); zoneInfo != nil {
			if zoneInfo.InBounds(msg) {
				return m, func() tea.Msg { return CloseDialogMsg{} }
			}
		}
	}

	return m, nil
}

// ViewString returns the dialog content as a string for compositing
func (m *messageDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()

	// Message text style based on type
	var messageStyle lipgloss.Style
	var titlePrefix string

	switch m.messageType {
	case MessageSuccess:
		messageStyle = styles.StatusSuccess.
			Padding(1, 2)
		titlePrefix = "✓ "

	case MessageWarning:
		messageStyle = styles.StatusWarn.
			Padding(1, 2)
		titlePrefix = "⚠ "

	case MessageError:
		messageStyle = styles.StatusError.
			Bold(true).
			Padding(1, 2)
		titlePrefix = "✗ "

	default: // MessageInfo
		messageStyle = lipgloss.NewStyle().
			Foreground(styles.ItemNormal.GetForeground()).
			Padding(1, 2)
		titlePrefix = "ℹ "
	}

	// Calculate content dimensions from layout
	contentWidth := m.layout.Width - 2

	// Wrap message text to fit width
	messageStyle = messageStyle.Width(contentWidth)
	content := messageStyle.Render(m.message)

	// Render OK button with automatic zone marking
	buttonRow := RenderCenteredButtons(
		contentWidth,
		ButtonSpec{Text: " OK ", Active: true},
	)

	// Combine message and button
	// Standardize to use TrimRight to prevent implicit gaps
	content = strings.TrimRight(content, "\n")
	buttonRow = strings.TrimRight(buttonRow, "\n")
	fullContent := lipgloss.JoinVertical(lipgloss.Left, content, buttonRow)

	// Add title with prefix and wrap in border
	fullTitle := titlePrefix + m.title
	dialogWithTitle := RenderDialog(fullTitle, fullContent, true, 0)

	// Add shadow
	dialog := AddShadow(dialogWithTitle)

	return dialog
}

func (m *messageDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// SetSize updates the dialog dimensions
func (m *messageDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *messageDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// 1. Shadow
	shadow := 0
	if currentConfig.UI.Shadow {
		shadow = DialogShadowHeight
	}

	// 2. Button
	buttons := DialogButtonHeight

	// 3. Overhead
	overhead := DialogBorderHeight + buttons + shadow

	m.layout = DialogLayout{
		Width:        m.width,
		Height:       m.height,
		ButtonHeight: buttons,
		ShadowHeight: shadow,
		Overhead:     overhead,
	}
}

// ShowMessageDialog displays a message dialog
func ShowMessageDialog(title, message string, msgType MessageType) {
	// If TUI is already running, show dialog within existing program
	if program != nil {
		program.Send(ShowMessageDialogMsg{
			Title:   title,
			Message: message,
			Type:    msgType,
		})
		return
	}

	// Otherwise, run standalone with backdrop
	helpText := "Press any key to continue"
	dialog := newMessageDialog(title, message, msgType)

	_, _ = RunDialogWithBackdrop(dialog, helpText, PositionCenter)
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
