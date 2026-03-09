package tui

import (
	"strings"

	"DockSTARTer2/internal/console"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// choiceDialogModel is a multi-button choice dialog.
// onResult receives the chosen button index (0-based), or -1 if cancelled via Esc.
type choiceDialogModel struct {
	baseDialogModel
	title    string
	question string
	choices  []string
	focused  int
	onResult func(int) tea.Msg
}

func newChoiceDialog(title, question string, choices []string) *choiceDialogModel {
	return &choiceDialogModel{
		baseDialogModel: baseDialogModel{id: "choice_dialog", focused: true},
		title:           title,
		question:        question,
		choices:         choices,
		focused:         0,
		onResult:        func(i int) tea.Msg { return CloseDialogMsg{Result: i} },
	}
}

func (m *choiceDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	submit := func(idx int) tea.Cmd {
		return func() tea.Msg { return m.onResult(idx) }
	}

	n := len(m.choices)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Esc):
			return m, submit(-1)
		case key.Matches(msg, Keys.Enter):
			return m, submit(m.focused)
		case key.Matches(msg, Keys.Left), key.Matches(msg, Keys.Up):
			if m.focused > 0 {
				m.focused--
			}
		case key.Matches(msg, Keys.Right), key.Matches(msg, Keys.Down):
			if m.focused < n-1 {
				m.focused++
			}
		case key.Matches(msg, Keys.Tab):
			m.focused = (m.focused + 1) % n
		case key.Matches(msg, Keys.ShiftTab):
			m.focused = (m.focused + n - 1) % n
		default:
			// First letter of each choice as hotkey
			k := strings.ToLower(msg.String())
			for i, choice := range m.choices {
				if len(choice) > 0 && strings.ToLower(string(choice[0])) == k {
					return m, submit(i)
				}
			}
		}

	case LayerHitMsg:
		if msg.Button == tea.MouseLeft {
			for i, choice := range m.choices {
				if buttonIDMatches(msg.ID, choice) {
					return m, submit(i)
				}
			}
		}
	}

	if _, ok := msg.(ToggleFocusedMsg); ok {
		return m, submit(m.focused)
	}

	if wheelMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if wheelMsg.Button == tea.MouseWheelUp && m.focused > 0 {
			m.focused--
		} else if wheelMsg.Button == tea.MouseWheelDown && m.focused < n-1 {
			m.focused++
		}
		return m, nil
	}

	return m, nil
}

func (m *choiceDialogModel) contentWidth() int {
	maxAllowed := m.layout.Width - 2
	w := maxLineWidth(m.question) + DialogBodyPadH

	// Minimum to fit all buttons with spacing
	minBtn := 4
	for _, c := range m.choices {
		minBtn += lipgloss.Width(c) + 4
	}
	if minBtn > w {
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

func (m *choiceDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()
	borderBG := ctx.Dialog.GetBackground()
	contentWidth := m.contentWidth()

	questionStyle := ctx.Dialog.Padding(1, 2).Width(contentWidth)
	questionText := strings.TrimRight(questionStyle.Render(console.Sprintf("%s", m.question)), "\n")

	specs := make([]ButtonSpec, len(m.choices))
	for i, c := range m.choices {
		specs[i] = ButtonSpec{Text: c, Active: i == m.focused}
	}
	buttonRow := strings.TrimRight(RenderCenteredButtonsCtx(contentWidth, ctx, specs...), "\n")

	spacer := lipgloss.NewStyle().Width(contentWidth).Background(borderBG).Render("")
	fullContent := lipgloss.JoinVertical(lipgloss.Left, questionText, spacer, buttonRow)

	return RenderDialogWithType(m.title, fullContent, m.focused >= 0, 0, DialogTypeConfirm)
}

func (m *choiceDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *choiceDialogModel) Layers() []*lipgloss.Layer { return m.layers(m.ViewString) }

func (m *choiceDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	contentWidth := m.contentWidth()

	questionStyle := ctx.Dialog.Padding(1, 2).Width(contentWidth)
	questionHeight := lipgloss.Height(questionStyle.Render(console.Sprintf("%s", m.question)))
	buttonY := 1 + questionHeight + 1

	specs := make([]ButtonSpec, len(m.choices))
	for i, c := range m.choices {
		specs[i] = ButtonSpec{Text: c, ZoneID: c}
	}
	return GetButtonHitRegions(m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20, specs...)
}
