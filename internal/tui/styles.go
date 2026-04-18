package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/theme"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// GetMenuItemID returns a unique ID for a menu item
func GetMenuItemID(menuID string, index int) string {
	return "item-" + menuID + "-" + strconv.Itoa(index)
}

// ParseMenuItemIndex parses a menu item ID of the form "item-<menuID>-<index>"
// and returns the index. Returns (0, false) if the id does not match.
func ParseMenuItemIndex(id, menuID string) (int, bool) {
	prefix := "item-" + menuID + "-"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	idx, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil {
		return 0, false
	}
	return idx, true
}

// HitRegion represents a clickable area for mouse hit testing
type HitRegion struct {
	ID     string
	X, Y   int
	Width  int
	Height int
	ZOrder int // Higher values are checked first (on top)
	Label  string
	Help   *HelpContext
}

// HitRegionProvider is implemented by components that have clickable areas
type HitRegionProvider interface {
	GetHitRegions(offsetX, offsetY int) []HitRegion
}

// HitRegions is a slice of HitRegion that can be sorted by ZOrder
type HitRegions []HitRegion

// FindHit returns the topmost region containing the point, or nil
func (regions HitRegions) FindHit(x, y int) *HitRegion {
	// Check in reverse order (higher ZOrder regions are at the end after sorting)
	for i := len(regions) - 1; i >= 0; i-- {
		r := &regions[i]
		if x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height {
			return r
		}
	}
	return nil
}

// ScanForHyperlinks scans a rendered string for OSC 8 hyperlinks and returns hit regions for them.
// offsetX and offsetY are the absolute screen coordinates of the top-left of the rendered block.
// Z is the base ZOrder for the new regions.
func ScanForHyperlinks(rendered string, offsetX, offsetY, baseZ int) []HitRegion {
	var regions []HitRegion
	lines := strings.Split(rendered, "\n")

	// OSC 8 Regex: \x1b]8;[params];[url]\a[content]\x1b]8;;\a
	// We support both \x07 (BEL) and \x1b\\ (ST) as terminators.
	re := regexp.MustCompile(`\x1b\]8;.*?;(.*?)(?:\x07|\x1b\\)(.*?)\x1b\]8;;(?:\x07|\x1b\\)`)

	for y, line := range lines {
		matches := re.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) < 6 {
				continue
			}
			url := line[match[2]:match[3]]
			content := line[match[4]:match[5]]

			prefix := line[:match[0]]
			// Use lipgloss.Width on prefix to get the visual X offset.
			// Lipgloss internally handles stripping ANSI for width calculation.
			visualX := lipgloss.Width(prefix)
			visualW := lipgloss.Width(content)

			regions = append(regions, HitRegion{
				ID:     "link:" + url,
				X:      offsetX + visualX,
				Y:      offsetY + y,
				Width:  visualW,
				Height: 1,
				ZOrder: baseZ + 50, // Top priority
				Label:  "Link: " + url,
			})
		}
	}
	return regions
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
	IDLogPanel      = "log_panel"
	IDLogToggle     = "log_toggle"
	IDLogResize     = "log_resize"
	IDLogViewport   = "log_viewport"
	IDConsoleInput  = "console_input"
	IDAppVersion  = "app_version"
	IDTmplVersion = "tmpl_version"

	// Status bar
	IDStatusBar = "status_bar"

	// Display Options IDs
	IDThemePanel   = "theme_panel"
	IDOptionsPanel = "options_panel"
	IDButtonPanel  = "button_panel"
	IDListPanel    = "list_panel"
	IDSaveButton   = "save_button"
	IDApplyButton  = "apply_button"
	IDBackButton   = "back_button"
	IDExitButton   = "exit_button"
	IDHeaderFlags  = "header_flags"
	IDHelpline     = "helpline"
)

