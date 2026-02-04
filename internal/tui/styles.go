package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"fmt"

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

// InitStyles initializes lipgloss styles from the current theme
func InitStyles(cfg config.AppConfig) {
	t := theme.Current

	// Border style based on LineCharacters setting
	if cfg.LineCharacters {
		currentStyles.Border = lipgloss.RoundedBorder()
		currentStyles.SepChar = "─"
	} else {
		currentStyles.Border = lipgloss.NormalBorder()
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
		Bold(true)

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
		Background(tcellToLipgloss(t.ScreenBG)).
		Foreground(tcellToLipgloss(t.ScreenFG))
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
		BorderTopForeground(currentStyles.BorderColor).
		BorderLeftForeground(currentStyles.BorderColor).
		BorderBottomForeground(currentStyles.Border2Color).
		BorderRightForeground(currentStyles.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// AddShadow adds a shadow effect to rendered content if shadow is enabled
func AddShadow(content string) string {
	if !currentConfig.Shadow {
		return content
	}

	// Get dimensions of the content
	contentWidth := lipgloss.Width(content)
	contentHeight := lipgloss.Height(content)

	// Create shadow elements
	// Bottom shadow: 1 char high, contentWidth wide, positioned 1 char right
	bottomShadow := currentStyles.Shadow.
		Width(contentWidth).
		Height(1).
		Render("")

	// Right shadow: contentHeight high, 1 char wide
	rightShadow := currentStyles.Shadow.
		Width(1).
		Height(contentHeight).
		Render("")

	// Corner shadow: 1x1
	cornerShadow := currentStyles.Shadow.
		Width(1).
		Height(1).
		Render("")

	// Stack the content with shadows using JoinVertical and JoinHorizontal
	// First, add right shadow to content
	contentWithRightShadow := lipgloss.JoinHorizontal(lipgloss.Top, content, rightShadow)

	// Then add bottom shadow and corner
	// Use dialog background for the spacer to avoid black rectangle
	spacer := currentStyles.Dialog.Width(1).Height(1).Render(" ")
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, spacer+bottomShadow, cornerShadow)

	// Combine vertically
	return lipgloss.JoinVertical(lipgloss.Left, contentWithRightShadow, bottomRow)
}
