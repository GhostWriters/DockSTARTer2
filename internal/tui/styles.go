package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// GetMenuItemID returns a unique ID for a menu item
func GetMenuItemID(menuID string, index int) string {
	return "item-" + menuID + "-" + strconv.Itoa(index)
}

// HitRegion represents a clickable area for mouse hit testing
type HitRegion struct {
	ID     string
	X, Y   int
	Width  int
	Height int
	ZOrder int // Higher values are checked first (on top)
}

// HitRegionProvider is implemented by components that have clickable areas
type HitRegionProvider interface {
	GetHitRegions(offsetX, offsetY int) []HitRegion
}

// HitRegions is a slice of HitRegion that can be sorted by ZOrder
type HitRegions []HitRegion

// FindHit returns the ID of the topmost region containing the point, or empty string
func (regions HitRegions) FindHit(x, y int) string {
	// Check in reverse order (higher ZOrder regions are at the end after sorting)
	for i := len(regions) - 1; i >= 0; i-- {
		r := regions[i]
		if x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height {
			return r.ID
		}
	}
	return ""
}

// Z-Level constants for layering (used for rendering and hit region ordering)
const (
	ZBackdrop = 0
	ZHeader   = 2
	ZHelpline = 4
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

	// Status bar
	IDStatusBar = "status_bar"

	// Display Options IDs
	IDThemePanel   = "theme_panel"
	IDOptionsPanel = "options_panel"
	IDButtonPanel  = "button_panel"
	IDListPanel    = "list_panel"
	IDApplyButton  = "apply_button"
	IDBackButton   = "back_button"
	IDExitButton   = "exit_button"
)

// Styles holds all lipgloss styles derived from the theme
type Styles struct {
	// Screen
	Screen lipgloss.Style

	// Dialog
	Dialog              lipgloss.Style
	DialogTitle         lipgloss.Style
	DialogTitleHelp     lipgloss.Style
	SubmenuTitle        lipgloss.Style
	SubmenuTitleFocused lipgloss.Style

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
	StatusBarBorder    lipgloss.Style
	StatusBarSeparator lipgloss.Style
	StatusBarSelected  lipgloss.Style

	// Help line
	HelpLine lipgloss.Style

	// Separator
	SepChar string

	// Settings
	LineCharacters    bool
	DrawBorders       bool
	DialogTitleAlign  string
	SubmenuTitleAlign string
	LogTitleAlign     string

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
	LineCharacters      bool
	DrawBorders         bool
	Type                DialogType
	Screen              lipgloss.Style
	Dialog              lipgloss.Style
	DialogTitle         lipgloss.Style
	DialogTitleHelp     lipgloss.Style
	SubmenuTitle        lipgloss.Style
	SubmenuTitleFocused lipgloss.Style
	Border              lipgloss.Border
	BorderColor         color.Color
	Border2Color        color.Color
	ButtonActive        lipgloss.Style
	ButtonInactive      lipgloss.Style
	ItemNormal          lipgloss.Style
	ItemSelected        lipgloss.Style
	TagNormal           lipgloss.Style
	TagSelected         lipgloss.Style
	TagKey              lipgloss.Style
	TagKeySelected      lipgloss.Style
	Shadow              lipgloss.Style
	ShadowColor         color.Color
	ShadowLevel         int
	HelpLine            lipgloss.Style
	StatusSuccess       lipgloss.Style
	StatusWarn          lipgloss.Style
	StatusError         lipgloss.Style
	Console             lipgloss.Style
	StatusBarSelected   lipgloss.Style
	LogPanelColor       color.Color
	DialogTitleAlign    string
	SubmenuTitleAlign   string
	LogTitleAlign       string
	Prefix              string // Prefix for semantic tag remapping (e.g. "Preview_")
	DrawShadow          bool   // Whether to draw shadows for this context
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
		LineCharacters:      currentStyles.LineCharacters,
		DrawBorders:         currentStyles.DrawBorders,
		Type:                DialogTypeInfo, // Default to info
		Screen:              currentStyles.Screen,
		Dialog:              currentStyles.Dialog,
		DialogTitle:         currentStyles.DialogTitle,
		DialogTitleHelp:     currentStyles.DialogTitleHelp,
		SubmenuTitle:        currentStyles.SubmenuTitle,
		SubmenuTitleFocused: currentStyles.SubmenuTitleFocused,
		Border:              currentStyles.Border,
		BorderColor:         currentStyles.BorderColor,
		Border2Color:        currentStyles.Border2Color,
		ButtonActive:        currentStyles.ButtonActive,
		ButtonInactive:      currentStyles.ButtonInactive,
		ItemNormal:          currentStyles.ItemNormal,
		ItemSelected:        currentStyles.ItemSelected,
		TagNormal:           currentStyles.TagNormal,
		TagSelected:         currentStyles.TagSelected,
		TagKey:              currentStyles.TagKey,
		TagKeySelected:      currentStyles.TagKeySelected,
		Shadow:              currentStyles.Shadow,
		ShadowColor:         currentStyles.ShadowColor,
		ShadowLevel:         currentConfig.UI.ShadowLevel,
		HelpLine:            currentStyles.HelpLine,
		StatusSuccess:       currentStyles.StatusSuccess,
		StatusWarn:          currentStyles.StatusWarn,
		StatusError:         currentStyles.StatusError,
		Console:             currentStyles.Console,
		StatusBarSelected:   currentStyles.StatusBarSelected,
		LogPanelColor:       currentStyles.LogPanelColor,
		DialogTitleAlign:    currentStyles.DialogTitleAlign,
		SubmenuTitleAlign:   currentStyles.SubmenuTitleAlign,
		LogTitleAlign:       currentStyles.LogTitleAlign,
		Prefix:              "", // Global context has no prefix
		DrawShadow:          currentConfig.UI.Shadow,
	}
}

