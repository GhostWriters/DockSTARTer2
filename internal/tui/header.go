package tui

import (
	"os"

	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/pflag"
)

// HeaderModel represents the header bar at the top of the TUI
type HeaderModel struct {
	width int

	// Cached values
	hostname string
	flags    []string
}

// NewHeaderModel creates a new header model
func NewHeaderModel() HeaderModel {
	hostname, _ := os.Hostname()

	var flags []string
	if v, _ := pflag.CommandLine.GetBool("verbose"); v {
		flags = append(flags, "VERBOSE")
	}
	if d, _ := pflag.CommandLine.GetBool("debug"); d {
		flags = append(flags, "DEBUG")
	}
	if f, _ := pflag.CommandLine.GetBool("force"); f {
		flags = append(flags, "FORCE")
	}
	if y, _ := pflag.CommandLine.GetBool("yes"); y {
		flags = append(flags, "YES")
	}

	return HeaderModel{
		hostname: hostname,
		flags:    flags,
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

// View renders the header
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

	// Wrap each section with alignment and header background
	leftSection := styles.HeaderBG.
		Width(leftSectionWidth).
		Align(lipgloss.Left).
		Render(left)

	centerSection := styles.HeaderBG.
		Width(centerWidth).
		Render(center)

	rightSection := styles.HeaderBG.
		Width(rightSectionWidth).
		Align(lipgloss.Right).
		Render(right)

	// Join the three sections
	return lipgloss.JoinHorizontal(lipgloss.Top, leftSection, centerSection, rightSection)
}

func (m HeaderModel) renderLeft() string {
	styles := GetStyles()

	// Build hostname with theme tag
	leftText := "{{_Theme_Hostname_}}" + m.hostname + "{{|-|}}"

	// Add flags if present
	if len(m.flags) > 0 {
		leftText += " {{_Theme_ApplicationFlagsBrackets_}}|{{|-|}}"
		for i, flag := range m.flags {
			if i > 0 {
				leftText += "{{_Theme_ApplicationFlagsSpace_}}|{{|-|}}"
			}
			leftText += "{{_Theme_ApplicationFlags_}}" + flag + "{{|-|}}"
		}
		leftText += "{{_Theme_ApplicationFlagsBrackets_}}|{{|-|}}"
	}

	// Translate theme tags and render with lipgloss, using header background as default
	return MaintainBackground(RenderThemeText(leftText, styles.HeaderBG), styles.HeaderBG)
}

func (m HeaderModel) renderCenter() string {
	styles := GetStyles()
	centerText := "{{_Theme_ApplicationName_}}" + version.ApplicationName + "{{|-|}}"
	return MaintainBackground(RenderThemeText(centerText, styles.HeaderBG), styles.HeaderBG)
}

func (m HeaderModel) renderRight() string {
	styles := GetStyles()
	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	var rightText string
	// Show update indicator: "?" if check failed, "*" if update available, " " otherwise
	if update.UpdateCheckError {
		rightText += "{{_Theme_ApplicationUpdate_}}?{{|-|}}"
		rightText += "{{_Theme_ApplicationVersion_}}A:[" + appVer + "]{{|-|}}"
	} else if update.AppUpdateAvailable {
		rightText += "{{_Theme_ApplicationUpdate_}}*{{|-|}}"
		rightText += "{{_Theme_ApplicationVersion_}}A:[{{|-|}}{{_Theme_ApplicationUpdate_}}" + appVer + "{{|-|}}{{_Theme_ApplicationVersion_}}]{{|-|}}"
	} else {
		rightText += " "
		rightText += "{{_Theme_ApplicationVersion_}}A:[" + appVer + "]{{|-|}}"
	}

	if update.UpdateCheckError {
		rightText += "{{_Theme_ApplicationUpdate_}}?{{|-|}}"
		rightText += "{{_Theme_ApplicationVersion_}}T:[" + tmplVer + "]{{|-|}}"
	} else if update.TmplUpdateAvailable {
		rightText += "{{_Theme_ApplicationUpdate_}}*{{|-|}}"
		rightText += "{{_Theme_ApplicationVersion_}}T:[{{|-|}}{{_Theme_ApplicationUpdate_}}" + tmplVer + "{{|-|}}{{_Theme_ApplicationVersion_}}]{{|-|}}"
	} else {
		rightText += " "
		rightText += "{{_Theme_ApplicationVersion_}}T:[" + tmplVer + "]{{|-|}}"
	}

	return MaintainBackground(RenderThemeText(rightText, styles.HeaderBG), styles.HeaderBG)
}
