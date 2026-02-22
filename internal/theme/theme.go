package theme

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/pelletier/go-toml/v2"
)

// StyleFlags holds ANSI style modifiers
type StyleFlags struct {
	Bold          bool
	Underline     bool
	Italic        bool
	Blink         bool
	Dim           bool
	Reverse       bool
	Strikethrough bool
	HighIntensity bool
}

// Apply applies all style flags to a lipgloss style
func (f StyleFlags) Apply(s lipgloss.Style) lipgloss.Style {
	return s.
		Bold(f.Bold).
		Underline(f.Underline).
		Italic(f.Italic).
		Blink(f.Blink).
		Faint(f.Dim).
		Reverse(f.Reverse).
		Strikethrough(f.Strikethrough)
}

// ResetFlags clears all text attributes from a style
func ResetFlags(s lipgloss.Style) lipgloss.Style {
	return StyleFlags{}.Apply(s)
}

// ThemeConfig holds colors derived from .dialogrc and theme.ini
type ThemeConfig struct {
	Name                     string
	ScreenFG                 color.Color
	ScreenBG                 color.Color
	DialogFG                 color.Color
	DialogBG                 color.Color
	BorderFG                 color.Color
	BorderBG                 color.Color
	Border2FG                color.Color
	Border2BG                color.Color
	TitleFG                  color.Color
	TitleBG                  color.Color
	TitleStyles              StyleFlags
	TitleHelpFG              color.Color
	TitleHelpBG              color.Color
	TitleHelpStyles          StyleFlags
	ShadowColor              color.Color
	ButtonActiveFG           color.Color
	ButtonActiveBG           color.Color
	ButtonActiveStyles       StyleFlags
	ButtonInactiveFG         color.Color
	ButtonInactiveBG         color.Color
	ButtonInactiveStyles     StyleFlags
	ItemSelectedFG           color.Color
	ItemSelectedBG           color.Color
	ItemSelectedStyles       StyleFlags
	ItemFG                   color.Color
	ItemBG                   color.Color
	ItemStyles               StyleFlags
	TagFG                    color.Color
	TagBG                    color.Color
	TagStyles                StyleFlags
	TagSelectedFG            color.Color
	TagSelectedBG            color.Color
	TagSelectedStyles        StyleFlags
	TagKeyFG                 color.Color
	TagKeyBG                 color.Color
	TagKeyStyles             StyleFlags
	TagKeySelectedFG         color.Color
	TagKeySelectedBG         color.Color
	TagKeySelectedStyles     StyleFlags
	HelplineFG               color.Color
	HelplineBG               color.Color
	HelplineStyles           StyleFlags
	ProgramFG                color.Color
	ProgramBG                color.Color
	ProgramStyles            StyleFlags
	LogBoxFG                 color.Color
	LogBoxBG                 color.Color
	LogBoxStyles             StyleFlags
	LogPanelFG               color.Color
	LogPanelBG               color.Color
	HeaderTag                string
	ProgressWaitingFG        color.Color
	ProgressWaitingBG        color.Color
	ProgressWaitingStyles    StyleFlags
	ProgressInProgressFG     color.Color
	ProgressInProgressBG     color.Color
	ProgressInProgressStyles StyleFlags
	ProgressCompletedFG      color.Color
	ProgressCompletedBG      color.Color
	ProgressCompletedStyles  StyleFlags
	VersionSelectedFG        color.Color
	VersionSelectedBG        color.Color
	VersionSelectedStyles    StyleFlags
	ListAppFG                color.Color
	ListAppBG                color.Color
	ListAppStyles            StyleFlags
	ListAppUserDefinedFG     color.Color
	ListAppUserDefinedBG     color.Color
	ListAppUserDefinedStyles StyleFlags
	HelpItemFG               color.Color
	HelpItemBG               color.Color
	HelpItemStyles           StyleFlags
	HelpTagFG                color.Color
	HelpTagBG                color.Color
	HelpTagStyles            StyleFlags
}

// Current holds the active theme configuration
var Current ThemeConfig

