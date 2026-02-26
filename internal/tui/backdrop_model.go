package tui

import (
	"DockSTARTer2/internal/strutil"
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
	header   *HeaderModel
	helpline *HelplineModel
}

// NewBackdropModel creates a new backdrop model
func NewBackdropModel(helpText string) *BackdropModel {
	return &BackdropModel{
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
func (m *BackdropModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *BackdropModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
func (m *BackdropModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	styles := GetStyles()
	var b strings.Builder

	// Header: Fill with StatusBar background, then draw content
	// Header handles its own wrapping and alignment for narrow terminals
	m.header.SetWidth(m.width - 2)
	headerContent := m.header.ViewString()
	headerHeight := lipgloss.Height(headerContent)

	// Render each header line - header already handles alignment,
	// we just add padding and background without forcing width (which would re-align)
	headerLines := strings.Split(headerContent, "\n")
	for _, line := range headerLines {
		// Pad line to fill width, then apply background
		// Use WidthWithoutZones because header contains zone markers for version clicks
		paddedLine := " " + line + " "
		if lineWidth := WidthWithoutZones(paddedLine); lineWidth < m.width {
			paddedLine += strutil.Repeat(" ", m.width-lineWidth)
		}
		b.WriteString(styles.StatusBar.Render(paddedLine))
		b.WriteString("\n")
	}

	// Separator: Fill with StatusBarSeparator background, then draw separator chars
	sep := strutil.Repeat(styles.SepChar, m.width-2)
	sepStyle := styles.StatusBarSeparator.
		Width(m.width).
		Padding(0, 1) // 1-char padding left/right
	b.WriteString(sepStyle.Render(sep))
	b.WriteString("\n")

	// Calculate content height using actual header height
	m.helpline.SetText(m.helpText)
	helplineView := m.helpline.ViewString(m.width)
	helplineHeight := lipgloss.Height(helplineView)

	// Content area = total height - header lines - separator (1) - helpline
	contentHeight := m.height - headerHeight - 1 - helplineHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

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

	return b.String()
}

// View implements tea.Model
// Matches AppModel.View() rendering approach for consistent spacing
func (m *BackdropModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// GetContentArea returns the dimensions available for overlay content
// This is the space between the header/separator and the helpline, accounting for shadow
func (m *BackdropModel) GetContentArea() (width, height int) {
	if m.width == 0 || m.height == 0 {
		return 0, 0
	}

	// Use Layout helpers for consistent calculations
	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow
	headerH := m.header.Height()

	return layout.ContentArea(m.width, m.height, hasShadow, headerH)
}
