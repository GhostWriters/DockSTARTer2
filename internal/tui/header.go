package tui

import (
	"fmt"
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

	// Calculate widths
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)

	// Calculate padding to center the app name and right-align version
	totalContentWidth := leftWidth + centerWidth + rightWidth
	if totalContentWidth >= m.width {
		// Not enough space, just concatenate
		return styles.HeaderBG.Width(m.width).Render(left + center + right)
	}

	// Calculate spaces needed
	remainingSpace := m.width - totalContentWidth
	leftPad := remainingSpace / 2
	rightPad := remainingSpace - leftPad

	// Build the header line
	result := left +
		strings.Repeat(" ", leftPad) +
		center +
		strings.Repeat(" ", rightPad) +
		right

	return styles.HeaderBG.Width(m.width).Render(result)
}

func (m HeaderModel) renderLeft() string {
	styles := GetStyles()

	// Hostname (bold)
	hostnameStyle := styles.HeaderBG.Bold(true)
	result := hostnameStyle.Render(m.hostname)

	// Flags
	if len(m.flags) > 0 {
		result += " "

		// Bracket style
		bracketStyle := styles.HeaderBG

		result += bracketStyle.Render("|")
		for i, flag := range m.flags {
			if i > 0 {
				result += bracketStyle.Render("|")
			}
			flagStyle := styles.HeaderBG.Bold(true)
			result += flagStyle.Render(flag)
		}
		result += bracketStyle.Render("|")
	}

	return result
}

func (m HeaderModel) renderCenter() string {
	styles := GetStyles()

	// Application name (bold)
	nameStyle := styles.HeaderBG.Bold(true)
	return nameStyle.Render(version.ApplicationName)
}

func (m HeaderModel) renderRight() string {
	styles := GetStyles()

	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	// App version with update indicator
	var appSection string
	if update.AppUpdateAvailable {
		// Yellow asterisk for update available
		updateStyle := lipgloss.NewStyle().
			Background(styles.HeaderBG.GetBackground()).
			Foreground(lipgloss.Color("#ffff00")). // Yellow
			Bold(true)
		appSection = updateStyle.Render("*")

		// Version in yellow
		verStyle := lipgloss.NewStyle().
			Background(styles.HeaderBG.GetBackground()).
			Foreground(lipgloss.Color("#ffff00"))
		appSection += styles.HeaderBG.Render("A:[") + verStyle.Render(appVer) + styles.HeaderBG.Render("]")
	} else {
		appSection = " " + styles.HeaderBG.Render(fmt.Sprintf("A:[%s]", appVer))
	}

	// Template version with update indicator
	var tmplSection string
	if update.TmplUpdateAvailable {
		// Yellow asterisk for update available
		updateStyle := lipgloss.NewStyle().
			Background(styles.HeaderBG.GetBackground()).
			Foreground(lipgloss.Color("#ffff00")). // Yellow
			Bold(true)
		tmplSection = updateStyle.Render("*")

		// Version in yellow
		verStyle := lipgloss.NewStyle().
			Background(styles.HeaderBG.GetBackground()).
			Foreground(lipgloss.Color("#ffff00"))
		tmplSection += styles.HeaderBG.Render("T:[") + verStyle.Render(tmplVer) + styles.HeaderBG.Render("]")
	} else {
		tmplSection = " " + styles.HeaderBG.Render(fmt.Sprintf("T:[%s]", tmplVer))
	}

	return appSection + tmplSection
}