// Load theme by name. Returns theme-defined defaults if found.
// If prefix is provided, semantic tags are registered with that prefix (e.g. "Preview_Theme_Screen")
// without affecting the global active theme (Current).
func Load(themeName string, prefix string) (*ThemeDefaults, error) {
	// 1. Initialize with defaults first (Classic colors)
	Default(prefix)

	// If main load, set Current name
	if prefix == "" {
		Current.Name = themeName
	}

	themesDir := paths.GetThemesDir()
	themePath := filepath.Join(themesDir, themeName+".ds2theme")

	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		// If theme doesn't exist, try falling back to "DockSTARTer"
		if themeName != "DockSTARTer" {
			return Load("DockSTARTer", prefix)
		}
		// If even DockSTARTer doesn't exist, we stay with minimal defaults
		return nil, nil
	}

	// 2. Parse .ds2theme (Overrides defaults)
	// If prefix is set, we use a temporary config for parsing
	targetConf := &Current
	if prefix != "" {
		tempConf := ThemeConfig{}
		targetConf = &tempConf
	}

	defaults, err := parseThemeTOML(themePath, prefix, targetConf)
	return defaults, err
}

// Apply updates the global console.Colors with theme-specific tags
func Apply() {
	// 0. Ensure base tags and color map are built from defaults FIRST
	// This prevents theme-specific registration from being wiped out later.
	console.RegisterBaseTags()
	console.BuildColorMap()

	// 1. Register component tags from Current config (Defaults)
	// This maps the struct fields (colors) to tags like {{|Theme_Screen|}}
	RegisterTags("", Current)
}

// prefixTag is a helper to consistently prefix theme-related semantic tags
func prefixTag(prefix, name string) string {
	if prefix == "" {
		return name
	}
	p := strings.TrimSuffix(prefix, "_")
	return p + "_" + name
}

// RegisterTags registers all theme components into the console semantic registry
func RegisterTags(prefix string, conf ThemeConfig) {
	regComp := func(name string, fg, bg color.Color) {
		fgName := console.GetColorStr(fg)
		bgName := console.GetColorStr(bg)
		// Register raw value (no delimiters)
		console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_"+name), fgName+":"+bgName+":")
	}

	regComp("Screen", conf.ScreenFG, conf.ScreenBG)
	regComp("Dialog", conf.DialogFG, conf.DialogBG)
	regComp("Border", conf.BorderFG, conf.BorderBG)
	regComp("Border2", conf.Border2FG, conf.Border2BG)

	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_Title"), buildRawTag(conf.TitleFG, conf.TitleBG, conf.TitleStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_TitleHelp"), buildRawTag(conf.TitleHelpFG, conf.TitleHelpBG, conf.TitleHelpStyles))

	regComp("ButtonActive", conf.ButtonActiveFG, conf.ButtonActiveBG)
	regComp("ButtonInactive", conf.ButtonInactiveFG, conf.ButtonInactiveBG)
	regComp("ItemSelected", conf.ItemSelectedFG, conf.ItemSelectedBG)
	regComp("Item", conf.ItemFG, conf.ItemBG)
	regComp("Tag", conf.TagFG, conf.TagBG)
	regComp("TagSelected", conf.TagSelectedFG, conf.TagSelectedBG)

	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_TagKey"), buildRawTag(conf.TagKeyFG, conf.TagKeyBG, conf.TagKeyStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_TagKeySelected"), buildRawTag(conf.TagKeySelectedFG, conf.TagKeySelectedBG, conf.TagKeySelectedStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_Shadow"), console.GetColorStr(conf.ShadowColor)+"::")

	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_Helpline"), buildRawTag(conf.HelplineFG, conf.HelplineBG, conf.HelplineStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ItemHelp"), buildRawTag(conf.HelplineFG, conf.HelplineBG, conf.HelplineStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_Program"), buildRawTag(conf.ProgramFG, conf.ProgramBG, conf.ProgramStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ProgramBox"), buildRawTag(conf.ProgramFG, conf.ProgramBG, conf.ProgramStyles))
	console.RegisterSemanticTagRaw("Theme_LogBox", buildRawTag(conf.LogBoxFG, conf.LogBoxBG, conf.LogBoxStyles))
	regComp("LogPanel", conf.LogPanelFG, conf.LogPanelBG)
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ProgressWaiting"), buildRawTag(conf.ProgressWaitingFG, conf.ProgressWaitingBG, conf.ProgressWaitingStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ProgressInProgress"), buildRawTag(conf.ProgressInProgressFG, conf.ProgressInProgressBG, conf.ProgressInProgressStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ProgressCompleted"), buildRawTag(conf.ProgressCompletedFG, conf.ProgressCompletedBG, conf.ProgressCompletedStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_VersionSelected"), buildRawTag(conf.VersionSelectedFG, conf.VersionSelectedBG, conf.VersionSelectedStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ListApp"), buildRawTag(conf.ListAppFG, conf.ListAppBG, conf.ListAppStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ListAppUserDefined"), buildRawTag(conf.ListAppUserDefinedFG, conf.ListAppUserDefinedBG, conf.ListAppUserDefinedStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_HelpItem"), buildRawTag(conf.HelpItemFG, conf.HelpItemBG, conf.HelpItemStyles))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_HelpTag"), buildRawTag(conf.HelpTagFG, conf.HelpTagBG, conf.HelpTagStyles))
}

