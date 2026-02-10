package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpDialogModel displays a keyboard shortcut reference dialog.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type helpDialogModel struct {
	help   help.Model
	width  int
	height int
}

func newHelpDialogModel() *helpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &helpDialogModel{help: h}
}

func (m *helpDialogModel) Init() tea.Cmd { return nil }

func (m *helpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Any key closes the help dialog (? toggles it off, Esc also works)
		_ = msg
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
	}
	return m, nil
}

func (m *helpDialogModel) View() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()

	// Set help model width to fit comfortably in the dialog
	m.help.Width = m.width - 12

	// Apply theme styles to the help component
	dimStyle := lipgloss.NewStyle().
		Foreground(styles.ItemNormal.GetForeground()).
		Background(styles.Dialog.GetBackground())
	keyStyle := lipgloss.NewStyle().
		Foreground(styles.TagKey.GetForeground()).
		Background(styles.Dialog.GetBackground()).
		Bold(true)
	sepStyle := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(styles.Dialog.GetBackground())

	m.help.Styles.ShortKey = keyStyle
	m.help.Styles.ShortDesc = dimStyle
	m.help.Styles.ShortSeparator = sepStyle
	m.help.Styles.FullKey = keyStyle
	m.help.Styles.FullDesc = dimStyle
	m.help.Styles.FullSeparator = sepStyle
	m.help.Styles.Ellipsis = dimStyle

	content := m.help.View(Keys)

	// Apply dialog background to each line to prevent color bleed
	lines := strings.Split(content, "\n")
	bgStyle := lipgloss.NewStyle().Background(styles.Dialog.GetBackground())
	for i, line := range lines {
		lines[i] = bgStyle.Render(line)
	}
	content = strings.Join(lines, "\n")

	// Wrap in padded dialog style
	paddedContent := styles.Dialog.
		Padding(0, 1).
		Render(content)

	dialogWithTitle := RenderDialog("Keyboard Shortcuts", paddedContent, true)
	return AddShadow(dialogWithTitle)
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *helpDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.Width = w - 12
}
