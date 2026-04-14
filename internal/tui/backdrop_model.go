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
	width          int
	height         int
	helpText       string
	header         *HeaderModel
	helpline       *HelplineModel
	cacheValid     bool
	cachedHeader   string
	cachedHelpline string
	cachedBg       string
	helplineHeight int // cached measured height of helpline; 0 = not yet measured
}

// NewBackdropModel creates a new backdrop model
func NewBackdropModel(helpText string) *BackdropModel {
	return &BackdropModel{
		helpText: helpText,
		header:   NewHeaderModel(),
		helpline: NewHelplineModel(),
	}
}

func (m *BackdropModel) invalidateCache() {
	m.cacheValid = false
	m.cachedHeader = ""
	m.cachedHelpline = ""
	m.cachedBg = ""
	m.helplineHeight = 0
}

// SetHelpText updates the help text displayed in the helpline.
// No-ops when the text hasn't changed to avoid dirtying the cache unnecessarily.
func (m *BackdropModel) SetHelpText(text string) {
	if text == m.helpText {
		return
	}
	m.helpText = text
	m.invalidateCache()
	if m.helpline != nil {
		m.helpline.SetText(text)
	}
}

// InvalidateBackdropCache forces a full re-render on the next Layers() call.
// Call this after a theme change or any other global state change.
func (m *BackdropModel) InvalidateBackdropCache() {
	m.invalidateCache()
}

// Init implements tea.Model
func (m *BackdropModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *BackdropModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.header != nil {
		prevFocus := m.header.GetFocus()
		updated, cmd := m.header.Update(msg)
		m.header = updated.(*HeaderModel)
		if m.header.GetFocus() != prevFocus {
			m.invalidateCache()
		}
		return m, cmd
	}
	return m, nil
}

// SetSize updates the backdrop dimensions
func (m *BackdropModel) SetSize(width, height int) {
	if width == m.width && height == m.height {
		return
	}
	m.width = width
	m.height = height
	m.invalidateCache()
	if m.header != nil {
		m.header.SetWidth(width - 2)
	}
	if m.helpline != nil {
		m.helpline.SetText(m.helpText)
	}
}

// ViewString returns the solid background box to fill the entire screen
func (m *BackdropModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.cacheValid && m.cachedBg != "" {
		return m.cachedBg
	}
	styles := GetStyles()
	bgStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	m.cachedBg = bgStyle.Width(m.width).Height(m.height).Render("")
	return m.cachedBg
}

// renderHeader returns the status bar header with its borders (cached)
func (m *BackdropModel) renderHeader() string {
	if m.header == nil {
		return ""
	}
	if m.cacheValid && m.cachedHeader != "" {
		return m.cachedHeader
	}

	styles := GetStyles()
	var b strings.Builder

	m.header.SetWidth(m.width - 2)
	headerContent := m.header.ViewString()

	lineChars := styles.LineCharacters
	focused := m.header.GetFocus() != HeaderFocusNone

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

	headerLines := strings.Split(headerContent, "\n")
	for _, line := range headerLines {
		paddedLine := line
		if lw := WidthWithoutZones(paddedLine); lw < m.width-2 {
			paddedLine += strutil.Repeat(" ", m.width-2-lw)
		}
		styledContent := styles.StatusBar.Render(paddedLine)
		b.WriteString(borderStyle.Render(leftChar) + styledContent + borderStyle.Render(rightChar) + "\n")
	}

	bottomBorder := borderStyle.Render(bottomLeftChar + strutil.Repeat(bottomChar, m.width-2) + bottomRightChar)
	b.WriteString(bottomBorder)

	m.cachedHeader = b.String()
	return m.cachedHeader
}

// renderHelpline returns the help text line positioned at the bottom (cached)
func (m *BackdropModel) renderHelpline() string {
	if m.helpline == nil {
		return ""
	}
	if m.cacheValid && m.cachedHelpline != "" {
		return m.cachedHelpline
	}
	m.helpline.SetText(m.helpText)
	m.cachedHelpline = m.helpline.ViewString(m.width)
	m.helplineHeight = lipgloss.Height(m.cachedHelpline)
	return m.cachedHelpline
}

// Layers returns the backdrop layers for visual compositing:
// 1. ZBackdrop: Solid background plane
// 2. ZHeader: The status bar at the top
// 3. ZHelpline: The help line at the bottom
func (m *BackdropModel) Layers() []*lipgloss.Layer {
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZBackdrop),
	}

	if headerStr := m.renderHeader(); headerStr != "" {
		layers = append(layers, lipgloss.NewLayer(headerStr).X(0).Y(0).Z(ZHeader).ID(IDStatusBar))
	}

	if helpStr := m.renderHelpline(); helpStr != "" {
		helpY := m.height - m.helplineHeight
		layers = append(layers, lipgloss.NewLayer(helpStr).X(0).Y(helpY).Z(ZHelpline))
	}

	// Mark all three sub-renders valid now that Layers() has run through them.
	m.cacheValid = true

	return layers
}

// GetHitRegions returns clickable regions for the backdrop (header version labels)
func (m *BackdropModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	// Full status bar background region (lower Z so specific version regions take priority).
	// Covers the header content lines + bottom border line.
	layout := GetLayout()
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
		Height: layout.ChromeHeight(headerH),
		ZOrder: ZHeader, // Match header layer
		Label:  "Status Bar",
	})

	if m.header != nil {
		// Version click targets are 1 char in from the left border char.
		regions = append(regions, m.header.GetHitRegions(offsetX+1, offsetY)...)
	}

	// Helpline
	helplineH := m.HelplineActualHeight()
	regions = append(regions, HitRegion{
		ID:     IDHelpline,
		X:      offsetX,
		Y:      offsetY + m.height - helplineH,
		Width:  m.width,
		Height: helplineH,
		ZOrder: ZHelpline,
		Label:  "Help Line",
		Help: &HelpContext{
			ScreenName: "Help Line",
			PageTitle:  "Navigation Info",
			PageText:   "Displays keyboard shortcuts and hints relevant to the active screen.",
			ItemText:   "Right-click for global options or press F1 for detailed help.",
		},
	})

	return regions
}

// View implements tea.Model
// Matches AppModel.View() rendering approach for consistent spacing
func (m *BackdropModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// ChromeHeight returns the total rendered height of the status bar chrome:
// header content lines + bottom border line.
func (m *BackdropModel) ChromeHeight() int {
	layout := GetLayout()
	headerH := 0
	if m.header != nil {
		m.header.SetWidth(m.width - 2)
		headerH = m.header.Height()
	}
	return layout.ChromeHeight(headerH)
}

// HelplineActualHeight returns the actual rendered height of the helpline.
// Uses a cached value when available to avoid re-rendering just for measurement.
func (m *BackdropModel) HelplineActualHeight() int {
	if m.helplineHeight > 0 {
		return m.helplineHeight
	}
	if m.helpline == nil || m.width == 0 {
		return GetLayout().HelplineHeight
	}
	// Render and cache
	m.helpline.SetText(m.helpText)
	m.cachedHelpline = m.helpline.ViewString(m.width)
	if h := lipgloss.Height(m.cachedHelpline); h > 0 {
		m.helplineHeight = h
		return h
	}
	return GetLayout().HelplineHeight
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

	return layout.ContentArea(m.width, m.height, hasShadow, false, headerH, m.HelplineActualHeight())
}
