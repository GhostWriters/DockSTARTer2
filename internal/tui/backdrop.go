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

	// Header with 1-char padding on left and right (matches AppModel.View())
	// Header width reduced by 2 for padding
	header := NewHeaderModel()
	header.SetWidth(width - 2)
	headerContent := header.View()
	headerStyle := lipgloss.NewStyle().
		Width(width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.Screen.GetBackground())
	b.WriteString(headerStyle.Render(headerContent))
	b.WriteString("\n")

	// Separator line with 1-char padding on left and right (matches AppModel.View())
	sep := strutil.Repeat(styles.SepChar, width-2)
	sepStyle := lipgloss.NewStyle().
		Width(width).
		PaddingLeft(1).
		PaddingRight(1).
		Background(styles.HeaderBG.GetBackground())
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

	// Account for shadow if enabled (2 chars wide on right, 1 line on bottom)
	shadowWidth := 0
	shadowHeight := 0
	if currentConfig.UI.Shadow {
		shadowWidth = 2
		shadowHeight = 1
	}

	// Available size for dialog content (accounting for outer borders and margins)
	// Remaining space for dialog: margin (2 per side) = 4
	availableWidth := width - 4 - shadowWidth

	// Remaining space for dialog: header/sep (2) + gap (1) + helpline (1) = 4
	availableHeight := height - 4 - shadowHeight - logPanelExtraHeight

	// Leave some margin
	if availableHeight < 5 {
		availableHeight = 5
	}

	return availableWidth, availableHeight
}
