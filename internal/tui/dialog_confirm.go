package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	// Unified layout (deterministic sizing)
	layout DialogLayout
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
				{Text: "Yes"},
				{Text: "No"},
			}
			if idx, found := CheckButtonHotkeys(msg, buttons); found {
				m.result = (idx == 0) // Yes is index 0
				m.confirmed = true
				return m, closeWithResult(m.result)
			}
		}

	case LayerHitMsg:
		switch msg.ID {
		case "Button.Yes":
			m.result = true
			m.confirmed = true
			return m, closeWithResult(true)
		case "Button.No":
			m.result = false
			m.confirmed = true
			return m, closeWithResult(false)
		}
	}

	return m, nil
}

// ViewString returns the dialog content as a string for compositing
func (m *confirmDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()
	borderBG := ctx.Dialog.GetBackground()

	// 1. Calculate ideal content width based on question text and buttons
	// Split question into lines to find longest line
	questionLines := strings.Split(m.question, "\n")
	maxQuestionWidth := 0
	for _, line := range questionLines {
		w := lipgloss.Width(line)
		if w > maxQuestionWidth {
			maxQuestionWidth = w
		}
	}
	// Add padding to question width (matching questionStyle.Padding(1, 2))
	contentWidth := maxQuestionWidth + 4

	// 2. Measure button requirements
	// RenderCenteredButtonsCtx uses maxButtonWidth+4 for each button
	btn1W := lipgloss.Width("Yes") + 4
	btn2W := lipgloss.Width("No") + 4
	minButtonWidth := btn1W + btn2W + 4 // Add some gap between buttons

	if minButtonWidth > contentWidth {
		contentWidth = minButtonWidth
	}

	// 3. Ensure it's at least as wide as the title
	titleW := lipgloss.Width(m.title) + 6 // Title + connectors + space
	if titleW > contentWidth {
		contentWidth = titleW
	}

	// 4. Constrain by available layout width
	if contentWidth > m.layout.Width-2 {
		contentWidth = m.layout.Width - 2
	}

	// Question text style - inherit from ctx.Dialog to get background
	questionStyle := ctx.Dialog.
		Padding(1, 2).
		Width(contentWidth)

	// Apply style and wrap
	questionText := questionStyle.Render(console.Sprintf("%s", m.question))

	// Render buttons using the standard button helper
	buttonRow := RenderCenteredButtonsCtx(
		contentWidth,
		ctx,
		ButtonSpec{Text: "Yes", Active: m.result},
		ButtonSpec{Text: "No", Active: !m.result},
	)

	// Build dialog content
	// Standardize to use TrimRight to prevent implicit gaps
	questionText = strings.TrimRight(questionText, "\n")
	buttonRow = strings.TrimRight(buttonRow, "\n")

	// Ensure a blank line between question and buttons carries the background
	spacer := lipgloss.NewStyle().
		Width(contentWidth).
		Background(borderBG).
		Render("")

	fullContent := lipgloss.JoinVertical(lipgloss.Left, questionText, spacer, buttonRow)

	// Wrap in border with title embedded (matching menu style) using confirm styling
	dialogWithTitle := RenderDialogWithType(m.title, fullContent, true, 0, DialogTypeConfirm)

	// Add shadow
	dialog := AddShadow(dialogWithTitle)

	return dialog
}

// View implements tea.Model
func (m *confirmDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// Layers implements LayeredView
func (m *confirmDialogModel) Layers() []*lipgloss.Layer {
	// Root dialog layer
	root := lipgloss.NewLayer(m.ViewString()).Z(ZDialog)

	// Calculate button hit layers
	// These positions are relative to the root dialog layer
	// Calculation from ViewString():
	// Y = 1 (top border) + questionHeight + 1 (spacer)
	ctx := GetActiveContext()
	questionStyle := ctx.Dialog.Padding(1, 2).Width(m.layout.Width - 2) // Approximate width
	questionHeight := lipgloss.Height(questionStyle.Render(m.question))

	buttonY := 1 + questionHeight + 1
	contentWidth := m.layout.Width - 2

	// RenderCenteredButtonsCtx splits contentWidth into numButtons sections
	numButtons := 2
	sectionWidth := contentWidth / numButtons

	// Yes Button (Left section)
	yesX := 1 + (sectionWidth-12)/2 // Approximate 12-char button width
	if yesX < 1 {
		yesX = 1
	}
	root.AddLayers(lipgloss.NewLayer(strutil.Repeat(" ", 12)).
		X(yesX).Y(buttonY).ID("Button.Yes").Z(1))

	// No Button (Right section)
	noX := 1 + sectionWidth + (sectionWidth-10)/2 // Approximate 10-char button width
	root.AddLayers(lipgloss.NewLayer(strutil.Repeat(" ", 10)).
		X(noX).Y(buttonY).ID("Button.No").Z(1))

	return []*lipgloss.Layer{root}
}

// SetSize updates the dialog dimensions
func (m *confirmDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *confirmDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// 1. Shadow
	shadow := 0
	if currentConfig.UI.Shadow {
		shadow = DialogShadowHeight
	}

	// 2. Buttons
	buttons := DialogButtonHeight

	// 3. Overhead
	overhead := DialogBorderHeight + buttons + shadow

	m.layout = DialogLayout{
		Width:        m.width,
		Height:       m.height,
		ButtonHeight: buttons,
		ShadowHeight: shadow,
		Overhead:     overhead,
	}
}

// ShowConfirmDialog displays a confirmation dialog and returns the result
func ShowConfirmDialog(title, question string, defaultYes bool) bool {
	helpText := "Y/N to choose | Enter to confirm | Esc to cancel"
	dialog := newConfirmDialog(title, question, defaultYes)

	// Otherwise, run standalone with backdrop
	header := NewHeaderModel()
	header.SetWidth(80) // Initial width
	headerH := header.Height()

	finalDialog, err := RunDialogWithBackdrop(dialog, helpText, GetPositionCenter(headerH))
	if err != nil {
		// Fallback to default on error
		return defaultYes
	}

	return finalDialog.result
}

// PromptConfirm displays a blocking confirmation dialog over the active ProgramBox.
// It is used by the console package via callback to prompt during background tasks.
func PromptConfirm(title, question string, defaultYes bool) bool {
	if console.GlobalYes {
		return true
	}
	if program == nil {
		return defaultYes
	}

	ch := make(chan bool)
	dialog := newConfirmDialog(title, question, defaultYes)
	dialog.onResult = func(r bool) tea.Msg {
		return SubDialogResultMsg{Result: r}
	}

	program.Send(SubDialogMsg{
		Model: dialog,
		Chan:  ch,
	})

	return <-ch
}
