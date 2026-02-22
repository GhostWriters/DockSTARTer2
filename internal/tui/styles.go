package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Styles holds all lipgloss styles derived from the theme
type Styles struct {
	// Screen
	Screen lipgloss.Style

	// Dialog
	Dialog          lipgloss.Style
	DialogTitle     lipgloss.Style
	DialogTitleHelp lipgloss.Style

	// Borders
	Border       lipgloss.Border
	BorderColor  color.Color
	Border2Color color.Color

	// Shadow
	Shadow      lipgloss.Style
	ShadowColor color.Color

	// Buttons
	ButtonActive   lipgloss.Style
	ButtonInactive lipgloss.Style

	// List items
	ItemNormal   lipgloss.Style
	ItemSelected lipgloss.Style

	// Tags (menu item labels)
	TagNormal      lipgloss.Style
	TagSelected    lipgloss.Style
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
	DrawBorders    bool

	// Semantic styles derived from theme tags
	StatusSuccess lipgloss.Style
	StatusWarn    lipgloss.Style
	StatusError   lipgloss.Style
	Console       lipgloss.Style

	// Log panel border/strip color
	LogPanelColor color.Color
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

// thickAsciiBorder simulates a thick border using ASCII characters (#===# style)
var thickAsciiBorder = lipgloss.Border{
	Top:         "=",
	Bottom:      "=",
	Left:        "H",
	Right:       "H",
	TopLeft:     "#",
	TopRight:    "#",
	BottomLeft:  "#",
	BottomRight: "#",
}

// roundedThickAsciiBorder simulates a thick border with rounded corners (.===H===. style)
var roundedThickAsciiBorder = lipgloss.Border{
	Top:         "=",
	Bottom:      "=",
	Left:        "H",
	Right:       "H",
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
	// Clear the semantic style cache to ensure real-time visual updates on theme swap
	ClearSemanticCache()

	defaults, _ := theme.Load(cfg.UI.Theme, "") // Updated: Use cfg.UI.Theme and get defaults
	if defaults != nil {
		// Apply theme defaults to the configuration if they are defined in the theme file.
		// This uses the existing ApplyThemeDefaults routine.
		theme.ApplyThemeDefaults(&cfg, *defaults)
	}

	// Store LineCharacters setting for later use
	currentStyles.LineCharacters = cfg.UI.LineCharacters // Updated: Use cfg.UI.LineCharacters
	currentStyles.DrawBorders = cfg.UI.Borders

	// Border style based on LineCharacters setting
	if cfg.UI.LineCharacters { // Updated: Use cfg.UI.LineCharacters
		currentStyles.Border = lipgloss.RoundedBorder()
		currentStyles.SepChar = "─"
	} else {
		currentStyles.Border = asciiBorder
		currentStyles.SepChar = "-"
	}

	// Screen background
	currentStyles.Screen = SemanticRawStyle("Theme_Screen")

	// Dialog
	currentStyles.Dialog = SemanticRawStyle("Theme_Dialog")

	currentStyles.DialogTitle = SemanticRawStyle("Theme_Title")

	currentStyles.DialogTitleHelp = SemanticRawStyle("Theme_TitleHelp")

	// Border colors
	switch cfg.UI.BorderColor {
	case 1:
		currentStyles.BorderColor = SemanticRawStyle("Theme_Border").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Theme_Border").GetForeground()
	case 2:
		currentStyles.BorderColor = SemanticRawStyle("Theme_Border2").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Theme_Border2").GetForeground()
	case 3:
		fallthrough
	default:
		currentStyles.BorderColor = SemanticRawStyle("Theme_Border").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Theme_Border2").GetForeground()
	}

	// Shadow
	currentStyles.ShadowColor = SemanticRawStyle("Theme_Shadow").GetBackground()
	currentStyles.Shadow = lipgloss.NewStyle().
		Background(currentStyles.ShadowColor)

	// Buttons (spacing handled at layout level)
	// Handle nil (inherit) backgrounds by falling back to DialogBG
	currentStyles.ButtonActive = SemanticRawStyle("Theme_ButtonActive")
	if currentStyles.ButtonActive.GetBackground() == nil {
		currentStyles.ButtonActive = currentStyles.ButtonActive.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ButtonInactive = SemanticRawStyle("Theme_ButtonInactive")
	if currentStyles.ButtonInactive.GetBackground() == nil {
		currentStyles.ButtonInactive = currentStyles.ButtonInactive.Background(currentStyles.Dialog.GetBackground())
	}

	// List items
	currentStyles.ItemNormal = SemanticRawStyle("Theme_Item")
	if currentStyles.ItemNormal.GetBackground() == nil {
		currentStyles.ItemNormal = currentStyles.ItemNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ItemSelected = SemanticRawStyle("Theme_ItemSelected")
	if currentStyles.ItemSelected.GetBackground() == nil {
		currentStyles.ItemSelected = currentStyles.ItemSelected.Background(currentStyles.Dialog.GetBackground())
	}

	// Tags
	currentStyles.TagNormal = SemanticRawStyle("Theme_Tag")
	if currentStyles.TagNormal.GetBackground() == nil {
		currentStyles.TagNormal = currentStyles.TagNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagSelected = SemanticRawStyle("Theme_TagSelected")
	if currentStyles.TagSelected.GetBackground() == nil {
		currentStyles.TagSelected = currentStyles.TagSelected.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKey = SemanticRawStyle("Theme_TagKey")
	if currentStyles.TagKey.GetBackground() == nil {
		currentStyles.TagKey = currentStyles.TagKey.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKeySelected = SemanticRawStyle("Theme_TagKeySelected")
	if currentStyles.TagKeySelected.GetBackground() == nil {
		currentStyles.TagKeySelected = currentStyles.TagKeySelected.Background(currentStyles.Dialog.GetBackground())
	}

	// Header
	currentStyles.HeaderBG = currentStyles.Screen

	// Help line
	currentStyles.HelpLine = SemanticRawStyle("Theme_Helpline")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	currentStyles.StatusSuccess = SemanticRawStyle("Theme_TitleNotice")
	currentStyles.StatusWarn = SemanticRawStyle("Theme_TitleWarn")
	currentStyles.StatusError = SemanticRawStyle("Theme_TitleError")
	currentStyles.Console = SemanticRawStyle("Theme_ProgramBox")

	currentStyles.LogPanelColor = SemanticRawStyle("Theme_LogPanel").GetForeground()
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
	borderBG := SemanticStyle("{{|Theme_Dialog|}}").GetBackground()

	borderStyle := lipgloss.NewStyle().
		Background(borderBG).
		Border(currentStyles.Border).
		BorderTopForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderLeftForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderBottomForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderRightForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG).
		Padding(0, 1)

	return style.Inherit(borderStyle)
}

// ApplyStraightBorder applies a 3D border with straight edges
// Uses asciiBorder or NormalBorder based on LineCharacters setting
func ApplyStraightBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := SemanticStyle("{{|Theme_Dialog|}}").GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.NormalBorder()
	} else {
		border = asciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderLeftForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderBottomForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderRightForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyThickBorder applies a 3D border with thick edges
// Uses thickAsciiBorder or ThickBorder based on LineCharacters setting
func ApplyThickBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	borderBG := SemanticStyle("{{|Theme_Dialog|}}").GetBackground()

	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.ThickBorder()
	} else {
		border = thickAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderLeftForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderBottomForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderRightForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyRoundedBorder applies a 3D border with rounded corners
// Uses roundedAsciiBorder or RoundedBorder based on LineCharacters setting
func ApplyRoundedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := SemanticStyle("{{|Theme_Dialog|}}").GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.RoundedBorder()
	} else {
		border = roundedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderLeftForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderBottomForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderRightForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplySlantedBorder applies a 3D border with slanted/beveled corners
// Uses slantedAsciiBorder or RoundedBorder based on LineCharacters setting
func ApplySlantedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	// Get the dialog background color for border backgrounds
	borderBG := SemanticStyle("{{|Theme_Dialog|}}").GetBackground()

	// Choose border style based on LineCharacters setting
	var border lipgloss.Border
	if useLineChars {
		border = lipgloss.RoundedBorder()
	} else {
		border = slantedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderLeftForeground(SemanticStyle("{{|Theme_Border|}}").GetForeground()).
		BorderBottomForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
		BorderRightForeground(SemanticStyle("{{|Theme_Border2|}}").GetForeground()).
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

	borderBG := SemanticRawStyle("Theme_Dialog").GetBackground()

	// Create style for light borders (top/left) - using theme border colors
	lightStyle := lipgloss.NewStyle().
		Foreground(SemanticRawStyle("Theme_Border").GetForeground()).
		Background(borderBG)

	// Create style for dark borders (bottom/right)
	darkColor := SemanticRawStyle("Theme_Border2").GetForeground()

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
	topLine := lightStyle.Render(border.TopLeft + strutil.Repeat(border.Top, totalWidth) + border.TopRight)
	result.WriteString(topLine)
	result.WriteString("\n")

	// Add padded content lines
	paddingStr := strutil.Repeat(" ", padding)
	for _, line := range lines {
		// Calculate how much padding needed on right to fill width
		lineWidth := lipgloss.Width(line)
		rightPad := maxWidth - lineWidth

		// Build the full line with proper width
		fullLine := paddingStr + line + strutil.Repeat(" ", rightPad) + paddingStr

		// Render each component separately
		leftBorder := lightStyle.Render(border.Left)
		rightBorder := darkStyle.Render(border.Right)

		// Style the content line with background
		styledContent := contentStyle.Width(0).Render(fullLine)

		// Join horizontally to preserve styles
		lineStr := lipgloss.JoinHorizontal(lipgloss.Top, leftBorder, styledContent, rightBorder)
		result.WriteString(lineStr)
		result.WriteString("\n")
	}

	// Bottom border: dark color
	bottomLine := darkStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, totalWidth) + border.BottomRight)
	result.WriteString(bottomLine)

	return result.String()
}

// AddShadow adds a shadow effect to rendered content if shadow is enabled
// Shadow is offset 1 character right and 1 down, with 2-char wide right shadow
func AddShadow(content string) string {
	if !currentConfig.UI.Shadow { // Updated: Use currentConfig.UI.Shadow
		return content // Changed `return inner` to `return content` as `inner` was not defined.
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
		shadowStyle := SemanticRawStyle("Theme_Shadow").
			Background(SemanticRawStyle("Theme_Screen").GetBackground())

		// Select shade character based on config
		var shadeChar string
		switch currentConfig.UI.ShadowLevel {
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

		shadowCell = shadowStyle.Render(strutil.Repeat(shadeChar, 2))
		bottomShadowChars = shadowStyle.Render(strutil.Repeat(shadeChar, contentWidth-1))
	} else {
		// ASCII mode: use solid background color
		shadowCell = SemanticRawStyle("Theme_Shadow").Width(2).Render(" ")
		bottomShadowChars = SemanticRawStyle("Theme_Shadow").Width(contentWidth - 1).Render(strutil.Repeat(" ", contentWidth-1))
	}

	spacerCell := lipgloss.NewStyle().
		Background(SemanticRawStyle("Theme_Screen").GetBackground()).
		Width(2).Render("  ")
	spacer1 := lipgloss.NewStyle().
		Background(SemanticStyle("{{|Theme_Screen|}}").GetBackground()).
		Width(1).Render(" ")

	var result strings.Builder

	// First line: content + spacer (no shadow on top row)
	line0 := lines[0]
	w0 := lipgloss.Width(line0)
	padding0 := ""
	if w0 < contentWidth {
		padding0 = strutil.Repeat(" ", contentWidth-w0)
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
			padding = strutil.Repeat(" ", contentWidth-w)
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

// AddPatternHalo surrounds content with a 1-cell halo of the shadow pattern
func AddPatternHalo(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Find maximum width
	maxWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	contentWidth := maxWidth
	if contentWidth%2 != 0 {
		contentWidth++
	}

	// Ensure halo uses the border color (Border2Color) as requested, to extend the border
	shadowStyle := SemanticStyle("{{|Theme_Border2|}}").
		Background(SemanticStyle("{{|Theme_Screen|}}").GetBackground())

	var shadeChar string

	// Force full block character for a solid border effect
	shadeChar = "█"

	// Create a single cell of shadow (2 characters wide)
	var shadowCell string
	if currentStyles.LineCharacters {
		shadowCell = shadowStyle.Render(strutil.Repeat(shadeChar, 2))
	} else {
		// Even in ASCII, we use the pattern style for the "halo" effect
		shadowCell = shadowStyle.Render(strutil.Repeat(shadeChar, 2))
	}

	// Horizontal shadow line (covers top/bottom + 2 cells for corners)
	// totalWidth = shadowCell(1) + contentWidth + shadowCell(1)
	gridWidth := contentWidth + 4
	numCells := gridWidth / 2
	horizontalShadow := strutil.Repeat(shadowCell, numCells)

	var result strings.Builder

	// Top halo row
	result.WriteString(horizontalShadow)
	result.WriteString("\n")

	// Content rows with halo on both sides
	for _, line := range lines {
		w := lipgloss.Width(line)
		padding := ""
		if w < contentWidth {
			padding = strutil.Repeat(" ", contentWidth-w)
		}
		result.WriteString(shadowCell)
		result.WriteString(line + padding)
		result.WriteString(shadowCell)
		result.WriteString("\n")
	}

	// Bottom halo row
	result.WriteString(horizontalShadow)

	return result.String()
}
