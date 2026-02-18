package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
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

// Common dialog positions
// All positions use OverlayTop with Y offset of 2 to start in the content area
// (below header line 1 and separator line 2)
var (
	// PositionCenter centers the dialog horizontally in the content area
	PositionCenter = DialogPosition{OverlayCenter, OverlayTop, 0, 2}
	// PositionTopLeft positions the dialog at top-left of content area (for maximized dialogs)
	PositionTopLeft = DialogPosition{OverlayLeft, OverlayTop, 2, 2}
)

// DialogWithBackdrop wraps any DialogModel with the standard backdrop
type DialogWithBackdrop[T DialogModel] struct {
	backdrop BackdropModel
	dialog   T
	position DialogPosition
	helpText string // static help text (used if dialog doesn't implement DynamicHelpProvider)
}

// NewDialogWithBackdrop creates a new wrapper with centered positioning
func NewDialogWithBackdrop[T DialogModel](dialog T, helpText string) DialogWithBackdrop[T] {
	return DialogWithBackdrop[T]{
		backdrop: NewBackdropModel(helpText),
		dialog:   dialog,
		position: PositionCenter,
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
	// DEBUG
	fmt.Fprintln(os.Stderr, "DEBUG: DialogWithBackdrop.Init called")
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
		// Update backdrop size via Update (which handles WindowSizeMsg)
		backdropModel, _ := m.backdrop.Update(msg)
		m.backdrop = backdropModel.(BackdropModel)

		// Resize dialog to fit available space (accounting for header/footer/shadow)
		availW, availH := GetAvailableDialogSize(wsm.Width, wsm.Height)
		m.dialog.SetSize(availW, availH)

		// Do not pass WindowSizeMsg to dialog, as we've already handled sizing
		// and we don't want the dialog to reset to full screen size.
		return m, nil
	}

	// Update backdrop
	backdropModel, cmd := m.backdrop.Update(msg)
	m.backdrop = backdropModel.(BackdropModel)
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
	output := Overlay(
		dialogContent,
		backdropContent,
		m.position.H,
		m.position.V,
		m.position.XOffset,
		m.position.YOffset,
	)

	// Scan zones at root level for mouse support
	v := tea.NewView(zone.Scan(output))
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// RunDialogWithBackdrop is a helper to run any dialog with the standard backdrop.
// It initializes the TUI, creates the wrapper, and runs the program.
func RunDialogWithBackdrop[T DialogModel](dialog T, helpText string, position DialogPosition) (T, error) {
	// Initialize global zone manager for mouse support (safe to call multiple times)
	zone.NewGlobal()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.UI.Theme); err == nil {
		InitStyles(cfg)
	}

	wrapper := NewDialogWithBackdrop(dialog, helpText).WithPosition(position)

	p := tea.NewProgram(wrapper)
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
