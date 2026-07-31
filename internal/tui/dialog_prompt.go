package tui

import (
	"time"

	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui/components/sinput"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptDialogModel represents a single-line text/password input dialog.
// Built as an outer container displayengine.MenuModel (title, buttons) with the question
// as a plain-text section and the input as a sinput section, matching the
// pattern used by Main Menu/Config Menu/.../Global Flags/Confirm dialog.
type promptDialogModel struct {
	outer           *displayengine.MenuModel
	questionSection *displayengine.MenuModel
	inputSection    *displayengine.MenuModel
	input           *sinput.Model
	result          string
	confirmed       bool
	onResult        func(string, bool) tea.Msg
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

	var inputSection *displayengine.MenuModel
	var inp *sinput.Model
	if sensitive {
		inputSection, inp = displayengine.NewPasswordSinputSection("prompt_dialog_input", "Password", initial)
	} else {
		inputSection, inp = displayengine.NewSinputSection("prompt_dialog_input", "", initial)
	}
	m.inputSection = inputSection
	m.input = inp

	// Keep the INS/OVR bottom-border label live across every keystroke/click
	// by wrapping the section's existing interceptor (already wired by
	// displayengine.NewSinputSection for typing/click/drag/paste/context-menu) rather than
	// replacing it.
	prevInterceptor := inputSection.Interceptor
	updateInsOvrLabel := func() {
		label := "INS"
		if (*inp).IsOverwrite() {
			label = "OVR"
		}
		inputSection.SetBottomBorderLabel(label)
	}
	inputSection.SetUpdateInterceptor(func(msg tea.Msg, menu *displayengine.MenuModel) (tea.Cmd, bool) {
		cmd, handled := prevInterceptor(msg, menu)
		updateInsOvrLabel()
		return cmd, handled
	})
	updateInsOvrLabel()

	outer := displayengine.NewMenuModel("prompt_dialog", title, "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(displayengine.DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "OK", ZoneID: "btn-select", Action: func() tea.Msg {
			m.result = (*inp).Value()
			m.confirmed = true
			return m.onResult(m.result, true)
		}, Help: "Confirm."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg {
			return m.onResult("", false)
		}, Help: "Cancel."},
	})

	questionSection := displayengine.NewPlainTextSection("prompt_dialog_question", question)
	questionSection.SetPlainTextStyle("", 1)
	outer.AddContentSection(questionSection)
	m.questionSection = questionSection
	outer.AddContentSection(inputSection)
	if sensitive {
		disclaimer := displayengine.NewPlainTextSection("prompt_dialog_disclaimer", "(password will not be logged)")
		disclaimer.SetPlainTextStyle("{{|Highlight|}}", 0)
		outer.AddContentSection(disclaimer)
	}

	m.outer = outer
	return m
}

// newPromptDialog creates a new text input dialog
func newPromptDialog(title, question string, sensitive bool, initialValue ...string) *promptDialogModel {
	return newPromptDialogModel(title, question, sensitive, func(res string, confirmed bool) tea.Msg {
		return displayengine.CloseDialogMsg{Result: promptResultMsg{result: res, confirmed: confirmed}}
	}, initialValue...)
}

// Init implements tea.Model
func (m *promptDialogModel) Init() tea.Cmd {
	return tea.Batch(m.outer.Init(), sinput.Blink)
}

// Update implements tea.Model
func (m *promptDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newOuter, cmd := m.outer.Update(msg)
	if outer, ok := newOuter.(*displayengine.MenuModel); ok {
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
// explicit floor -- matching the fixed-width convention used by other small
// non-maximized dialogs (e.g. FlagsToggleDialog, WebDisplayDialog). The
// question section can still grow the dialog past that floor (up to
// whatever room is actually available) when its content -- e.g. a shell
// command on the sudo-password prompt -- needs more room than 60 columns to
// avoid an awkward word-wrap; the input box grows to match automatically
// since SetSize hands every section the same content width.
func (m *promptDialogModel) SetSize(width, height int) {
	dialogWidth := 60
	if m.questionSection != nil {
		layout := displayengine.GetLayout()
		maxAvailable := width - layout.BorderWidth() - layout.ContentMarginWidth()
		if maxAvailable < 1 {
			maxAvailable = 1
		}
		// SectionNaturalWidth measures in content-width scale (excluding
		// border/margin); convert back to outer-width scale to compare
		// against dialogWidth, matching calculateSectionLayout's own
		// natural-width-to-outer-width conversion.
		natural := m.questionSection.SectionNaturalWidth(maxAvailable)
		outerNatural := natural + layout.BorderWidth() + layout.ContentMarginWidth()
		if outerNatural > dialogWidth {
			dialogWidth = outerNatural
		}
	}
	if dialogWidth > width {
		dialogWidth = width
	}
	m.outer.SetSize(dialogWidth, height)
}

// IsMaximized lets the AppModel know its size state
func (m *promptDialogModel) IsMaximized() bool {
	return m.outer.IsMaximized()
}

// SetFocused propagates focus state. ApplySectionFocus must be called after
// SetFocused (see its doc comment) -- without it, the input section never
// receives sub-focus (SetSubFocused), so it doesn't accept keystrokes even
// though the dialog itself is focused.
func (m *promptDialogModel) SetFocused(f bool) {
	m.outer.SetFocused(f)
	m.outer.ApplySectionFocus()
}

// Layers implements LayeredView for compositing
func (m *promptDialogModel) Layers() []*lipgloss.Layer {
	return m.outer.Layers()
}

// GetHitRegions implements displayengine.HitRegionProvider for mouse hit testing
func (m *promptDialogModel) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
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

	layout := displayengine.GetLayout()
	largeTitleOffset := 0
	if m.outer.Layout.LargeTitleBar {
		largeTitleOffset = displayengine.LargeTitleBarOverhead
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

	header := displayengine.NewHeaderModel()
	header.SetWidth(80)
	headerH := header.Height()

	finalDialog, err := RunDialogWithBackdrop(dialog, helpText, displayengine.GetPositionCenter(headerH))
	if err != nil {
		return "", false
	}

	return finalDialog.result, finalDialog.confirmed
}
