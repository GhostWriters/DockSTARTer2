package classic

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	semstyle "github.com/GhostWriters/semstyle/lg"
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

// hyperlinkRegex matches an OSC 8 hyperlink span: \x1b]8;[params];[url]\a[content]\x1b]8;;\a
// Both \x07 (BEL) and \x1b\\ (ST) terminators are supported. Shared by
// ScanForHyperlinks (hit-region detection) and StripHyperlinks (removing the
// link wrapper for sessions where it can't resolve to anything useful).
var hyperlinkRegex = regexp.MustCompile(`\x1b\]8;.*?;(.*?)(?:\x07|\x1b\\)(.*?)\x1b\]8;;(?:\x07|\x1b\\)`)

// StripHyperlinks removes OSC 8 hyperlink wrappers from rendered text while
// keeping the label's own styling (e.g. SGR color codes) intact -- for
// notices referencing a local file path, a file:// link only ever resolves
// on the machine the DS2 process itself is running on, so it's meaningless
// (not just suboptimal) for SSH and web sessions: unlike an https:// docs
// link, there's no clipboard-copy fallback that helps either, since the
// remote user's machine doesn't have that file at all. Callers should call
// this for any non-local session before displaying such text.
func StripHyperlinks(rendered string) string {
	return hyperlinkRegex.ReplaceAllString(rendered, "$2")
}

// HyperlinkPath renders path as a clickable OSC 8 hyperlink to itself
// (file://path). styleTag is a semantic tag ("{{|Tag|}}") or a direct style
// code ("{{[fg:bg:attrs]}}"), resolved the same way as any other themed
// text (see theme.ThemeSemanticStyle). Intended for building a path out of
// several independently-clickable segments (e.g. each directory component
// plus the filename, each opening that specific file or folder) -- call this
// once per segment rather than trying to express multiple destinations in a
// single call. See StripHyperlinks for why this is local-session-only.
//
// This renders immediately (like calling style.Render() directly) -- it is
// NOT deferred semstyle tag markup, so it's meant for callers that already
// have a resolved rendering context (e.g. building a viewport string
// directly), not for building a logger.Notice/Info message string that gets
// resolved later per-destination (console/file/TUI). For that case, build
// the raw tag with strutil.FileURL yourself instead (see console.FormatFilePath).
func HyperlinkPath(styleTag, path string) string {
	return theme.ThemeSemanticStyle(styleTag).Hyperlink(strutil.FileURL(path)).Render(path)
}

// HyperlinkText renders text as a clickable OSC 8 hyperlink to path
// (file://path). styleTag is a semantic tag or direct style code, same as
// HyperlinkPath. Use this when the visible label should differ from the path
// itself (e.g. a friendly name); use HyperlinkPath when the path is its own
// label. See HyperlinkPath's doc comment for the immediate-vs-deferred
// rendering distinction, and StripHyperlinks for why this is
// local-session-only.
func HyperlinkText(styleTag, path, text string) string {
	return theme.ThemeSemanticStyle(styleTag).Hyperlink(strutil.FileURL(path)).Render(text)
}

