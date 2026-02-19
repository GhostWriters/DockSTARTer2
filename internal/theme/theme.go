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
}

// Current holds the active theme configuration
var Current ThemeConfig

// Load theme by name
func Load(themeName string) error {
	// Initialize with defaults first
	Default()

	themesDir := paths.GetThemesDir()
	themePath := filepath.Join(themesDir, themeName+".ds2theme")

	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		// If theme doesn't exist, try falling back to "DockSTARTer"
		if themeName != "DockSTARTer" {
			return Load("DockSTARTer")
		}
		// If even DockSTARTer doesn't exist, we stay with minimal defaults
		return nil
	}

	// Parse .ds2theme (Overrides defaults)
	// fmt.Println("DEBUG: Loading theme from:", themePath)
	_ = parseThemeTOML(themePath)

	// Synchronize themed values to console semantic tags
	Apply()

	return nil
}

// Apply updates the global console.Colors with theme-specific tags
func Apply() {
	// 0. Ensure base tags and color map are built from defaults FIRST
	// This prevents theme-specific registration from being wiped out later.
	console.RegisterBaseTags()
	console.BuildColorMap()

	// 1. Register component tags from Current config (Defaults)
	// This maps the struct fields (colors) to tags like {{|ThemeScreen|}}
	updateTagsFromCurrent()
}

