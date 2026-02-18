package tui

import (
	"DockSTARTer2/internal/console"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2" // used for zone.Get in Update()
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
	onResult   func(bool) tea.Msg // Optional: Custom message generator for result
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
		onResult: func(r bool) tea.Msg {
			return CloseDialogMsg{Result: r}
		},
	}
}

func (m *confirmDialogModel) Init() tea.Cmd {
	return nil
}

func (m *confirmDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Helper to close dialog with result
	closeWithResult := func(result bool) tea.Cmd {
		return func() tea.Msg { return m.onResult(result) }
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Esc):
			m.result = false
			m.confirmed = true
			return m, closeWithResult(false)

		case key.Matches(msg, Keys.Enter):
			m.confirmed = true
			return m, closeWithResult(m.result)

		case key.Matches(msg, Keys.ForceQuit):
			m.result = false
			m.confirmed = true
			return m, closeWithResult(false)

		default:
			// Arrow keys toggle between Yes and No
			if key.Matches(msg, Keys.Left) || key.Matches(msg, Keys.Right) ||
				key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) {
				m.result = !m.result
				return m, nil
			}
			// Tab/ShiftTab also toggle
			if key.Matches(msg, Keys.Tab) || key.Matches(msg, Keys.ShiftTab) {
				m.result = !m.result
				return m, nil
			}
			// Check dynamic hotkeys for buttons (Yes/No)
			buttons := []ButtonSpec{
				{Text: " Yes "},
				{Text: " No "},
			}
			if idx, found := CheckButtonHotkeys(msg, buttons); found {
				m.result = (idx == 0) // Yes is index 0
				m.confirmed = true
				return m, closeWithResult(m.result)
			}
		}

	case tea.MouseClickMsg:
		// Check if Yes button was clicked (auto-generated zone ID: "Button.Yes")
		if zoneInfo := zone.Get("Button.Yes"); zoneInfo != nil {
			if zoneInfo.InBounds(msg) {
				m.result = true
				m.confirmed = true
				return m, closeWithResult(true)
			}
		}

		// Check if No button was clicked (auto-generated zone ID: "Button.No")
		if zoneInfo := zone.Get("Button.No"); zoneInfo != nil {
			if zoneInfo.InBounds(msg) {
				m.result = false
				m.confirmed = true
				return m, closeWithResult(false)
			}
		}
	}

	return m, nil
}

// ViewString returns the dialog content as a string for compositing
func (m *confirmDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()

	// Question text
	questionStyle := lipgloss.NewStyle().
		Padding(1, 2)

	// Apply semantic coloring (e.g. {{|Version|}})
	questionText := questionStyle.Render(console.Sprintf("%s", m.question))

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
	dialog := AddShadow(dialogWithTitle)

	return dialog
}

func (m *confirmDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// SetSize updates the dialog dimensions
func (m *confirmDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ShowConfirmDialog displays a confirmation dialog and returns the result
func ShowConfirmDialog(title, question string, defaultYes bool) bool {
	helpText := "Y/N to choose | Enter to confirm | Esc to cancel"
	dialog := newConfirmDialog(title, question, defaultYes)

	finalDialog, err := RunDialogWithBackdrop(dialog, helpText, PositionCenter)
	if err != nil {
		// Fallback to default on error
		return defaultYes
	}

	return finalDialog.result
}

// PromptConfirm displays a blocking confirmation dialog over the active ProgramBox.
// It is used by the console package via callback to prompt during background tasks.
func PromptConfirm(title, question string, defaultYes bool) bool {
	if program == nil {
		return defaultYes
	}

	ch := make(chan bool)
	dialog := newConfirmDialog(title, question, defaultYes)
	dialog.onResult = func(r bool) tea.Msg {
		return CloseDialogMsg{Result: r}
	}

	program.Send(SubDialogMsg{
		Model: dialog,
		Chan:  ch,
	})

	return <-ch
}
