package dialogs

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// MessageDialog shows a message with an OK button
type MessageDialog struct {
	title      string
	message    string
	dialogType tui.DialogType
	width      int
	height     int
}

// NewMessageDialog creates a new message dialog
func NewMessageDialog(title, message string, dialogType tui.DialogType) *MessageDialog {
	return &MessageDialog{
		title:      title,
		message:    message,
		dialogType: dialogType,
	}
}

// NewInfoDialog creates an info message dialog
func NewInfoDialog(title, message string) *MessageDialog {
	return NewMessageDialog(title, message, tui.DialogTypeInfo)
}

// NewSuccessDialog creates a success message dialog
func NewSuccessDialog(title, message string) *MessageDialog {
	return NewMessageDialog(title, message, tui.DialogTypeSuccess)
}

// NewWarningDialog creates a warning message dialog
func NewWarningDialog(title, message string) *MessageDialog {
	return NewMessageDialog(title, message, tui.DialogTypeWarning)
}

// NewErrorDialog creates an error message dialog
func NewErrorDialog(title, message string) *MessageDialog {
	return NewMessageDialog(title, message, tui.DialogTypeError)
}

// Init implements tea.Model
func (d *MessageDialog) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (d *MessageDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc", " ":
			return d, func() tea.Msg {
				return tui.CloseDialogMsg{}
			}
		}
	}
	return d, nil
}

// View implements tea.Model
func (d *MessageDialog) View() tea.View {
	// Calculate content width
	contentWidth := len(d.message)
	if len(d.title) > contentWidth {
		contentWidth = len(d.title)
	}
	contentWidth += 4 // Padding

	// Build content
	content := d.message + "\n\n" + tui.RenderButton("OK", true)

	return tui.RenderDialogBox(
		d.title,
		content,
		d.dialogType,
		contentWidth,
		5,
		d.width,
		d.height,
	)
}

// SetSize sets the dialog container dimensions
func (d *MessageDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// ConfirmDialog shows a Yes/No confirmation dialog
type ConfirmDialog struct {
	title      string
	question   string
	defaultYes bool
	focusYes   bool
	width      int
	height     int
	result     chan bool
}

// NewConfirmDialog creates a new confirmation dialog
func NewConfirmDialog(title, question string, defaultYes bool, result chan bool) *ConfirmDialog {
	return &ConfirmDialog{
		title:      title,
		question:   question,
		defaultYes: defaultYes,
		focusYes:   defaultYes,
		result:     result,
	}
}

// Init implements tea.Model
func (d *ConfirmDialog) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (d *ConfirmDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "left", "right":
			d.focusYes = !d.focusYes
		case "enter":
			if d.result != nil {
				d.result <- d.focusYes
			}
			return d, func() tea.Msg {
				return tui.CloseDialogMsg{Result: d.focusYes}
			}
		case "esc":
			if d.result != nil {
				d.result <- false
			}
			return d, func() tea.Msg {
				return tui.CloseDialogMsg{Result: false}
			}
		case "y", "Y":
			if d.result != nil {
				d.result <- true
			}
			return d, func() tea.Msg {
				return tui.CloseDialogMsg{Result: true}
			}
		case "n", "N":
			if d.result != nil {
				d.result <- false
			}
			return d, func() tea.Msg {
				return tui.CloseDialogMsg{Result: false}
			}
		}
	}
	return d, nil
}

// View implements tea.Model
func (d *ConfirmDialog) View() tea.View {
	// Calculate content width
	contentWidth := len(d.question)
	if len(d.title) > contentWidth {
		contentWidth = len(d.title)
	}
	contentWidth += 4 // Padding

	// Build buttons with dialog background spacing
	styles := tui.GetStyles()
	yesBtn := tui.RenderButton("Yes", d.focusYes)
	noBtn := tui.RenderButton("No", !d.focusYes)

	// Calculate button widths and create sections with dialog background
	yesWidth := lipgloss.Width(yesBtn)
	noWidth := lipgloss.Width(noBtn)
	yesSection := styles.Dialog.Width(yesWidth + 4).Align(lipgloss.Center).Render(yesBtn)
	noSection := styles.Dialog.Width(noWidth + 4).Align(lipgloss.Center).Render(noBtn)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesSection, noSection)

	// Build content
	content := d.question + "\n\n" + buttons

	return tui.RenderDialogBox(
		d.title,
		content,
		tui.DialogTypeConfirm,
		contentWidth,
		5,
		d.width,
		d.height,
	)
}

// SetSize sets the dialog container dimensions
func (d *ConfirmDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}
