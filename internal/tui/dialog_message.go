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
	baseDialogModel
	title       string
	message     string
	messageType MessageType
	onResult    func() tea.Msg
}

// newMessageDialog creates a new message dialog
func newMessageDialog(title, message string, msgType MessageType) *messageDialogModel {
	return &messageDialogModel{
		baseDialogModel: baseDialogModel{id: "message_dialog", focused: true},
		title:           title,
		message:         message,
		messageType:     msgType,
	}
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
		return m, func() tea.Msg { return CloseDialogMsg{Result: true} }

	case LayerHitMsg:
		// Middle click is handled by AppModel (global Space mapping)
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}
		// Left click on OK button closes
		// Check for suffixes to support prefixed IDs (e.g., "message_dialog.OK")
		if msg.Button == tea.MouseLeft {
			if buttonIDMatches(msg.ID, "OK") {
				return m, func() tea.Msg { return CloseDialogMsg{Result: true} }
			}
		}
	}

	// Middle-click dismisses the dialog
	if _, ok := msg.(ToggleFocusedMsg); ok {
		return m, func() tea.Msg { return CloseDialogMsg{Result: true} }
	}

	return m, nil
}

// titlePrefix returns the icon prefix for this message type.
func (m *messageDialogModel) titlePrefix() string {
	switch m.messageType {
	case MessageSuccess:
		return "✓ "
	case MessageWarning:
		return "⚠ "
	case MessageError:
		return "✗ "
	default:
		return "ℹ "
	}
}

// messageStyle returns the base text style for this message type (without width).
func (m *messageDialogModel) messageStyle() lipgloss.Style {
	styles := GetStyles()
	switch m.messageType {
	case MessageSuccess:
		return styles.StatusSuccess.Padding(1, 2)
	case MessageWarning:
		return styles.StatusWarn.Padding(1, 2)
	case MessageError:
		return styles.StatusError.Bold(true).Padding(1, 2)
	default:
		return lipgloss.NewStyle().Foreground(styles.ItemNormal.GetForeground()).Padding(1, 2)
	}
}

// contentWidth calculates the ideal dialog inner width.
func (m *messageDialogModel) contentWidth() int {
	maxAllowed := m.layout.Width - 2
	w := maxLineWidth(m.message) + DialogBodyPadH
	if minBtn := lipgloss.Width(" OK ") + 4; minBtn > w {
		w = minBtn
	}
	fullTitle := m.titlePrefix() + GetPlainText(m.title)
	if tw := lipgloss.Width(fullTitle) + 6; tw > w {
		w = tw
	}
	if w > maxAllowed {
		w = maxAllowed
	}
	return w
}

// ViewString returns the dialog content as a string for compositing
func (m *messageDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	contentWidth := m.contentWidth()
	fullTitle := m.titlePrefix() + GetPlainText(m.title)

	// Wrap message text to fit width
	messageStyle := m.messageStyle().Width(contentWidth)
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
	dialogWithTitle := RenderDialog(fullTitle, fullContent, m.focused, 0)

	return dialogWithTitle
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
	contentWidth := m.contentWidth()
	messageHeight := lipgloss.Height(m.messageStyle().Width(contentWidth).Render(m.message))

	// buttonY: border (1) + message with padding
	buttonY := 1 + messageHeight

	// Use centralized button hit region helper with dialog ID for disambiguation
	// Must include Text to properly calculate button width
	return GetButtonHitRegions(
		m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20,
		ButtonSpec{Text: "OK", ZoneID: "OK"},
	)
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
