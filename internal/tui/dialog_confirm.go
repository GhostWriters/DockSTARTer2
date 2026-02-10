package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
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
func newConfirmDialog(title, question string, defaultYes bool) *confirmDialogModel {
	return &confirmDialogModel{
		title:      title,
		question:   question,
		defaultYes: defaultYes,
		result:     defaultYes,
	}
}

func (m *confirmDialogModel) Init() tea.Cmd {
	return nil
}

func (m *confirmDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Esc):
			m.result = false
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, Keys.Enter):
			m.confirmed = true
			return m, tea.Quit

		case key.Matches(msg, Keys.ForceQuit):
			m.result = false
			m.confirmed = true
			return m, tea.Quit

		default:
			// Check dynamic hotkeys for buttons (Yes/No)
			buttons := []ButtonSpec{
				{Text: " Yes "},
				{Text: " No "},
			}
			if idx, found := CheckButtonHotkeys(msg, buttons); found {
				m.result = (idx == 0) // Yes is index 0
				m.confirmed = true
				return m, tea.Quit
			}
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if Yes button was clicked (auto-generated zone ID: "Button.Yes")
			if zoneInfo := zone.Get("Button.Yes"); zoneInfo != nil {
				if zoneInfo.InBounds(msg) {
					m.result = true
					m.confirmed = true
					return m, tea.Quit
				}
			}

			// Check if No button was clicked (auto-generated zone ID: "Button.No")
			if zoneInfo := zone.Get("Button.No"); zoneInfo != nil {
				if zoneInfo.InBounds(msg) {
					m.result = false
					m.confirmed = true
					return m, tea.Quit
				}
			}
		}
	}

	return m, nil
}

func (m *confirmDialogModel) View() string {
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
	// Zone marking is handled automatically by RenderCenteredButtons (zone IDs: "Button.Yes", "Button.No")
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
	dialogWithTitle := RenderDialog(m.title, paddedContent, true)

	// Add shadow (matching menu style)
	dialogWithTitle = AddShadow(dialogWithTitle)

	// Just return the dialog content - backdrop will be handled by overlay
	return dialogWithTitle
}

// SetSize updates the dialog dimensions
func (m *confirmDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// confirmWithBackdrop wraps a confirmation dialog with backdrop using overlay
type confirmWithBackdrop struct {
	backdrop BackdropModel
	dialog   *confirmDialogModel
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
	m.dialog = dialogModel.(*confirmDialogModel)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m confirmWithBackdrop) View() string {
	// Use overlay to composite dialog over backdrop
	// overlay.Composite(foreground, background, xPos, yPos, xOffset, yOffset)
	output := overlay.Composite(
		m.dialog.View(),   // foreground (dialog content)
		m.backdrop.View(), // background (backdrop base)
		overlay.Center,
		overlay.Center,
		0,
		0,
	)

	// Scan zones at root level for mouse support
	return zone.Scan(output)
}

// ShowConfirmDialog displays a confirmation dialog and returns the result
func ShowConfirmDialog(title, question string, defaultYes bool) bool {
	// Initialize global zone manager for mouse support (safe to call multiple times)
	zone.NewGlobal()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.UI.Theme); err == nil {
		InitStyles(cfg)
	}

	helpText := "Y/N to choose | Enter to confirm | Esc to cancel"
	model := confirmWithBackdrop{
		backdrop: NewBackdropModel(helpText),
		dialog:   newConfirmDialog(title, question, defaultYes),
	}

	p := tea.NewProgram(model, tea.WithMouseAllMotion())

	finalModel, err := p.Run()
	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")
	if err != nil {
		// Fallback to default on error
		return defaultYes
	}

	if m, ok := finalModel.(confirmWithBackdrop); ok {
		return m.dialog.result
	}

	return defaultYes
}