func updateTagsFromCurrent() {
	regComp := func(name string, fg, bg color.Color) {
		fgName := console.GetColorStr(fg)
		bgName := console.GetColorStr(bg)
		// Register raw value (no delimiters)
		console.RegisterSemanticTagRaw("Theme_"+name, fgName+":"+bgName+":")
	}

	regComp("Screen", Current.ScreenFG, Current.ScreenBG)
	regComp("Dialog", Current.DialogFG, Current.DialogBG)
	regComp("Border", Current.BorderFG, Current.BorderBG)
	regComp("Border2", Current.Border2FG, Current.Border2BG)

	console.RegisterSemanticTagRaw("Theme_Title", buildRawTag(Current.TitleFG, Current.TitleBG, Current.TitleStyles))
	console.RegisterSemanticTagRaw("Theme_TitleHelp", buildRawTag(Current.TitleHelpFG, Current.TitleHelpBG, Current.TitleHelpStyles))

	regComp("ButtonActive", Current.ButtonActiveFG, Current.ButtonActiveBG)
	regComp("ItemSelected", Current.ItemSelectedFG, Current.ItemSelectedBG)
	regComp("Item", Current.ItemFG, Current.ItemBG)
	regComp("Tag", Current.TagFG, Current.TagBG)
	regComp("TagSelected", Current.TagSelectedFG, Current.TagSelectedBG)

	console.RegisterSemanticTagRaw("Theme_TagKey", buildRawTag(Current.TagKeyFG, Current.TagKeyBG, Current.TagKeyStyles))
	console.RegisterSemanticTagRaw("Theme_TagKeySelected", buildRawTag(Current.TagKeySelectedFG, Current.TagKeySelectedBG, Current.TagKeySelectedStyles))
	console.RegisterSemanticTagRaw("Theme_Shadow", console.GetColorStr(Current.ShadowColor)+"::")

	console.RegisterSemanticTagRaw("Theme_Helpline", buildRawTag(Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles))
	console.RegisterSemanticTagRaw("Theme_ItemHelp", buildRawTag(Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles))
	console.RegisterSemanticTagRaw("Theme_Program", buildRawTag(Current.ProgramFG, Current.ProgramBG, Current.ProgramStyles))
	console.RegisterSemanticTagRaw("Theme_ProgramBox", buildRawTag(Current.ProgramFG, Current.ProgramBG, Current.ProgramStyles))
	console.RegisterSemanticTagRaw("Theme_LogBox", buildRawTag(Current.LogBoxFG, Current.LogBoxBG, Current.LogBoxStyles))
	regComp("LogPanel", Current.LogPanelFG, Current.LogPanelBG)
	console.RegisterSemanticTagRaw("Theme_ProgressWaiting", buildRawTag(Current.ProgressWaitingFG, Current.ProgressWaitingBG, Current.ProgressWaitingStyles))
	console.RegisterSemanticTagRaw("Theme_ProgressInProgress", buildRawTag(Current.ProgressInProgressFG, Current.ProgressInProgressBG, Current.ProgressInProgressStyles))
	console.RegisterSemanticTagRaw("Theme_ProgressCompleted", buildRawTag(Current.ProgressCompletedFG, Current.ProgressCompletedBG, Current.ProgressCompletedStyles))
	console.RegisterSemanticTagRaw("Theme_VersionSelected", buildRawTag(Current.VersionSelectedFG, Current.VersionSelectedBG, Current.VersionSelectedStyles))
	console.RegisterSemanticTagRaw("Theme_ListApp", buildRawTag(Current.ListAppFG, Current.ListAppBG, Current.ListAppStyles))
	console.RegisterSemanticTagRaw("Theme_ListAppUserDefined", buildRawTag(Current.ListAppUserDefinedFG, Current.ListAppUserDefinedBG, Current.ListAppUserDefinedStyles))
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

// Default initializes the Current ThemeConfig with standard DockSTARTer colors (Classic)
// All colors are resolved through tcell to RGB/hex values for proper color profile support
func Default() {
	Current = ThemeConfig{}
	Apply()

	// Register basic theme fallbacks to prevent literal tags if theme files fail to load
	console.RegisterSemanticTagRaw("Theme_ApplicationName", "::B")
	console.RegisterSemanticTagRaw("Theme_ApplicationVersion", "-")
	console.RegisterSemanticTagRaw("Theme_ApplicationVersionBrackets", "-")
	console.RegisterSemanticTagRaw("Theme_ApplicationVersionSpace", console.StripDelimiters(console.ExpandTags(console.WrapSemantic("Theme_Screen"))))
	console.RegisterSemanticTagRaw("Theme_ApplicationFlags", "-")
	console.RegisterSemanticTagRaw("Theme_ApplicationFlagsBrackets", "-")
	console.RegisterSemanticTagRaw("Theme_ApplicationFlagsSpace", console.StripDelimiters(console.ExpandTags(console.WrapSemantic("Theme_Screen"))))
	console.RegisterSemanticTagRaw("Theme_ApplicationUpdate", "yellow::")
	console.RegisterSemanticTagRaw("Theme_ApplicationUpdateBrackets", "-")
	console.RegisterSemanticTagRaw("Theme_TitleQuestion", "black:yellow:B")
	console.RegisterSemanticTagRaw("Theme_Hostname", "::B")

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

func parseThemeTOML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return err
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
	resolvedValues := make(map[string]string)
	visiting := make(map[string]bool)

	// Maintains consistent registration/mapping logic from INI version
	for key, raw := range rawValues {
		styleValue, err := resolveThemeValue(raw, rawValues, visiting, semPre, semSuf, dirPre, dirSuf)
		if err != nil {
			// Fallback to raw expansion for robustness
			styleValue = console.StripDelimiters(console.ExpandTags(raw))
		}

		resolvedValues[key] = styleValue
		// Register using raw value (no delimiters)
		console.RegisterSemanticTagRaw("Theme_"+key, styleValue)

		// Map known keys to Current struct fields
		fg, bg := parseRawToColor(styleValue)
		switch key {
		case "Screen":
			Current.ScreenFG, Current.ScreenBG = fg, bg
		case "Dialog":
			Current.DialogFG, Current.DialogBG = fg, bg
		case "Border":
			Current.BorderFG, Current.BorderBG = fg, bg
		case "Border2":
			Current.Border2FG, Current.Border2BG = fg, bg
		case "Title":
			Current.TitleFG, Current.TitleBG, Current.TitleStyles = parseRawWithStyles(styleValue)
		case "TitleHelp":
			Current.TitleHelpFG, Current.TitleHelpBG, Current.TitleHelpStyles = parseRawWithStyles(styleValue)
		case "BoxTitle":
			// Only set if Title wasn't explicitly provided in theme
			if _, titleExists := rawValues["Title"]; !titleExists {
				Current.TitleFG, Current.TitleBG, Current.TitleStyles = parseRawWithStyles(styleValue)
			}
		case "Shadow":
			Current.ShadowColor = fg
		case "ButtonActive":
			fg, bg, styles := parseRawWithStyles(styleValue)
			Current.ButtonActiveFG, Current.ButtonActiveBG = fg, bg
			Current.ButtonActiveStyles = styles
		case "ButtonInactive":
			fg, bg, styles := parseRawWithStyles(styleValue)
			Current.ButtonInactiveFG, Current.ButtonInactiveBG = fg, bg
			Current.ButtonInactiveStyles = styles
		case "ItemSelected":
			Current.ItemSelectedFG, Current.ItemSelectedBG, Current.ItemSelectedStyles = parseRawWithStyles(styleValue)
		case "Item":
			Current.ItemFG, Current.ItemBG, Current.ItemStyles = parseRawWithStyles(styleValue)
		case "Tag":
			Current.TagFG, Current.TagBG, Current.TagStyles = parseRawWithStyles(styleValue)
		case "TagSelected":
			Current.TagSelectedFG, Current.TagSelectedBG, Current.TagSelectedStyles = parseRawWithStyles(styleValue)
		case "TagKey":
			Current.TagKeyFG, Current.TagKeyBG, Current.TagKeyStyles = parseRawWithStyles(styleValue)
		case "TagKeySelected":
			Current.TagKeySelectedFG, Current.TagKeySelectedBG, Current.TagKeySelectedStyles = parseRawWithStyles(styleValue)
		case "Helpline", "ItemHelp", "itemhelp_color":
			Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles = parseRawWithStyles(styleValue)
		case "Program", "ProgramBox":
			Current.ProgramFG, Current.ProgramBG, Current.ProgramStyles = parseRawWithStyles(styleValue)
		case "LogBox":
			Current.LogBoxFG, Current.LogBoxBG, Current.LogBoxStyles = parseRawWithStyles(styleValue)
		case "LogPanel":
			Current.LogPanelFG, Current.LogPanelBG = fg, bg
		case "Header":
			Current.HeaderTag = styleValue
		case "ProgressWaiting":
			Current.ProgressWaitingFG, Current.ProgressWaitingBG, Current.ProgressWaitingStyles = parseRawWithStyles(styleValue)
		case "ProgressInProgress":
			Current.ProgressInProgressFG, Current.ProgressInProgressBG, Current.ProgressInProgressStyles = parseRawWithStyles(styleValue)
		case "ProgressCompleted":
			Current.ProgressCompletedFG, Current.ProgressCompletedBG, Current.ProgressCompletedStyles = parseRawWithStyles(styleValue)
		case "VersionSelected":
			Current.VersionSelectedFG, Current.VersionSelectedBG, Current.VersionSelectedStyles = parseRawWithStyles(styleValue)
		case "ListApp":
			Current.ListAppFG, Current.ListAppBG, Current.ListAppStyles = parseRawWithStyles(styleValue)
		case "ListAppUserDefined":
			Current.ListAppUserDefinedFG, Current.ListAppUserDefinedBG, Current.ListAppUserDefinedStyles = parseRawWithStyles(styleValue)
		}
	}

	// 2. Re-apply tags based on updated Current
	console.RegisterBaseTags()
	console.BuildColorMap()

	return nil
}
