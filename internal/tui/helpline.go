package tui

import (
	"charm.land/lipgloss/v2"
)

// HelplineModel represents the help line at the bottom of the TUI
type HelplineModel struct {
	text string
}

// NewHelplineModel creates a new helpline model
func NewHelplineModel() HelplineModel {
	return HelplineModel{}
}

// SetText updates the help text
func (m *HelplineModel) SetText(text string) {
	m.text = text
}

// View renders the helpline
func (m HelplineModel) View(width int) string {
	styles := GetStyles()

	// Center the help text
	helpStyle := styles.HelpLine.Width(width).Align(lipgloss.Center)
	return MaintainBackground(helpStyle.Render(m.text), styles.HelpLine)
}
