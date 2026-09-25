package classic

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	semstyle "github.com/GhostWriters/semstyle/lg"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"sync"

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
// Both \x07 (BEL) and \x1b\\ (ST) terminators are supported. Used by
// ScanForHyperlinks (hit-region detection), which needs the paired
// url/content capture groups.
var hyperlinkRegex = regexp.MustCompile(`\x1b\]8;.*?;(.*?)(?:\x07|\x1b\\)(.*?)\x1b\]8;;(?:\x07|\x1b\\)`)

// osc8MarkerRegex matches a single OSC 8 marker (open or close) independent
// of pairing. Unlike hyperlinkRegex, which matches a whole open-content-close
// span and so breaks on nested hyperlinks (e.g. an image inside a link --
// the inner close consumes the outer's closer too, leaving the outer's open
// marker stranded), this removes every marker on its own regardless of
// nesting depth, since StripHyperlinks only needs the markers gone, not to
// correlate which open belongs to which close.
var osc8MarkerRegex = regexp.MustCompile(`\x1b\]8;[^\x07\x1b]*(?:\x07|\x1b\\)`)

// StripHyperlinks removes OSC 8 hyperlink markers from rendered text while
// keeping the visible content and its own styling intact -- a file:// link
// only resolves on the machine DS2 itself runs on, so it's meaningless for
// SSH and web sessions (unlike an https:// docs link, there's no useful
// fallback since the remote machine doesn't have the file). Call for any
// non-local session before displaying such text.
func StripHyperlinks(rendered string) string {
	return osc8MarkerRegex.ReplaceAllString(rendered, "")
}

// HyperlinkPath renders path as a clickable OSC 8 hyperlink to itself
// (file://path). styleTag is a semantic tag ("{{|Tag|}}") or a direct style
// code ("{{[fg:bg:attrs]}}"), resolved like any other themed text. Call once
// per independently-clickable path segment (e.g. each directory component
// plus the filename) rather than trying to express multiple destinations in
// one call. See StripHyperlinks for why this is local-session-only.
//
// Renders immediately (like style.Render()) -- not deferred semstyle
// markup, so it's for callers with an already-resolved rendering context,
// not for a logger.Notice/Info message resolved later per-destination
// (console/file/TUI). For that, build the raw tag with strutil.FileURL
// yourself (see console.FormatFilePath).
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
	IDOverridePanel    = "ansi_overrides_panel"
	IDTintPanel        = "tint_schemes_panel"
	IDButtonPanel      = "button_panel"
	IDListPanel        = "list_panel"
	IDSaveButton       = "save_button"
	IDRefreshButton    = "refresh_button"
	IDApplyButton      = "apply_button"
	IDResetButton      = "reset_button"
	IDBackButton       = "back_button"
	IDExitButton       = "exit_button"
	IDHeaderFlags      = "header_flags"
	IDHeaderWebDisplay = "header_web_display"
	IDHelpline         = "helpline"

	// INS/OVR mode toggle hit region ID
	IDInsOvr = "ins_ovr"

	// Title bar widget IDs (suffix appended to menu ID)
	IDTitleWidgetRefresh    = "title_widget_refresh"
	IDTitleWidgetHelp       = "title_widget_help"
	IDTitleWidgetClose      = "title_widget_close"
	IDTitleWidgetMaximize   = "title_widget_maximize"
	IDTitleWidgetSideBySide = "title_widget_sidebyside"
	IDTitleWidgetStacked    = "title_widget_stacked"

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
	LargeButtonSpinner lipgloss.Style // Spinner at edges of bordered button when processing

	// Title bar icon widgets
	IconFocused          lipgloss.Style
	IconPressed          lipgloss.Style
	IconInactive         lipgloss.Style
	IconHelpInactive     lipgloss.Style
	IconRefreshInactive  lipgloss.Style
	IconExitInactive     lipgloss.Style
	IconResizeUpInactive lipgloss.Style
	IconResizeDnInactive lipgloss.Style
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
	LineCharacters     bool
	DrawBorders        bool
	LargeButtons       bool
	LargeTitleBars     bool
	DialogTitleAlign   string
	SubmenuTitleAlign  string
	PanelTitleAlign    string
	CheckboxBrackets   string
	RadioBrackets      string
	MenuBrackets       bool
	LineNumberBrackets bool

	// Option value (dropdown/inline value in flow menus)
	OptionValueFocused lipgloss.Style

	// Semantic styles derived from theme tags
	StatusSuccess lipgloss.Style
	StatusWarn    lipgloss.Style
	Console       lipgloss.Style

	// Panel title color
	PanelTitleColor color.Color
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
	IconHelpInactive     lipgloss.Style
	IconRefreshInactive  lipgloss.Style
	IconExitInactive     lipgloss.Style
	IconResizeUpInactive lipgloss.Style
	IconResizeDnInactive lipgloss.Style
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
	LargeButtonSpinner   lipgloss.Style
	Shadow               lipgloss.Style
	ShadowColor          color.Color
	ShadowLevel          int
	HelpLine             lipgloss.Style
	StatusSuccess        lipgloss.Style
	StatusWarn           lipgloss.Style
	Console              lipgloss.Style
	OptionValueFocused   lipgloss.Style
	StatusBarFocused     lipgloss.Style
	PanelTitleColor      color.Color
	DialogTitleAlign     string
	SubmenuTitleAlign    string
	PanelTitleAlign      string
	CheckboxBrackets     string
	RadioBrackets        string
	MenuBrackets         bool
	LineNumberBrackets   bool
	Prefix               string // Prefix for semantic tag remapping (e.g. "Preview_")
	DrawShadow           bool   // Whether to draw shadows for this context
}

