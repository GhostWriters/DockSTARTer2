package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	overlay "github.com/rmhubbert/bubbletea-overlay"
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
		return ""
	}

	styles := GetStyles()

	// Question text
	questionStyle := lipgloss.NewStyle().
		Foreground(styles.ItemSelected.GetForeground()).
		Bold(true).
		Padding(1, 2)

	questionText := questionStyle.Render(m.question)

	// Calculate content width based on question text (with reasonable min/max)
	contentWidth := lipgloss.Width(questionText)
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Render buttons using the standard button helper (ensures consistency)
	buttonRow := RenderCenteredButtons(
		contentWidth,
		ButtonSpec{Text: " Yes ", Active: m.result},
		ButtonSpec{Text: " No ", Active: !m.result},
	)

	// Build dialog content
	content := questionText + "\n\n" + buttonRow

	// Add padding to content (border will be added by RenderDialogWithTitle)
	paddedContent := styles.Dialog.
		Padding(0, 1).
		Render(content)

	// Wrap in border with title embedded (matching menu style)
	dialogWithTitle := RenderDialogWithTitle(m.title, paddedContent)

	// Add shadow (matching menu style)
	dialogWithTitle = AddShadow(dialogWithTitle)

	// Just return the dialog content - backdrop will be handled by overlay
	return dialogWithTitle
}

// confirmWithBackdrop wraps a confirmation dialog with backdrop using overlay
type confirmWithBackdrop struct {
	backdrop BackdropModel
	dialog   confirmDialogModel
}

func (m confirmWithBackdrop) Init() tea.Cmd {
	return tea.Batch(m.backdrop.Init(), m.dialog.Init())
}

func (m confirmWithBackdrop) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Update backdrop
	backdropModel, cmd := m.backdrop.Update(msg)
	m.backdrop = backdropModel.(BackdropModel)
	cmds = append(cmds, cmd)

	// Update dialog
	dialogModel, cmd := m.dialog.Update(msg)
	m.dialog = dialogModel.(confirmDialogModel)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m confirmWithBackdrop) View() string {
	// Use overlay to composite dialog over backdrop
	// overlay.Composite(foreground, background, xPos, yPos, xOffset, yOffset)
	return overlay.Composite(
		m.dialog.View(),    // foreground (dialog content)
		m.backdrop.View(),  // background (backdrop base)
		overlay.Center,
		overlay.Center,
		0,
		0,
	)
}

// ShowConfirmDialog displays a confirmation dialog and returns the result
func ShowConfirmDialog(title, question string, defaultYes bool) bool {
	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.Theme); err == nil {
		InitStyles(cfg)
	}

	helpText := "Y/N to choose | Enter to confirm | Esc to cancel"
	model := confirmWithBackdrop{
		backdrop: NewBackdropModel(helpText),
		dialog:   newConfirmDialog(title, question, defaultYes),
	}

	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		// Fallback to default on error
		return defaultYes
	}

	if m, ok := finalModel.(confirmWithBackdrop); ok {
		return m.dialog.result
	}

	return defaultYes
}
