package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	onResult    func() tea.Msg // Optional: Custom message generator for result

	// Unified layout (deterministic sizing)
	layout DialogLayout
	id     string
}

// newMessageDialog creates a new message dialog
func newMessageDialog(title, message string, msgType MessageType) *messageDialogModel {
	return &messageDialogModel{
		id:          "message_dialog",
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
		m.calculateLayout()
		return m, nil

	case tea.KeyPressMsg:
		// Any key press closes the dialog
		// Use CloseDialogMsg so AppModel can handle it when running within existing TUI
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case LayerHitMsg:
		// Middle click is handled by AppModel (global Space mapping)
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}
		// Left click on OK button closes
		// Check for suffixes to support prefixed IDs (e.g., "message_dialog.OK")
		if msg.Button == tea.MouseLeft {
			if strings.HasSuffix(msg.ID, ".OK") || msg.ID == "Button.OK" {
				return m, func() tea.Msg { return CloseDialogMsg{} }
			}
		}
	}

	// Middle-click dismisses the dialog
	if _, ok := msg.(ToggleFocusedMsg); ok {
		return m, func() tea.Msg { return CloseDialogMsg{} }
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

// View implements tea.Model
func (m *messageDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// Layers returns a single layer with the dialog content for visual compositing
func (m *messageDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZDialog).ID(m.id),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *messageDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	styles := GetStyles()

	// Calculate message height to find button position
	var messageStyle lipgloss.Style
	switch m.messageType {
	case MessageSuccess:
		messageStyle = styles.StatusSuccess.Padding(1, 2)
	case MessageWarning:
		messageStyle = styles.StatusWarn.Padding(1, 2)
	case MessageError:
		messageStyle = styles.StatusError.Bold(true).Padding(1, 2)
	default:
		messageStyle = lipgloss.NewStyle().Foreground(styles.ItemNormal.GetForeground()).Padding(1, 2)
	}

	contentWidth := m.layout.Width - 2
	messageStyle = messageStyle.Width(contentWidth)
	messageHeight := lipgloss.Height(messageStyle.Render(m.message))

	// buttonY: border (1) + message with padding
	buttonY := 1 + messageHeight

	// Use centralized button hit region helper with dialog ID for disambiguation
	// Must include Text to properly calculate button width
	return GetButtonHitRegions(
		m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20,
		ButtonSpec{Text: "OK", ZoneID: "OK"},
	)
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

	header := NewHeaderModel()
	header.SetWidth(80)
	headerH := header.Height()

	_, _ = RunDialogWithBackdrop(dialog, helpText, GetPositionCenter(headerH))
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
