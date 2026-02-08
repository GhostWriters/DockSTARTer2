package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds all lipgloss styles derived from the theme
type Styles struct {
	// Screen
	Screen lipgloss.Style

	// Dialog
	Dialog      lipgloss.Style
	DialogTitle lipgloss.Style

	// Borders
	Border       lipgloss.Border
	BorderColor  lipgloss.TerminalColor
	Border2Color lipgloss.TerminalColor

	// Shadow
	Shadow      lipgloss.Style
	ShadowColor lipgloss.TerminalColor

	// Buttons
	ButtonActive   lipgloss.Style
	ButtonInactive lipgloss.Style

	// List items
	ItemNormal   lipgloss.Style
	ItemSelected lipgloss.Style

	// Tags (menu item labels)
	TagNormal      lipgloss.Style
	TagKey         lipgloss.Style // First letter highlight
	TagKeySelected lipgloss.Style

	// Header
	HeaderBG lipgloss.Style

	// Help line
	HelpLine lipgloss.Style

	// Separator
	SepChar string

	// Settings
	LineCharacters bool

	// Semantic styles derived from theme tags
	StatusSuccess lipgloss.Style
	StatusWarn    lipgloss.Style
	StatusError   lipgloss.Style
	Console       lipgloss.Style
}

// currentStyles holds the active styles
var currentStyles Styles

// GetStyles returns the current styles
func GetStyles() Styles {
	return currentStyles
}

// asciiBorder defines a simple ASCII-only border for terminals without Unicode support
var asciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
}

// roundedAsciiBorder defines a softer ASCII border with rounded appearance for buttons
var roundedAsciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     ".",
	TopRight:    ".",
	BottomLeft:  "'",
	BottomRight: "'",
}

// slantedAsciiBorder defines a beveled ASCII border with slanted corners
var slantedAsciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "/",
	TopRight:    "\\",
	BottomLeft:  "\\",
	BottomRight: "/",
}

// InitStyles initializes lipgloss styles from the current theme
func InitStyles(cfg config.AppConfig) {
	t := theme.Current

	// Store LineCharacters setting for later use
	currentStyles.LineCharacters = cfg.LineCharacters

	// Border style based on LineCharacters setting
	if cfg.LineCharacters {
		currentStyles.Border = lipgloss.RoundedBorder()
		currentStyles.SepChar = "─"
	} else {
		currentStyles.Border = asciiBorder
		currentStyles.SepChar = "-"
	}

	// Screen background
	currentStyles.Screen = lipgloss.NewStyle().
		Background(t.ScreenBG).
		Foreground(t.ScreenFG)

	// Dialog
	currentStyles.Dialog = lipgloss.NewStyle().
		Background(t.DialogBG).
		Foreground(t.DialogFG)

	currentStyles.DialogTitle = lipgloss.NewStyle().
		Background(t.TitleBG).
		Foreground(t.TitleFG).
		Bold(t.TitleBold).
		Underline(t.TitleUnderline)

	// Border colors
	currentStyles.BorderColor = t.BorderFG
	currentStyles.Border2Color = t.Border2FG

	// Shadow
	currentStyles.ShadowColor = t.ShadowColor
	currentStyles.Shadow = lipgloss.NewStyle().
		Background(t.ShadowColor)

	// Buttons (spacing handled at layout level)
	currentStyles.ButtonActive = ApplyFlags(lipgloss.NewStyle().
		Background(t.ButtonActiveBG).
		Foreground(t.ButtonActiveFG), t.ButtonActiveStyles)

	currentStyles.ButtonInactive = ApplyFlags(lipgloss.NewStyle().
		Background(t.ButtonInactiveBG).
		Foreground(t.ButtonInactiveFG), t.ButtonInactiveStyles)

	// List items
	currentStyles.ItemNormal = lipgloss.NewStyle().
		Background(t.ItemBG).
		Foreground(t.ItemFG)

	currentStyles.ItemSelected = lipgloss.NewStyle().
		Background(t.ItemSelectedBG).
		Foreground(t.ItemSelectedFG)

	// Tags
	currentStyles.TagNormal = lipgloss.NewStyle().
		Background(t.TagBG).
		Foreground(t.TagFG)

	currentStyles.TagKey = lipgloss.NewStyle().
		Background(t.TagBG).
		Foreground(t.TagKeyFG)

	currentStyles.TagKeySelected = lipgloss.NewStyle().
		Background(t.ItemSelectedBG).
		Foreground(t.TagKeySelectedFG)

	// Header
	currentStyles.HeaderBG = lipgloss.NewStyle().
		Background(t.ScreenBG).
		Foreground(t.ScreenFG)

	// Help line
	currentStyles.HelpLine = lipgloss.NewStyle().
		Background(t.ItemHelpBG).
		Foreground(t.ItemHelpFG)

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	currentStyles.StatusSuccess = ApplyTagsToStyle("{{_ThemeTitleNotice_}}", lipgloss.NewStyle(), lipgloss.NewStyle())
	currentStyles.StatusWarn = ApplyTagsToStyle("{{_ThemeTitleWarn_}}", lipgloss.NewStyle(), lipgloss.NewStyle())
	currentStyles.StatusError = ApplyTagsToStyle("{{_ThemeTitleError_}}", lipgloss.NewStyle(), lipgloss.NewStyle())
	currentStyles.Console = ApplyTagsToStyle("{{_ThemeProgram_}}", lipgloss.NewStyle(), lipgloss.NewStyle())
}

