package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v3"
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
}

// currentStyles holds the active styles
var currentStyles Styles

// GetStyles returns the current styles
func GetStyles() Styles {
	return currentStyles
}

// tcellToLipgloss converts a tcell.Color to lipgloss.Color
func tcellToLipgloss(c tcell.Color) lipgloss.Color {
	if c == tcell.ColorDefault {
		return lipgloss.Color("")
	}
	return lipgloss.Color(fmt.Sprintf("#%06x", c.Hex()))
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
		Background(tcellToLipgloss(t.ScreenBG)).
		Foreground(tcellToLipgloss(t.ScreenFG))

	// Dialog
	currentStyles.Dialog = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.DialogBG)).
		Foreground(tcellToLipgloss(t.DialogFG))

	currentStyles.DialogTitle = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.TitleBG)).
		Foreground(tcellToLipgloss(t.TitleFG)).
		Bold(t.TitleBold).
		Underline(t.TitleUnderline)

	// Border colors
	currentStyles.BorderColor = tcellToLipgloss(t.BorderFG)
	currentStyles.Border2Color = tcellToLipgloss(t.Border2FG)

	// Shadow
	currentStyles.ShadowColor = tcellToLipgloss(t.ShadowColor)
	currentStyles.Shadow = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ShadowColor))

	// Buttons (spacing handled at layout level)
	currentStyles.ButtonActive = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ButtonActiveBG)).
		Foreground(tcellToLipgloss(t.ButtonActiveFG))

	currentStyles.ButtonInactive = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ButtonInactiveBG)).
		Foreground(tcellToLipgloss(t.ButtonInactiveFG))

	// List items
	currentStyles.ItemNormal = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ItemBG)).
		Foreground(tcellToLipgloss(t.ItemFG))

	currentStyles.ItemSelected = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ItemSelectedBG)).
		Foreground(tcellToLipgloss(t.ItemSelectedFG))

	// Tags
	currentStyles.TagNormal = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.TagBG)).
		Foreground(tcellToLipgloss(t.TagFG))

	currentStyles.TagKey = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.TagBG)).
		Foreground(tcellToLipgloss(t.TagKeyFG))

	currentStyles.TagKeySelected = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ItemSelectedBG)).
		Foreground(tcellToLipgloss(t.TagKeySelectedFG))

	// Header
	currentStyles.HeaderBG = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ScreenBG)).
		Foreground(tcellToLipgloss(t.ScreenFG))

	// Help line
	currentStyles.HelpLine = lipgloss.NewStyle().
		Background(tcellToLipgloss(t.ItemHelpBG)).
		Foreground(tcellToLipgloss(t.ItemHelpFG))
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

	// Get width from first line
	contentWidth := lipgloss.Width(lines[0])

	// Create shadow cells (2 chars wide for right shadow)
	shadowCell := currentStyles.Shadow.Width(2).Height(1).Render("")
	spacerCell := lipgloss.NewStyle().
		Background(currentStyles.Screen.GetBackground()).
		Width(2).Height(1).Render("")

	var result strings.Builder

	// First line: content + spacer (no shadow on top row)
	result.WriteString(lines[0])
	result.WriteString(spacerCell)
	result.WriteString("\n")

	// Middle and last content lines: content + 2-char shadow on right
	for i := 1; i < len(lines); i++ {
		result.WriteString(lines[i])
		result.WriteString(shadowCell)
		result.WriteString("\n")
	}

	// Bottom shadow row: 1-char spacer + shadow across (width-1) + 2-char corner shadow
	// This creates the proper 1-right, 1-down offset
	spacer1 := lipgloss.NewStyle().
		Background(currentStyles.Screen.GetBackground()).
		Width(1).Height(1).Render("")
	result.WriteString(spacer1)
	bottomShadow := currentStyles.Shadow.Width(contentWidth - 1).Height(1).Render("")
	result.WriteString(bottomShadow)
	result.WriteString(shadowCell)

	return result.String()
}
