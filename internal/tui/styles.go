package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
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
	ZPanel    = 20
	ZDialog   = 30
	ZHalo     = 35
	ZOverlay  = 40

	ZModalBaseOffset = 100 // Z gap above the highest screen layer for the first modal
	ZModalStackStep  = 100 // Additional Z gap for each subsequent stacked modal
)

// Global Layer IDs for hit testing
const (
	IDPanel         = "panel"
	IDPanelToggle   = "panel_toggle"
	IDPanelResize   = "panel_resize"
	IDPanelViewport = "panel_viewport"
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

	// INS/OVR mode toggle hit region ID
	IDInsOvr = "ins_ovr"

	// Title bar widget IDs (suffix appended to menu ID)
	IDTitleWidgetRefresh = "title_widget_refresh"
	IDTitleWidgetHelp    = "title_widget_help"
	IDTitleWidgetClose   = "title_widget_close"

	// Panel resize widget IDs
	IDPanelResizeUp = "panel_resize_up"
	IDPanelResizeDn = "panel_resize_dn"
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
	LargeTitleArea      lipgloss.Style

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
	ButtonSpinner      lipgloss.Style // Spinner flanking flat button label when processing
	ButtonSpinnerLarge lipgloss.Style // Spinner at edges of bordered button when processing

	// Title bar icon widgets
	IconFocused          lipgloss.Style
	IconPressed          lipgloss.Style
	IconInactive         lipgloss.Style
	HelpIconInactive     lipgloss.Style
	RefreshIconInactive  lipgloss.Style
	ExitIconInactive     lipgloss.Style
	ResizeUpIconInactive lipgloss.Style
	ResizeDnIconInactive lipgloss.Style
	ButtonKeyActive      lipgloss.Style
	ButtonKeyInactive    lipgloss.Style

	// List items
	ItemNormal   lipgloss.Style
	ItemFocused lipgloss.Style

	// Tags (menu item labels)
	TagNormal      lipgloss.Style
	TagFocused    lipgloss.Style
	TagKey         lipgloss.Style // First letter highlight
	TagKeyFocused lipgloss.Style
	TagSpinner    lipgloss.Style // Spinner flanking the tag when processing

	// Header
	HeaderBG           lipgloss.Style
	StatusBar          lipgloss.Style
	StatusBarBorder    lipgloss.Style
	StatusBarFocused  lipgloss.Style

	// Help line
	HelpLine lipgloss.Style

	// Separator
	SepChar string

	// Settings
	LineCharacters    bool
	DrawBorders       bool
	LargeButtons     bool
	LargeTitleBars    bool
	DialogTitleAlign  string
	SubmenuTitleAlign string
	PanelTitleAlign   string

	// Option value (dropdown/inline value in flow menus)
	OptionValueFocused lipgloss.Style

	// Semantic styles derived from theme tags
	StatusSuccess lipgloss.Style
	StatusWarn    lipgloss.Style
	Console       lipgloss.Style

	// Console panel title color
	ConsoleTitleColor color.Color
}

