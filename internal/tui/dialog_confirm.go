package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// confirmDialogModel represents a yes/no confirmation dialog. Built as an
// outer container MenuModel (title, buttons) with the question as a single
// plain-text content section, matching the pattern used by Main Menu/Config
// Menu/Options Menu/Config Apps Menu/Global Flags.
type confirmDialogModel struct {
	outer     *MenuModel
	result    bool
	confirmed bool
	onResult  func(bool) tea.Msg
}

func newConfirmDialogModel(title, question string, defaultYes bool, onResult func(bool) tea.Msg) *confirmDialogModel {
	m := &confirmDialogModel{
		result:   defaultYes,
		onResult: onResult,
	}

	// confirm_dialog_question deliberately avoids being a substring of
	// "confirm_dialog" the other way around -- MatchesID uses
	// strings.Contains, so distinct, non-overlapping ids are safe by
	// construction (learned from the Global Flags ID-collision bug).
	outer := NewMenuModel("confirm_dialog", title, "", nil)
	outer.SetMaximized(false) // grow to fit, matching original behavior
	outer.SetIsDialog(true)
	outer.SetDialogType(DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetButtons([]ButtonDef{
		{Label: "Yes", ZoneID: "btn-yes", Action: func() tea.Msg {
			m.result = true
			m.confirmed = true
			return m.onResult(true)
		}, Help: "Confirm."},
		{Label: "No", ZoneID: "btn-cancel", Action: func() tea.Msg {
			m.result = false
			m.confirmed = true
			return m.onResult(false)
		}, Help: "Cancel."},
	})
	if defaultYes {
		outer.SetFocusedBtnIndex(0)
	} else {
		outer.SetFocusedBtnIndex(1)
	}
	questionSection := NewPlainTextSection("confirm_dialog_question", question)
	questionSection.SetPlainTextStyle("", 1)
	outer.AddContentSection(questionSection)

	m.outer = outer
	return m
}

// newConfirmDialog creates a new confirmation dialog
func newConfirmDialog(title, question string, defaultYes bool) *confirmDialogModel {
	return newConfirmDialogModel(title, question, defaultYes, func(r bool) tea.Msg {
		return CloseDialogMsg{Result: r}
	})
}

// NewConfirmModel creates a public confirmation dialog with custom callbacks.
func NewConfirmModel(title, question string, defaultYes bool, onConfirm, onCancel func() tea.Msg) tea.Model {
	return newConfirmDialogModel(title, question, defaultYes, func(r bool) tea.Msg {
		if r && onConfirm != nil {
			return onConfirm()
		}
		if !r && onCancel != nil {
			return onCancel()
		}
		return CloseDialogMsg{Result: r}
	})
}

// Init implements tea.Model
func (m *confirmDialogModel) Init() tea.Cmd {
	return m.outer.Init()
}

// Update implements tea.Model
func (m *confirmDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newOuter, cmd := m.outer.Update(msg)
	if outer, ok := newOuter.(*MenuModel); ok {
		m.outer = outer
	}
	return m, cmd
}

// View implements tea.Model
func (m *confirmDialogModel) View() tea.View {
	return m.outer.View()
}

// ViewString implements ViewStringer for overlay compositing
func (m *confirmDialogModel) ViewString() string {
	return m.outer.ViewString()
}

// SetSize implements sizing
func (m *confirmDialogModel) SetSize(width, height int) {
	m.outer.SetSize(width, height)
}

// IsMaximized lets the AppModel know its size state
func (m *confirmDialogModel) IsMaximized() bool {
	return m.outer.IsMaximized()
}

// SetFocused propagates focus state
func (m *confirmDialogModel) SetFocused(f bool) {
	m.outer.SetFocused(f)
}

// Layers implements LayeredView for compositing
func (m *confirmDialogModel) Layers() []*lipgloss.Layer {
	return m.outer.Layers()
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *confirmDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	return m.outer.GetHitRegions(offsetX, offsetY)
}

// IsScrollbarDragging contributes to the sbDragger interface for mouse motion forwarding
func (m *confirmDialogModel) IsScrollbarDragging() bool {
	return m.outer.IsScrollbarDragging()
}

// HelpText returns help info
func (m *confirmDialogModel) HelpText() string {
	return m.outer.HelpText()
}

// AdvanceSpinners advances any active button spinner.
func (m *confirmDialogModel) AdvanceSpinners(now time.Time) bool {
	return m.outer.AdvanceSpinners(now)
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
