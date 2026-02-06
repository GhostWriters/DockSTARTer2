package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmDialogModel represents a yes/no confirmation dialog
type confirmDialogModel struct {
	title      string
	question   string
	defaultYes bool
	result     bool
	confirmed  bool
	width      int
	height     int
}

type confirmResultMsg struct {
	result bool
}

// newConfirmDialog creates a new confirmation dialog
func newConfirmDialog(title, question string, defaultYes bool) confirmDialogModel {
	return confirmDialogModel{
		title:      title,
		question:   question,
		defaultYes: defaultYes,
		result:     defaultYes,
	}
}

func (m confirmDialogModel) Init() tea.Cmd {
	return nil
}

func (m confirmDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.result = true
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N", "esc"))):
			m.result = false
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.result = false
			m.confirmed = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m confirmDialogModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	styles := GetStyles()

	// Question text
	questionStyle := lipgloss.NewStyle().
		Foreground(styles.ItemSelected).
		Bold(true).
		Padding(1, 2)

	// Button styles
	yesStyle := styles.Button
	noStyle := styles.Button

	if m.result {
		yesStyle = styles.ButtonActive
	} else {
		noStyle = styles.ButtonActive
	}

	yesBtn := yesStyle.Render(" Yes ")
	noBtn := noStyle.Render(" No ")

	// Build dialog content
	content := questionStyle.Render(m.question) + "\n\n" +
		lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "  ", noBtn)

	// Wrap in dialog box with title
	dialogStyle := styles.Dialog.
		Padding(0, 1)
	dialogStyle = ApplyStraightBorder(dialogStyle, styles.LineCharacters)

	dialog := dialogStyle.Render(content)

	// Add title
	dialogWithTitle := renderBorderWithTitleStatic(m.title, dialog)

	// Center on screen
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogWithTitle,
	)
}

// renderBorderWithTitleStatic is a static version of renderBorderWithTitle for dialog use
func renderBorderWithTitleStatic(title, content string) string {
	styles := GetStyles()

	// For simplicity in dialogs, just prepend the title
	titleStyle := styles.DialogTitle.Copy().
		Padding(0, 1).
		Bold(true)

	titleBar := titleStyle.Render(title)

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content)
}

// ShowConfirmDialog displays a confirmation dialog and returns the result
func ShowConfirmDialog(title, question string, defaultYes bool) bool {
	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.Theme); err == nil {
		InitStyles(cfg)
	}

	model := newConfirmDialog(title, question, defaultYes)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		// Fallback to default on error
		return defaultYes
	}

	if m, ok := finalModel.(confirmDialogModel); ok {
		return m.result
	}

	return defaultYes
}