// ScanForHyperlinks scans a rendered string for OSC 8 hyperlinks and returns hit regions for them.
// offsetX and offsetY are the absolute screen coordinates of the top-left of the rendered block.
// Z is the base ZOrder for the new regions.
func ScanForHyperlinks(rendered string, offsetX, offsetY, baseZ int) []HitRegion {
	var regions []HitRegion
	lines := strings.Split(rendered, "\n")

	for y, line := range lines {
		matches := hyperlinkRegex.FindAllStringSubmatchIndex(line, -1)
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
	IDAppVersion    = "app_version"
	IDTmplVersion   = "tmpl_version"

	// Status bar
	IDStatusBar = "status_bar"

	// Display Options IDs
	IDThemePanel       = "theme_panel"
	IDOptionsPanel     = "options_panel"
	IDButtonPanel      = "button_panel"
	IDListPanel        = "list_panel"
	IDSaveButton       = "save_button"
	IDApplyButton      = "apply_button"
	IDBackButton       = "back_button"
	IDExitButton       = "exit_button"
	IDHeaderFlags      = "header_flags"
	IDHeaderWebDisplay = "header_web_display"
	IDHelpline         = "helpline"

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
	Dialog               lipgloss.Style
	ContentBackground    lipgloss.Style
	DialogTitle          lipgloss.Style
	DialogTitleHelp      lipgloss.Style
	SubmenuTitle         lipgloss.Style
	SubmenuTitleFocused  lipgloss.Style
	SubmenuTitleDisabled lipgloss.Style
	LargeTitleArea       lipgloss.Style

	// Borders
	Border               lipgloss.Border
	BorderColor          color.Color
	Border2Color         color.Color
	BorderDisabledColor  color.Color
	Border2DisabledColor color.Color
	BorderFlags          semstyle.StyleFlags
	Border2Flags         semstyle.StyleFlags
	BorderDisabledFlags  semstyle.StyleFlags
	Border2DisabledFlags semstyle.StyleFlags

	// Shadow
	Shadow      lipgloss.Style
	ShadowColor color.Color

	// Buttons
	ButtonActive       lipgloss.Style
	ButtonInactive     lipgloss.Style
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
	ItemNormal  lipgloss.Style
	ItemFocused lipgloss.Style

	// Tags (menu item labels)
	TagNormal     lipgloss.Style
	TagFocused    lipgloss.Style
	TagKey        lipgloss.Style // First letter highlight
	TagKeyFocused lipgloss.Style
	TagSpinner    lipgloss.Style // Spinner flanking the tag when processing

	// Header
	HeaderBG         lipgloss.Style
	StatusBar        lipgloss.Style
	StatusBarBorder  lipgloss.Style
	StatusBarFocused lipgloss.Style

	// Help line
	HelpLine lipgloss.Style

	// Separator
	SepChar string

	// Settings
	LineCharacters    bool
	DrawBorders       bool
	LargeButtons      bool
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
	LineCharacters bool
	DrawBorders    bool
	// AngledBorder forces the slanted/beveled border style independent of
	// Type -- Type == DialogTypeConfirm already implies it by default (see
	// the ctx.Type == DialogTypeConfirm checks), but a dialog that wants
	// both the angled border AND a non-Confirm Type's title color (e.g. a
	// colored message dialog) sets this explicitly instead.
	AngledBorder bool
	// SquareBorder forces the square border style even when Type ==
	// DialogTypeConfirm would otherwise imply angled -- checked first so it
	// wins over both AngledBorder and the DialogTypeConfirm default.
	SquareBorder         bool
	LargeButtons         bool
	LargeTitleBars       bool
	Type                 DialogType
	Screen               lipgloss.Style
	Dialog               lipgloss.Style
	ContentBackground    lipgloss.Style
	DialogTitle          lipgloss.Style
	DialogTitleHelp      lipgloss.Style
	SubmenuTitle         lipgloss.Style
	SubmenuTitleFocused  lipgloss.Style
	SubmenuTitleDisabled lipgloss.Style
	LargeTitleArea       lipgloss.Style
	Border               lipgloss.Border
	BorderColor          color.Color
	Border2Color         color.Color
	BorderDisabledColor  color.Color
	Border2DisabledColor color.Color
	BorderFlags          semstyle.StyleFlags
	Border2Flags         semstyle.StyleFlags
	BorderDisabledFlags  semstyle.StyleFlags
	Border2DisabledFlags semstyle.StyleFlags
	ButtonActive         lipgloss.Style
	ButtonInactive       lipgloss.Style
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
	ItemNormal           lipgloss.Style
	ItemFocused          lipgloss.Style
	TagNormal            lipgloss.Style
	TagFocused           lipgloss.Style
	TagKey               lipgloss.Style
	TagKeyFocused        lipgloss.Style
	TagSpinner           lipgloss.Style
	ButtonSpinner        lipgloss.Style
	ButtonSpinnerLarge   lipgloss.Style
	Shadow               lipgloss.Style
	ShadowColor          color.Color
	ShadowLevel          int
	HelpLine             lipgloss.Style
	StatusSuccess        lipgloss.Style
	StatusWarn           lipgloss.Style
	Console              lipgloss.Style
	OptionValueFocused   lipgloss.Style
	StatusBarFocused     lipgloss.Style
	ConsoleTitleColor    color.Color
	DialogTitleAlign     string
	SubmenuTitleAlign    string
	PanelTitleAlign      string
	Prefix               string // Prefix for semantic tag remapping (e.g. "Preview_")
	DrawShadow           bool   // Whether to draw shadows for this context
}

// CurrentStyles holds the active styles
var CurrentStyles Styles

// GetStyles returns the current styles
func GetStyles() Styles {
	return CurrentStyles
}

// GetActiveContext returns the current global styles as a StyleContext
func GetActiveContext() StyleContext {
	return StyleContext{
		LineCharacters:       CurrentStyles.LineCharacters,
		DrawBorders:          CurrentStyles.DrawBorders,
		LargeButtons:         CurrentStyles.LargeButtons,
		LargeTitleBars:       CurrentStyles.LargeTitleBars,
		Type:                 DialogTypeInfo, // Default to info
		Screen:               CurrentStyles.Screen,
		Dialog:               CurrentStyles.Dialog,
		ContentBackground:    CurrentStyles.ContentBackground,
		DialogTitle:          CurrentStyles.DialogTitle,
		DialogTitleHelp:      CurrentStyles.DialogTitleHelp,
		SubmenuTitle:         CurrentStyles.SubmenuTitle,
		SubmenuTitleFocused:  CurrentStyles.SubmenuTitleFocused,
		SubmenuTitleDisabled: CurrentStyles.SubmenuTitleDisabled,
		LargeTitleArea:       CurrentStyles.LargeTitleArea,
		Border:               CurrentStyles.Border,
		BorderColor:          CurrentStyles.BorderColor,
		Border2Color:         CurrentStyles.Border2Color,
		BorderDisabledColor:  CurrentStyles.BorderDisabledColor,
		Border2DisabledColor: CurrentStyles.Border2DisabledColor,
		BorderFlags:          CurrentStyles.BorderFlags,
		Border2Flags:         CurrentStyles.Border2Flags,
		BorderDisabledFlags:  CurrentStyles.BorderDisabledFlags,
		Border2DisabledFlags: CurrentStyles.Border2DisabledFlags,
		ButtonActive:         CurrentStyles.ButtonActive,
		ButtonInactive:       CurrentStyles.ButtonInactive,
		IconFocused:          CurrentStyles.IconFocused,
		IconPressed:          CurrentStyles.IconPressed,
		IconInactive:         CurrentStyles.IconInactive,
		HelpIconInactive:     CurrentStyles.HelpIconInactive,
		RefreshIconInactive:  CurrentStyles.RefreshIconInactive,
		ExitIconInactive:     CurrentStyles.ExitIconInactive,
		ResizeUpIconInactive: CurrentStyles.ResizeUpIconInactive,
		ResizeDnIconInactive: CurrentStyles.ResizeDnIconInactive,
		ButtonKeyActive:      CurrentStyles.ButtonKeyActive,
		ButtonKeyInactive:    CurrentStyles.ButtonKeyInactive,
		ItemNormal:           CurrentStyles.ItemNormal,
		ItemFocused:          CurrentStyles.ItemFocused,
		TagNormal:            CurrentStyles.TagNormal,
		TagFocused:           CurrentStyles.TagFocused,
		TagKey:               CurrentStyles.TagKey,
		TagKeyFocused:        CurrentStyles.TagKeyFocused,
		TagSpinner:           CurrentStyles.TagSpinner,
		ButtonSpinner:        CurrentStyles.ButtonSpinner,
		ButtonSpinnerLarge:   CurrentStyles.ButtonSpinnerLarge,
		Shadow:               CurrentStyles.Shadow,
		ShadowColor:          CurrentStyles.ShadowColor,
		ShadowLevel:          currentConfig.UI.ShadowLevel,
		HelpLine:             CurrentStyles.HelpLine,
		StatusSuccess:        CurrentStyles.StatusSuccess,
		StatusWarn:           CurrentStyles.StatusWarn,
		Console:              CurrentStyles.Console,
		OptionValueFocused:   CurrentStyles.OptionValueFocused,
		StatusBarFocused:     CurrentStyles.StatusBarFocused,
		ConsoleTitleColor:    CurrentStyles.ConsoleTitleColor,
		DialogTitleAlign:     CurrentStyles.DialogTitleAlign,
		SubmenuTitleAlign:    CurrentStyles.SubmenuTitleAlign,
		PanelTitleAlign:      CurrentStyles.PanelTitleAlign,
		Prefix:               "", // Global context has no prefix
		DrawShadow:           currentConfig.UI.Shadow,
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

// currentConfig holds the loaded app configuration
var currentConfig config.AppConfig

// CurrentConfig returns the app configuration most recently passed to InitStyles.
// This is the read-only accessor for callers outside classic (e.g. internal/tui)
// that need the same live config classic's own rendering code reads.
func CurrentConfig() config.AppConfig {
	return currentConfig
}

// InitStyles initializes lipgloss styles from the current theme
func InitStyles(cfg config.AppConfig) {
	// Clear the semantic style cache to ensure real-time visual updates on theme swap
	ClearSemanticCache()

	// Update the global config so IsShadowEnabled(), GetActiveContext().ShadowLevel,
	// and any other currentConfig readers see the new values immediately.
	currentConfig = cfg

	// Store LineCharacters setting for later use
	CurrentStyles.LineCharacters = cfg.UI.LineCharacters
	CurrentStyles.DrawBorders = cfg.UI.Borders
	CurrentStyles.LargeButtons = cfg.UI.LargeButtons
	CurrentStyles.LargeTitleBars = cfg.UI.LargeTitleBars

	// Border style based on LineCharacters setting
	if cfg.UI.LineCharacters { // Updated: Use cfg.UI.LineCharacters
		CurrentStyles.Border = lipgloss.RoundedBorder()
		CurrentStyles.SepChar = "─"
	} else {
		CurrentStyles.Border = AsciiBorder
		CurrentStyles.SepChar = "-"
	}

	// Screen background
	CurrentStyles.Screen = SemanticRawStyle("Screen")

	// Dialog
	CurrentStyles.Dialog = SemanticRawStyle("Dialog")
	CurrentStyles.ContentBackground = CurrentStyles.Dialog

	CurrentStyles.DialogTitle = SemanticRawStyle("Title")

	CurrentStyles.DialogTitleHelp = SemanticRawStyle("TitleHelp")

	// Border colors and flags
	switch cfg.UI.BorderColor {
	case 1:
		CurrentStyles.BorderColor = SemanticRawStyle("Border").GetForeground()
		CurrentStyles.Border2Color = SemanticRawStyle("Border").GetForeground()
		CurrentStyles.BorderFlags = semstyle.CodeToFlags(semstyle.GetRawTagCode("border"))
		CurrentStyles.Border2Flags = CurrentStyles.BorderFlags
	case 2:
		CurrentStyles.BorderColor = SemanticRawStyle("Border2").GetForeground()
		CurrentStyles.Border2Color = SemanticRawStyle("Border2").GetForeground()
		CurrentStyles.BorderFlags = semstyle.CodeToFlags(semstyle.GetRawTagCode("border2"))
		CurrentStyles.Border2Flags = CurrentStyles.BorderFlags
	case 3:
		fallthrough
	default:
		CurrentStyles.BorderColor = SemanticRawStyle("Border").GetForeground()
		CurrentStyles.Border2Color = SemanticRawStyle("Border2").GetForeground()
		CurrentStyles.BorderFlags = semstyle.CodeToFlags(semstyle.GetRawTagCode("border"))
		CurrentStyles.Border2Flags = semstyle.CodeToFlags(semstyle.GetRawTagCode("border2"))
	}

	CurrentStyles.BorderDisabledColor = SemanticRawStyle("BorderDisabled").GetForeground()
	CurrentStyles.Border2DisabledColor = SemanticRawStyle("Border2Disabled").GetForeground()
	CurrentStyles.BorderDisabledFlags = semstyle.CodeToFlags(semstyle.GetRawTagCode("borderdisabled"))
	CurrentStyles.Border2DisabledFlags = semstyle.CodeToFlags(semstyle.GetRawTagCode("border2disabled"))

	// Shadow defines the shadow color and any attributes (e.g. dim, bold) for shade characters.
	shadowDef := SemanticRawStyle("Shadow")
	CurrentStyles.ShadowColor = shadowDef.GetForeground()
	if CurrentStyles.ShadowColor == nil {
		CurrentStyles.ShadowColor = shadowDef.GetBackground()
	}
	CurrentStyles.Shadow = shadowDef.UnsetBackground()

	// Buttons (spacing handled at layout level)
	// lipgloss v2 GetBackground() returns NoColor{} (never nil) for unset colors.
	// Use type assertion to detect truly unset backgrounds and fall back to DialogBG.
	CurrentStyles.ButtonActive = SemanticRawStyle("ButtonActive")
	if _, noBG := CurrentStyles.ButtonActive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonActive = CurrentStyles.ButtonActive.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.ButtonInactive = SemanticRawStyle("ButtonInactive")
	if _, noBG := CurrentStyles.ButtonInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonInactive = CurrentStyles.ButtonInactive.Background(CurrentStyles.Dialog.GetBackground())
	}

	// Title bar icon widgets
	CurrentStyles.IconFocused = SemanticRawStyle("IconFocused")
	if _, noBG := CurrentStyles.IconFocused.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.IconFocused = CurrentStyles.IconFocused.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.IconPressed = SemanticRawStyle("IconPressed")
	if _, noBG := CurrentStyles.IconPressed.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.IconPressed = CurrentStyles.IconPressed.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.IconInactive = SemanticRawStyle("IconInactive")
	if _, noBG := CurrentStyles.IconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.IconInactive = CurrentStyles.IconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.HelpIconInactive = SemanticRawStyle("HelpIconInactive")
	if _, noBG := CurrentStyles.HelpIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.HelpIconInactive = CurrentStyles.HelpIconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.RefreshIconInactive = SemanticRawStyle("RefreshIconInactive")
	if _, noBG := CurrentStyles.RefreshIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.RefreshIconInactive = CurrentStyles.RefreshIconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.ExitIconInactive = SemanticRawStyle("ExitIconInactive")
	if _, noBG := CurrentStyles.ExitIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ExitIconInactive = CurrentStyles.ExitIconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.ResizeUpIconInactive = SemanticRawStyle("ResizeUpIconInactive")
	if _, noBG := CurrentStyles.ResizeUpIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ResizeUpIconInactive = CurrentStyles.ResizeUpIconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.ResizeDnIconInactive = SemanticRawStyle("ResizeDnIconInactive")
	if _, noBG := CurrentStyles.ResizeDnIconInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ResizeDnIconInactive = CurrentStyles.ResizeDnIconInactive.Background(CurrentStyles.Dialog.GetBackground())
	}
	CurrentStyles.ButtonKeyActive = SemanticRawStyle("ButtonKeyActive")
	if _, noBG := CurrentStyles.ButtonKeyActive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonKeyActive = CurrentStyles.ButtonKeyActive.Background(CurrentStyles.ButtonActive.GetBackground())
	}
	CurrentStyles.ButtonKeyInactive = SemanticRawStyle("ButtonKeyInactive")
	if _, noBG := CurrentStyles.ButtonKeyInactive.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonKeyInactive = CurrentStyles.ButtonKeyInactive.Background(CurrentStyles.ButtonInactive.GetBackground())
	}

	// List items
	CurrentStyles.ItemNormal = SemanticRawStyle("Item")
	if _, noBG := CurrentStyles.ItemNormal.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ItemNormal = CurrentStyles.ItemNormal.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.ItemFocused = SemanticRawStyle("ItemFocused")
	if _, noBG := CurrentStyles.ItemFocused.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ItemFocused = CurrentStyles.ItemFocused.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.OptionValueFocused = SemanticRawStyle("OptionValueFocused")
	if _, noBG := CurrentStyles.OptionValueFocused.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.OptionValueFocused = CurrentStyles.OptionValueFocused.Background(CurrentStyles.Dialog.GetBackground())
	}

	// Tags
	CurrentStyles.TagNormal = SemanticRawStyle("Tag")
	if _, noBG := CurrentStyles.TagNormal.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.TagNormal = CurrentStyles.TagNormal.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.TagFocused = SemanticRawStyle("TagFocused")
	if _, noBG := CurrentStyles.TagFocused.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.TagFocused = CurrentStyles.TagFocused.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.TagKey = SemanticRawStyle("TagKey")
	if _, noBG := CurrentStyles.TagKey.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.TagKey = CurrentStyles.TagKey.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.TagKeyFocused = SemanticRawStyle("TagKeyFocused")
	if _, noBG := CurrentStyles.TagKeyFocused.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.TagKeyFocused = CurrentStyles.TagKeyFocused.Background(CurrentStyles.Dialog.GetBackground())
	}

	CurrentStyles.TagSpinner = SemanticRawStyle("TagSpinner")
	if _, noBG := CurrentStyles.TagSpinner.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.TagSpinner = CurrentStyles.TagSpinner.Background(CurrentStyles.Dialog.GetBackground())
	}
	if _, noFG := CurrentStyles.TagSpinner.GetForeground().(lipgloss.NoColor); noFG {
		CurrentStyles.TagSpinner = CurrentStyles.TagSpinner.Foreground(CurrentStyles.Dialog.GetForeground())
	}

	CurrentStyles.ButtonSpinner = SemanticRawStyle("ButtonSpinner")
	if _, noBG := CurrentStyles.ButtonSpinner.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonSpinner = CurrentStyles.ButtonSpinner.Background(CurrentStyles.ButtonActive.GetBackground())
	}
	if _, noFG := CurrentStyles.ButtonSpinner.GetForeground().(lipgloss.NoColor); noFG {
		CurrentStyles.ButtonSpinner = CurrentStyles.ButtonSpinner.Foreground(CurrentStyles.ButtonActive.GetForeground())
	}

	CurrentStyles.ButtonSpinnerLarge = SemanticRawStyle("ButtonSpinnerLarge")
	if _, noBG := CurrentStyles.ButtonSpinnerLarge.GetBackground().(lipgloss.NoColor); noBG {
		CurrentStyles.ButtonSpinnerLarge = CurrentStyles.ButtonSpinnerLarge.Background(CurrentStyles.ButtonActive.GetBackground())
	}
	if _, noFG := CurrentStyles.ButtonSpinnerLarge.GetForeground().(lipgloss.NoColor); noFG {
		CurrentStyles.ButtonSpinnerLarge = CurrentStyles.ButtonSpinnerLarge.Foreground(CurrentStyles.ButtonActive.GetForeground())
	}

	// Header / Status Bar
	CurrentStyles.StatusBar = SemanticRawStyle("StatusBar")
	CurrentStyles.StatusBarFocused = SemanticRawStyle("StatusBarFocused")
	CurrentStyles.StatusBarBorder = SemanticRawStyle("StatusBarBorder")
	{
		// Fallback for themes that don't define StatusBarBorder: use full StatusBar style.
		_, noFG := CurrentStyles.StatusBarBorder.GetForeground().(lipgloss.NoColor)
		_, noBG := CurrentStyles.StatusBarBorder.GetBackground().(lipgloss.NoColor)
		if noFG && noBG {
			CurrentStyles.StatusBarBorder = CurrentStyles.StatusBar
		}
	}
	CurrentStyles.HeaderBG = CurrentStyles.StatusBar // Backwards compatibility

	// Help line
	CurrentStyles.HelpLine = SemanticRawStyle("Helpline")

	// Submenu Title
	CurrentStyles.SubmenuTitle = SemanticRawStyle("TitleSubMenu")
	CurrentStyles.SubmenuTitleFocused = SemanticRawStyle("TitleSubMenuFocused")
	CurrentStyles.SubmenuTitleDisabled = SemanticRawStyle("TitleSubMenuDisabled")

	// Large Title Bar
	CurrentStyles.LargeTitleArea = SemanticRawStyle("LargeTitleArea")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	CurrentStyles.StatusSuccess = SemanticRawStyle("TitleNotice")
	CurrentStyles.StatusWarn = SemanticRawStyle("TitleWarn")
	CurrentStyles.Console = theme.ConsoleSemanticRawStyle("ProgramBox")

	CurrentStyles.ConsoleTitleColor = SemanticRawStyle("ConsoleTitle").GetForeground()

	CurrentStyles.DialogTitleAlign = cfg.UI.DialogTitleAlign
	CurrentStyles.SubmenuTitleAlign = cfg.UI.SubmenuTitleAlign
	CurrentStyles.PanelTitleAlign = cfg.UI.PanelTitleAlign
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
