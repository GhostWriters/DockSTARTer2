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
	if m.helpline != nil {
		m.helpline.SetText(text)
	}
}

// Init implements tea.Model
func (m *BackdropModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *BackdropModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.header != nil {
		updated, cmd := m.header.Update(msg)
		m.header = updated.(*HeaderModel)
		return m, cmd
	}
	return m, nil
}

// SetSize updates the backdrop dimensions
func (m *BackdropModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Header width reduced by 2 for padding left/right
	if m.header != nil {
		m.header.SetWidth(width - 2)
	}
	if m.helpline != nil {
		m.helpline.SetText(m.helpText)
	}
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
	headerContent := ""
	headerHeight := 0
	if m.header != nil {
		m.header.SetWidth(m.width - 2)
		headerContent = m.header.ViewString()
		headerHeight = lipgloss.Height(headerContent)
	}

	// Border style: use the full StatusBarBorder style (fg + bg) for border cells.
	// Focused (any version is selected) → ThickRounded; otherwise → Rounded.
	// No top border — the status bar is flush with the top of the terminal.
	focused := m.header != nil && m.header.GetFocus() != HeaderFocusNone
	lineChars := styles.LineCharacters

	borderFG := styles.StatusBarBorder.GetForeground()
	if borderFG == nil {
		borderFG = styles.StatusBar.GetForeground()
	}
	borderBG := styles.StatusBarBorder.GetBackground()
	if borderBG == nil {
		borderBG = styles.StatusBar.GetBackground()
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderFG).Background(borderBG)

	var leftChar, rightChar, bottomChar, bottomLeftChar, bottomRightChar string
	if lineChars {
		bottomLeftChar = "╰"
		bottomRightChar = "╯"
		if focused {
			leftChar = "┃"
			rightChar = "┃"
			bottomChar = "━"
		} else {
			leftChar = "│"
			rightChar = "│"
			bottomChar = "─"
		}
	} else {
		bottomLeftChar = "'"
		bottomRightChar = "'"
		if focused {
			leftChar = "H"
			rightChar = "H"
			bottomChar = "="
		} else {
			leftChar = "|"
			rightChar = "|"
			bottomChar = "-"
		}
	}

	// Render each header line with left/right border chars.
	// Header content is m.width-2 wide; border chars occupy the remaining 2 columns.
	headerLines := strings.Split(headerContent, "\n")
	for _, line := range headerLines {
		// Pad to fill the content width (m.width - 2) so the background extends fully.
		// Use WidthWithoutZones because header lines contain zone markers for version clicks.
		paddedLine := line
		if lw := WidthWithoutZones(paddedLine); lw < m.width-2 {
			paddedLine += strutil.Repeat(" ", m.width-2-lw)
		}
		styledContent := styles.StatusBar.Render(paddedLine)
		b.WriteString(borderStyle.Render(leftChar) + styledContent + borderStyle.Render(rightChar) + "\n")
	}

	// Bottom border (replaces the old separator line — same 1-line height).
	bottomBorder := borderStyle.Render(bottomLeftChar + strutil.Repeat(bottomChar, m.width-2) + bottomRightChar)
	b.WriteString(bottomBorder + "\n")

	// Calculate content height using actual header height
	helplineView := ""
	helplineHeight := 0
	if m.helpline != nil {
		m.helpline.SetText(m.helpText)
		helplineView = m.helpline.ViewString(m.width)
		helplineHeight = lipgloss.Height(helplineView)
	}

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

// Layers returns the backdrop layer for visual compositing
func (m *BackdropModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZBackdrop),
	}
}

// GetHitRegions returns clickable regions for the backdrop (header version labels)
func (m *BackdropModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	// Full status bar background region (lower Z so specific version regions take priority).
	// Covers the header content lines + bottom border line.
	headerH := 1
	if m.header != nil {
		m.header.SetWidth(m.width - 2)
		headerH = m.header.Height()
	}
	regions = append(regions, HitRegion{
		ID:     IDStatusBar,
		X:      offsetX,
		Y:      offsetY,
		Width:  m.width,
		Height: headerH + 1, // content lines + bottom border
		ZOrder: ZBackdrop,
	})

	if m.header != nil {
		// Version click targets are 1 char in from the left border char.
		regions = append(regions, m.header.GetHitRegions(offsetX+1, offsetY)...)
	}

	return regions
}

// View implements tea.Model
// Matches AppModel.View() rendering approach for consistent spacing
func (m *BackdropModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// ChromeHeight returns the total rendered height of the status bar chrome:
// header content lines + bottom border line.
// Use this instead of hardcoding constants when computing content Y offsets.
func (m *BackdropModel) ChromeHeight() int {
	if m.header == nil {
		return 1 // just the bottom border line
	}
	m.header.SetWidth(m.width - 2) // ensure width is current before measuring
	return m.header.Height() + 1   // header content + bottom border
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
	headerH := 1
	if m.header != nil {
		headerH = m.header.Height()
	}

	return layout.ContentArea(m.width, m.height, hasShadow, headerH)
}
