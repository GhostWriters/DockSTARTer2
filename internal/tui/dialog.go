package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
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

// BorderPair holds the border styles for focused and unfocused states
type BorderPair struct {
	Focused   lipgloss.Border
	Unfocused lipgloss.Border
}

// RenderDialogBox renders content in a centered dialog box
func RenderDialogBox(title, content string, dialogType DialogType, width, height, containerWidth, containerHeight int) string {
	styles := GetStyles()

	// Get title style based on dialog type
	titleStyle := styles.DialogTitle
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(styles.StatusSuccess.GetForeground())
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(styles.StatusWarn.GetForeground())
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(styles.StatusError.GetForeground())
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

	renderedLabel := RenderHotkeyLabel(label, focused)
	return style.Render("<" + renderedLabel + ">")
}

// RenderHotkeyLabel styles the first letter of a label with the theme's hotkey color
func RenderHotkeyLabel(label string, focused bool) string {
	styles := GetStyles()

	// Normalize label: remove spacing but keep it for rendering if needed
	trimmed := strings.TrimSpace(label)
	if len(trimmed) == 0 {
		return label
	}

	// Determine styles
	var charStyle, restStyle lipgloss.Style
	if focused {
		charStyle = styles.TagKeySelected
		restStyle = styles.ButtonActive
	} else {
		charStyle = styles.TagKey
		restStyle = styles.ButtonInactive
	}

	// Handle leading spaces if they were trimmed
	prefix := ""
	if strings.HasPrefix(label, " ") {
		prefix = strings.Repeat(" ", len(label)-len(strings.TrimLeft(label, " ")))
	}

	// Apply styles
	firstChar := string(trimmed[0])
	rest := trimmed[1:]

	return prefix + charStyle.Render(firstChar) + restStyle.Render(rest)
}

// RenderButtonRow renders a row of buttons centered
func RenderButtonRow(buttons ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Center, buttons...)
}

// ButtonSpec defines a button to render
type ButtonSpec struct {
	Text   string
	Active bool
	ZoneID string // Optional zone ID for mouse support (empty = no zone marking)
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

		renderedLabel := RenderHotkeyLabel(btn.Text, btn.Active)
		renderedButtons = append(renderedButtons, buttonStyle.Render(renderedLabel))
	}

	// Divide available width into equal sections (one per button)
	numButtons := len(buttons)
	sectionWidth := contentWidth / numButtons

	// Center each button in its section and mark zones
	var sections []string
	for i, btn := range renderedButtons {
		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(styles.Dialog.GetBackground()).
			Render(btn)

		// Generate zone ID from button text if not provided
		zoneID := buttons[i].ZoneID
		if zoneID == "" {
			// Auto-generate zone ID from button text
			// Format: "Button.<normalized-text>"
			// E.g., " OK " -> "Button.OK", " Yes " -> "Button.Yes"
			normalizedText := strings.TrimSpace(buttons[i].Text)
			zoneID = "Button." + normalizedText
		}

		// Mark zone with the ID
		centeredBtn = zone.Mark(zoneID, centeredBtn)

		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
}

// CheckButtonHotkeys checks if a key matches the first letter of any button.
// Returns button index and true if a match is found.
func CheckButtonHotkeys(msg tea.KeyMsg, buttons []ButtonSpec) (int, bool) {
	if msg.Type != tea.KeyRunes {
		return -1, false
	}
	keyRune := strings.ToLower(string(msg.Runes))

	for i, btn := range buttons {
		// Normalize button text (remove brackets/spaces)
		text := strings.TrimSpace(btn.Text)
		text = strings.Trim(text, "<>")
		if len(text) > 0 {
			firstChar := strings.ToLower(string(text[0]))
			if firstChar == keyRune {
				return i, true
			}
		}
	}
	return -1, false
}

// RenderDialog renders a dialog with optional title embedded in the top border.
// If title is empty, renders a plain top border without title.
// focused=true uses a thick border (active dialog), focused=false uses normal border (background dialog).
// Optional borders parameter allows overriding the default theme borders.
func RenderDialog(title, content string, focused bool, borders ...BorderPair) string {
	styles := GetStyles()

	var border lipgloss.Border
	if len(borders) > 0 {
		if focused {
			border = borders[0].Focused
		} else {
			border = borders[0].Unfocused
		}
	} else {
		if styles.LineCharacters {
			if focused {
				border = lipgloss.ThickBorder()
			} else {
				border = lipgloss.NormalBorder()
			}
		} else {
			if focused {
				border = thickAsciiBorder
			} else {
				border = asciiBorder
			}
		}
	}

	return renderDialogWithBorder(title, content, border, focused, true, true)
}

