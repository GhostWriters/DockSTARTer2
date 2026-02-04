package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// DialogType represents the type of dialog for styling
type DialogType int

const (
	DialogTypeInfo DialogType = iota
	DialogTypeSuccess
	DialogTypeWarning
	DialogTypeError
	DialogTypeConfirm
)

// RenderDialogBox renders content in a centered dialog box
func RenderDialogBox(title, content string, dialogType DialogType, width, height, containerWidth, containerHeight int) string {
	styles := GetStyles()

	// Get title style based on dialog type
	titleStyle := styles.DialogTitle
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(lipgloss.Color("#00ff00")) // Green
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(lipgloss.Color("#ffff00")) // Yellow
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(lipgloss.Color("#ff0000")) // Red
	}

	// Render title
	titleRendered := titleStyle.
		Width(width).
		Align(lipgloss.Center).
		Render(title)

	// Render content
	contentRendered := styles.Dialog.
		Width(width).
		Align(lipgloss.Center).
		Render(content)

	// Combine title and content
	inner := lipgloss.JoinVertical(lipgloss.Center, titleRendered, contentRendered)

	// Wrap in border with 3D effect
	borderStyle := lipgloss.NewStyle().
		Border(styles.Border).
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)
	borderStyle = Apply3DBorder(borderStyle)
	dialogBox := borderStyle.Render(inner)

	// Add shadow effect
	dialogBox = AddShadow(dialogBox)

	// Center in container
	centered := lipgloss.Place(
		containerWidth,
		containerHeight,
		lipgloss.Center,
		lipgloss.Center,
		dialogBox,
		lipgloss.WithWhitespaceBackground(styles.Screen.GetBackground()),
	)

	return centered
}

// RenderButton renders a button with the given label and focus state
func RenderButton(label string, focused bool) string {
	styles := GetStyles()

	style := styles.ButtonInactive
	if focused {
		style = styles.ButtonActive
	}

	return style.Render("<" + label + ">")
}

// RenderButtonRow renders a row of buttons centered
func RenderButtonRow(buttons ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Center, buttons...)
}