// GetStyles returns the Styles for the active theme prefix and tint (see
// semstyle.RunWithRenderScope), building and caching them on first use in
// that scope.
func GetStyles() Styles {
	key := StylesScopeKey()
	stylesMu.Lock()
	s, ok := stylesCache[key]
	stylesMu.Unlock()
	if ok {
		return s
	}
	s = buildStyles(ActiveAppearance())
	stylesMu.Lock()
	stylesCache[key] = s
	stylesMu.Unlock()
	return s
}

// GetActiveContext returns the active scope's styles (see GetStyles) as a StyleContext
func GetActiveContext() StyleContext {
	cs := GetStyles()
	return StyleContext{
		LineCharacters:       cs.LineCharacters,
		DrawBorders:          cs.DrawBorders,
		LargeButtons:         cs.LargeButtons,
		LargeTitleBars:       cs.LargeTitleBars,
		Type:                 DialogTypeInfo, // Default to info
		Screen:               cs.Screen,
		Dialog:               cs.Dialog,
		ContentBackground:    cs.ContentBackground,
		DialogTitle:          cs.DialogTitle,
		DialogTitleHelp:      cs.DialogTitleHelp,
		SubmenuTitle:         cs.SubmenuTitle,
		SubmenuTitleFocused:  cs.SubmenuTitleFocused,
		SubmenuTitleDisabled: cs.SubmenuTitleDisabled,
		LargeTitleArea:       cs.LargeTitleArea,
		Border:               cs.Border,
		BorderColor:          cs.BorderColor,
		Border2Color:         cs.Border2Color,
		BorderDisabledColor:  cs.BorderDisabledColor,
		Border2DisabledColor: cs.Border2DisabledColor,
		BorderFlags:          cs.BorderFlags,
		Border2Flags:         cs.Border2Flags,
		BorderDisabledFlags:  cs.BorderDisabledFlags,
		Border2DisabledFlags: cs.Border2DisabledFlags,
		ButtonActive:         cs.ButtonActive,
		ButtonInactive:       cs.ButtonInactive,
		IconFocused:          cs.IconFocused,
		IconPressed:          cs.IconPressed,
		IconInactive:         cs.IconInactive,
		IconHelpInactive:     cs.IconHelpInactive,
		IconRefreshInactive:  cs.IconRefreshInactive,
		IconExitInactive:     cs.IconExitInactive,
		IconResizeUpInactive: cs.IconResizeUpInactive,
		IconResizeDnInactive: cs.IconResizeDnInactive,
		ButtonKeyActive:      cs.ButtonKeyActive,
		ButtonKeyInactive:    cs.ButtonKeyInactive,
		ItemNormal:           cs.ItemNormal,
		ItemFocused:          cs.ItemFocused,
		TagNormal:            cs.TagNormal,
		TagFocused:           cs.TagFocused,
		TagKey:               cs.TagKey,
		TagKeyFocused:        cs.TagKeyFocused,
		TagSpinner:           cs.TagSpinner,
		ButtonSpinner:        cs.ButtonSpinner,
		LargeButtonSpinner:   cs.LargeButtonSpinner,
		Shadow:               cs.Shadow,
		ShadowColor:          cs.ShadowColor,
		ShadowLevel:          ActiveAppearance().ShadowLevel,
		HelpLine:             cs.HelpLine,
		StatusSuccess:        cs.StatusSuccess,
		StatusWarn:           cs.StatusWarn,
		Console:              cs.Console,
		OptionValueFocused:   cs.OptionValueFocused,
		StatusBarFocused:     cs.StatusBarFocused,
		PanelTitleColor:      cs.PanelTitleColor,
		DialogTitleAlign:     cs.DialogTitleAlign,
		SubmenuTitleAlign:    cs.SubmenuTitleAlign,
		PanelTitleAlign:      cs.PanelTitleAlign,
		CheckboxBrackets:     cs.CheckboxBrackets,
		RadioBrackets:        cs.RadioBrackets,
		MenuBrackets:         cs.MenuBrackets,
		LineNumberBrackets:   cs.LineNumberBrackets,
		Prefix:               "", // Global context has no prefix
		DrawShadow:           ActiveAppearance().Shadow,
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

// hasHardResetField reports whether tagName's raw fg:bg:flags code has an
// explicit "~" in the given field index (0 = fg, 1 = bg) -- semstyle's
// "defer entirely to the real terminal" marker. A lipgloss.NoColor{} alone
// can't distinguish that deliberate choice from a theme simply never
// specifying a color there (both resolve to the same zero value), so
// styleWithFallback consults the raw code directly instead.
func hasHardResetField(tagName string, field int) bool {
	parts := strings.Split(semstyle.GetRawTagCode(tagName), ":")
	return field < len(parts) && parts[field] == "~"
}

// styleWithFallback resolves tagName the same way SemanticRawStyle does,
// filling in fallback's foreground and/or background on whichever channel
// tagName leaves unset -- unless the theme explicitly authored a hard reset
// ("~") on that channel, which must stay truly unset rather than silently
// inheriting a color the author opted out of. Every interior dialog element
// this is used for (buttons, icons, tags, items) always sits against a
// themed background, so an unset foreground has no plausible reason to fall
// through to the raw terminal default instead of matching fallback's own
// foreground -- unlike ProgramBox's "~" fallback, which deliberately wants
// the real terminal's own colors.
func styleWithFallback(tagName string, fallback lipgloss.Style) lipgloss.Style {
	style := SemanticRawStyle(tagName)
	if _, noBG := style.GetBackground().(lipgloss.NoColor); noBG && !hasHardResetField(tagName, 1) {
		style = style.Background(fallback.GetBackground())
	}
	if _, noFG := style.GetForeground().(lipgloss.NoColor); noFG && !hasHardResetField(tagName, 0) {
		style = style.Foreground(fallback.GetForeground())
	}
	return style
}

// stylesCache holds one fully built Styles per (active theme prefix, active
// tint key) pair seen since the last InitStyles, so each session renders
// with its own theme and tint -- both are fixed into a lipgloss.Style when
// it's built (see semstyle.ToColorCtx), so one shared struct can't serve
// sessions that differ in either.
var (
	stylesMu    sync.Mutex
	stylesCache = map[string]Styles{}
)

// InitStyles records cfg as the live configuration and discards every
// cached Styles, so the next GetStyles call for each theme/tint scope
// rebuilds from cfg and the currently registered theme tags.
func InitStyles(cfg config.AppConfig) {
	// Clear the semantic style cache to ensure real-time visual updates on theme swap
	ClearSemanticCache()

	stylesMu.Lock()
	// Update the global config so IsShadowEnabled(), GetActiveContext().ShadowLevel,
	// and any other currentConfig readers see the new values immediately.
	currentConfig = cfg
	stylesCache = map[string]Styles{}
	stylesMu.Unlock()
}

// InvalidateStyles discards every cached Styles and semantic render without
// changing the live configuration -- for when a theme namespace or tint is
// re-registered under a scope key that may already be cached.
func InvalidateStyles() {
	ClearSemanticCache()
	stylesMu.Lock()
	stylesCache = map[string]Styles{}
	stylesMu.Unlock()
}

// StylesScopeKey identifies the active connType/theme/tint scope; anything
// cached from resolved colors or per-connType settings must include it in
// its key.
func StylesScopeKey() string {
	return console.ActiveConnType() + "|" + semstyle.ActiveThemePrefix() + "|" + semstyle.ActiveTintKey()
}

// ActiveAppearance returns the rendering connType's Appearance (see
// console.ActiveConnType).
func ActiveAppearance() config.Appearance {
	ct := console.ActiveConnType()
	stylesMu.Lock()
	defer stylesMu.Unlock()
	return currentConfig.Appearance.ForConnType(ct)
}

// buildStyles resolves every Styles field from a and whichever theme
// prefix and tint are active right now.
func buildStyles(a config.Appearance) Styles {
	var s Styles

	// Store LineCharacters setting for later use
	s.LineCharacters = a.LineCharacters
	s.DrawBorders = a.Borders
	s.LargeButtons = a.LargeButtons
	s.LargeTitleBars = a.LargeTitleBars

	// Border style based on LineCharacters setting
	if a.LineCharacters {
		s.Border = lipgloss.RoundedBorder()
		s.SepChar = "─"
	} else {
		s.Border = AsciiBorder
		s.SepChar = "-"
	}

	// Screen background
	s.Screen = SemanticRawStyle("Screen")

	// Dialog
	s.Dialog = SemanticRawStyle("Dialog")
	s.ContentBackground = s.Dialog

	s.DialogTitle = SemanticRawStyle("Title")

	s.DialogTitleHelp = SemanticRawStyle("TitleHelp")

	// Border colors and flags, merged per the Border Color mode setting.
	borderOverrides := ResolveThemeOverrides(a.BorderColor, "")
	s.BorderColor = borderOverrides["Border"].Style.GetForeground()
	s.Border2Color = borderOverrides["Border2"].Style.GetForeground()
	s.BorderFlags = borderOverrides["Border"].Flags
	s.Border2Flags = borderOverrides["Border2"].Flags

	s.BorderDisabledColor = borderOverrides["BorderDisabled"].Style.GetForeground()
	s.Border2DisabledColor = borderOverrides["Border2Disabled"].Style.GetForeground()
	s.BorderDisabledFlags = borderOverrides["BorderDisabled"].Flags
	s.Border2DisabledFlags = borderOverrides["Border2Disabled"].Flags

	// Shadow defines the shadow color and any attributes (e.g. dim, bold) for shade characters.
	shadowDef := SemanticRawStyle("Shadow")
	s.ShadowColor = shadowDef.GetForeground()
	if s.ShadowColor == (lipgloss.NoColor{}) {
		s.ShadowColor = shadowDef.GetBackground()
	}
	s.Shadow = shadowDef.UnsetBackground()

	// Buttons (spacing handled at layout level)
	// lipgloss v2 GetBackground() returns NoColor{} (never nil) for unset colors.
	// Use type assertion to detect truly unset colors and fall back to Dialog's.
	s.ButtonActive = styleWithFallback("ButtonActive", s.Dialog)
	s.ButtonInactive = styleWithFallback("ButtonInactive", s.Dialog)

	// Title bar icon widgets
	s.IconFocused = styleWithFallback("IconFocused", s.Dialog)
	s.IconPressed = styleWithFallback("IconPressed", s.Dialog)
	s.IconInactive = styleWithFallback("IconInactive", s.Dialog)
	s.IconHelpInactive = styleWithFallback("IconHelpInactive", s.Dialog)
	s.IconRefreshInactive = styleWithFallback("IconRefreshInactive", s.Dialog)
	s.IconExitInactive = styleWithFallback("IconExitInactive", s.Dialog)
	s.IconResizeUpInactive = styleWithFallback("IconResizeUpInactive", s.Dialog)
	s.IconResizeDnInactive = styleWithFallback("IconResizeDnInactive", s.Dialog)
	s.ButtonKeyActive = styleWithFallback("ButtonKeyActive", s.ButtonActive)
	s.ButtonKeyInactive = styleWithFallback("ButtonKeyInactive", s.ButtonInactive)

	// List items
	s.ItemNormal = styleWithFallback("Item", s.Dialog)
	s.ItemFocused = styleWithFallback("ItemFocused", s.Dialog)
	s.OptionValueFocused = styleWithFallback("OptionValueFocused", s.Dialog)

	// Tags
	s.TagNormal = styleWithFallback("Tag", s.Dialog)
	s.TagFocused = styleWithFallback("TagFocused", s.Dialog)
	s.TagKey = styleWithFallback("TagKey", s.Dialog)
	s.TagKeyFocused = styleWithFallback("TagKeyFocused", s.Dialog)
	s.TagSpinner = styleWithFallback("TagSpinner", s.Dialog)

	s.ButtonSpinner = styleWithFallback("ButtonSpinner", s.ButtonActive)
	s.LargeButtonSpinner = styleWithFallback("LargeButtonSpinner", s.ButtonActive)

	// Header / Status Bar
	s.StatusBar = SemanticRawStyle("StatusBar")
	s.StatusBarFocused = SemanticRawStyle("StatusBarFocused")
	s.StatusBarBorder = SemanticRawStyle("StatusBarBorder")
	{
		// Fallback for themes that don't define StatusBarBorder: use full StatusBar style.
		_, noFG := s.StatusBarBorder.GetForeground().(lipgloss.NoColor)
		_, noBG := s.StatusBarBorder.GetBackground().(lipgloss.NoColor)
		if noFG && noBG {
			s.StatusBarBorder = s.StatusBar
		}
	}
	s.HeaderBG = s.StatusBar // Backwards compatibility

	// Help line
	s.HelpLine = SemanticRawStyle("Helpline")

	// Submenu Title
	s.SubmenuTitle = SemanticRawStyle("TitleSubMenu")
	s.SubmenuTitleFocused = SemanticRawStyle("TitleSubMenuFocused")
	// ResolveDisabledStyle, not a plain lookup: themes rarely define an explicit
	// TitleSubMenuDisabled tag, and a bare lookup of a tag the theme never
	// registered resolves to an undefined/generic style instead of falling back
	// to TitleSubMenu with Bold stripped and Dim applied (the same rule every
	// other disabled element uses -- see ResolveDisabledStyle).
	s.SubmenuTitleDisabled, _ = ResolveDisabledStyle("TitleSubMenu")

	// Large Title Bar
	s.LargeTitleArea = SemanticRawStyle("LargeTitleArea")

	// Initialize semantic styles from console color tags (Theme-specific to avoid log interference)
	s.StatusSuccess = SemanticRawStyle("TitleNotice")
	s.StatusWarn = SemanticRawStyle("TitleWarn")
	s.Console = theme.ConsoleSemanticRawStyle("ProgramBox")

	s.PanelTitleColor = SemanticRawStyle("PanelTitle").GetForeground()

	s.DialogTitleAlign = a.DialogTitleAlign
	s.SubmenuTitleAlign = a.SubmenuTitleAlign
	s.PanelTitleAlign = a.PanelTitleAlign
	s.CheckboxBrackets = a.CheckboxBrackets
	s.RadioBrackets = a.RadioBrackets
	s.MenuBrackets = a.MenuBrackets
	s.LineNumberBrackets = a.LineNumberBrackets

	return s
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
