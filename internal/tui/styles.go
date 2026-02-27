package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// GetMenuItemID returns a unique layer ID for a menu item
func GetMenuItemID(menuID string, index int) string {
	return "item-" + menuID + "-" + strconv.Itoa(index)
}

// Z-Level constants for layering
const (
	ZBackdrop = 0
	ZScreen   = 10
	ZLogPanel = 20
	ZDialog   = 30
	ZHalo     = 35
	ZOverlay  = 40
)

// Global Layer IDs for hit testing
const (
	IDLogPanel    = "log_panel"
	IDLogToggle   = "log_toggle"
	IDLogResize   = "log_resize"
	IDLogViewport = "log_viewport"
	IDAppVersion  = "app_version"
	IDTmplVersion = "tmpl_version"

	// Display Options IDs
	IDThemePanel   = "theme_panel"
	IDOptionsPanel = "options_panel"
	IDApplyButton  = "apply_button"
	IDBackButton   = "back_button"
	IDExitButton   = "exit_button"
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
	HeaderBG           lipgloss.Style
	StatusBar          lipgloss.Style
	StatusBarSeparator lipgloss.Style

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

// StyleContext holds a subset of Styles for decoupled rendering
type StyleContext struct {
	LineCharacters  bool
	DrawBorders     bool
	Screen          lipgloss.Style
	Dialog          lipgloss.Style
	DialogTitle     lipgloss.Style
	DialogTitleHelp lipgloss.Style
	Border          lipgloss.Border
	BorderColor     color.Color
	Border2Color    color.Color
	ButtonActive    lipgloss.Style
	ButtonInactive  lipgloss.Style
	ItemNormal      lipgloss.Style
	ItemSelected    lipgloss.Style
	TagNormal       lipgloss.Style
	TagSelected     lipgloss.Style
	TagKey          lipgloss.Style
	TagKeySelected  lipgloss.Style
	Shadow          lipgloss.Style
	ShadowColor     color.Color
	ShadowLevel     int
	HelpLine        lipgloss.Style
	StatusSuccess   lipgloss.Style
	StatusWarn      lipgloss.Style
	StatusError     lipgloss.Style
	Console         lipgloss.Style
	LogPanelColor   color.Color
}

// currentStyles holds the active styles
var currentStyles Styles

// GetStyles returns the current styles
func GetStyles() Styles {
	return currentStyles
}

// GetActiveContext returns the current global styles as a StyleContext
func GetActiveContext() StyleContext {
	return StyleContext{
		LineCharacters:  currentStyles.LineCharacters,
		DrawBorders:     currentStyles.DrawBorders,
		Screen:          currentStyles.Screen,
		Dialog:          currentStyles.Dialog,
		DialogTitle:     currentStyles.DialogTitle,
		DialogTitleHelp: currentStyles.DialogTitleHelp,
		Border:          currentStyles.Border,
		BorderColor:     currentStyles.BorderColor,
		Border2Color:    currentStyles.Border2Color,
		ButtonActive:    currentStyles.ButtonActive,
		ButtonInactive:  currentStyles.ButtonInactive,
		ItemNormal:      currentStyles.ItemNormal,
		ItemSelected:    currentStyles.ItemSelected,
		TagNormal:       currentStyles.TagNormal,
		TagSelected:     currentStyles.TagSelected,
		TagKey:          currentStyles.TagKey,
		TagKeySelected:  currentStyles.TagKeySelected,
		Shadow:          currentStyles.Shadow,
		ShadowColor:     currentStyles.ShadowColor,
		ShadowLevel:     currentConfig.UI.ShadowLevel,
		HelpLine:        currentStyles.HelpLine,
		StatusSuccess:   currentStyles.StatusSuccess,
		StatusWarn:      currentStyles.StatusWarn,
		StatusError:     currentStyles.StatusError,
		Console:         currentStyles.Console,
		LogPanelColor:   currentStyles.LogPanelColor,
	}
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

// RoundedAsciiBorder defines a softer ASCII border with rounded appearance for buttons
var RoundedAsciiBorder = lipgloss.Border{
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

// RoundedThickAsciiBorder simulates a thick border with rounded corners (.===H===. style)
var RoundedThickAsciiBorder = lipgloss.Border{
	Top:         "=",
	Bottom:      "=",
	Left:        "H",
	Right:       "H",
	TopLeft:     ".",
	TopRight:    ".",
	BottomLeft:  "'",
	BottomRight: "'",
}

// ThickRoundedBorder defines a thick border with rounded corners (━┃╭╮ style)
var ThickRoundedBorder = lipgloss.Border{
	Top:         "━",
	Bottom:      "━",
	Left:        "┃",
	Right:       "┃",
	TopLeft:     "╭",
	TopRight:    "╮",
	BottomLeft:  "╰",
	BottomRight: "╯",
}

// SlantedAsciiBorder defines a beveled ASCII border with slanted corners
var SlantedAsciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "/",
	TopRight:    "\\",
	BottomLeft:  "\\",
	BottomRight: "/",
}

// SlantedBorder defines a beveled border with slanted corners (Unicode)
var SlantedBorder = lipgloss.Border{
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	TopLeft:     "◢",
	TopRight:    "◣",
	BottomLeft:  "◥",
	BottomRight: "◤",
}

// SlantedThickBorder defines a thick beveled border with slanted corners (Unicode)
var SlantedThickBorder = lipgloss.Border{
	Top:         "━",
	Bottom:      "━",
	Left:        "┃",
	Right:       "┃",
	TopLeft:     "◢",
	TopRight:    "◣",
	BottomLeft:  "◥",
	BottomRight: "◤",
}

// SlantedThickAsciiBorder defines a thick beveled ASCII border with slanted corners
var SlantedThickAsciiBorder = lipgloss.Border{
	Top:         "=",
	Bottom:      "=",
	Left:        "H",
	Right:       "H",
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

	// Update the global config so IsShadowEnabled(), GetActiveContext().ShadowLevel,
	// and any other currentConfig readers see the new values immediately.
	currentConfig = cfg

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
	// Theme_Shadow defines the shadow color (foreground is used for shade characters like ░▒▓)
	shadowDef := SemanticRawStyle("Theme_Shadow")
	currentStyles.ShadowColor = shadowDef.GetForeground()
	if currentStyles.ShadowColor == nil {
		currentStyles.ShadowColor = shadowDef.GetBackground()
	}
	// Create shadow style with just the foreground color for shade chars
	currentStyles.Shadow = lipgloss.NewStyle().Foreground(currentStyles.ShadowColor)

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

	// Header / Status Bar
	currentStyles.StatusBar = SemanticRawStyle("Theme_StatusBar")
	currentStyles.StatusBarSeparator = SemanticRawStyle("Theme_StatusBarSeparator")
	currentStyles.HeaderBG = currentStyles.StatusBar // Backwards compatibility

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
	return Apply3DBorderCtx(style, GetActiveContext())
}

// Apply3DBorderCtx applies 3D border effect using a specific context
func Apply3DBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	borderStyle := lipgloss.NewStyle().
		Background(borderBG).
		Border(ctx.Border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG).
		Padding(0, 1)

	return style.Inherit(borderStyle)
}

// ApplyStraightBorder applies a 3D border with straight edges
// Uses asciiBorder or NormalBorder based on ctx.LineCharacters setting
func ApplyStraightBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyStraightBorderCtx(style, ctx)
}

