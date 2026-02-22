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

	// Left section: hostname + flags
	// Render content for each section
	left := m.renderLeft()
	center := m.renderCenter()
	right := m.renderRight()

	// Calculate center width (strip ANSI codes for accurate width)
	centerWidth := lipgloss.Width(center)

	// Divide width into three sections
	// Center gets the exact space it needs, sides split the rest
	remainingWidth := m.width - centerWidth
	leftSectionWidth := remainingWidth / 2
	rightSectionWidth := remainingWidth - leftSectionWidth

	// Build padded strings manually using strutil.Repeat to avoid lipgloss width panics on negative sizes
	leftAligned := left
	if w := lipgloss.Width(left); leftSectionWidth > w {
		leftAligned += strutil.Repeat(" ", leftSectionWidth-w)
	}

	rightAligned := right
	if w := lipgloss.Width(right); rightSectionWidth > w {
		rightAligned = strutil.Repeat(" ", rightSectionWidth-w) + rightAligned
	}

	leftSection := styles.HeaderBG.Render(leftAligned)
	centerSection := styles.HeaderBG.Render(center)
	rightSection := styles.HeaderBG.Render(rightAligned)

	// Join the three sections
	return lipgloss.JoinHorizontal(lipgloss.Top, leftSection, centerSection, rightSection)
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

func (m HeaderModel) renderRight() string {
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

	appText := renderVersionBlock(appVer, "A", update.AppUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusApp, ZoneAppVersion)
	tmplText := renderVersionBlock(tmplVer, "T", update.TmplUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusTmpl, ZoneTmplVersion)

	// Join them
	return lipgloss.JoinHorizontal(lipgloss.Top, appText, tmplText)
}