// Unload unregisters all theme-prefixed tags from the console registry.
func Unload(prefix string) {
	if prefix == "" {
		return // Cannot unload main theme
	}
	// Component list (matches RegisterTags)
	components := []string{
		"Screen", "Dialog", "Border", "Border2", "Title", "TitleHelp",
		"ButtonActive", "ButtonInactive", "ItemSelected", "Item", "Tag", "TagSelected",
		"TagKey", "TagKeySelected", "Shadow", "Helpline", "ItemHelp", "Program",
		"ProgramBox", "LogBox", "LogPanel", "ProgressWaiting", "ProgressInProgress",
		"ProgressCompleted", "VersionSelected", "ListApp", "ListAppUserDefined",
		"HelpItem", "HelpTag",
	}
	for _, comp := range components {
		console.UnregisterColor(prefixTag(prefix, "Theme_"+comp))
	}
}

// buildRawTag constructs a raw "fg:bg:flags" string (no delimiters)
func buildRawTag(fg, bg color.Color, styles StyleFlags) string {
	fgStr := console.GetColorStr(fg)
	bgStr := console.GetColorStr(bg)
	flags := ""
	if styles.Bold {
		flags += "B"
	}
	if styles.Underline {
		flags += "U"
	}
	if styles.Italic {
		flags += "I"
	}
	if styles.Blink {
		flags += "L"
	}
	if styles.Dim {
		flags += "D"
	}
	if styles.Reverse {
		flags += "R"
	}
	if styles.Strikethrough {
		flags += "S"
	}
	if styles.HighIntensity {
		flags += "H"
	}
	return fgStr + ":" + bgStr + ":" + flags
}

// Default initializes the Current configuration with standard DockSTARTer colors (Classic)
// If prefix is provided, semantic tags are registered with that prefix.
func Default(prefix string) {
	conf := ThemeConfig{}
	// Only update global Current if prefix is empty
	if prefix == "" {
		Current = conf
	}
	RegisterTags(prefix, conf)

	// Register basic theme fallbacks
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationName"), "::B")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationVersion"), "-")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationVersionBrackets"), "-")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationVersionSpace"), console.StripDelimiters(console.ExpandTags(console.WrapSemantic(prefixTag(prefix, "Theme_Screen")))))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationFlags"), "-")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationFlagsBrackets"), "-")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationFlagsSpace"), console.StripDelimiters(console.ExpandTags(console.WrapSemantic(prefixTag(prefix, "Theme_Screen")))))
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationUpdate"), "yellow::")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_ApplicationUpdateBrackets"), "-")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_TitleQuestion"), "black:yellow:B")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_Hostname"), "::B")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_HelpItem"), "black::")
	console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_HelpTag"), "black:red:")
}

// parseColor converts a color name or hex string to a color.Color.
//
// Standard ANSI Color Reference (for tcell/lipgloss mapping):
// black   = 0 (#000000)
// red     = 1 (#800000 / #ff0000)
// green   = 2 (#008000)
// yellow  = 3 (#808000 / #ffff00)
// blue    = 4 (#000080)
// magenta = 5 (#800080 / #ff00ff) (Aliased to Fuchsia)
// cyan    = 6 (#008080 / #00ffff) (Aliased to Aqua)
// white   = 7 (#c0c0c0)
func parseColor(c string) color.Color {
	c = strings.ToLower(strings.TrimSpace(c))

	// 1. Try resolving with helpers in console package (Supports extended names and aliases)
	if hexVal := console.GetHexForColor(c); hexVal != "" {
		return lipgloss.Color(hexVal)
	}

	// 2. Hex codes (Fallback if tcell didn't catch it, though tcell handles many)
	if strings.HasPrefix(c, "#") {
		return lipgloss.Color(c)
	}
	return nil
}

