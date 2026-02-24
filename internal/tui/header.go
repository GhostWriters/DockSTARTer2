package tui

import (
	"os"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// HeaderFocus states for interactive elements
type HeaderFocus int

const (
	HeaderFocusNone HeaderFocus = iota
	HeaderFocusApp
	HeaderFocusTmpl
)

// Zone IDs
const (
	ZoneAppVersion  = "header_app_ver"
	ZoneTmplVersion = "header_tmpl_ver"
)

// HeaderModel represents the header bar at the top of the TUI
type HeaderModel struct {
	width int

	// Cached values
	hostname string
	flags    []string
	focus    HeaderFocus
}

// NewHeaderModel creates a new header model
func NewHeaderModel() HeaderModel {
	hostname, _ := os.Hostname()

	var flags []string
	if console.Verbose() {
		flags = append(flags, "VERBOSE")
	}
	if console.Debug() {
		flags = append(flags, "DEBUG")
	}
	if console.Force() {
		flags = append(flags, "FORCE")
	}
	if console.AssumeYes() {
		flags = append(flags, "YES")
	}

	return HeaderModel{
		hostname: hostname,
		flags:    flags,
		focus:    HeaderFocusNone,
	}
}

// Init implements tea.Model
func (m HeaderModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m HeaderModel) Update(msg tea.Msg) (HeaderModel, tea.Cmd) {
	return m, nil
}

// SetWidth sets the header width
func (m *HeaderModel) SetWidth(width int) {
	m.width = width
}

// Refresh updates the header (called when update status changes)
func (m *HeaderModel) Refresh() {
	// Nothing to cache currently, but could be used for update status
}

// SetFocus sets the focus state of the header
func (m *HeaderModel) SetFocus(f HeaderFocus) {
	m.focus = f
}

// GetFocus returns the current focus state
func (m *HeaderModel) GetFocus() HeaderFocus {
	return m.focus
}

// HandleMouse handles mouse events for the header
// Returns true if the event was handled (and potentially a command)
func (m *HeaderModel) HandleMouse(msg tea.MouseClickMsg) (bool, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return false, nil
	}

	if zi := zone.Get(ZoneAppVersion); zi != nil && zi.InBounds(msg) {
		m.SetFocus(HeaderFocusApp)
		return true, nil
	}
	if zi := zone.Get(ZoneTmplVersion); zi != nil && zi.InBounds(msg) {
		m.SetFocus(HeaderFocusTmpl)
		return true, nil
	}

	return false, nil
}

// View renders the header as a string (used by backdrop for compositing)
func (m HeaderModel) View() string {
	styles := GetStyles()

	// Render content for each section
	left := m.renderLeft()
	center := m.renderCenter()
	appVer, tmplVer := m.renderVersions()

	// Calculate widths (use WidthWithoutZones for accurate measurement with zone markers)
	leftWidth := WidthWithoutZones(left)
	centerWidth := WidthWithoutZones(center)
	appVerWidth := WidthWithoutZones(appVer)
	tmplVerWidth := WidthWithoutZones(tmplVer)

	// Try to fit everything on one line: left | center | appVer tmplVer
	totalWidth := leftWidth + centerWidth + appVerWidth + tmplVerWidth + 2 // +2 for spacing
	if totalWidth <= m.width {
		// Single line layout - fill entire width first, then place content
		right := appVer + tmplVer
		rightWidth := appVerWidth + tmplVerWidth

		remainingWidth := m.width - centerWidth
		leftSectionWidth := remainingWidth / 2
		rightSectionWidth := remainingWidth - leftSectionWidth

		// Build left section (content + padding to fill section)
		leftPadding := leftSectionWidth - leftWidth
		if leftPadding < 0 {
			leftPadding = 0
		}

		// Build right section (padding + content to right-align)
		rightPadding := rightSectionWidth - rightWidth
		if rightPadding < 0 {
			rightPadding = 0
		}

		// Combine all into one line and render with single background style
		fullLine := left + strutil.Repeat(" ", leftPadding) + center + strutil.Repeat(" ", rightPadding) + right

		// Ensure line fills entire width (handles any rounding issues)
		lineWidth := WidthWithoutZones(fullLine)
		if lineWidth < m.width {
			fullLine += strutil.Repeat(" ", m.width-lineWidth)
		}

		return MaintainBackground(fullLine, styles.HeaderBG)
	}

	// Try fitting left | center | appVer on line 1, tmplVer on line 2
	line1Width := leftWidth + centerWidth + appVerWidth + 2
	if line1Width <= m.width {
		// Line 1: left + center + appVer (with proper spacing)
		remainingWidth := m.width - centerWidth
		leftSectionWidth := remainingWidth / 2
		rightSectionWidth := remainingWidth - leftSectionWidth

		leftPadding := leftSectionWidth - leftWidth
		if leftPadding < 0 {
			leftPadding = 0
		}

		rightPadding := rightSectionWidth - appVerWidth
		if rightPadding < 0 {
			rightPadding = 0
		}

		// Build line 1 as single string
		fullLine1 := left + strutil.Repeat(" ", leftPadding) + center + strutil.Repeat(" ", rightPadding) + appVer

		// Ensure line fills entire width
		line1VisualWidth := WidthWithoutZones(fullLine1)
		if line1VisualWidth < m.width {
			fullLine1 += strutil.Repeat(" ", m.width-line1VisualWidth)
		}
		line1 := MaintainBackground(fullLine1, styles.HeaderBG)

		// Line 2: tmplVer right-aligned with proper background
		padding2 := m.width - tmplVerWidth
		if padding2 < 0 {
			padding2 = 0
		}
		fullLine2 := strutil.Repeat(" ", padding2) + tmplVer

		// Ensure line 2 fills entire width
		line2VisualWidth := WidthWithoutZones(fullLine2)
		if line2VisualWidth < m.width {
			fullLine2 += strutil.Repeat(" ", m.width-line2VisualWidth)
		}
		line2 := MaintainBackground(fullLine2, styles.HeaderBG)

		return line1 + "\n" + line2
	}

	// Fallback: left + center on line 1, both versions on line 2
	// Build line 1: content + padding to fill width
	fullLine1 := left + " " + center
	line1VisualWidth := WidthWithoutZones(fullLine1)
	if line1VisualWidth < m.width {
		fullLine1 += strutil.Repeat(" ", m.width-line1VisualWidth)
	}
	line1 := MaintainBackground(fullLine1, styles.HeaderBG)

	// Line 2: padding + both versions (right-aligned)
	right := appVer + tmplVer
	rightWidth := appVerWidth + tmplVerWidth
	padding2 := m.width - rightWidth
	if padding2 < 0 {
		padding2 = 0
	}
	fullLine2 := strutil.Repeat(" ", padding2) + right

	// Ensure line 2 fills entire width
	line2VisualWidth := WidthWithoutZones(fullLine2)
	if line2VisualWidth < m.width {
		fullLine2 += strutil.Repeat(" ", m.width-line2VisualWidth)
	}
	line2 := MaintainBackground(fullLine2, styles.HeaderBG)

	return line1 + "\n" + line2
}

