package tui

import (
	"os"
	"strings"

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
	left := m.renderLeft()

	// Center section: application name
	center := m.renderCenter()

	// Right section: version info with update indicators
	right := m.renderRight()

	// Calculate visual widths (lipgloss.Width strips ANSI codes)
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)

	// Total content width
	totalContentWidth := leftWidth + centerWidth + rightWidth
	if totalContentWidth >= m.width {
		// Not enough space, just concatenate
		headerLine := left + center + right
		return styles.HeaderBG.Width(m.width).Render(headerLine)
	}

	// Center the app name in the middle of the screen
	// Calculate where the center text should start to be truly centered
	centerPos := (m.width - centerWidth) / 2

	// Left padding: space between left content and center
	leftPad := centerPos - leftWidth
	if leftPad < 1 {
		leftPad = 1
	}

	// Right padding: space between center and right content
	// Position where right content should start
	rightPos := m.width - rightWidth
	rightPad := rightPos - centerPos - centerWidth
	if rightPad < 1 {
		rightPad = 1
	}

	// Build the header with explicit spacing
	headerLine := left +
		strings.Repeat(" ", leftPad) +
		center +
		strings.Repeat(" ", rightPad) +
		right

	// Apply header background to the entire line
	return styles.HeaderBG.Width(m.width).Render(headerLine)
}

func (m HeaderModel) renderLeft() string {
	// Build hostname with theme tag
	leftText := "{{_ThemeHostname_}}" + m.hostname + "{{|-|}}"

	// Add flags if present
	if len(m.flags) > 0 {
		leftText += " {{_ThemeApplicationFlagsBrackets_}}|{{|-|}}"
		for i, flag := range m.flags {
			if i > 0 {
				leftText += "{{_ThemeApplicationFlagsSpace_}}|{{|-|}}"
			}
			leftText += "{{_ThemeApplicationFlags_}}" + flag + "{{|-|}}"
		}
		leftText += "{{_ThemeApplicationFlagsBrackets_}}|{{|-|}}"
	}

	// Translate theme tags and render with lipgloss
	return RenderThemeText(leftText)
}

func (m HeaderModel) renderCenter() string {
	centerText := "{{_ThemeApplicationName_}}" + version.ApplicationName + "{{|-|}}"
	return RenderThemeText(centerText)
}

func (m HeaderModel) renderRight() string {
	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	var rightText string
	if update.AppUpdateAvailable {
		rightText += "{{_ThemeApplicationUpdate_}}*{{|-|}}"
	} else {
		rightText += " "
	}
	rightText += "{{_ThemeApplicationVersion_}}A:[" + appVer + "]{{|-|}}"

	if update.TmplUpdateAvailable {
		rightText += "{{_ThemeApplicationUpdate_}}*{{|-|}}"
	} else {
		rightText += " "
	}
	rightText += "{{_ThemeApplicationVersion_}}T:[" + tmplVer + "]{{|-|}}"

	return RenderThemeText(rightText)
}