// parseRawToColor parses a raw style string (fg:bg:flags) to colors
func parseRawToColor(raw string) (fg, bg color.Color) {
	parts := strings.Split(raw, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	}
	return
}

// parseRawWithStyles parses a raw style string and extracts colors and style flags
func parseRawWithStyles(raw string) (fg, bg color.Color, styles StyleFlags) {
	parts := strings.Split(raw, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	}
	// Parse style flags (third part)
	if len(parts) > 2 {
		flags := parts[2]
		styles.Bold = strings.Contains(flags, "B")
		styles.Underline = strings.Contains(flags, "U")
		styles.Italic = strings.Contains(flags, "I")
		styles.Blink = strings.Contains(flags, "L")
		styles.Dim = strings.Contains(flags, "D")
		styles.Reverse = strings.Contains(flags, "R")
		styles.Strikethrough = strings.Contains(flags, "S")
		styles.HighIntensity = strings.Contains(flags, "H")
	}
	return
}

// resolveThemeValue recursively resolves a theme value string, handling semantic references and overrides.
// It supports chaining (A->B->C) and partial overlays.
// Uses file-specific delimiters (semPre/semSuf for semantic, dirPre/dirSuf for direct).
// Returns a RAW style string (fg:bg:flags) without any delimiters.
func resolveThemeValue(raw string, rawValues map[string]string, visiting map[string]bool,
	semPre, semSuf, dirPre, dirSuf string) (string, error) {

	var finalFG, finalBG string
	var finalFlags string

	// Helper to merge a raw style string (fg:bg:flags)
	mergeStyle := func(styleStr string) {
		// Strip any delimiters to get raw content
		inner := console.StripDelimiters(styleStr)
		parts := strings.Split(inner, ":")

		if len(parts) > 0 && parts[0] != "" {
			finalFG = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			finalBG = parts[1]
		}
		if len(parts) > 2 {
			// Merge flags (concatenate - renderer handles ordering)
			for _, f := range parts[2] {
				finalFlags += string(f)
			}
		}
	}

	// Iterate through the string seeking {{ }}
	cur := raw
	for {
		start := strings.Index(cur, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(cur[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		tag := cur[start:end]

		if strings.HasPrefix(tag, dirPre) {
			// Direct style tag - extract and merge
			mergeStyle(tag)
		} else if strings.HasPrefix(tag, semPre) {
			// Semantic reference - extract tag name
			refKey := strings.TrimSuffix(strings.TrimPrefix(tag, semPre), semSuf)

			// Check if it's a theme key reference (Theme_...)
			if strings.HasPrefix(refKey, "Theme_") {
				targetKey := strings.TrimPrefix(refKey, "Theme_")

				// Resolve it recursively
				resolvedRef, err := resolveThemeValue(rawValues[targetKey], rawValues, visiting,
					semPre, semSuf, dirPre, dirSuf)
				if err == nil {
					mergeStyle(resolvedRef)
				} else {
					// Maybe it's a global semantic tag?
					expanded := console.ExpandTags(tag)
					if expanded != tag && expanded != "" {
						mergeStyle(expanded)
					}
				}
			} else {
				// Regular semantic tag (e.g. Notice)
				expanded := console.ExpandTags(tag)
				if expanded != tag && expanded != "" {
					mergeStyle(expanded)
				}
			}
		}

		cur = cur[end:]
	}

	// Return RAW style string (no delimiters)
	return fmt.Sprintf("%s:%s:%s", finalFG, finalBG, finalFlags), nil
}

type ThemeDefaults struct {
	Borders        *bool `toml:"borders"`
	LineCharacters *bool `toml:"line_characters"`
	Shadow         *bool `toml:"shadow"`
	ShadowLevel    *int  `toml:"shadow_level"`
	Scrollbar      *bool `toml:"scrollbar"`
	BorderColor    *int  `toml:"border_color"`
}

type ThemeFile struct {
	Metadata struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
		Author      string `toml:"author"`
	} `toml:"metadata"`
	Syntax *struct {
		SemanticPrefix string `toml:"semantic_prefix"`
		SemanticSuffix string `toml:"semantic_suffix"`
		DirectPrefix   string `toml:"direct_prefix"`
		DirectSuffix   string `toml:"direct_suffix"`
	} `toml:"syntax"`
	Defaults *ThemeDefaults    `toml:"defaults"`
	Colors   map[string]string `toml:"colors"`
}

// GetThemeFile reads a theme file and returns its structured content without applying it.
func GetThemeFile(themeName string) (ThemeFile, error) {
	themePath := filepath.Join(paths.GetThemesDir(), themeName+".ds2theme")
	data, err := os.ReadFile(themePath)
	if err != nil {
		return ThemeFile{}, err
	}

	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return ThemeFile{}, err
	}
	return tf, nil
}

// ApplyThemeDefaults updates the app config with any defaults provided by the theme.
// It returns a map of all settings provided by the theme and their values.
func ApplyThemeDefaults(conf *config.AppConfig, defaults ThemeDefaults) map[string]string {
	applied := make(map[string]string)
	if defaults.Borders != nil {
		conf.UI.Borders = *defaults.Borders
		applied["Borders"] = fmt.Sprintf("%v", conf.UI.Borders)
	}
	if defaults.LineCharacters != nil {
		conf.UI.LineCharacters = *defaults.LineCharacters
		applied["Line Characters"] = fmt.Sprintf("%v", conf.UI.LineCharacters)
	}
	if defaults.Shadow != nil {
		conf.UI.Shadow = *defaults.Shadow
		applied["Shadow"] = fmt.Sprintf("%v", conf.UI.Shadow)
	}
	if defaults.ShadowLevel != nil {
		conf.UI.ShadowLevel = *defaults.ShadowLevel
		applied["Shadow Level"] = fmt.Sprintf("%d", conf.UI.ShadowLevel)
	}
	if defaults.Scrollbar != nil {
		conf.UI.Scrollbar = *defaults.Scrollbar
		applied["Scrollbar"] = fmt.Sprintf("%v", conf.UI.Scrollbar)
	}
	if defaults.BorderColor != nil {
		conf.UI.BorderColor = *defaults.BorderColor
		applied["Border Color"] = fmt.Sprintf("%d", conf.UI.BorderColor)
	}
	return applied
}

func parseThemeTOML(path string, prefix string, targetConf *ThemeConfig) (*ThemeDefaults, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return nil, err
	}

	// Get file-specific delimiters (or use code defaults)
	semPre, semSuf := console.SemanticPrefix, console.SemanticSuffix
	dirPre, dirSuf := console.DirectPrefix, console.DirectSuffix
	if tf.Syntax != nil {
		if tf.Syntax.SemanticPrefix != "" {
			semPre = tf.Syntax.SemanticPrefix
		}
		if tf.Syntax.SemanticSuffix != "" {
			semSuf = tf.Syntax.SemanticSuffix
		}
		if tf.Syntax.DirectPrefix != "" {
			dirPre = tf.Syntax.DirectPrefix
		}
		if tf.Syntax.DirectSuffix != "" {
			dirSuf = tf.Syntax.DirectSuffix
		}
	}

	// 1. Resolve values and register/apply them
	// We need to resolve references (e.g., TitleSuccess -> Title) before parsing colors
	rawValues := tf.Colors
	visiting := make(map[string]bool)

	// Maintains consistent registration/mapping logic from INI version
	for key, raw := range rawValues {
		styleValue, err := resolveThemeValue(raw, rawValues, visiting, semPre, semSuf, dirPre, dirSuf)
		if err != nil {
			// Fallback to raw expansion for robustness
			styleValue = console.StripDelimiters(console.ExpandTags(raw))
		}

		// Register using raw value (no delimiters)
		console.RegisterSemanticTagRaw(prefixTag(prefix, "Theme_"+key), styleValue)

		// Map known keys to target configuration struct fields
		fg, bg := parseRawToColor(styleValue)
		switch key {
		case "Screen":
			targetConf.ScreenFG, targetConf.ScreenBG = fg, bg
		case "Dialog":
			targetConf.DialogFG, targetConf.DialogBG = fg, bg
		case "Border":
			targetConf.BorderFG, targetConf.BorderBG = fg, bg
		case "Border2":
			targetConf.Border2FG, targetConf.Border2BG = fg, bg
		case "Title":
			targetConf.TitleFG, targetConf.TitleBG, targetConf.TitleStyles = parseRawWithStyles(styleValue)
		case "TitleHelp":
			targetConf.TitleHelpFG, targetConf.TitleHelpBG, targetConf.TitleHelpStyles = parseRawWithStyles(styleValue)
		case "BoxTitle":
			// Only set if Title wasn't explicitly provided in theme
			if _, titleExists := rawValues["Title"]; !titleExists {
				targetConf.TitleFG, targetConf.TitleBG, targetConf.TitleStyles = parseRawWithStyles(styleValue)
			}
		case "Shadow":
			targetConf.ShadowColor = fg
		case "ButtonActive":
			fg, bg, styles := parseRawWithStyles(styleValue)
			targetConf.ButtonActiveFG, targetConf.ButtonActiveBG = fg, bg
			targetConf.ButtonActiveStyles = styles
		case "ButtonInactive":
			fg, bg, styles := parseRawWithStyles(styleValue)
			targetConf.ButtonInactiveFG, targetConf.ButtonInactiveBG = fg, bg
			targetConf.ButtonInactiveStyles = styles
		case "ItemSelected":
			targetConf.ItemSelectedFG, targetConf.ItemSelectedBG, targetConf.ItemSelectedStyles = parseRawWithStyles(styleValue)
		case "Item":
			targetConf.ItemFG, targetConf.ItemBG, targetConf.ItemStyles = parseRawWithStyles(styleValue)
		case "Tag":
			targetConf.TagFG, targetConf.TagBG, targetConf.TagStyles = parseRawWithStyles(styleValue)
		case "TagSelected":
			targetConf.TagSelectedFG, targetConf.TagSelectedBG, targetConf.TagSelectedStyles = parseRawWithStyles(styleValue)
		case "TagKey":
			targetConf.TagKeyFG, targetConf.TagKeyBG, targetConf.TagKeyStyles = parseRawWithStyles(styleValue)
		case "TagKeySelected":
			targetConf.TagKeySelectedFG, targetConf.TagKeySelectedBG, targetConf.TagKeySelectedStyles = parseRawWithStyles(styleValue)
		case "Helpline", "ItemHelp", "itemhelp_color":
			targetConf.HelplineFG, targetConf.HelplineBG, targetConf.HelplineStyles = parseRawWithStyles(styleValue)
		case "Program", "ProgramBox":
			targetConf.ProgramFG, targetConf.ProgramBG, targetConf.ProgramStyles = parseRawWithStyles(styleValue)
		case "LogBox":
			targetConf.LogBoxFG, targetConf.LogBoxBG, targetConf.LogBoxStyles = parseRawWithStyles(styleValue)
		case "LogPanel":
			targetConf.LogPanelFG, targetConf.LogPanelBG = fg, bg
		case "Header":
			targetConf.HeaderTag = styleValue
		case "ProgressWaiting":
			targetConf.ProgressWaitingFG, targetConf.ProgressWaitingBG, targetConf.ProgressWaitingStyles = parseRawWithStyles(styleValue)
		case "ProgressInProgress":
			targetConf.ProgressInProgressFG, targetConf.ProgressInProgressBG, targetConf.ProgressInProgressStyles = parseRawWithStyles(styleValue)
		case "ProgressCompleted":
			targetConf.ProgressCompletedFG, targetConf.ProgressCompletedBG, targetConf.ProgressCompletedStyles = parseRawWithStyles(styleValue)
		case "VersionSelected":
			targetConf.VersionSelectedFG, targetConf.VersionSelectedBG, targetConf.VersionSelectedStyles = parseRawWithStyles(styleValue)
		case "ListApp":
			targetConf.ListAppFG, targetConf.ListAppBG, targetConf.ListAppStyles = parseRawWithStyles(styleValue)
		case "ListAppUserDefined":
			targetConf.ListAppUserDefinedFG, targetConf.ListAppUserDefinedBG, targetConf.ListAppUserDefinedStyles = parseRawWithStyles(styleValue)
		case "HelpItem":
			targetConf.HelpItemFG, targetConf.HelpItemBG, targetConf.HelpItemStyles = parseRawWithStyles(styleValue)
		case "HelpTag":
			targetConf.HelpTagFG, targetConf.HelpTagBG, targetConf.HelpTagStyles = parseRawWithStyles(styleValue)
		}
	}

	// 2. Re-apply tags if loading main theme
	if prefix == "" {
		console.RegisterBaseTags()
		console.BuildColorMap()
	}

	return tf.Defaults, nil
}
