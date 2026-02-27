package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// DialogModel is the interface that dialogs must implement to use the generic wrapper
type DialogModel interface {
	tea.Model
	ViewString() string
	SetSize(width, height int)
}

// DynamicHelpProvider is an optional interface for dialogs that provide dynamic help text
type DynamicHelpProvider interface {
	GetHelpText() string
}

// DialogPosition specifies where to position the dialog on the backdrop
type DialogPosition struct {
	H       OverlayPosition
	V       OverlayPosition
	XOffset int
	YOffset int
}

// GetPositionCenter returns a centered dialog position using Layout helpers
func GetPositionCenter(headerH int) DialogPosition {
	layout := GetLayout()
	return DialogPosition{OverlayCenter, OverlayTop, 0, layout.ContentStartY(headerH)}
}

// GetPositionTopLeft returns top-left position for maximized dialogs using Layout helpers
func GetPositionTopLeft(headerH int) DialogPosition {
	layout := GetLayout()
	return DialogPosition{OverlayLeft, OverlayTop, layout.EdgeIndent, layout.ContentStartY(headerH)}
}

// DialogWithBackdrop wraps any DialogModel with the standard backdrop
type DialogWithBackdrop[T DialogModel] struct {
	backdrop *BackdropModel
	dialog   T
	position DialogPosition
	helpText string // static help text (used if dialog doesn't implement DynamicHelpProvider)
}

// NewDialogWithBackdrop creates a new wrapper with centered positioning
func NewDialogWithBackdrop[T DialogModel](dialog T, helpText string) DialogWithBackdrop[T] {
	header := NewHeaderModel()
	// Dummy size for initial height calculation
	header.SetWidth(80)
	headerH := header.Height()

	return DialogWithBackdrop[T]{
		backdrop: NewBackdropModel(helpText),
		dialog:   dialog,
		position: GetPositionCenter(headerH),
		helpText: helpText,
	}
}

// WithPosition sets a custom position for the dialog
func (m DialogWithBackdrop[T]) WithPosition(pos DialogPosition) DialogWithBackdrop[T] {
	m.position = pos
	return m
}

// Dialog returns the wrapped dialog model
func (m DialogWithBackdrop[T]) Dialog() T {
	return m.dialog
}

func (m DialogWithBackdrop[T]) Init() tea.Cmd {

	return tea.Batch(m.backdrop.Init(), m.dialog.Init())
}

func (m DialogWithBackdrop[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle CloseDialogMsg - when running standalone, this means quit
	if _, ok := msg.(CloseDialogMsg); ok {
		return m, tea.Quit
	}

	// Handle WindowSizeMsg specifically to enforce constraints
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		// Update backdrop size
		m.backdrop.SetSize(wsm.Width, wsm.Height)

		// Resize dialog to fit available space (accounting for header/footer/shadow)
		availW, availH := GetAvailableDialogSize(wsm.Width, wsm.Height)
		m.dialog.SetSize(availW, availH)

		// Update position if it was centered/topleft
		headerH := m.backdrop.header.Height()
		// We assume if it was centered or at ContentStartY, it should stay that way
		// but we don't have a record of the "mode".
		// For simplicity, we just recalculate based on current position's Y if it's aligned to old chrome
		// But in DialogWithBackdrop, usually it's either centered or top-left.
		// If YOffset matches old ContentStartY, update it.
		layout := GetLayout()
		m.position.YOffset = layout.ContentStartY(headerH)

		// Do not pass WindowSizeMsg to dialog, as we've already handled sizing
		// and we don't want the dialog to reset to full screen size.
		return m, nil
	}

	// Update backdrop (non-sizing messages)
	_, cmd := m.backdrop.Update(msg)
	cmds = append(cmds, cmd)

	// Update dialog
	dialogModel, cmd := m.dialog.Update(msg)
	// Type assert back to T
	m.dialog = dialogModel.(T)
	cmds = append(cmds, cmd)

	// Update helpText from dialog's dynamic help if available
	if provider, ok := any(m.dialog).(DynamicHelpProvider); ok {
		if helpText := provider.GetHelpText(); helpText != "" {
			m.backdrop.SetHelpText(helpText)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m DialogWithBackdrop[T]) View() tea.View {
	dialogContent := m.dialog.ViewString()
	backdropContent := m.backdrop.ViewString()

	// If dialog isn't ready yet, just show backdrop
	if dialogContent == "" {
		v := tea.NewView(backdropContent)
		v.MouseMode = tea.MouseModeAllMotion
		return v
	}

	// Composite dialog over backdrop at the specified position
	// Ensure YOffset is current
	headerH := m.backdrop.header.Height()
	layout := GetLayout()
	m.position.YOffset = layout.ContentStartY(headerH)

	output := Overlay(
		dialogContent,
		backdropContent,
		m.position.H,
		m.position.V,
		m.position.XOffset,
		m.position.YOffset,
	)

	// Component view (ready for hit-testing in compositor)
	v := tea.View{Content: output}
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

func RunDialogWithBackdrop[T DialogModel](dialog T, helpText string, position DialogPosition) (T, error) {

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if _, err := theme.Load(cfg.UI.Theme, ""); err == nil {
		InitStyles(cfg)
	}

	wrapper := NewDialogWithBackdrop(dialog, helpText).WithPosition(position)

	p := NewProgram(wrapper)
	finalModel, err := p.Run()

	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")

	if err != nil {
		return dialog, err
	}

	if m, ok := finalModel.(DialogWithBackdrop[T]); ok {
		return m.dialog, nil
	}

	return dialog, nil
}
