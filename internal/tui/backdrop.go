package tui

import (
	"DockSTARTer2/internal/strutil"
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderWithBackdrop renders dialog content with header and helpline
// This matches the AppModel.View() rendering approach for consistent spacing
// fullScreen: true = dialog fills available space, false = dialog is centered
func RenderWithBackdrop(dialogContent string, helpText string, width, height int, fullScreen bool) string {
	if width == 0 || height == 0 {
		return "Loading..."
	}

	styles := GetStyles()
	var b strings.Builder

	// Header: Fill with StatusBar background, then draw content
	// Using Width() ensures the line fills exactly width - no padding calculation needed
	header := NewHeaderModel()
	header.SetWidth(width - 2)
	headerContent := header.View()
	headerStyle := styles.StatusBar.
		Width(width).
		Padding(0, 1) // 1-char padding left/right
	b.WriteString(headerStyle.Render(headerContent))
	b.WriteString("\n")

	// Separator: Fill with StatusBarSeparator background, then draw separator chars
	sep := strutil.Repeat(styles.SepChar, width-2)
	sepStyle := styles.StatusBarSeparator.
		Width(width).
		Padding(0, 1) // 1-char padding left/right
	b.WriteString(sepStyle.Render(sep))
	b.WriteString("\n")

	// Calculate content height (matches AppModel.View())
	// Total layout: header (1 line) + separator (1 line) + content + helpline (1 line)
	helpline := NewHelplineModel()
	helpline.SetText(helpText)
	helplineView := helpline.View(width)
	helplineHeight := lipgloss.Height(helplineView)

	contentHeight := height - 2 - helplineHeight // -2 for header and separator lines

	// Ensure we have at least some space
	if contentHeight < 3 {
		contentHeight = 3
	}

	// Content area
	var content string
	if fullScreen {
		// Full-screen: dialog fills the available space
		content = lipgloss.NewStyle().
			Width(width).
			Height(contentHeight).
			Background(styles.Screen.GetBackground()).
			Render(dialogContent)
	} else {
		// Centered: dialog is placed in the center of available space using Overlay
		bg := lipgloss.NewStyle().
			Width(width).
			Height(contentHeight).
			Background(styles.Screen.GetBackground()).
			Render("")
		content = Overlay(dialogContent, bg, OverlayCenter, OverlayCenter, 0, 0)
	}

	// Ensure content fills the height (matches AppModel.View())
	contentLines := strings.Count(content, "\n") + 1
	if contentLines < contentHeight {
		content += strutil.Repeat("\n", contentHeight-contentLines)
	}

	// Apply screen background to content area (matches AppModel.View())
	contentStyle := lipgloss.NewStyle().
		Width(width).
		Height(contentHeight).
		Background(styles.Screen.GetBackground())
	b.WriteString(contentStyle.Render(content))

	// Helpline (matches AppModel.View())
	b.WriteString("\n")
	b.WriteString(helplineView)

	return b.String()
}

// logPanelExtraHeight is set by the log panel when it expands so that
// GetAvailableDialogSize can shrink dialogs to avoid overlap.
var logPanelExtraHeight int

// GetAvailableDialogSize returns the maximum size for dialog content
// accounting for header, separator, helpline, shadow, and log panel.
func GetAvailableDialogSize(width, height int) (int, int) {
	if width == 0 || height == 0 {
		return 0, 0
	}

	// Use Layout helpers for consistent calculations
	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow

	availableWidth, availableHeight := layout.ContentArea(width, height, hasShadow)

	// Account for log panel if visible
	availableHeight -= logPanelExtraHeight

	// Ensure minimum height
	if availableHeight < 5 {
		availableHeight = 5
	}

	return availableWidth, availableHeight
}
