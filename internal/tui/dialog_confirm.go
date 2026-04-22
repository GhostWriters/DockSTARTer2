package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// confirmDialogModel represents a yes/no confirmation dialog
type confirmDialogModel struct {
	baseDialogModel
	title      string
	question   string
	defaultYes bool
	result     bool
	confirmed  bool
	onResult   func(bool) tea.Msg
}


// newConfirmDialog creates a new confirmation dialog
func newConfirmDialog(title, question string, defaultYes bool) *confirmDialogModel {
	return &confirmDialogModel{
		baseDialogModel: baseDialogModel{id: "confirm_dialog", focused: true},
		title:           title,
		question:        question,
		defaultYes:      defaultYes,
		result:          defaultYes,
		onResult: func(r bool) tea.Msg {
			return CloseDialogMsg{Result: r}
		},
	}
}

// NewConfirmModel creates a public confirmation dialog with custom callbacks.
func NewConfirmModel(title, question string, defaultYes bool, onConfirm, onCancel func() tea.Msg) tea.Model {
	return &confirmDialogModel{
		baseDialogModel: baseDialogModel{id: "confirm_dialog", focused: true},
		title:           title,
		question:        question,
		defaultYes:      defaultYes,
		result:          defaultYes,
		onResult: func(r bool) tea.Msg {
			if r && onConfirm != nil {
				return onConfirm()
			}
			if !r && onCancel != nil {
				return onCancel()
			}
			return CloseDialogMsg{Result: r}
		},
	}
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
		m.calculateLayout()
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
			if key.Matches(msg, Keys.Tab) || key.Matches(msg, Keys.ShiftTab) || msg.String() == " " {
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
		// Middle click is handled by AppModel (global Space mapping)
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}

		// Left click on buttons triggers action via onResult so sub-dialog channels work correctly
		if msg.Button == tea.MouseLeft {
			if ButtonIDMatches(msg.ID, "Yes") {
				m.result = true
				m.confirmed = true
				return m, closeWithResult(true)
			}
			if ButtonIDMatches(msg.ID, "No") {
				m.result = false
				m.confirmed = true
				return m, closeWithResult(false)
			}
		}
	}

	// Middle-click activates the currently focused button (Yes or No)
	if _, ok := msg.(ToggleFocusedMsg); ok {
		m.confirmed = true
		return m, closeWithResult(m.result)
	}

	// Scroll wheel selects between Yes (up) and No (down) with clamping — no wrap.
	if wheelMsg, ok := msg.(tea.MouseWheelMsg); ok {
		switch wheelMsg.Button {
		case tea.MouseWheelUp:
			m.result = true // Yes (first option — clamps here on repeated up)
		case tea.MouseWheelDown:
			m.result = false // No (last option — clamps here on repeated down)
		}
		return m, nil
	}

	return m, nil
}

// contentWidth calculates the ideal dialog inner width.
func (m *confirmDialogModel) contentWidth() int {
	maxAllowed := m.layout.Width - 2
	w := maxLineWidth(m.question) + DialogBodyPadH
	if minBtn := lipgloss.Width("Yes") + 4 + lipgloss.Width("No") + 4 + 4 + DialogBodyPadH; minBtn > w {
		w = minBtn
	}
	if tw := lipgloss.Width(GetPlainText(m.title)) + 6; tw > w {
		w = tw
	}
	if w > maxAllowed {
		w = maxAllowed
	}
	return w
}

// ViewString returns the dialog content as a string for compositing
func (m *confirmDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()
	borderBG := ctx.Dialog.GetBackground()
	contentWidth := m.contentWidth()

	// Question text style - inherit from ctx.Dialog to get background
	questionStyle := ctx.Dialog.
		Padding(1, 2).
		Width(contentWidth)

	// Process TUI theme tags (using base dialog style for color resets only),
	// then apply the full questionStyle (padding + width) uniformly to the whole block.
	questionText := questionStyle.Render(RenderThemeText(m.question, ctx.Dialog))

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
	dialogWithTitle := RenderDialogWithType(m.title, fullContent, m.focused, 0, DialogTypeConfirm)

	return dialogWithTitle
}

// View implements tea.Model
func (m *confirmDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *confirmDialogModel) Layers() []*lipgloss.Layer { return m.layers(m.ViewString) }

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *confirmDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	contentWidth := m.contentWidth()

	questionStyle := ctx.Dialog.Padding(1, 2).Width(contentWidth)
	questionHeight := lipgloss.Height(questionStyle.Render(GetPlainText(m.question)))

	// buttonY: border (1) + question with padding + spacer (1)
	buttonY := 1 + questionHeight + 1

	// Use centralized button hit region helper with dialog ID for disambiguation
	// Must include Text to properly calculate button width
	regions := GetButtonHitRegions(
		HelpContext{ScreenName: m.title, PageTitle: "Question", PageText: m.question},
		m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20,
		ButtonSpec{Text: "Yes", ZoneID: "Yes", Help: "Select this option."},
		ButtonSpec{Text: "No", ZoneID: "No", Help: "Select this option."},
	)

	// Dialog background
	regions = append(regions, HitRegion{
		ID:     m.id,
		X:      offsetX,
		Y:      offsetY,
		Width:  contentWidth + 2,
		Height: buttonY + 2, // buttonRow (1) + border (1 more)
		ZOrder: ZDialog,
		Label:  "Confirm",
		Help: &HelpContext{
			ScreenName: m.title,
			PageTitle:  "Question",
			PageText:   m.question,
		},
	})

	return regions
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