// ApplyFlags applies ANSI style modifiers to a lipgloss.Style
func ApplyFlags(style lipgloss.Style, flags theme.StyleFlags) lipgloss.Style {
	style = style.
		Bold(flags.Bold).
		Underline(flags.Underline).
		Italic(flags.Italic).
		Blink(flags.Blink).
		Faint(flags.Dim).
		Reverse(flags.Reverse).
		Strikethrough(flags.Strikethrough)

	if flags.HighIntensity {
		if fg := style.GetForeground(); fg != nil {
			style = style.Foreground(brightenColor(fg))
		}
		if bg := style.GetBackground(); bg != nil {
			style = style.Background(brightenColor(bg))
		}
	}

	return style
}

// Helper functions for common style operations

// CenterText centers text within a given width
func CenterText(s string, width int) string {
	textWidth := lipgloss.Width(s)
	if textWidth >= width {
		return s
	}
	leftPad := (width - textWidth) / 2
	return lipgloss.NewStyle().PaddingLeft(leftPad).Render(s)
}

// PadRight pads text to fill width
func PadRight(s string, width int) string {
	textWidth := lipgloss.Width(s)
	if textWidth >= width {
		return s
	}
	return s + lipgloss.NewStyle().Width(width-textWidth).Render("")
}

// Apply3DBorder applies 3D border effect to a style
// Light color on top/left, dark color on bottom/right
func Apply3DBorder(style lipgloss.Style) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := currentStyles.Dialog.GetBackground()

	return style.
		Border(currentStyles.Border).
		BorderTopForeground(currentStyles.BorderColor).
		BorderLeftForeground(currentStyles.BorderColor).
		BorderBottomForeground(currentStyles.Border2Color).
		BorderRightForeground(currentStyles.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyStraightBorder applies a 3D border with straight edges
// Uses asciiBorder or NormalBorder based on LineCharacters setting
func ApplyStraightBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := currentStyles.Dialog.GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.NormalBorder()
	} else {
		border = asciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(currentStyles.BorderColor).
		BorderLeftForeground(currentStyles.BorderColor).
		BorderBottomForeground(currentStyles.Border2Color).
		BorderRightForeground(currentStyles.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyRoundedBorder applies a 3D border with rounded corners
// Uses roundedAsciiBorder or RoundedBorder based on LineCharacters setting
func ApplyRoundedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := currentStyles.Dialog.GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.RoundedBorder()
	} else {
		border = roundedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(currentStyles.BorderColor).
		BorderLeftForeground(currentStyles.BorderColor).
		BorderBottomForeground(currentStyles.Border2Color).
		BorderRightForeground(currentStyles.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplySlantedBorder applies a 3D border with slanted/beveled corners
// Uses slantedAsciiBorder or RoundedBorder based on LineCharacters setting
func ApplySlantedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := currentStyles.Dialog.GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.RoundedBorder()
	} else {
		border = slantedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(currentStyles.BorderColor).
		BorderLeftForeground(currentStyles.BorderColor).
		BorderBottomForeground(currentStyles.Border2Color).
		BorderRightForeground(currentStyles.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// Render3DBorder manually renders content with a 3D border effect
// This ensures proper color rendering for each border side
func Render3DBorder(content string, padding int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Find maximum line width (accounting for ANSI codes)
	maxWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	totalWidth := maxWidth + padding*2

	borderBG := currentStyles.Dialog.GetBackground()

	// Create style for light borders (top/left) - using theme border colors
	lightStyle := lipgloss.NewStyle().
		Foreground(currentStyles.BorderColor).
		Background(borderBG)

	// Create style for dark borders (bottom/right)
	// Use a darker/contrasting color - if Border2Color is too dark, use gray
	darkColor := currentStyles.Border2Color
	// If Border2Color appears to be black or very dark, use a visible gray instead
	darkColorStr := fmt.Sprintf("%v", darkColor)
	if strings.Contains(darkColorStr, "000000") || strings.Contains(darkColorStr, "Black") {
		darkColor = lipgloss.Color("#666666") // Medium gray for visibility
	}

	darkStyle := lipgloss.NewStyle().
		Foreground(darkColor).
		Background(borderBG)

	// Create style for content area with background
	contentStyle := lipgloss.NewStyle().
		Background(borderBG).
		Width(totalWidth)

	// Get border characters
	border := currentStyles.Border

	var result strings.Builder

	// Top border: light color
	topLine := lightStyle.Render(border.TopLeft + strings.Repeat(border.Top, totalWidth) + border.TopRight)
	result.WriteString(topLine)
	result.WriteString("\n")

	// Add padded content lines
	paddingStr := strings.Repeat(" ", padding)
	for _, line := range lines {
		// Calculate how much padding needed on right to fill width
		lineWidth := lipgloss.Width(line)
		rightPad := maxWidth - lineWidth

		// Build the full line with proper width
		fullLine := paddingStr + line + strings.Repeat(" ", rightPad) + paddingStr

		// Render each component separately
		leftBorder := lightStyle.Render(border.Left)
		rightBorder := darkStyle.Render(border.Right)

		// Style the content line with background
		styledContent := contentStyle.Copy().Width(0).Render(fullLine)

		// Join horizontally to preserve styles
		lineStr := lipgloss.JoinHorizontal(lipgloss.Top, leftBorder, styledContent, rightBorder)
		result.WriteString(lineStr)
		result.WriteString("\n")
	}

	// Bottom border: dark color
	bottomLine := darkStyle.Render(border.BottomLeft + strings.Repeat(border.Bottom, totalWidth) + border.BottomRight)
	result.WriteString(bottomLine)

	return result.String()
}

// AddShadow adds a shadow effect to rendered content if shadow is enabled
// Shadow is offset 1 character right and 1 down, with 2-char wide right shadow
func AddShadow(content string) string {
	if !currentConfig.Shadow {
		return content
	}

	// Split content into lines
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Calculate max width from all lines
	contentWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > contentWidth {
			contentWidth = w
		}
	}

	var shadowCell, bottomShadowChars string

	if currentStyles.LineCharacters {
		// Unicode mode: use shade characters with shadow color foreground
		shadowStyle := lipgloss.NewStyle().
			Foreground(currentStyles.ShadowColor).
			Background(currentStyles.Screen.GetBackground())

		// Select shade character based on config
		var shadeChar string
		switch currentConfig.ShadowLevel {
		case 1:
			shadeChar = "░" // Light shade (25%)
		case 2:
			shadeChar = "▒" // Medium shade (50%)
		case 3:
			shadeChar = "▓" // Dark shade (75%)
		case 4:
			shadeChar = "█" // Full block (100%)
		default:
			shadeChar = "▓" // Default to dark if invalid/unset
		}

		shadowCell = shadowStyle.Render(strings.Repeat(shadeChar, 2))
		bottomShadowChars = shadowStyle.Render(strings.Repeat(shadeChar, contentWidth-1))
	} else {
		// ASCII mode: use solid background color
		shadowCell = currentStyles.Shadow.Width(2).Height(1).Render("")
		bottomShadowChars = currentStyles.Shadow.Width(contentWidth - 1).Height(1).Render("")
	}

	spacerCell := lipgloss.NewStyle().
		Background(currentStyles.Screen.GetBackground()).
		Width(2).Height(1).Render("")
	spacer1 := lipgloss.NewStyle().
		Background(currentStyles.Screen.GetBackground()).
		Width(1).Height(1).Render("")

	var result strings.Builder

	// First line: content + spacer (no shadow on top row)
	line0 := lines[0]
	w0 := lipgloss.Width(line0)
	padding0 := ""
	if w0 < contentWidth {
		padding0 = strings.Repeat(" ", contentWidth-w0)
	}
	result.WriteString(line0 + padding0)
	result.WriteString(spacerCell)
	result.WriteString("\n")

	// Middle and last content lines: content + 2-char shadow on right
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		w := lipgloss.Width(line)
		padding := ""
		if w < contentWidth {
			padding = strings.Repeat(" ", contentWidth-w)
		}
		result.WriteString(line + padding)
		result.WriteString(shadowCell)
		result.WriteString("\n")
	}

	// Bottom shadow row: 1-char spacer + shadow across (width-1) + 2-char corner shadow
	// This creates the proper 1-right, 1-down offset
	result.WriteString(spacer1)
	result.WriteString(bottomShadowChars)
	result.WriteString(shadowCell)

	return result.String()
}