// AsciiBorder defines a simple ASCII-only border for terminals without Unicode support
var AsciiBorder = lipgloss.Border{
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
	// We no longer apply theme defaults here to respect user overrides in the config file.
	// Theme defaults are only applied when selecting a theme in the DisplayOptionsScreen.
	_ = defaults

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
		currentStyles.Border = AsciiBorder
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
	// Explicitly unset background to ensure it is clear/transparent.
	currentStyles.Shadow = lipgloss.NewStyle().Foreground(currentStyles.ShadowColor).UnsetBackground()

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
	currentStyles.StatusBarSelected = SemanticRawStyle("Theme_StatusBarSelected")
	currentStyles.StatusBarBorder = SemanticRawStyle("Theme_StatusBarBorder")
	if currentStyles.StatusBarBorder.GetForeground() == nil && currentStyles.StatusBarBorder.GetBackground() == nil {
		// Fallback for themes that don't define StatusBarBorder
		currentStyles.StatusBarBorder = currentStyles.StatusBar
	}
	currentStyles.HeaderBG = currentStyles.StatusBar // Backwards compatibility

	// Help line
	currentStyles.HelpLine = SemanticRawStyle("Theme_Helpline")

	// Submenu Title
	currentStyles.SubmenuTitle = SemanticRawStyle("Theme_TitleSubMenu")
	currentStyles.SubmenuTitleFocused = SemanticRawStyle("Theme_TitleSubMenuFocused")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	currentStyles.StatusSuccess = SemanticRawStyle("Theme_TitleNotice")
	currentStyles.StatusWarn = SemanticRawStyle("Theme_TitleWarn")
	currentStyles.StatusError = SemanticRawStyle("Theme_TitleError")
	currentStyles.Console = SemanticRawStyle("Theme_ProgramBox")

	currentStyles.LogPanelColor = SemanticRawStyle("Theme_LogPanel").GetForeground()

	currentStyles.DialogTitleAlign = cfg.UI.DialogTitleAlign
	currentStyles.SubmenuTitleAlign = cfg.UI.SubmenuTitleAlign
	currentStyles.LogTitleAlign = cfg.UI.LogTitleAlign
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

// CenterText centers text within a given width.
// It is ANSI-aware: it strips escape codes before measuring to correctly
// handle pre-styled strings.
func CenterText(s string, width int) string {
	textWidth := lipgloss.Width(GetPlainText(s))
	if textWidth >= width {
		return s
	}
	leftPad := (width - textWidth) / 2
	rightPad := width - textWidth - leftPad
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

// PadRight pads text to fill width
func PadRight(s string, width int) string {
	textWidth := lipgloss.Width(s)
	if textWidth >= width {
		return s
	}
	return s + lipgloss.NewStyle().Width(width-textWidth).Render("")
}
