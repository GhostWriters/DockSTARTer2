package tui

import (
	"strings"

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

	// Wrap in border (3D effect for rounded borders)
	borderStyle := lipgloss.NewStyle().
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

// ButtonSpec defines a button to render
type ButtonSpec struct {
	Text   string
	Active bool
}

// RenderCenteredButtons renders buttons centered in sections (matching menu style)
// This ensures consistent button placement across all dialogs
func RenderCenteredButtons(contentWidth int, buttons ...ButtonSpec) string {
	if len(buttons) == 0 {
		return ""
	}

	styles := GetStyles()

	// Find the maximum button text width
	maxButtonWidth := 0
	for _, btn := range buttons {
		width := lipgloss.Width(btn.Text)
		if width > maxButtonWidth {
			maxButtonWidth = width
		}
	}

	// Render each button with fixed width and rounded border
	var renderedButtons []string
	for _, btn := range buttons {
		var buttonStyle lipgloss.Style
		if btn.Active {
			buttonStyle = styles.ButtonActive
		} else {
			buttonStyle = styles.ButtonInactive
		}

		buttonStyle = buttonStyle.Copy().Width(maxButtonWidth).Align(lipgloss.Center)
		buttonStyle = ApplyRoundedBorder(buttonStyle, styles.LineCharacters)
		renderedButtons = append(renderedButtons, buttonStyle.Render(btn.Text))
	}

	// Divide available width into equal sections (one per button)
	numButtons := len(buttons)
	sectionWidth := contentWidth / numButtons

	// Center each button in its section
	var sections []string
	for _, btn := range renderedButtons {
		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(styles.Dialog.GetBackground()).
			Render(btn)
		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
}

// RenderDialog renders a dialog with optional title embedded in the top border
// If title is empty, renders a plain top border without title
func RenderDialog(title, content string) string {
	styles := GetStyles()

	// Use straight border for dialogs
	var border lipgloss.Border
	if styles.LineCharacters {
		border = lipgloss.NormalBorder()
	} else {
		border = asciiBorder
	}

	// Style definitions
	borderBG := styles.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(borderBG)
	titleStyle := styles.DialogTitle.Copy().
		Background(borderBG)

	// Get actual content width
	lines := strings.Split(content, "\n")
	actualWidth := 0
	if len(lines) > 0 {
		actualWidth = lipgloss.Width(lines[0])
	}

	var result strings.Builder

	// Top border (with or without title)
	result.WriteString(borderStyleLight.Render(border.TopLeft))
	if title == "" {
		// Plain top border without title
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, actualWidth)))
	} else {
		// Top border with embedded title using T connectors
		// Format: ────┤ Title ├────
		// Spaces are rendered with border style, not title style
		var leftT, rightT string
		if styles.LineCharacters {
			leftT = "┤"
			rightT = "├"
		} else {
			leftT = "+"
			rightT = "+"
		}
		// Total title section width: leftT + space + title + space + rightT
		titleSectionLen := 1 + 1 + lipgloss.Width(title) + 1 + 1
		leftPad := (actualWidth - titleSectionLen) / 2
		rightPad := actualWidth - titleSectionLen - leftPad
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, leftPad)))
		result.WriteString(borderStyleLight.Render(leftT))
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(titleStyle.Render(title))
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(borderStyleLight.Render(rightT))
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, rightPad)))
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	// Content lines with left/right borders
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		result.WriteString(line)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	// Bottom border
	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strings.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}
