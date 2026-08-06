package tui

import (
	"DockSTARTer2/internal/displayengine"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// MessageType represents the type of message dialog
type MessageType int

const (
	MessageInfo MessageType = iota
	MessageSuccess
	MessageWarning
	MessageError
)

// messageDialogModel represents a message dialog. Built as an outer
// container displayengine.MenuModel (title, OK button) with the message as a plain-text
// section, matching the pattern used by Main Menu/.../Confirm/Prompt dialogs.
type messageDialogModel struct {
	outer *displayengine.MenuModel
}

func dialogTypeForMessage(msgType MessageType) displayengine.DialogType {
	switch msgType {
	case MessageSuccess:
		return displayengine.DialogTypeSuccess
	case MessageWarning:
		return displayengine.DialogTypeWarning
	case MessageError:
		return displayengine.DialogTypeError
	default:
		return displayengine.DialogTypeInfo
	}
}

// messageThemeTag returns the plain-text theme tag for the message body.
// All message types render with plain (untagged) text -- the border/title
// color from dialogTypeForMessage is what distinguishes severity.
func messageThemeTag(_ MessageType) string {
	return ""
}

// newMessageDialog creates a new message dialog
func newMessageDialog(title, message string, msgType MessageType) *messageDialogModel {
	outer := displayengine.NewMenuModel("message_dialog", title, "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(dialogTypeForMessage(msgType))
	outer.SetBorderStyle(displayengine.BorderStyleRounded)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: " OK ", ZoneID: "btn-select", Action: func() tea.Msg { return displayengine.CloseDialogMsg{Result: true} }, Help: "Dismiss this message."},
	})
	outer.SetEscAction(func() tea.Msg { return displayengine.CloseDialogMsg{Result: true} })

	messageSection := displayengine.NewPlainTextSection("message_dialog_text", message)
	messageSection.SetPlainTextStyle(messageThemeTag(msgType), 1)
	outer.AddContentSection(messageSection)

	return &messageDialogModel{outer: outer}
}

// Init implements tea.Model
func (m *messageDialogModel) Init() tea.Cmd {
	return m.outer.Init()
}

// Update implements tea.Model
func (m *messageDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Any key press closes the dialog, matching the original's "press any
	// key to continue" behavior -- displayengine.MenuModel's own key handling only closes
	// on Enter/Esc/hotkeys, so intercept KeyPressMsg here first. Skip the
	// intercept while the title bar [?]/[x] widgets have focus so Tab/Enter/
	// arrow navigation there still works via the outer's own handling.
	if _, ok := msg.(tea.KeyPressMsg); ok && !m.outer.TitleBarFocused() {
		return m, m.outer.SetProcessingBtnDeferred("btn-select", func() tea.Msg { return displayengine.CloseDialogMsg{Result: true} })
	}

	newOuter, cmd := m.outer.Update(msg)
	if outer, ok := newOuter.(*displayengine.MenuModel); ok {
		m.outer = outer
	}
	return m, cmd
}

// View implements tea.Model
func (m *messageDialogModel) View() tea.View {
	return m.outer.View()
}

// ViewString implements ViewStringer for overlay compositing
func (m *messageDialogModel) ViewString() string {
	return m.outer.ViewString()
}

// SetSize implements sizing
func (m *messageDialogModel) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	m.outer.SetSize(width, height)
}

// IsMaximized lets the AppModel know its size state
func (m *messageDialogModel) IsMaximized() bool {
	return m.outer.IsMaximized()
}

// SetFocused propagates focus state
func (m *messageDialogModel) SetFocused(f bool) {
	m.outer.SetFocused(f)
}

// Layers implements LayeredView for compositing
func (m *messageDialogModel) Layers() []*lipgloss.Layer {
	return m.outer.Layers()
}

// GetHitRegions implements displayengine.HitRegionProvider for mouse hit testing
func (m *messageDialogModel) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	return m.outer.GetHitRegions(offsetX, offsetY)
}

// IsScrollbarDragging contributes to the sbDragger interface for mouse motion forwarding
func (m *messageDialogModel) IsScrollbarDragging() bool {
	return m.outer.IsScrollbarDragging()
}

// HelpText returns help info
func (m *messageDialogModel) HelpText() string {
	return m.outer.HelpText()
}

// AdvanceSpinners advances any active button spinner.
func (m *messageDialogModel) AdvanceSpinners(now time.Time) bool {
	return m.outer.AdvanceSpinners(now)
}

