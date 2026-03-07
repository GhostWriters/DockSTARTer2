package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// hasExplicitBackground returns true if the style has a meaningful background color set.
// When a theme tag uses '-' for background, ApplyStyleCode calls style.Background(nil),
// which lipgloss stores as NoColor{} — the absence of color.
// We detect this by direct type assertion, since NoColor{}.RGBA() returns full alpha (0xFFFF)
// making alpha-based detection unreliable.
func hasExplicitBackground(s lipgloss.Style) bool {
	bg := s.GetBackground()
	if bg == nil {
		return false
	}
	_, isNoColor := bg.(lipgloss.NoColor)
	return !isNoColor
}

// DialogType represents the type of dialog for styling
type DialogType int

const (
	DialogTypeInfo DialogType = iota
	DialogTypeSuccess
	DialogTypeWarning
	DialogTypeError
	DialogTypeConfirm
)

// BorderPair holds the border styles for focused and unfocused states
type BorderPair struct {
	Focused   lipgloss.Border
	Unfocused lipgloss.Border
}

// Dialog sizing constants for deterministic layout
const (
	DialogBorderHeight = 2 // Top + Bottom outer borders
	DialogShadowHeight = 1 // Bottom shadow offset
	DialogShadowWidth  = 2 // Right shadow offset
	DialogButtonHeight = 3 // Standard button row
	DialogBodyPadH     = 4 // Horizontal padding for body text: Padding(1,2) = 2 chars each side
)

// Modal Z-order constants
const (
	ZModalBaseOffset = 100 // Z gap above the highest screen layer for the first modal
	ZModalStackStep  = 100 // Additional Z gap for each subsequent stacked modal
)

// DialogLayout stores pre-calculated vertical budgeting for a dialog.
// This implements the "calculate once, use everywhere" pattern.
type DialogLayout struct {
	Width          int
	Height         int
	HeaderHeight   int
	CommandHeight  int
	ViewportHeight int
	ButtonHeight   int
	ShadowHeight   int
	Overhead       int

	// Hit region positions (calculated during render)
	ListX    int // X offset to first list item content
	ListY    int // Y offset to first list item
	ButtonY  int // Y offset to button row
	ContentW int // Width of content area (inside borders)
}

// maxLineWidth returns the maximum lipgloss display width across all lines of text
// after stripping theme tags. Used by dialog contentWidth() methods.
func maxLineWidth(text string) int {
	maxW := 0
	for _, line := range strings.Split(GetPlainText(text), "\n") {
		if w := lipgloss.Width(line); w > maxW {
			maxW = w
		}
	}
	return maxW
}

// baseDialogModel holds fields and promoted methods shared by the simple dialog types
// (confirm, message, prompt). View() and Layers() are kept on the outer type because
// they depend on the outer type's ViewString().
type baseDialogModel struct {
	id      string
	width   int
	height  int
	focused bool
	layout  DialogLayout
}

func (b *baseDialogModel) Init() tea.Cmd { return nil }

func (b *baseDialogModel) SetSize(w, h int) {
	b.width = w
	b.height = h
	b.calculateLayout()
}

func (b *baseDialogModel) SetFocused(f bool) { b.focused = f }

func (b *baseDialogModel) calculateLayout() {
	if b.width == 0 || b.height == 0 {
		return
	}
	b.layout = newStandardDialogLayout(b.width, b.height)
}

// newStandardDialogLayout builds a DialogLayout for a dialog with borders, buttons, and optional shadow.
// All three simple dialog types (confirm, message, prompt) share this calculation.
func newStandardDialogLayout(width, height int) DialogLayout {
	shadow := 0
	if currentConfig.UI.Shadow {
		shadow = DialogShadowHeight
	}
	buttons := DialogButtonHeight
	return DialogLayout{
		Width:        width,
		Height:       height,
		ButtonHeight: buttons,
		ShadowHeight: shadow,
		Overhead:     DialogBorderHeight + buttons + shadow,
	}
}

// EnforceDialogLayout appends a button row (if specified) to the content
// and enforces the deterministic height budget if the dialog is maximized.
func EnforceDialogLayout(content string, buttons []ButtonSpec, layout DialogLayout, maximized bool) string {
	// Standardize to prevent implicit gaps
	content = strings.TrimRight(content, "\n")

	// Append buttons if any
	if len(buttons) > 0 {
		buttonRow := RenderCenteredButtons(layout.Width-2, buttons...)
		buttonRow = strings.TrimRight(buttonRow, "\n")
		content = lipgloss.JoinVertical(lipgloss.Left, content, buttonRow)
	}

	// Force total content height to match the calculated budget
	// only if maximized. Otherwise it has its intrinsic height.
	if maximized {
		heightBudget := layout.Height - DialogBorderHeight - layout.ShadowHeight
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(GetStyles().Dialog.GetBackground()).
				Render(content)
		}
	}

	return content
}

// RenderDialogBox renders content in a centered dialog box
func RenderDialogBox(title, content string, dialogType DialogType, width, height, containerWidth, containerHeight int) string {
	return RenderDialogBoxCtx(title, content, dialogType, width, height, containerWidth, containerHeight, GetActiveContext())
}

// RenderDialogBoxCtx renders content in a centered dialog box using a specific context
func RenderDialogBoxCtx(title, content string, dialogType DialogType, width, height, containerWidth, containerHeight int, ctx StyleContext) string {
	// Get title style based on dialog type
	titleStyle := ctx.DialogTitle
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(ctx.StatusSuccess.GetForeground())
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(ctx.StatusWarn.GetForeground())
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(ctx.StatusError.GetForeground())
	case DialogTypeConfirm:
		titleStyle = SemanticRawStyle("Theme_TitleQuestion") // Question style is semantic
	}

	// Render title
	titleRendered := titleStyle.
		Width(width).
		Align(lipgloss.Center).
		Render(title)

	// Render content
	contentRendered := ctx.Dialog.
		Width(width).
		Align(lipgloss.Center).
		Render(content)

	// Combine title and content
	inner := lipgloss.JoinVertical(lipgloss.Center, titleRendered, contentRendered)

	// Wrap in border (3D effect for rounded borders)
	borderStyle := lipgloss.NewStyle().
		Background(ctx.Dialog.GetBackground()).
		Padding(0, 1)
	borderStyle = Apply3DBorderCtx(borderStyle, ctx)
	dialogBox := borderStyle.Render(inner)

	// Center in container using Overlay for transparency support
	bg := lipgloss.NewStyle().
		Width(containerWidth).
		Height(containerHeight).
		Background(ctx.Screen.GetBackground()).
		Render("")

	return Overlay(dialogBox, bg, OverlayCenter, OverlayCenter, 0, 0)
}