// StyleContext holds a subset of Styles for decoupled rendering
type StyleContext struct {
	LineCharacters      bool
	DrawBorders         bool
	LargeButtons       bool
	LargeTitleBars      bool
	Type                DialogType
	Screen              lipgloss.Style
	Dialog              lipgloss.Style
	ContentBackground   lipgloss.Style
	DialogTitle         lipgloss.Style
	DialogTitleHelp     lipgloss.Style
	SubmenuTitle        lipgloss.Style
	SubmenuTitleFocused lipgloss.Style
	LargeTitleArea      lipgloss.Style
	Border              lipgloss.Border
	BorderColor         color.Color
	Border2Color        color.Color
	BorderFlags         theme.StyleFlags
	Border2Flags        theme.StyleFlags
	ButtonActive        lipgloss.Style
	ButtonInactive      lipgloss.Style
	IconFocused            lipgloss.Style
	IconPressed            lipgloss.Style
	IconInactive           lipgloss.Style
	HelpIconInactive       lipgloss.Style
	RefreshIconInactive    lipgloss.Style
	ExitIconInactive       lipgloss.Style
	ResizeUpIconInactive   lipgloss.Style
	ResizeDnIconInactive   lipgloss.Style
	ButtonKeyActive        lipgloss.Style
	ButtonKeyInactive      lipgloss.Style
	ItemNormal             lipgloss.Style
	ItemFocused        lipgloss.Style
	TagNormal           lipgloss.Style
	TagFocused         lipgloss.Style
	TagKey              lipgloss.Style
	TagKeyFocused      lipgloss.Style
	TagSpinner         lipgloss.Style
	ButtonSpinner      lipgloss.Style
	ButtonSpinnerLarge lipgloss.Style
	Shadow              lipgloss.Style
	ShadowColor         color.Color
	ShadowLevel         int
	HelpLine            lipgloss.Style
	StatusSuccess       lipgloss.Style
	StatusWarn          lipgloss.Style
	Console             lipgloss.Style
	OptionValueFocused lipgloss.Style
	StatusBarFocused   lipgloss.Style
	ConsoleTitleColor   color.Color
	DialogTitleAlign    string
	SubmenuTitleAlign   string
	PanelTitleAlign     string
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
		LargeButtons:       currentStyles.LargeButtons,
		LargeTitleBars:      currentStyles.LargeTitleBars,
		Type:                DialogTypeInfo, // Default to info
		Screen:              currentStyles.Screen,
		Dialog:              currentStyles.Dialog,
		ContentBackground:   currentStyles.ContentBackground,
		DialogTitle:         currentStyles.DialogTitle,
		DialogTitleHelp:     currentStyles.DialogTitleHelp,
		SubmenuTitle:        currentStyles.SubmenuTitle,
		SubmenuTitleFocused: currentStyles.SubmenuTitleFocused,
		LargeTitleArea:      currentStyles.LargeTitleArea,
		Border:              currentStyles.Border,
		BorderColor:         currentStyles.BorderColor,
		Border2Color:        currentStyles.Border2Color,
		BorderFlags:         currentStyles.BorderFlags,
		Border2Flags:        currentStyles.Border2Flags,
		ButtonActive:        currentStyles.ButtonActive,
		ButtonInactive:      currentStyles.ButtonInactive,
		IconFocused:            currentStyles.IconFocused,
		IconPressed:            currentStyles.IconPressed,
		IconInactive:           currentStyles.IconInactive,
		HelpIconInactive:       currentStyles.HelpIconInactive,
		RefreshIconInactive:    currentStyles.RefreshIconInactive,
		ExitIconInactive:       currentStyles.ExitIconInactive,
		ResizeUpIconInactive:   currentStyles.ResizeUpIconInactive,
		ResizeDnIconInactive:   currentStyles.ResizeDnIconInactive,
		ButtonKeyActive:        currentStyles.ButtonKeyActive,
		ButtonKeyInactive:      currentStyles.ButtonKeyInactive,
		ItemNormal:             currentStyles.ItemNormal,
		ItemFocused:        currentStyles.ItemFocused,
		TagNormal:           currentStyles.TagNormal,
		TagFocused:         currentStyles.TagFocused,
		TagKey:              currentStyles.TagKey,
		TagKeyFocused:      currentStyles.TagKeyFocused,
		TagSpinner:         currentStyles.TagSpinner,
		ButtonSpinner:      currentStyles.ButtonSpinner,
		ButtonSpinnerLarge: currentStyles.ButtonSpinnerLarge,
		Shadow:              currentStyles.Shadow,
		ShadowColor:         currentStyles.ShadowColor,
		ShadowLevel:         currentConfig.UI.ShadowLevel,
		HelpLine:            currentStyles.HelpLine,
		StatusSuccess:       currentStyles.StatusSuccess,
		StatusWarn:          currentStyles.StatusWarn,
		Console:             currentStyles.Console,
		OptionValueFocused: currentStyles.OptionValueFocused,
		StatusBarFocused:   currentStyles.StatusBarFocused,
		ConsoleTitleColor:   currentStyles.ConsoleTitleColor,
		DialogTitleAlign:    currentStyles.DialogTitleAlign,
		SubmenuTitleAlign:   currentStyles.SubmenuTitleAlign,
		PanelTitleAlign:     currentStyles.PanelTitleAlign,
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
	currentStyles.LargeButtons = cfg.UI.LargeButtons
	currentStyles.LargeTitleBars = cfg.UI.LargeTitleBars

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

	// Shadow defines the shadow color and any attributes (e.g. dim, bold) for shade characters.
	shadowDef := SemanticRawStyle("Shadow")
	currentStyles.ShadowColor = shadowDef.GetForeground()
	if currentStyles.ShadowColor == nil {
		currentStyles.ShadowColor = shadowDef.GetBackground()
	}
	currentStyles.Shadow = shadowDef.UnsetBackground()

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

	// Title bar icon widgets
	currentStyles.IconFocused = SemanticRawStyle("IconFocused")
	if _, noBG := currentStyles.IconFocused.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.IconFocused = currentStyles.IconFocused.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.IconPressed = SemanticRawStyle("IconPressed")
	if _, noBG := currentStyles.IconPressed.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.IconPressed = currentStyles.IconPressed.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.IconInactive = SemanticRawStyle("IconInactive")
	if _, noBG := currentStyles.IconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.IconInactive = currentStyles.IconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.HelpIconInactive = SemanticRawStyle("HelpIconInactive")
	if _, noBG := currentStyles.HelpIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.HelpIconInactive = currentStyles.HelpIconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.RefreshIconInactive = SemanticRawStyle("RefreshIconInactive")
	if _, noBG := currentStyles.RefreshIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.RefreshIconInactive = currentStyles.RefreshIconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.ExitIconInactive = SemanticRawStyle("ExitIconInactive")
	if _, noBG := currentStyles.ExitIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ExitIconInactive = currentStyles.ExitIconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.ResizeUpIconInactive = SemanticRawStyle("ResizeUpIconInactive")
	if _, noBG := currentStyles.ResizeUpIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ResizeUpIconInactive = currentStyles.ResizeUpIconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.ResizeDnIconInactive = SemanticRawStyle("ResizeDnIconInactive")
	if _, noBG := currentStyles.ResizeDnIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ResizeDnIconInactive = currentStyles.ResizeDnIconInactive.Background(currentStyles.Dialog.GetBackground())
	}
	currentStyles.ButtonKeyActive = SemanticRawStyle("ButtonKeyActive")
	if _, noBG := currentStyles.ButtonKeyActive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonKeyActive = currentStyles.ButtonKeyActive.Background(currentStyles.ButtonActive.GetBackground())
	}
	currentStyles.ButtonKeyInactive = SemanticRawStyle("ButtonKeyInactive")
	if _, noBG := currentStyles.ButtonKeyInactive.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonKeyInactive = currentStyles.ButtonKeyInactive.Background(currentStyles.ButtonInactive.GetBackground())
	}

	// List items
	currentStyles.ItemNormal = SemanticRawStyle("Item")
	if _, noBG := currentStyles.ItemNormal.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ItemNormal = currentStyles.ItemNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ItemFocused = SemanticRawStyle("ItemFocused")
	if _, noBG := currentStyles.ItemFocused.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ItemFocused = currentStyles.ItemFocused.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.OptionValueFocused = SemanticRawStyle("OptionValueFocused")
	if _, noBG := currentStyles.OptionValueFocused.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.OptionValueFocused = currentStyles.OptionValueFocused.Background(currentStyles.Dialog.GetBackground())
	}

	// Tags
	currentStyles.TagNormal = SemanticRawStyle("Tag")
	if _, noBG := currentStyles.TagNormal.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagNormal = currentStyles.TagNormal.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagFocused = SemanticRawStyle("TagFocused")
	if _, noBG := currentStyles.TagFocused.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagFocused = currentStyles.TagFocused.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKey = SemanticRawStyle("TagKey")
	if _, noBG := currentStyles.TagKey.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagKey = currentStyles.TagKey.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagKeyFocused = SemanticRawStyle("TagKeyFocused")
	if _, noBG := currentStyles.TagKeyFocused.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagKeyFocused = currentStyles.TagKeyFocused.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.TagSpinner = SemanticRawStyle("TagSpinner")
	if _, noBG := currentStyles.TagSpinner.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.TagSpinner = currentStyles.TagSpinner.Background(currentStyles.Dialog.GetBackground())
	}

	currentStyles.ButtonSpinner = SemanticRawStyle("ButtonSpinner")
	if _, noBG := currentStyles.ButtonSpinner.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonSpinner = currentStyles.ButtonSpinner.Background(currentStyles.ButtonActive.GetBackground())
	}

	currentStyles.ButtonSpinnerLarge = SemanticRawStyle("ButtonSpinnerLarge")
	if _, noBG := currentStyles.ButtonSpinnerLarge.GetBackground().(lipgloss.NoColor); noBG {
		currentStyles.ButtonSpinnerLarge = currentStyles.ButtonSpinnerLarge.Background(currentStyles.ButtonActive.GetBackground())
	}

	// Header / Status Bar
	currentStyles.StatusBar = SemanticRawStyle("StatusBar")
	currentStyles.StatusBarFocused = SemanticRawStyle("StatusBarFocused")
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

	// Large Title Bar
	currentStyles.LargeTitleArea = SemanticRawStyle("LargeTitleArea")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	currentStyles.StatusSuccess = SemanticRawStyle("TitleNotice")
	currentStyles.StatusWarn = SemanticRawStyle("TitleWarn")
	currentStyles.Console = theme.ConsoleSemanticRawStyle("ProgramBox")

	currentStyles.ConsoleTitleColor = SemanticRawStyle("ConsoleTitle").GetForeground()

	currentStyles.DialogTitleAlign = cfg.UI.DialogTitleAlign
	currentStyles.SubmenuTitleAlign = cfg.UI.SubmenuTitleAlign
	currentStyles.PanelTitleAlign = cfg.UI.PanelTitleAlign
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
	return strutil.Repeat(" ", leftPad) + s + strutil.Repeat(" ", rightPad)
}

// PadRight pads text to fill width
func PadRight(s string, width int) string {
	textWidth := lipgloss.Width(s)
	if textWidth >= width {
		return s
	}
	return s + lipgloss.NewStyle().Width(width-textWidth).Render("")
}