// Styles holds all lipgloss styles derived from the theme
type Styles struct {
	// Screen
	Screen lipgloss.Style

	// Dialog
	Dialog              lipgloss.Style
	ContentBackground   lipgloss.Style
	DialogTitle         lipgloss.Style
	DialogTitleHelp     lipgloss.Style
	SubmenuTitle        lipgloss.Style
	SubmenuTitleFocused lipgloss.Style

	// Borders
	Border       lipgloss.Border
	BorderColor  color.Color
	Border2Color color.Color
	BorderFlags  theme.StyleFlags
	Border2Flags theme.StyleFlags

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
	ButtonBorders     bool
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
	ButtonBorders       bool
	Type                DialogType
	Screen              lipgloss.Style
	Dialog              lipgloss.Style
	ContentBackground   lipgloss.Style
	DialogTitle         lipgloss.Style
	DialogTitleHelp     lipgloss.Style
	SubmenuTitle        lipgloss.Style
	SubmenuTitleFocused lipgloss.Style
	Border              lipgloss.Border
	BorderColor         color.Color
	Border2Color        color.Color
	BorderFlags         theme.StyleFlags
	Border2Flags        theme.StyleFlags
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
		ButtonBorders:       currentStyles.ButtonBorders,
		Type:                DialogTypeInfo, // Default to info
		Screen:              currentStyles.Screen,
		Dialog:              currentStyles.Dialog,
		ContentBackground:   currentStyles.ContentBackground,
		DialogTitle:         currentStyles.DialogTitle,
		DialogTitleHelp:     currentStyles.DialogTitleHelp,
		SubmenuTitle:        currentStyles.SubmenuTitle,
		SubmenuTitleFocused: currentStyles.SubmenuTitleFocused,
		Border:              currentStyles.Border,
		BorderColor:         currentStyles.BorderColor,
		Border2Color:        currentStyles.Border2Color,
		BorderFlags:         currentStyles.BorderFlags,
		Border2Flags:        currentStyles.Border2Flags,
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

	// Update the global config so IsShadowEnabled(), GetActiveContext().ShadowLevel,
	// and any other currentConfig readers see the new values immediately.
	currentConfig = cfg

	// Store LineCharacters setting for later use
	currentStyles.LineCharacters = cfg.UI.LineCharacters
	currentStyles.DrawBorders = cfg.UI.Borders
	currentStyles.ButtonBorders = cfg.UI.ButtonBorders

	// Border style based on LineCharacters setting
	if cfg.UI.LineCharacters { // Updated: Use cfg.UI.LineCharacters
		currentStyles.Border = lipgloss.RoundedBorder()
		currentStyles.SepChar = "─"
	} else {
		currentStyles.Border = AsciiBorder
		currentStyles.SepChar = "-"
	}

	// Screen background
	currentStyles.Screen = SemanticRawStyle("Screen")

	// Dialog
	currentStyles.Dialog = SemanticRawStyle("Dialog")
	currentStyles.ContentBackground = currentStyles.Dialog

	currentStyles.DialogTitle = SemanticRawStyle("Title")

	currentStyles.DialogTitleHelp = SemanticRawStyle("TitleHelp")

	// Border colors and flags
	switch cfg.UI.BorderColor {
	case 1:
		currentStyles.BorderColor = SemanticRawStyle("Border").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Border").GetForeground()
		currentStyles.BorderFlags = theme.StyleFlagsFromCode(console.GetRawTagCode("border"))
		currentStyles.Border2Flags = currentStyles.BorderFlags
	case 2:
		currentStyles.BorderColor = SemanticRawStyle("Border2").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Border2").GetForeground()
		currentStyles.BorderFlags = theme.StyleFlagsFromCode(console.GetRawTagCode("border2"))
		currentStyles.Border2Flags = currentStyles.BorderFlags
	case 3:
		fallthrough
	default:
		currentStyles.BorderColor = SemanticRawStyle("Border").GetForeground()
		currentStyles.Border2Color = SemanticRawStyle("Border2").GetForeground()
		currentStyles.BorderFlags = theme.StyleFlagsFromCode(console.GetRawTagCode("border"))
		currentStyles.Border2Flags = theme.StyleFlagsFromCode(console.GetRawTagCode("border2"))
	}

	// Shadow
	// Shadow defines the shadow color (foreground is used for shade characters like ░▒▓)
	shadowDef := SemanticRawStyle("Shadow")
	currentStyles.ShadowColor = shadowDef.GetForeground()
	if currentStyles.ShadowColor == nil {
		currentStyles.ShadowColor = shadowDef.GetBackground()
	}
	// Create shadow style with just the foreground color for shade chars
	// Explicitly unset background to ensure it is clear/transparent.
	currentStyles.Shadow = lipgloss.NewStyle().Foreground(currentStyles.ShadowColor).UnsetBackground()

	// Buttons (spacing handled at layout level)
	// lipgloss v2 GetBackground() returns NoColor{} (never nil) for unset colors.
	// Use type assertion to detect truly unset backgrounds and fall back to DialogBG.
	currentStyles.ButtonActive = SemanticRawStyle("ButtonActive")
	if _, noBG := currentStyles.ButtonActive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonActive = currentStyles.ButtonActive.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ButtonInactive = SemanticRawStyle("ButtonInactive")
	if _, noBG := currentStyles.ButtonInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonInactive = currentStyles.ButtonInactive.Background(currentStyles.Dialog.GetBackground())
	}

	// List items
	currentStyles.ItemNormal = SemanticRawStyle("Item")
	if _, noBG := currentStyles.ItemNormal.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ItemNormal = currentStyles.ItemNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ItemSelected = SemanticRawStyle("ItemSelected")
	if _, noBG := currentStyles.ItemSelected.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ItemSelected = currentStyles.ItemSelected.Background(currentStyles.Dialog.GetBackground())
	}

	// Tags
	currentStyles.TagNormal = SemanticRawStyle("Tag")
	if _, noBG := currentStyles.TagNormal.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagNormal = currentStyles.TagNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagSelected = SemanticRawStyle("TagSelected")
	if _, noBG := currentStyles.TagSelected.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagSelected = currentStyles.TagSelected.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKey = SemanticRawStyle("TagKey")
	if _, noBG := currentStyles.TagKey.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagKey = currentStyles.TagKey.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKeySelected = SemanticRawStyle("TagKeySelected")
	if _, noBG := currentStyles.TagKeySelected.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagKeySelected = currentStyles.TagKeySelected.Background(currentStyles.Dialog.GetBackground())
	}

	// Header / Status Bar
	currentStyles.StatusBar = SemanticRawStyle("StatusBar")
	currentStyles.StatusBarSeparator = SemanticRawStyle("StatusBarSeparator")
	currentStyles.StatusBarSelected = SemanticRawStyle("StatusBarSelected")
	currentStyles.StatusBarBorder = SemanticRawStyle("StatusBarBorder")
	{
		// Fallback for themes that don't define StatusBarBorder: use full StatusBar style.
		_, noFG := currentStyles.StatusBarBorder.GetForeground().(lipgloss.NoColor)
		_, noBG := currentStyles.StatusBarBorder.GetBackground().(lipgloss.NoColor)
		if noFG && noBG {
			currentStyles.StatusBarBorder = currentStyles.StatusBar
		}
	}
	currentStyles.HeaderBG = currentStyles.StatusBar // Backwards compatibility

	// Help line
	currentStyles.HelpLine = SemanticRawStyle("Helpline")

	// Submenu Title
	currentStyles.SubmenuTitle = SemanticRawStyle("TitleSubMenu")
	currentStyles.SubmenuTitleFocused = SemanticRawStyle("TitleSubMenuFocused")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	currentStyles.StatusSuccess = SemanticRawStyle("TitleNotice")
	currentStyles.StatusWarn = SemanticRawStyle("TitleWarn")
	currentStyles.StatusError = SemanticRawStyle("TitleError")
	currentStyles.Console = theme.ConsoleSemanticRawStyle("ProgramBox")

	currentStyles.LogPanelColor = SemanticRawStyle("LogPanel").GetForeground()

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
