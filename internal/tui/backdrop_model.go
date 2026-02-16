package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// BackdropModel represents the shared background for all dialogs
// It renders the header, fills the middle space, and shows the helpline
type BackdropModel struct {
	width    int
	height   int
	helpText string
	header   HeaderModel
	helpline HelplineModel
}

// NewBackdropModel creates a new backdrop model
func NewBackdropModel(helpText string) BackdropModel {
	return BackdropModel{
		helpText: helpText,
		header:   NewHeaderModel(),
		helpline: NewHelplineModel(),
	}
}

// SetHelpText updates the help text displayed in the helpline
func (m *BackdropModel) SetHelpText(text string) {
	m.helpText = text
	m.helpline.SetText(text)
}

// Init implements tea.Model
func (m BackdropModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m BackdropModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Header width reduced by 2 for padding left/right
		m.header.SetWidth(msg.Width - 2)
		m.helpline.SetText(m.helpText)
	}
	return m, nil
}

// ViewString returns the backdrop content as a string for compositing
func (m BackdropModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	styles := GetStyles()
	var b strings.Builder

	// Header with 1-char padding on left and right (matches AppModel.View())
	// Header width reduced by 2 for padding
	m.header.SetWidth(m.width - 2)
	headerContent := m.header.View()
	headerStyle := lipgloss.NewStyle().
		Width(m.width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.Screen.GetBackground())
	b.WriteString(headerStyle.Render(headerContent))
	b.WriteString("\n")

	// Separator line with 1-char padding on left and right (matches AppModel.View())
	sep := strings.Repeat(styles.SepChar, m.width-2)
	sepStyle := lipgloss.NewStyle().
		Width(m.width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.HeaderBG.GetBackground())
	b.WriteString(sepStyle.Render(sep))
	b.WriteString("\n")

	// Calculate content height (matches AppModel.View())
	m.helpline.SetText(m.helpText)
	helplineView := m.helpline.View(m.width)
	helplineHeight := lipgloss.Height(helplineView)

	contentHeight := m.height - 2 - helplineHeight // -2 for header and separator lines

	if contentHeight < 0 {
		contentHeight = 0
	}

	// Fill middle space with screen background (matches AppModel.View())
	middleSpace := lipgloss.NewStyle().
		Width(m.width).
		Height(contentHeight).
		Background(styles.Screen.GetBackground()).
		Render("")

	b.WriteString(middleSpace)

	// Helpline (matches AppModel.View())
	b.WriteString("\n")
	b.WriteString(helplineView)

	return b.String()
}

// View implements tea.Model
// Matches AppModel.View() rendering approach for consistent spacing
func (m BackdropModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// GetContentArea returns the dimensions available for overlay content
// This is the space between the header/separator and the helpline, accounting for shadow
func (m BackdropModel) GetContentArea() (width, height int) {
	if m.width == 0 || m.height == 0 {
		return 0, 0
	}

	// Calculate header height (1 line with padding)
	headerHeight := 1

	// Calculate separator height (1 line)
	separatorHeight := 1

	// Calculate helpline height (1 line)
	helplineHeight := 1

	// Account for shadow if enabled (2 chars wide on right, 1 line on bottom)
	shadowWidth := 0
	shadowHeight := 0
	if currentConfig.UI.Shadow {
		shadowWidth = 2
		shadowHeight = 1
	}

	// Available content area (accounting for shadow)
	contentWidth := m.width - shadowWidth
	contentHeight := m.height - headerHeight - separatorHeight - helplineHeight - shadowHeight
	if contentHeight < 5 {
		contentHeight = 5
	}

	return contentWidth, contentHeight
}
