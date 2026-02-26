package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// DialogMode specifies how a dialog should be positioned
type DialogMode int

const (
	// DialogCentered centers the dialog in the content area
	DialogCentered DialogMode = iota
	// DialogMaximized fills the content area (1-char indent from edges)
	DialogMaximized
	// DialogVerticalMaximized fills vertical space but centers horizontally
	DialogVerticalMaximized
)

// Layout contains all the constants and calculations for TUI positioning.
// This is the single source of truth - no magic numbers elsewhere.
type Layout struct {
	// Screen chrome (always present)
	SeparatorHeight int // 1 line

	// Helpline at bottom
	HelplineHeight int // 1 line

	// Dialog chrome
	DialogBorder  int // 2 (top + bottom border)
	DialogPadding int // Padding inside dialog (0-2 typically)
	ButtonHeight  int // 3 lines for button row

	// Shadow (when enabled)
	ShadowWidth  int // 2 chars to the right
	ShadowHeight int // 1 line at the bottom

	// Margins/indents
	EdgeIndent        int // 1 char indent from screen edge for maximized dialogs
	GapBeforeHelpline int // 1 line gap before helpline

	// Gutter for side-by-side layouts
	GutterWidth int // 1 char between panels
}

// DefaultLayout returns the standard layout configuration
func DefaultLayout() Layout {
	return Layout{
		SeparatorHeight:   1,
		HelplineHeight:    1,
		DialogBorder:      2,
		DialogPadding:     0,
		ButtonHeight:      3,
		ShadowWidth:       2,
		ShadowHeight:      1,
		EdgeIndent:        2, // 2 chars margin on each side of content area
		GapBeforeHelpline: 1,
		GutterWidth:       1,
	}
}

// GetLayout returns the current layout configuration
// This can be extended to read from config if needed
func GetLayout() Layout {
	return DefaultLayout()
}

// -------------------------------------------------------------------
// Computed properties
// -------------------------------------------------------------------

// ChromeHeight returns the total height of screen chrome (header + separator)
func (l Layout) ChromeHeight(headerHeight int) int {
	return headerHeight + l.SeparatorHeight
}

// ContentStartY returns the Y coordinate where content area begins (just under separator)
func (l Layout) ContentStartY(headerHeight int) int {
	return l.ChromeHeight(headerHeight)
}

// BottomChrome returns total height reserved at bottom (gap + helpline)
func (l Layout) BottomChrome() int {
	return l.GapBeforeHelpline + l.HelplineHeight
}

// -------------------------------------------------------------------
// Content area calculations
// -------------------------------------------------------------------

// ContentArea returns the dimensions available for dialogs/screens
// This is the space between header/separator and helpline
func (l Layout) ContentArea(screenW, screenH int, hasShadow bool, headerHeight int) (width, height int) {
	shadowW, shadowH := 0, 0
	if hasShadow {
		shadowW, shadowH = l.ShadowWidth, l.ShadowHeight
	}

	// Width: screen minus edge indents and shadow
	width = screenW - (l.EdgeIndent * 2) - shadowW

	// Height: screen minus top chrome, bottom chrome, and shadow
	// Subtracting shadowH here ensures the dialog + its shadow fits
	// ABOVE the GapBeforeHelpline, leaving that line blank.
	height = screenH - l.ChromeHeight(headerHeight) - l.BottomChrome() - shadowH

	// Ensure minimums
	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	return width, height
}

// DialogPosition returns the X, Y coordinates for a dialog based on mode
func (l Layout) DialogPosition(mode DialogMode, dialogW, dialogH, screenW, screenH int, hasShadow bool, headerHeight int) (x, y int) {
	contentStartY := l.ContentStartY(headerHeight)
	shadowW, _ := 0, 0
	if hasShadow {
		shadowW, _ = l.ShadowWidth, l.ShadowHeight
	}

	switch mode {
	case DialogMaximized:
		// X: edge indent from left
		// Y: just under separator
		return l.EdgeIndent, contentStartY

	case DialogVerticalMaximized:
		// X: centered horizontally (accounting for shadow)
		// Y: just under separator
		x = (screenW - dialogW - shadowW) / 2
		if x < l.EdgeIndent {
			x = l.EdgeIndent
		}
		return x, contentStartY

	case DialogCentered:
		// Center in content area
		contentW, contentH := l.ContentArea(screenW, screenH, hasShadow, headerHeight)

		// Center horizontally in content area
		x = l.EdgeIndent + (contentW-dialogW)/2
		if x < l.EdgeIndent {
			x = l.EdgeIndent
		}

		// Center vertically in content area with an optical bias (+1)
		// Since contentH already subtracted shadowH, we don't subtract it again here.
		y = contentStartY + (contentH-dialogH+1)/2
		if y < contentStartY {
			y = contentStartY
		}
		return x, y
	}

	return l.EdgeIndent, contentStartY
}

