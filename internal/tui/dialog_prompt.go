package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/theme"
	"strings"

	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptDialogModel represents a text input prompt dialog
type promptDialogModel struct {
	baseDialogModel
	title        string
	question     string
	input        sinput.Model
	inputScreenX int
	inputRelY    int // row of the input text within the dialog (for hardware cursor)
	result       string
	confirmed    bool
	onResult     func(string, bool) tea.Msg
	focusedItem  FocusItem // FocusList=Input, FocusSelectBtn=OK, FocusBackBtn=Cancel
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
		baseDialogModel: baseDialogModel{id: "prompt_dialog", focused: true},
		title:           title,
		question:        question,
		input:           sinput.New(ti),
		focusedItem:     FocusList,
		onResult: func(res string, val bool) tea.Msg {
			return CloseDialogMsg{Result: promptResultMsg{result: res, confirmed: val}}
		},
	}
}

func (m *promptDialogModel) Init() tea.Cmd {
	return sinput.Blink
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

		case key.Matches(msg, Keys.CycleTab):
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

		case key.Matches(msg, Keys.CycleShiftTab):
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
			switch m.focusedItem {
			case FocusSelectBtn:
				m.focusedItem = FocusBackBtn
				return m, nil
			case FocusBackBtn:
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

	case tea.MouseMotionMsg:
		if m.input.IsSelecting() {
			m.input.HandleDragTo(msg.X)
		}
		return m, nil

	case tea.MouseReleaseMsg:
		m.input.EndDrag()
		return m, nil

	case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case LayerHitMsg:
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}
		if msg.Button == tea.MouseRight && ButtonIDMatches(msg.ID, "prompt_input") {
			return m, ShowInputContextMenu(m.input, msg.X, msg.Y, m.width, m.height)
		}
		if msg.Button == tea.MouseLeft {
			if ButtonIDMatches(msg.ID, "OK") {
				m.result = m.input.Value()
				m.confirmed = true
				return m, closeWithResult(m.result, true)
			}
			if ButtonIDMatches(msg.ID, "Cancel") {
				return m, closeWithResult("", false)
			}
			if ButtonIDMatches(msg.ID, "prompt_input") {
				m.focusedItem = FocusList
				m.input.Focus()
				m.input.HandleClick(msg.X)
				return m, nil
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
func (m *promptDialogModel) contentWidth() int {
	maxAllowed := m.layout.Width - 2
	w := maxLineWidth(m.question) + DialogBodyPadH

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

// GetInputCursor returns the cursor position (relative to dialog top-left),
// cursor shape, and whether the cursor should be shown.
// Implements InputCursorProvider for AppModel.View().
func (m *promptDialogModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if m.focusedItem != FocusList || !m.input.Focused() {
		return 0, 0, tea.CursorBar, false
	}
	// 1 outer_left + 1 inner_left + 1 pad_left + cursor column within textinput view
	relX = 3 + m.input.CursorColumn()
	relY = m.inputRelY
	if m.input.IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

func (m *promptDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()
	contentWidth := m.contentWidth()

	// Question text — matches dialog_confirm.go: Padding(1, 2), Width(contentWidth)
	questionText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentWidth).Render(console.Sprintf("%s", m.question)),
		"\n")

	// Input field with inner border
	inputFocused := m.focusedItem == FocusList
	borderBG := ctx.Dialog.GetBackground()
	sInnerW := contentWidth - 2
	if sInnerW < 1 {
		sInnerW = 1
	}

	inputContent := strings.TrimRight(ctx.Dialog.Padding(0, 1).Width(sInnerW).Render(m.input.View()), "\n")
	renderedInput := strings.TrimRight(RenderBorderedBoxCtx(
		"", inputContent, sInnerW, 0, inputFocused, true, true,
		ctx.SubmenuTitleAlign, "", ctx,
	), "\n")

	// Inject INS/OVR label into the bottom-left of the input box border.
	modeLabel := "INS"
	if m.input.IsOverwrite() {
		modeLabel = "OVR"
	}
	inputLines := strings.Split(renderedInput, "\n")
	if len(inputLines) > 0 {
		inputLines[len(inputLines)-1] = BuildLabeledBottomBorderCtx(contentWidth, modeLabel, inputFocused, ctx)
		renderedInput = strings.Join(inputLines, "\n")
	}

	// Disclaimer (only for sensitive/password prompts)
	var disclaimerText string
	if m.input.EchoMode == textinput.EchoPassword {
		disclaimerText = strings.TrimRight(
			ctx.Dialog.Padding(0, 2).
				Foreground(theme.ThemeSemanticStyle("{{|Highlight|}}").GetForeground()).
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

func (m *promptDialogModel) Layers() []*lipgloss.Layer { return m.layers(m.ViewString) }

func (m *promptDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	contentWidth := m.contentWidth()

	questionHeight := lipgloss.Height(
		ctx.Dialog.Padding(1, 2).Width(contentWidth).Render(m.question))

	inputFocused := m.focusedItem == FocusList
	borderedInputStyle := ApplyInnerBorderCtx(
		ctx.Dialog.Padding(0, 1).Width(contentWidth),
		inputFocused, ctx)
	inputHeight := lipgloss.Height(borderedInputStyle.Render(m.input.View()))

	disclaimerHeight := 0
	if m.input.EchoMode == textinput.EchoPassword {
		disclaimerHeight = lipgloss.Height(
			ctx.Dialog.Padding(0, 2).Width(contentWidth).Render("(password will not be logged)"))
	}

	// Input hit region: outer_border(1) + questionH + inner_border_top(1) = input text row
	// Text X: outer_border(1) + inner_border(1) + padding(1) + promptW
	inputTextY := 1 + questionHeight + 1
	m.inputRelY = inputTextY
	m.inputScreenX = offsetX + 1 + 1 + 1 + m.input.PromptWidth()
	m.input.SetScreenTextX(m.inputScreenX)

	// buttonY: outer border (1) + question + input + disclaimer + spacer (1)
	buttonY := 1 + questionHeight + inputHeight + disclaimerHeight + 1

	regions := []HitRegion{{
		ID:     "prompt_input",
		X:      offsetX + 1,
		Y:      offsetY + inputTextY,
		Width:  contentWidth,
		Height: 1,
		ZOrder: ZDialog + 10,
		Label:  "Input Field",
		Help: &HelpContext{
			ScreenName: "Prompt",
			PageTitle:  "Editing",
			PageText:   m.question,
			ItemText:   "Type your response and press Enter to confirm, or Esc to cancel.",
		},
	}}

	// Dialog background
	regions = append(regions, HitRegion{
		ID:     m.id,
		X:      offsetX,
		Y:      offsetY,
		Width:  contentWidth + 2,
		Height: buttonY + 2,
		ZOrder: ZDialog,
		Label:  "Prompt",
		Help: &HelpContext{
			ScreenName: m.title,
			PageTitle:  "Input Prompt",
			PageText:   m.question,
		},
	})

	regions = append(regions, GetButtonHitRegions(
		HelpContext{ScreenName: m.title, PageTitle: "Input Prompt", PageText: m.question},
		m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20,
		ButtonSpec{Text: "OK", ZoneID: "OK", Help: "Save changes and return."},
		ButtonSpec{Text: "Cancel", ZoneID: "Cancel", Help: "Discard changes and return."},
	)...)
	return regions
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