// RenderUniformBlockDialog renders a dialog with block borders and uniform dark colors (no 3D effect).
// It also disables specialized T-connectors for the title for a more solid "frame" look.
func RenderUniformBlockDialog(title, content string) string {
	styles := GetStyles()
	borders := GetBlockBorders(styles.LineCharacters)
	return renderDialogWithBorder(title, content, borders.Focused, true, false, false)
}

// GetBlockBorders returns a BorderPair with solid block borders for both states
func GetBlockBorders(lineCharacters bool) BorderPair {
	var block lipgloss.Border
	if lineCharacters {
		block = lipgloss.BlockBorder()
	} else {
		block = lipgloss.Border{
			Top:         "█",
			Bottom:      "█",
			Left:        "█",
			Right:       "█",
			TopLeft:     "█",
			TopRight:    "█",
			BottomLeft:  "█",
			BottomRight: "█",
		}
	}
	return BorderPair{Focused: block, Unfocused: block}
}

// renderDialogWithBorder is the internal shared rendering logic.
// It handles title centering, background maintenance, and padding.
// If threeD is false, it uses a uniform border color (Border2Color).
// If useConnectors is true, it uses T-junctions (┤, ┫, etc.) to embed the title.
func renderDialogWithBorder(title, content string, border lipgloss.Border, focused bool, threeD bool, useConnectors bool) string {
	if title != "" && !strings.HasSuffix(title, "{{|-|}}") {
		title += "{{|-|}}"
	}
	styles := GetStyles()

	// Style definitions
	borderBG := styles.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(borderBG)

	// If not 3D, use the dark/secondary color for EVERYTHING
	if !threeD {
		borderStyleLight = borderStyleDark
	}

	// Prepare title style (default)
	titleStyle := styles.DialogTitle.Copy().
		Background(borderBG)

	// Parse color tags from title and render as rich text
	title = RenderThemeText(title, titleStyle)

	// Get actual content width (maximum width of all lines)
	lines := strings.Split(content, "\n")
	actualWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > actualWidth {
			actualWidth = w
		}
	}

	// For halo/shadow alignment, ensure actualWidth makes the total dialog even.
	// borders are 1 char each, so total width = 1 + actualWidth + 1
	// if actualWidth is even, total width is even.
	if actualWidth%2 != 0 {
		actualWidth++
	}

	var result strings.Builder

	// Top border (with or without title)
	result.WriteString(borderStyleLight.Render(border.TopLeft))
	if title == "" {
		// Plain top border without title
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, actualWidth)))
	} else {
		// Top border with embedded title
		var leftT, rightT string
		if useConnectors {
			if styles.LineCharacters {
				if focused {
					leftT = "┫"
					rightT = "┣"
				} else {
					leftT = "┤"
					rightT = "├"
				}
			} else {
				if focused {
					leftT = "H" // thick ASCII T-connector
					rightT = "H"
				} else {
					leftT = "|"
					rightT = "|"
				}
			}
		} else {
			// No specialized connectors: just use the standard border character
			leftT = border.Top
			rightT = border.Top
		}

		// Total title section width: leftT + space + title + space + rightT
		titleSectionLen := 1 + 1 + lipgloss.Width(title) + 1 + 1

		// Ensure dialog is wide enough for title
		if titleSectionLen > actualWidth {
			actualWidth = titleSectionLen
		}

		leftPad := (actualWidth - titleSectionLen) / 2
		rightPad := actualWidth - titleSectionLen - leftPad
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, leftPad)))
		result.WriteString(borderStyleLight.Render(leftT))
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(title)
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(borderStyleLight.Render(rightT))
		result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, rightPad)))
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	// Content lines with left/right borders
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		// Pad line to actualWidth to ensure borders align
		textWidth := lipgloss.Width(line)
		padding := ""
		if textWidth < actualWidth {
			padding = lipgloss.NewStyle().Background(borderBG).Render(strings.Repeat(" ", actualWidth-textWidth))
		}
		// Use MaintainBackground to ensure internal resets don't bleed to black
		fullLine := MaintainBackground(line+padding, styles.Dialog)
		result.WriteString(fullLine)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	// Bottom border
	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strings.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}
