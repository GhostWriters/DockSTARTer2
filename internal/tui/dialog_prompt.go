package tui

import (
	"time"

	"DockSTARTer2/internal/tui/components/sinput"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptDialogModel represents a single-line text/password input dialog.
// Built as an outer container MenuModel (title, buttons) with the question
// as a plain-text section and the input as a sinput section, matching the
// pattern used by Main Menu/Config Menu/.../Global Flags/Confirm dialog.
type promptDialogModel struct {
	outer        *MenuModel
	inputSection *MenuModel
	input        *sinput.Model
	result       string
	confirmed    bool
	onResult     func(string, bool) tea.Msg
}

type promptResultMsg struct {
	result    string
	confirmed bool
}

func newPromptDialogModel(title, question string, sensitive bool, onResult func(string, bool) tea.Msg, initialValue ...string) *promptDialogModel {
	initial := ""
	if len(initialValue) > 0 {
		initial = initialValue[0]
	}

	m := &promptDialogModel{onResult: onResult}

	var inputSection *MenuModel
	var inp *sinput.Model
	if sensitive {
		inputSection, inp = NewPasswordSinputSection("prompt_dialog_input", "", initial)
	} else {
		inputSection, inp = NewSinputSection("prompt_dialog_input", "", initial)
	}
	m.inputSection = inputSection
	m.input = inp

	// Keep the INS/OVR bottom-border label live across every keystroke/click
	// by wrapping the section's existing interceptor (already wired by
	// NewSinputSection for typing/click/drag/paste/context-menu) rather than
	// replacing it.
	prevInterceptor := inputSection.interceptor
	updateInsOvrLabel := func() {
		label := "INS"
		if (*inp).IsOverwrite() {
			label = "OVR"
		}
		inputSection.SetBottomBorderLabel(label)
	}
	inputSection.SetUpdateInterceptor(func(msg tea.Msg, menu *MenuModel) (tea.Cmd, bool) {
		cmd, handled := prevInterceptor(msg, menu)
		updateInsOvrLabel()
		return cmd, handled
	})
	updateInsOvrLabel()

	outer := NewMenuModel("prompt_dialog", title, "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetButtons([]ButtonDef{
		{Label: "OK", ZoneID: "btn-select", Action: func() tea.Msg {
			m.result = (*inp).Value()
			m.confirmed = true
			return m.onResult(m.result, true)
		}, Help: "Confirm."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg {
			return m.onResult("", false)
		}, Help: "Cancel."},
	})

	questionSection := NewPlainTextSection("prompt_dialog_question", question)
	questionSection.SetPlainTextStyle("", 1)
	outer.AddContentSection(questionSection)
	outer.AddContentSection(inputSection)
	if sensitive {
		disclaimer := NewPlainTextSection("prompt_dialog_disclaimer", "(password will not be logged)")
		disclaimer.SetPlainTextStyle("{{|Highlight|}}", 0)
		outer.AddContentSection(disclaimer)
	}

	m.outer = outer
	return m
}

// newPromptDialog creates a new text input dialog
func newPromptDialog(title, question string, sensitive bool, initialValue ...string) *promptDialogModel {
	return newPromptDialogModel(title, question, sensitive, func(res string, confirmed bool) tea.Msg {
		return CloseDialogMsg{Result: promptResultMsg{result: res, confirmed: confirmed}}
	}, initialValue...)
}

// Init implements tea.Model
func (m *promptDialogModel) Init() tea.Cmd {
	return tea.Batch(m.outer.Init(), sinput.Blink)
}

// Update implements tea.Model
func (m *promptDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newOuter, cmd := m.outer.Update(msg)
	if outer, ok := newOuter.(*MenuModel); ok {
		m.outer = outer
	}
	return m, cmd
}

// View implements tea.Model
func (m *promptDialogModel) View() tea.View {
	return m.outer.View()
}

// ViewString implements ViewStringer for overlay compositing
func (m *promptDialogModel) ViewString() string {
	return m.outer.ViewString()
}

// SetSize implements sizing. The sinput section has no narrower natural
// width than whatever it's given (SectionNaturalWidth always returns
// maxWidth for it), so unlike plain-text-only dialogs this one needs an
// explicit cap -- matching the fixed-width convention used by other small
// non-maximized dialogs (e.g. FlagsToggleDialog, WebDisplayDialog).
func (m *promptDialogModel) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	m.outer.SetSize(width, height)
}

// IsMaximized lets the AppModel know its size state
func (m *promptDialogModel) IsMaximized() bool {
	return m.outer.IsMaximized()
}

// SetFocused propagates focus state
func (m *promptDialogModel) SetFocused(f bool) {
	m.outer.SetFocused(f)
}

// Layers implements LayeredView for compositing
func (m *promptDialogModel) Layers() []*lipgloss.Layer {
	return m.outer.Layers()
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *promptDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	return m.outer.GetHitRegions(offsetX, offsetY)
}

// IsScrollbarDragging contributes to the sbDragger interface for mouse motion forwarding
func (m *promptDialogModel) IsScrollbarDragging() bool {
	return m.outer.IsScrollbarDragging()
}

// HelpText returns help info
func (m *promptDialogModel) HelpText() string {
	return m.outer.HelpText()
}

// AdvanceSpinners advances any active button spinner.
func (m *promptDialogModel) AdvanceSpinners(now time.Time) bool {
	return m.outer.AdvanceSpinners(now)
}

// GetInputCursor returns the cursor position (relative to dialog top-left),
// cursor shape, and whether the cursor should be shown.
// Implements InputCursorProvider for AppModel.View().
func (m *promptDialogModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	sections := m.outer.GetContentSections()
	inputIdx := -1
	for i, sec := range sections {
		if sec == m.inputSection {
			inputIdx = i
			break
		}
	}
	if inputIdx < 0 || m.outer.GetFocusedSection() != inputIdx {
		return 0, 0, tea.CursorBar, false
	}

	layout := GetLayout()
	largeTitleOffset := 0
	if m.outer.layout.LargeTitleBar {
		largeTitleOffset = LargeTitleBarOverhead
	}

	contentWidth := m.outer.Width() - layout.BorderWidth() - layout.ContentMarginWidth()
	relY = layout.SingleBorder() + largeTitleOffset
	for i := 0; i < inputIdx; i++ {
		relY += sections[i].SectionHeight(contentWidth)
	}
	relY += layout.SingleBorder()

	relX = layout.SingleBorder() + layout.SingleMargin() + (*m.input).PromptWidth() + (*m.input).CursorColumn()
	if (*m.input).IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

// ShowPromptDialog displays a prompt dialog and returns the text and confirmed bool.
func ShowPromptDialog(title, question string, sensitive bool, initialValue ...string) (string, bool) {
	helpText := "Type to input | Tab to switch | Enter to confirm | Esc to cancel"
	dialog := newPromptDialog(title, question, sensitive, initialValue...)

	header := NewHeaderModel()
	header.SetWidth(80)
	headerH := header.Height()

	finalDialog, err := RunDialogWithBackdrop(dialog, helpText, GetPositionCenter(headerH))
	if err != nil {
		return "", false
	}

	return finalDialog.result, finalDialog.confirmed
}
