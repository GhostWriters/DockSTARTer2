package tui

import (
	"DockSTARTer2/internal/strutil"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
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

// Dialog sizing constants for deterministic layout
const (
	DialogBorderHeight = 2 // Top + Bottom outer borders
	DialogShadowHeight = 1 // Bottom shadow line
	DialogButtonHeight = 3 // Standard button row
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
}

// CalculateBaseOverhead returns the overhead lines for a standard dialog without a viewport
func CalculateBaseOverhead(hasShadow bool, hasButtons bool) int {
	overhead := DialogBorderHeight
	if hasShadow {
		overhead += DialogShadowHeight
	}
	if hasButtons {
		overhead += DialogButtonHeight
	}
	return overhead
}

// CalculateContentHeight returns the remaining vertical budget for content
func CalculateContentHeight(totalHeight int, overhead int) int {
	h := totalHeight - overhead
	if h < 0 {
		return 0
	}
	return h
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

	// Add shadow effect
	dialogBox = AddShadowCtx(dialogBox, ctx)

	// Center in container using Overlay for transparency support
	bg := lipgloss.NewStyle().
		Width(containerWidth).
		Height(containerHeight).
		Background(ctx.Screen.GetBackground()).
		Render("")

	return Overlay(dialogBox, bg, OverlayCenter, OverlayCenter, 0, 0)
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
		prefix = strutil.Repeat(" ", len(label)-len(strings.TrimLeft(label, " ")))
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

// RenderCenteredButtons renders buttons centered in sections
func RenderCenteredButtons(contentWidth int, buttons ...ButtonSpec) string {
	return RenderCenteredButtonsCtx(contentWidth, GetActiveContext(), buttons...)
}

// RenderCenteredButtonsCtx renders buttons centered using a specific context
func RenderCenteredButtonsCtx(contentWidth int, ctx StyleContext, buttons ...ButtonSpec) string {
	if len(buttons) == 0 {
		return ""
	}

	// Find the maximum button text width
	maxButtonWidth := 0
	for _, btn := range buttons {
		width := lipgloss.Width(btn.Text)
		if width > maxButtonWidth {
			maxButtonWidth = width
		}
	}

	// Render each button with fixed width and rounded border
	buttonContentWidth := maxButtonWidth + 4
	var renderedButtons []string
	for _, btn := range buttons {
		var buttonStyle lipgloss.Style
		if btn.Active {
			buttonStyle = ctx.ButtonActive
		} else {
			buttonStyle = ctx.ButtonInactive
		}

		buttonStyle = buttonStyle.Width(buttonContentWidth).Align(lipgloss.Center)
		buttonStyle = ApplyRoundedBorderCtx(buttonStyle, ctx)

		// RenderHotkeyLabel needs to handle focus too, but it uses GetStyles() inside.
		// Let's pass the context if we refactor it, or just use the existing one for now.
		// Actually, RenderHotkeyLabel should also be context-aware.
		renderedLabel := RenderHotkeyLabelCtx(btn.Text, btn.Active, ctx)
		renderedButtons = append(renderedButtons, buttonStyle.Render(renderedLabel))
	}

	numButtons := len(buttons)
	sectionWidth := contentWidth / numButtons

	var sections []string
	for i, btn := range renderedButtons {
		zoneID := buttons[i].ZoneID
		if zoneID == "" {
			normalizedText := strings.TrimSpace(buttons[i].Text)
			zoneID = "Button." + normalizedText
		}

		markedBtn := zone.Mark(zoneID, btn)

		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(ctx.Dialog.GetBackground()).
			Render(markedBtn)

		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
}

// RenderHotkeyLabelCtx styles the first letter of a label with the theme's hotkey color using a context
func RenderHotkeyLabelCtx(label string, focused bool, ctx StyleContext) string {
	trimmed := strings.TrimSpace(label)
	if len(trimmed) == 0 {
		return label
	}

	var charStyle, restStyle lipgloss.Style
	if focused {
		charStyle = ctx.TagKeySelected
		restStyle = ctx.ButtonActive
	} else {
		charStyle = ctx.TagKey
		restStyle = ctx.ButtonInactive
	}

	prefix := ""
	if strings.HasPrefix(label, " ") {
		prefix = strutil.Repeat(" ", len(label)-len(strings.TrimLeft(label, " ")))
	}

	firstChar := string(trimmed[0])
	rest := trimmed[1:]

	return prefix + charStyle.Render(firstChar) + restStyle.Render(rest)
}

// CheckButtonHotkeys checks if a key matches the first letter of any button.
// Returns button index and true if a match is found.
// NOTE: In Bubble Tea v2, KeyMsg is now a union type - use tea.KeyPressMsg for key press events.
func CheckButtonHotkeys(msg tea.KeyPressMsg, buttons []ButtonSpec) (int, bool) {
	// In v2, msg.Text contains the printable character(s)
	if msg.Text == "" {
		return -1, false
	}
	keyRune := strings.ToLower(msg.Text)

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
func RenderDialog(title, content string, focused bool, targetHeight int, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, DialogTypeInfo, GetActiveContext(), borders...)
}

// RenderDialogCtx renders a dialog using a specific context
func RenderDialogCtx(title, content string, focused bool, targetHeight int, ctx StyleContext, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, DialogTypeInfo, ctx, borders...)
}

// RenderDialogWithType renders a dialog with a specific type for title styling.
func RenderDialogWithType(title, content string, focused bool, targetHeight int, dialogType DialogType, borders ...BorderPair) string {
	return RenderDialogWithTypeCtx(title, content, focused, targetHeight, dialogType, GetActiveContext(), borders...)
}

// RenderDialogWithTypeCtx renders a dialog with a specific type using a specific context
func RenderDialogWithTypeCtx(title, content string, focused bool, targetHeight int, dialogType DialogType, ctx StyleContext, borders ...BorderPair) string {
	var border lipgloss.Border
	if len(borders) > 0 {
		if focused {
			border = borders[0].Focused
		} else {
			border = borders[0].Unfocused
		}
	} else {
		if ctx.LineCharacters {
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

	titleStyle := ctx.DialogTitle
	switch dialogType {
	case DialogTypeSuccess:
		titleStyle = titleStyle.Foreground(ctx.StatusSuccess.GetForeground())
	case DialogTypeWarning:
		titleStyle = titleStyle.Foreground(ctx.StatusWarn.GetForeground())
	case DialogTypeError:
		titleStyle = titleStyle.Foreground(ctx.StatusError.GetForeground())
	case DialogTypeConfirm:
		titleStyle = SemanticRawStyle("Theme_TitleQuestion") // Semantic
	}

	return renderDialogWithBorderCtx(title, content, border, focused, targetHeight, true, true, titleStyle, ctx)
}

// RenderUniformBlockDialog renders a dialog with block borders and uniform dark colors
func RenderUniformBlockDialog(title, content string) string {
	return RenderUniformBlockDialogCtx(title, content, GetActiveContext())
}

// RenderUniformBlockDialogCtx renders a uniform block dialog using specific context
func RenderUniformBlockDialogCtx(title, content string, ctx StyleContext) string {
	borders := GetBlockBorders(ctx.LineCharacters)
	return renderDialogWithBorderCtx(title, content, borders.Focused, true, 0, false, false, ctx.DialogTitleHelp, ctx)
}

// RenderBorderedBoxCtx renders a dialog with title and borders using a specific context.
// Unlike renderDialogWithBorderCtx, this accepts a known contentWidth instead of measuring content.
func RenderBorderedBoxCtx(rawTitle, content string, contentWidth int, targetHeight int, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	if !ctx.DrawBorders {
		border = lipgloss.HiddenBorder()
	} else if ctx.LineCharacters {
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

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(ctx.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(borderBG)
	titleStyle := ctx.DialogTitle.
		Background(borderBG)

	// Process the title with full semantic theme tagging support
	renderedTitle := MaintainBackground(RenderThemeText(rawTitle, titleStyle), titleStyle)

	lines := strings.Split(content, "\n")
	// Trust the passed contentWidth - don't expand based on line widths
	// Zone markers and other invisible ANSI sequences can inflate lipgloss.Width()
	// causing incorrect width calculations. The caller knows the correct width.
	actualWidth := contentWidth

	if targetHeight > 2 {
		contentHeight := len(lines)
		neededPadding := (targetHeight - 2) - contentHeight
		if neededPadding > 0 {
			for i := 0; i < neededPadding; i++ {
				lines = append(lines, "")
			}
		}
	}

	var leftT, rightT string
	if !ctx.DrawBorders {
		leftT = " "
		rightT = " "
	} else if ctx.LineCharacters {
		if focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	titleSectionLen := 1 + 1 + lipgloss.Width(renderedTitle) + 1 + 1
	leftPad := (actualWidth - titleSectionLen) / 2
	rightPad := actualWidth - titleSectionLen - leftPad

	var result strings.Builder

	result.WriteString(borderStyleLight.Render(border.TopLeft))
	result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, leftPad)))
	result.WriteString(borderStyleLight.Render(leftT))
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(renderedTitle)
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(borderStyleLight.Render(rightT))
	result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPad)))
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		// Use WidthWithoutZones to get accurate visual width (zone markers are invisible)
		textWidth := WidthWithoutZones(line)

		var fullLine string
		if textWidth > actualWidth {
			// Truncate lines that are too wide to prevent bleeding
			truncated := TruncateRight(line, actualWidth)
			fullLine = MaintainBackground(truncated, ctx.Dialog)
		} else if textWidth < actualWidth {
			// Pad lines that are too narrow
			padding := lipgloss.NewStyle().Background(borderBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
			fullLine = MaintainBackground(line+padding, ctx.Dialog)
		} else {
			fullLine = MaintainBackground(line, ctx.Dialog)
		}

		result.WriteString(fullLine)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strutil.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
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

// renderDialogWithBorderCtx handles internal shared rendering logic using a specific context
func renderDialogWithBorderCtx(title, content string, border lipgloss.Border, focused bool, targetHeight int, threeD bool, useConnectors bool, titleStyle lipgloss.Style, ctx StyleContext) string {
	if title != "" && !strings.HasSuffix(title, "{{[-]}}") {
		title += "{{[-]}}"
	}

	borderBG := ctx.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(ctx.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(borderBG)

	if !threeD {
		borderStyleLight = borderStyleDark
	}

	titleStyle = lipgloss.NewStyle().
		Background(borderBG).
		Inherit(titleStyle)

	title = RenderThemeText(title, titleStyle)
	content = RenderThemeText(content, ctx.Dialog)

	lines := strings.Split(content, "\n")
	actualWidth := 0
	for _, line := range lines {
		// Use WidthWithoutZones to avoid zone markers inflating width
		w := WidthWithoutZones(line)
		if w > actualWidth {
			actualWidth = w
		}
	}

	if actualWidth%2 != 0 {
		actualWidth++
	}

	if targetHeight > 2 {
		contentHeight := len(lines)
		neededPadding := (targetHeight - 2) - contentHeight
		if neededPadding > 0 {
			for i := 0; i < neededPadding; i++ {
				lines = append(lines, "")
			}
		}
	}

	var result strings.Builder

	result.WriteString(borderStyleLight.Render(border.TopLeft))
	if title == "" {
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, actualWidth)))
	} else {
		var leftT, rightT string
		if useConnectors {
			if ctx.LineCharacters {
				if focused {
					leftT = "┫"
					rightT = "┣"
				} else {
					leftT = "┤"
					rightT = "├"
				}
			} else {
				if focused {
					leftT = "H"
					rightT = "H"
				} else {
					leftT = "|"
					rightT = "|"
				}
			}
		} else {
			leftT = border.Top
			rightT = border.Top
		}

		titleSectionLen := 1 + 1 + lipgloss.Width(title) + 1 + 1
		if titleSectionLen > actualWidth {
			actualWidth = titleSectionLen
		}

		leftPad := (actualWidth - titleSectionLen) / 2
		rightPad := actualWidth - titleSectionLen - leftPad
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, leftPad)))
		result.WriteString(borderStyleLight.Render(leftT))
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(title)
		result.WriteString(borderStyleLight.Render(" "))
		result.WriteString(borderStyleLight.Render(rightT))
		result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPad)))
	}
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		// Use WidthWithoutZones to get accurate visual width (zone markers are invisible)
		textWidth := WidthWithoutZones(line)
		padding := ""
		if textWidth < actualWidth {
			padding = lipgloss.NewStyle().Background(borderBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
		}
		fullLine := MaintainBackground(line+padding, ctx.Dialog)
		result.WriteString(fullLine)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strutil.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}

// ZoneMark wraps content in a zone identifier
func ZoneMark(id string, content string) string {
	return zone.Mark(id, content)
}

// ZoneClick returns true if the message is a left-click within the specified zone
func ZoneClick(msg tea.Msg, id string) bool {
	if mouseMsg, ok := msg.(tea.MouseClickMsg); ok && mouseMsg.Button == tea.MouseLeft {
		if info := zone.Get(id); info != nil {
			return info.InBounds(mouseMsg)
		}
	}
	return false
}
