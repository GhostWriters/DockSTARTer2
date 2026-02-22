package tui

import (
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
	"context"
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
		m.SetSize(msg.Width, msg.Height)
	}
	return m, nil
}

// SetSize updates the backdrop dimensions
func (m *BackdropModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Header width reduced by 2 for padding left/right
	m.header.SetWidth(width - 2)
	m.helpline.SetText(m.helpText)
}

// ViewString returns the backdrop content as a string for compositing
func (m BackdropModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	styles := GetStyles()
	var b strings.Builder

	logger.Info(context.Background(), "Backdrop: Rendering Header")

	// Header with 1-char padding on left and right (matches AppModel.View())
	// Header width reduced by 2 for padding
	m.header.SetWidth(m.width - 2)
	headerContent := m.header.View()
	headerLine := " " + headerContent + " "
	if padLen := m.width - lipgloss.Width(headerLine); padLen > 0 {
		headerLine += strutil.Repeat(" ", padLen)
	}
	headerStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	b.WriteString(headerStyle.Render(headerLine))
	b.WriteString("\n")

	logger.Info(context.Background(), "Backdrop: Rendering Separator")

	// Separator line with 1-char padding on left and right (matches AppModel.View())
	sep := strutil.Repeat(styles.SepChar, m.width-2)
	sepLine := " " + sep + " "
	if padLen := m.width - lipgloss.Width(sepLine); padLen > 0 {
		sepLine += strutil.Repeat(" ", padLen)
	}
	sepStyle := lipgloss.NewStyle().Background(styles.HeaderBG.GetBackground())
	b.WriteString(sepStyle.Render(sepLine))
	b.WriteString("\n")

	logger.Info(context.Background(), "Backdrop: Rendering Helpline")

	// Calculate content height (matches AppModel.View())
	m.helpline.SetText(m.helpText)
	helplineView := m.helpline.View(m.width)
	helplineHeight := lipgloss.Height(helplineView)

	contentHeight := m.height - 2 - helplineHeight // -2 for header and separator lines
	if contentHeight < 0 {
		contentHeight = 0
	}

	logger.Info(context.Background(), "Backdrop: Rendering Filler Rows (height: %d)", contentHeight)

	// Fill middle space with screen background
	// We build it row by row to ensure each line is exactly m.width wide with background color
	bgStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	fillerRow := bgStyle.Render(strutil.Repeat(" ", m.width))

	for i := 0; i < contentHeight; i++ {
		b.WriteString(fillerRow)
		b.WriteString("\n")
	}

	// Helpline (matches AppModel.View())
	b.WriteString(helplineView)

	logger.Debug(context.Background(), "Backdrop: ViewString complete")

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

	// Account for shadow if enabled (2 chars wide on right, 1 line on bottom)
	shadowWidth := 0
	if currentConfig.UI.Shadow {
		shadowWidth = 2
	}

	// Available content area (accounting for shadow and margins)
	// Remaining space for dialog: margin (2 per side) = 4
	contentWidth := m.width - 4 - shadowWidth

	// Remaining space for dialog:
	// - Header/Sep: 2
	// - Gap after Sep: 1
	// - Space before helpline: 1
	// - Helpline: 1
	// Total overhead: 5 lines for a single space between box and helpline if shadow is off.
	// If shadow is on, it takes 1 extra line, so we subtract 1 more.
	overhead := 4
	if currentConfig.UI.Shadow {
		overhead = 5
	}
	contentHeight := m.height - overhead

	if contentHeight < 5 {
		contentHeight = 5
	}

	return contentWidth, contentHeight
}