// MaximizedDialogSize returns the dimensions for a maximized dialog
func (l Layout) MaximizedDialogSize(screenW, screenH int, hasShadow bool, headerHeight int) (width, height int) {
	return l.ContentArea(screenW, screenH, hasShadow, headerHeight)
}

// -------------------------------------------------------------------
// Dialog content height calculations
// -------------------------------------------------------------------

// DialogContentHeight returns the height available for content inside a dialog
// Parameters:
//   - dialogH: total dialog height (including border)
//   - headerHeight: height of any header/subtitle inside the dialog (not the screen header)
//   - hasButtons: whether the dialog has a button row
//   - hasShadow: whether shadow is enabled
func (l Layout) DialogContentHeight(dialogH int, headerHeight int, hasButtons bool, hasShadow bool) int {
	overhead := l.DialogBorder + l.DialogPadding + headerHeight
	if hasButtons {
		overhead += l.ButtonHeight
	}
	if hasShadow {
		// NOTE: Shadow height is NOT subtracted here because the total height
		// provided (dialogH) from ContentArea already excludes it to reserve space.
		// If we subtract it again, the dialog will be too short.
	}

	h := dialogH - overhead
	if h < 1 {
		h = 1
	}
	return h
}

// -------------------------------------------------------------------
// Side-by-side layout (for display_options.go)
// -------------------------------------------------------------------

// SideBySideLayout returns the widths and X positions for two side-by-side panels
func (l Layout) SideBySideLayout(screenW int, hasShadow bool) (leftW, rightW, leftX, rightX int) {
	shadowW := 0
	if hasShadow {
		shadowW = l.ShadowWidth
	}

	// Available width after edges and gutter
	available := screenW - (l.EdgeIndent * 2) - l.GutterWidth - shadowW

	// Split evenly (left panel is slightly smaller if odd)
	leftW = available / 2
	rightW = available - leftW

	// Positions
	leftX = l.EdgeIndent
	rightX = l.EdgeIndent + leftW + l.GutterWidth

	return leftW, rightW, leftX, rightX
}

// -------------------------------------------------------------------
// Bordered box calculations - THE source of truth for border math
// -------------------------------------------------------------------

// BorderWidth returns the horizontal border overhead (left + right)
func (l Layout) BorderWidth() int {
	return 2 // 1 char left border + 1 char right border
}

// BorderHeight returns the vertical border overhead (top + bottom)
func (l Layout) BorderHeight() int {
	return 2 // 1 line top border + 1 line bottom border
}

// InnerContentSize returns the content size available inside a bordered box
// Use this when you have a total box size and need to know how much content fits
func (l Layout) InnerContentSize(totalW, totalH int) (contentW, contentH int) {
	contentW = totalW - l.BorderWidth()
	contentH = totalH - l.BorderHeight()
	if contentW < 0 {
		contentW = 0
	}
	if contentH < 0 {
		contentH = 0
	}
	return contentW, contentH
}

// OuterTotalSize returns the total box size given content dimensions
// Use this when you have content and need to know the final rendered size
func (l Layout) OuterTotalSize(contentW, contentH int) (totalW, totalH int) {
	return contentW + l.BorderWidth(), contentH + l.BorderHeight()
}

// -------------------------------------------------------------------
// Content clipping helpers
// -------------------------------------------------------------------

// ConstrainWidth clips each line of content to maxWidth
// This should be called BEFORE rendering to prevent overflow
func (l Layout) ConstrainWidth(content string, maxWidth int) string {
	if maxWidth <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > maxWidth {
			lines[i] = TruncateRight(line, maxWidth)
		}
	}
	return strings.Join(lines, "\n")
}

// -------------------------------------------------------------------
// High-level helpers (zero calculations in caller)
// -------------------------------------------------------------------

// PlaceDialog returns a LayerSpec for a dialog with computed position
func (l Layout) PlaceDialog(content string, screenW, screenH int, mode DialogMode, hasShadow bool, zIndex int, headerHeight int) LayerSpec {
	dialogW := lipgloss.Width(content)
	dialogH := lipgloss.Height(content)

	x, y := l.DialogPosition(mode, dialogW, dialogH, screenW, screenH, hasShadow, headerHeight)

	return LayerSpec{
		Content: content,
		X:       x,
		Y:       y,
		Z:       zIndex,
	}
}

// PlaceSideBySide returns LayerSpecs for two side-by-side panels
func (l Layout) PlaceSideBySide(left, right string, screenW, screenH int, hasShadow bool, zIndex int, headerHeight int) []LayerSpec {
	_, _, leftX, rightX := l.SideBySideLayout(screenW, hasShadow)
	y := l.ContentStartY(headerHeight)

	return []LayerSpec{
		{Content: left, X: leftX, Y: y, Z: zIndex},
		{Content: right, X: rightX, Y: y, Z: zIndex},
	}
}
