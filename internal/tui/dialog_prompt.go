package tui

import (
	"DockSTARTer2/internal/console"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptDialogModel represents a text input prompt dialog
type promptDialogModel struct {
	title       string
	question    string
	input       textinput.Model
	result      string
	confirmed   bool
	width       int
	height      int
	onResult    func(string, bool) tea.Msg
	focused     bool
	focusedItem FocusItem // FocusList=Input, FocusSelectBtn=OK, FocusBackBtn=Cancel

	// Unified layout
	layout DialogLayout
	id     string
}

type promptResultMsg struct {
	result    string
	confirmed bool
}

// newPromptDialog creates a new text input dialog
func newPromptDialog(title, question string, sensitive bool) *promptDialogModel {
	ti := textinput.New()
	if sensitive {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	ti.Focus()
	ti.CharLimit = 156

	// Apply theme-consistent styles to the textinput, inheriting the dialog background
	styles := GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	ti.SetStyles(tiStyles)

	return &promptDialogModel{
		id:          "prompt_dialog",
		title:       title,
		question:    question,
		input:       ti,
		focused:     true,
		focusedItem: FocusList,
		onResult: func(res string, val bool) tea.Msg {
			return CloseDialogMsg{Result: promptResultMsg{result: res, confirmed: val}}
		},
	}
}

func (m *promptDialogModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *promptDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	closeWithResult := func(val string, confirmed bool) tea.Cmd {
		return func() tea.Msg { return m.onResult(val, confirmed) }
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Esc), key.Matches(msg, Keys.ForceQuit):
			return m, closeWithResult("", false)

		case key.Matches(msg, Keys.Tab):
			// Cycle: Input -> OK -> Cancel -> Input
			switch m.focusedItem {
			case FocusList:
				m.focusedItem = FocusSelectBtn
				m.input.Blur()
			case FocusSelectBtn:
				m.focusedItem = FocusBackBtn
			case FocusBackBtn:
				m.focusedItem = FocusList
				m.input.Focus()
			default:
				m.focusedItem = FocusList
				m.input.Focus()
			}
			return m, nil

		case key.Matches(msg, Keys.ShiftTab):
			// Reverse cycle: Input -> Cancel -> OK -> Input
			switch m.focusedItem {
			case FocusList:
				m.focusedItem = FocusBackBtn
				m.input.Blur()
			case FocusBackBtn:
				m.focusedItem = FocusSelectBtn
			case FocusSelectBtn:
				m.focusedItem = FocusList
				m.input.Focus()
			default:
				m.focusedItem = FocusList
				m.input.Focus()
			}
			return m, nil

		case key.Matches(msg, Keys.Left), key.Matches(msg, Keys.Right):
			// Toggle between OK and Cancel when on the button row
			if m.focusedItem == FocusSelectBtn {
				m.focusedItem = FocusBackBtn
				return m, nil
			} else if m.focusedItem == FocusBackBtn {
				m.focusedItem = FocusSelectBtn
				return m, nil
			}

		case key.Matches(msg, Keys.Enter):
			if m.focusedItem == FocusBackBtn {
				return m, closeWithResult("", false)
			}
			// OK or Enter directly on input
			m.result = m.input.Value()
			m.confirmed = true
			return m, closeWithResult(m.result, true)
		}

		// Handle button hotkeys when not on the input
		if m.focusedItem != FocusList {
			buttons := []ButtonSpec{{Text: "OK"}, {Text: "Cancel"}}
			if idx, found := CheckButtonHotkeys(msg, buttons); found {
				if idx == 0 {
					m.result = m.input.Value()
					m.confirmed = true
					return m, closeWithResult(m.result, true)
				}
				return m, closeWithResult("", false)
			}
		}

	case LayerHitMsg:
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}
		if msg.Button == tea.MouseLeft {
			if strings.HasSuffix(msg.ID, ".OK") || msg.ID == "Button.OK" {
				m.result = m.input.Value()
				m.confirmed = true
				return m, closeWithResult(m.result, true)
			}
			if strings.HasSuffix(msg.ID, ".Cancel") || msg.ID == "Button.Cancel" {
				return m, closeWithResult("", false)
			}
		}
	}

	// Route remaining messages (e.g. typing) to the textinput
	if m.focusedItem == FocusList {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// contentWidth calculates the ideal dialog inner width.
func (m *promptDialogModel) contentWidth(ctx StyleContext) int {
	maxAllowed := m.layout.Width - 2

	// Start from the question text width — same as confirm dialog: +4 for Padding(1,2)
	stripped := GetPlainText(m.question)
	maxQ := 0
	for _, line := range strings.Split(stripped, "\n") {
		if w := lipgloss.Width(line); w > maxQ {
			maxQ = w
		}
	}
	w := maxQ + DialogBodyPadH

	// Input field: same Padding(0,1) so same +4 for the inner border
	if iw := lipgloss.Width(m.input.View()) + 4; iw > w {
		w = iw
	}

	// Buttons
	minBtn := lipgloss.Width("OK") + 4 + lipgloss.Width("Cancel") + 4 + 4
	if minBtn > w {
		w = minBtn
	}

	// Title
	if tw := lipgloss.Width(GetPlainText(m.title)) + 6; tw > w {
		w = tw
	}

	if w > maxAllowed {
		w = maxAllowed
	}
	return w
}

func (m *promptDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()
	contentWidth := m.contentWidth(ctx)

	// Question text — matches dialog_confirm.go: Padding(1, 2), Width(contentWidth)
	questionText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentWidth).Render(console.Sprintf("%s", m.question)),
		"\n")

	// Input field with inner border — use same Padding(0, 1).Width(contentWidth) as base
	// so total row width matches questionText. ApplyInnerBorderCtx adds the border on top.
	inputFocused := m.focusedItem == FocusList
	borderBG := ctx.Dialog.GetBackground()
	borderedInputStyle := ApplyInnerBorderCtx(
		ctx.Dialog.Padding(0, 1).Width(contentWidth),
		inputFocused, ctx)
	renderedInput := strings.TrimRight(borderedInputStyle.Render(m.input.View()), "\n")

	// Disclaimer (only for sensitive/password prompts)
	var disclaimerText string
	if m.input.EchoMode == textinput.EchoPassword {
		disclaimerText = strings.TrimRight(
			ctx.Dialog.Padding(0, 2).
				Foreground(SemanticStyle("{{|Theme_Highlight|}}").GetForeground()).
				Width(contentWidth).
				Render(console.Sprintf("(password will not be logged)")),
			"\n")
	}

	// Spacer + buttons (same pattern as dialog_confirm.go)
	spacer := lipgloss.NewStyle().Width(contentWidth).Background(borderBG).Render("")
	buttonRow := strings.TrimRight(RenderCenteredButtonsCtx(
		contentWidth, ctx,
		ButtonSpec{Text: "OK", Active: m.focusedItem == FocusList || m.focusedItem == FocusSelectBtn},
		ButtonSpec{Text: "Cancel", Active: m.focusedItem == FocusBackBtn},
	), "\n")

	// Assemble
	parts := []string{questionText, renderedInput}
	if disclaimerText != "" {
		parts = append(parts, disclaimerText)
	}
	parts = append(parts, spacer, buttonRow)

	return RenderDialogWithType(m.title, lipgloss.JoinVertical(lipgloss.Left, parts...), m.focused, 0, DialogTypeConfirm)
}