// ApplyStraightBorderCtx applies a straight border using a specific context
func ApplyStraightBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.NormalBorder()
	} else {
		border = asciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyThickBorder applies a 3D border with thick edges
func ApplyThickBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyThickBorderCtx(style, ctx)
}

// ApplyThickBorderCtx applies a thick border using a specific context
func ApplyThickBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.ThickBorder()
	} else {
		border = thickAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyRoundedBorder applies a 3D border with rounded corners
func ApplyRoundedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyRoundedBorderCtx(style, ctx)
}

// ApplyRoundedBorderCtx applies a rounded border using a specific context
func ApplyRoundedBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.RoundedBorder()
	} else {
		border = RoundedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplyInnerBorder applies a rounded border that becomes thick when focused
func ApplyInnerBorder(style lipgloss.Style, focused bool, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplyInnerBorderCtx(style, focused, ctx)
}

// ApplyInnerBorderCtx applies a rounded border that becomes thick when focused
func ApplyInnerBorderCtx(style lipgloss.Style, focused bool, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// ApplySlantedBorder applies a 3D border with slanted/beveled corners
func ApplySlantedBorder(style lipgloss.Style, useLineChars bool) lipgloss.Style {
	ctx := GetActiveContext()
	ctx.LineCharacters = useLineChars
	return ApplySlantedBorderCtx(style, ctx)
}

// ApplySlantedBorderCtx applies a slanted border using a specific context
func ApplySlantedBorderCtx(style lipgloss.Style, ctx StyleContext) lipgloss.Style {
	borderBG := ctx.Dialog.GetBackground()

	var border lipgloss.Border
	if ctx.LineCharacters {
		border = SlantedBorder
	} else {
		border = SlantedAsciiBorder
	}

	return style.
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG)
}

// Render3DBorder manually renders content with a 3D border effect
func Render3DBorder(content string, padding int) string {
	return Render3DBorderCtx(content, padding, GetActiveContext())
}

// Render3DBorderCtx renders content with 3D border using specific context
func Render3DBorderCtx(content string, padding int, ctx StyleContext) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	maxWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	totalWidth := maxWidth + padding*2
	borderBG := ctx.Dialog.GetBackground()

	lightStyle := lipgloss.NewStyle().
		Foreground(ctx.BorderColor).
		Background(borderBG)

	darkStyle := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(borderBG)

	contentStyle := lipgloss.NewStyle().
		Background(borderBG).
		Width(totalWidth)

	border := ctx.Border
	var result strings.Builder

	topLine := lightStyle.Render(border.TopLeft + strutil.Repeat(border.Top, totalWidth) + border.TopRight)
	result.WriteString(topLine)
	result.WriteString("\n")

	paddingStr := strutil.Repeat(" ", padding)
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		rightPad := maxWidth - lineWidth
		fullLine := paddingStr + line + strutil.Repeat(" ", rightPad) + paddingStr

		leftBorder := lightStyle.Render(border.Left)
		rightBorder := darkStyle.Render(border.Right)
		styledContent := contentStyle.Width(0).Render(fullLine)

		lineStr := lipgloss.JoinHorizontal(lipgloss.Top, leftBorder, styledContent, rightBorder)
		result.WriteString(lineStr)
		result.WriteString("\n")
	}

	bottomLine := darkStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, totalWidth) + border.BottomRight)
	result.WriteString(bottomLine)

	return result.String()
}