func (m HeaderModel) renderLeft() string {
	styles := GetStyles()

	// Build hostname with theme tag
	leftText := "{{|Theme_Hostname|}}" + m.hostname + "{{[-]}}"

	// Add flags if present
	if len(m.flags) > 0 {
		leftText += " {{|Theme_ApplicationFlagsBrackets|}}|{{[-]}}"
		for i, flag := range m.flags {
			if i > 0 {
				leftText += "{{|Theme_ApplicationFlagsSpace|}}|{{[-]}}"
			}
			leftText += "{{|Theme_ApplicationFlags|}}" + flag + "{{[-]}}"
		}
		leftText += "{{|Theme_ApplicationFlagsBrackets|}}|{{[-]}}"
	}

	// Translate theme tags and render with lipgloss, using header background as default
	return MaintainBackground(RenderThemeText(leftText, styles.HeaderBG), styles.HeaderBG)
}

func (m HeaderModel) renderCenter() string {
	styles := GetStyles()
	centerText := "{{|Theme_ApplicationName|}}" + version.ApplicationName + "{{[-]}}"
	return MaintainBackground(RenderThemeText(centerText, styles.HeaderBG), styles.HeaderBG)
}

// renderVersions returns app version and template version as separate strings
func (m HeaderModel) renderVersions() (appText, tmplText string) {
	styles := GetStyles()
	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	// Helper to render version blocks
	// format: [StatusIcon] [Label][ [Version] ]
	renderVersionBlock := func(ver string, label string, isAvailable bool, isError bool, isFocused bool, zoneID string) string {
		var text string

		// 1. Status Icon / Prefix
		if isError {
			text += "{{|Theme_ApplicationUpdate|}}?{{|Theme_StatusBar|}}"
		} else if isAvailable {
			text += "{{|Theme_ApplicationUpdate|}}*{{|Theme_StatusBar|}}"
		} else {
			text += " "
		}

		// 2. Label + Open Bracket (Standard or Update color)
		// Typically label is cyan/Theme_ApplicationVersion
		text += "{{|Theme_ApplicationVersion|}}" + label + ":[{{|Theme_StatusBar|}}"

		// 3. Version Number (The Interactive Part)
		// If Focused -> Selection Style
		// If Update/Error -> Update Style (Red/Yellow)
		// Else -> Default Style (Inherit or specific)
		var verStyled string
		if isFocused {
			verStyled = "{{|Theme_VersionSelected|}}" + ver + "{{|Theme_StatusBar|}}"
		} else if isError || isAvailable {
			verStyled = "{{|Theme_ApplicationUpdate|}}" + ver + "{{|Theme_StatusBar|}}"
		} else {
			// Inherit Theme_ApplicationVersion for standard look, but since we reset before, we must apply it
			verStyled = "{{|Theme_ApplicationVersion|}}" + ver + "{{|Theme_StatusBar|}}"
		}
		text += verStyled

		// 4. Close Bracket
		text += "{{|Theme_ApplicationVersion|}}]{{|Theme_StatusBar|}}"

		// 5. Wrap in Zone for clicking (Mouse area covers full block)
		return zone.Mark(zoneID, MaintainBackground(RenderThemeText(text, styles.HeaderBG), styles.HeaderBG))
	}

	appText = renderVersionBlock(appVer, "A", update.AppUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusApp, ZoneAppVersion)
	tmplText = renderVersionBlock(tmplVer, "T", update.TmplUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusTmpl, ZoneTmplVersion)

	return appText, tmplText
}

// renderRight returns both versions combined (for backwards compatibility)
func (m HeaderModel) renderRight() string {
	appText, tmplText := m.renderVersions()
	return lipgloss.JoinHorizontal(lipgloss.Top, appText, tmplText)
}