func (m *promptDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *promptDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZDialog).ID(m.id),
	}
}

func (m *promptDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	maxAllowed := m.layout.Width - 2

	questionHeight := lipgloss.Height(
		ctx.Dialog.Padding(1, 2).Width(maxAllowed).Render(m.question))

	inputFocused := m.focusedItem == FocusList
	borderedInputStyle := ApplyInnerBorderCtx(
		ctx.Dialog.Padding(0, 1).Width(maxAllowed),
		inputFocused, ctx)
	inputHeight := lipgloss.Height(borderedInputStyle.Render(m.input.View()))

	disclaimerHeight := 0
	if m.input.EchoMode == textinput.EchoPassword {
		disclaimerHeight = lipgloss.Height(
			ctx.Dialog.Padding(0, 2).Width(maxAllowed).Render("(password will not be logged)"))
	}

	// buttonY: outer border (1) + question + input + disclaimer + spacer (1)
	buttonY := 1 + questionHeight + inputHeight + disclaimerHeight + 1

	return GetButtonHitRegions(
		m.id, offsetX+1, offsetY+buttonY, maxAllowed, ZDialog+20,
		ButtonSpec{Text: "OK", ZoneID: "OK"},
		ButtonSpec{Text: "Cancel", ZoneID: "Cancel"},
	)
}

func (m *promptDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *promptDialogModel) SetFocused(f bool) {
	m.focused = f
}

func (m *promptDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.layout = newStandardDialogLayout(m.width, m.height)
}

// ShowPromptDialog displays a prompt dialog and returns the text and confirmed bool.
func ShowPromptDialog(title, question string, sensitive bool) (string, bool) {
	helpText := "Type to input | Tab to switch | Enter to confirm | Esc to cancel"
	dialog := newPromptDialog(title, question, sensitive)

	header := NewHeaderModel()
	header.SetWidth(80)
	headerH := header.Height()

	finalDialog, err := RunDialogWithBackdrop(dialog, helpText, GetPositionCenter(headerH))
	if err != nil {
		return "", false
	}

	return finalDialog.result, finalDialog.confirmed
}

// PromptText displays a blocking prompt dialog over the active ProgramBox.
// It is used by the console package via callback to ask for text during background tasks.
func PromptText(title, question string, sensitive bool) (string, error) {
	if program == nil {
		return "", console.ErrUserAborted
	}

	ch := make(chan promptResultMsg)
	program.Send(ShowPromptDialogMsg{
		Title:      title,
		Question:   question,
		Sensitive:  sensitive,
		ResultChan: ch,
	})

	result := <-ch
	if !result.confirmed {
		return "", console.ErrUserAborted
	}
	return result.result, nil
}