// AddPatternHalo surrounds content with a 1-cell halo.
// If haloBg is provided, it uses shared background for a solid look.
func AddPatternHalo(content string, haloBg ...color.Color) string {
	var bg color.Color
	if len(haloBg) > 0 {
		bg = haloBg[0]
	}
	return AddHalo(content, bg)
}

// AddHalo adds a halo effect (uniform outline) to rendered content if shadow is enabled
func AddHalo(content string, haloBg color.Color) string {
	return AddHaloCtx(content, haloBg, GetActiveContext())
}

// AddHaloCtx adds a halo effect using a specific context.
// If haloBg is provided, it renders a solid halo matching that color.
func AddHaloCtx(content string, haloBg color.Color, ctx StyleContext) string {
	// Split content into lines
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Use WidthWithoutZones to get accurate visual width
	contentWidth := 0
	for _, line := range lines {
		w := WidthWithoutZones(line)
		if w > contentWidth {
			contentWidth = w
		}
	}

	var haloCell, horizontalHalo string

	if haloBg != nil {
		// Solid mode: use space characters with the provided background color
		solidStyle := lipgloss.NewStyle().Background(haloBg)
		haloCell = solidStyle.Render("  ")
		horizontalHalo = solidStyle.Render(strutil.Repeat(" ", contentWidth+4))
	} else if ctx.LineCharacters {
		// Unicode mode: use shade characters (░▒▓█)
		haloStyle := ctx.Shadow.Background(ctx.Screen.GetBackground())

		var shadeChar string
		switch ctx.ShadowLevel {
		case 0:
			shadeChar = " "
		case 1:
			shadeChar = "░"
		case 2:
			shadeChar = "▒"
		case 3:
			shadeChar = "▓"
		case 4:
			shadeChar = "█"
		default:
			shadeChar = "▒"
		}

		haloCell = haloStyle.Render(strutil.Repeat(shadeChar, 2))
		horizontalHalo = haloStyle.Render(strutil.Repeat(shadeChar, contentWidth+4))
	} else {
		// ASCII mode
		if ctx.ShadowLevel == 4 {
			solidStyle := lipgloss.NewStyle().Background(ctx.ShadowColor)
			haloCell = solidStyle.Render("  ")
			horizontalHalo = solidStyle.Render(strutil.Repeat(" ", contentWidth+4))
		} else {
			asciiHaloStyle := ctx.Shadow.Background(ctx.Screen.GetBackground())
			var asciiShadeChar string
			switch ctx.ShadowLevel {
			case 0:
				asciiShadeChar = " "
			case 1:
				asciiShadeChar = "."
			case 2:
				asciiShadeChar = ":"
			case 3:
				asciiShadeChar = "#"
			default:
				asciiShadeChar = ":"
			}
			haloCell = asciiHaloStyle.Render(strutil.Repeat(asciiShadeChar, 2))
			horizontalHalo = asciiHaloStyle.Render(strutil.Repeat(asciiShadeChar, contentWidth+4))
		}
	}

	var result strings.Builder

	// Top Halo
	result.WriteString(horizontalHalo)
	result.WriteString("\n")

	// Content with side halos
	for _, line := range lines {
		w := WidthWithoutZones(line)
		padding := ""
		if w < contentWidth {
			padding = strutil.Repeat(" ", contentWidth-w)
		}
		result.WriteString(haloCell)
		result.WriteString(line + padding)
		result.WriteString(haloCell)
		result.WriteString("\n")
	}

	// Bottom Halo
	result.WriteString(horizontalHalo)

	return result.String()
}

// AddShadow adds a shadow effect to rendered content if shadow is enabled
func AddShadow(content string) string {
	return AddShadowCtx(content, GetActiveContext())
}

// AddShadowCtx adds a shadow effect using a specific context
func AddShadowCtx(content string, ctx StyleContext) string {
	// Split content into lines
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Use WidthWithoutZones to get accurate visual width
	contentWidth := 0
	for _, line := range lines {
		w := WidthWithoutZones(line)
		if w > contentWidth {
			contentWidth = w
		}
	}

	var shadowCell, bottomShadowChars string

	if ctx.LineCharacters {
		// Unicode mode: use shade characters (░▒▓█)
		shadowStyle := ctx.Shadow.
			Background(ctx.Screen.GetBackground())

		var shadeChar string
		switch ctx.ShadowLevel {
		case 0:
			shadeChar = " "
		case 1:
			shadeChar = "░"
		case 2:
			shadeChar = "▒"
		case 3:
			shadeChar = "▓"
		case 4:
			shadeChar = "█"
		default:
			shadeChar = "▒"
		}

		shadowCell = shadowStyle.Render(strutil.Repeat(shadeChar, 2))
		bottomShadowChars = shadowStyle.Render(strutil.Repeat(shadeChar, contentWidth-1))
	} else {
		// ASCII mode
		if ctx.ShadowLevel == 4 {
			solidStyle := lipgloss.NewStyle().Background(ctx.ShadowColor)
			shadowCell = solidStyle.Render("  ")
			bottomShadowChars = solidStyle.Render(strutil.Repeat(" ", contentWidth-1))
		} else {
			asciiShadowStyle := ctx.Shadow.
				Background(ctx.Screen.GetBackground())

			var asciiShadeChar string
			switch ctx.ShadowLevel {
			case 0:
				asciiShadeChar = " "
			case 1:
				asciiShadeChar = "."
			case 2:
				asciiShadeChar = ":"
			case 3:
				asciiShadeChar = "#"
			default:
				asciiShadeChar = ":"
			}

			shadowCell = asciiShadowStyle.Render(strutil.Repeat(asciiShadeChar, 2))
			bottomShadowChars = asciiShadowStyle.Render(strutil.Repeat(asciiShadeChar, contentWidth-1))
		}
	}

	spacerCell := lipgloss.NewStyle().
		Background(ctx.Screen.GetBackground()).
		Width(2).Render("  ")
	spacer1 := lipgloss.NewStyle().
		Background(ctx.Screen.GetBackground()).
		Width(1).Render(" ")

	var result strings.Builder

	line0 := lines[0]
	w0 := WidthWithoutZones(line0)
	padding0 := ""
	if w0 < contentWidth {
		padding0 = strutil.Repeat(" ", contentWidth-w0)
	}
	result.WriteString(line0 + padding0)
	result.WriteString(spacerCell)
	result.WriteString("\n")

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		w := WidthWithoutZones(line)
		padding := ""
		if w < contentWidth {
			padding = strutil.Repeat(" ", contentWidth-w)
		}
		result.WriteString(line + padding)
		result.WriteString(shadowCell)
		result.WriteString("\n")
	}

	result.WriteString(spacer1)
	result.WriteString(bottomShadowChars)
	result.WriteString(shadowCell)

	return result.String()
}
