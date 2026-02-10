package theme

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// ThemeConfig holds colors derived from .dialogrc and theme.ini
type ThemeConfig struct {
	ScreenFG             lipgloss.TerminalColor
	ScreenBG             lipgloss.TerminalColor
	DialogFG             lipgloss.TerminalColor
	DialogBG             lipgloss.TerminalColor
	BorderFG             lipgloss.TerminalColor
	BorderBG             lipgloss.TerminalColor
	Border2FG            lipgloss.TerminalColor
	Border2BG            lipgloss.TerminalColor
	TitleFG              lipgloss.TerminalColor
	TitleBG              lipgloss.TerminalColor
	TitleStyles          StyleFlags
	TitleHelpFG          lipgloss.TerminalColor
	TitleHelpBG          lipgloss.TerminalColor
	TitleHelpStyles      StyleFlags
	ShadowColor          lipgloss.TerminalColor
	ButtonActiveFG       lipgloss.TerminalColor
	ButtonActiveBG       lipgloss.TerminalColor
	ButtonActiveStyles   StyleFlags
	ButtonInactiveFG     lipgloss.TerminalColor
	ButtonInactiveBG     lipgloss.TerminalColor
	ButtonInactiveStyles StyleFlags
	ItemSelectedFG       lipgloss.TerminalColor
	ItemSelectedBG       lipgloss.TerminalColor
	ItemSelectedStyles   StyleFlags
	ItemFG               lipgloss.TerminalColor
	ItemBG               lipgloss.TerminalColor
	ItemStyles           StyleFlags
	TagFG                lipgloss.TerminalColor
	TagBG                lipgloss.TerminalColor
	TagStyles            StyleFlags
	TagSelectedFG        lipgloss.TerminalColor
	TagSelectedBG        lipgloss.TerminalColor
	TagSelectedStyles    StyleFlags
	TagKeyFG             lipgloss.TerminalColor
	TagKeyBG             lipgloss.TerminalColor
	TagKeyStyles         StyleFlags
	TagKeySelectedFG     lipgloss.TerminalColor
	TagKeySelectedBG     lipgloss.TerminalColor
	TagKeySelectedStyles StyleFlags
	HelplineFG           lipgloss.TerminalColor
	HelplineBG           lipgloss.TerminalColor
	HelplineStyles       StyleFlags
	ProgramFG            lipgloss.TerminalColor
	ProgramBG            lipgloss.TerminalColor
	ProgramStyles        StyleFlags
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
		// If theme doesn't exist, we just stay with defaults
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
	// This maps the struct fields (colors) to tags like {{_ThemeScreen_}}
	updateTagsFromCurrent()

	// 2. Update global cview styles to match theme globals

	// Register ThemeReset AFTER all other tags are loaded
	// We check if "VersionBrackets" uses Reverse. If so, we must invert our colors
	// because the Reverse flag is sticky and we cannot use {{_-_}} to clear it (causes black flash).
	bracketsDef := console.GetColorDefinition("ThemeApplicationVersionBrackets")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")

	bgStr := console.GetColorStr(Current.ScreenBG)
	fgStr := console.GetColorStr(Current.ScreenFG)

	if isReversed {
		// Reverse is active: Swapping FG/BG will result in Correct orientation after rendering
		// BUT we must ensure the logical BACKGROUND is the ScreenBG so that subsequent tags (like "T:")
		// inherit the stable background.
		console.RegisterSemanticTag("Theme_Reset", "{{|"+bgStr+":"+bgStr+"|}}")
	} else {
		// Normal: Set FG/BG normally.
		console.RegisterSemanticTag("Theme_Reset", "{{|"+fgStr+":"+bgStr+"|}}")
	}
}

func updateTagsFromCurrent() {
	regComp := func(name string, fg, bg lipgloss.TerminalColor) {
		fgName := console.GetColorStr(fg)
		bgName := console.GetColorStr(bg)
		tag := "{{|" + fgName + ":" + bgName + "|}}"
		console.RegisterSemanticTag("Theme_"+name, tag)
	}

	regComp("Screen", Current.ScreenFG, Current.ScreenBG)
	regComp("Dialog", Current.DialogFG, Current.DialogBG)
	regComp("Border", Current.BorderFG, Current.BorderBG)
	regComp("Border2", Current.Border2FG, Current.Border2BG)

	console.RegisterSemanticTag("Theme_Title", buildTag(Current.TitleFG, Current.TitleBG, Current.TitleStyles))
	console.RegisterSemanticTag("Theme_TitleHelp", buildTag(Current.TitleHelpFG, Current.TitleHelpBG, Current.TitleHelpStyles))

	regComp("ButtonActive", Current.ButtonActiveFG, Current.ButtonActiveBG)
	regComp("ItemSelected", Current.ItemSelectedFG, Current.ItemSelectedBG)
	regComp("Item", Current.ItemFG, Current.ItemBG)
	regComp("Tag", Current.TagFG, Current.TagBG)
	regComp("TagSelected", Current.TagSelectedFG, Current.TagSelectedBG)

	console.RegisterSemanticTag("Theme_TagKey", buildTag(Current.TagKeyFG, Current.TagKeyBG, Current.TagKeyStyles))
	console.RegisterSemanticTag("Theme_TagKeySelected", buildTag(Current.TagKeySelectedFG, Current.TagKeySelectedBG, Current.TagKeySelectedStyles))
	console.RegisterSemanticTag("Theme_Shadow", "{{|"+console.GetColorStr(Current.ShadowColor)+"|}}")

	console.RegisterSemanticTag("Theme_Helpline", buildTag(Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles))
	console.RegisterSemanticTag("Theme_ItemHelp", buildTag(Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles))
	console.RegisterSemanticTag("Theme_Program", buildTag(Current.ProgramFG, Current.ProgramBG, Current.ProgramStyles))
}

// buildTag constructs a {{|fg:bg:flags|}} string
func buildTag(fg, bg lipgloss.TerminalColor, styles StyleFlags) string {
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
	return "{{|" + fgStr + ":" + bgStr + ":" + flags + "|}}"
}

// Default initializes the Current ThemeConfig with standard DockSTARTer colors (Classic)
// All colors are resolved through tcell to RGB/hex values for proper color profile support
func Default() {
	Current = ThemeConfig{
		ScreenFG:         parseColor("black"),
		ScreenBG:         parseColor("silver"),
		DialogFG:         parseColor("black"),
		DialogBG:         parseColor("cyan"),
		BorderFG:         parseColor("bright-white"),
		BorderBG:         parseColor("cyan"),
		Border2FG:        parseColor("black"),
		Border2BG:        parseColor("cyan"),
		TitleFG:          parseColor("black"),
		TitleBG:          parseColor("cyan"),
		TitleHelpFG:      parseColor("black"),
		TitleHelpBG:      parseColor("cyan"),
		ShadowColor:      parseColor("black"),
		ButtonActiveFG:   parseColor("bright-white"),
		ButtonActiveBG:   parseColor("red"),
		ButtonInactiveFG: parseColor("black"),
		ButtonInactiveBG: parseColor("cyan"),
		ItemSelectedFG:   parseColor("black"),
		ItemSelectedBG:   parseColor("red"),
		ItemFG:           parseColor("black"),
		ItemBG:           parseColor("cyan"),
		TagFG:            parseColor("black"),
		TagBG:            parseColor("cyan"),
		TagSelectedFG:    parseColor("black"),
		TagSelectedBG:    parseColor("red"),
		TagKeyFG:         parseColor("red"),
		TagKeyBG:         parseColor("cyan"),
		TagKeySelectedFG: parseColor("black"),
		TagKeySelectedBG: parseColor("red"),
		HelplineFG:       parseColor("black"),
		HelplineBG:       parseColor("cyan"),
		ProgramFG:        parseColor("bright-white"),
		ProgramBG:        parseColor("black"),
	}
	Apply()

	// Register basic theme fallbacks to prevent literal tags if theme files fail to load
	console.RegisterSemanticTag("Theme_ApplicationName", "{{|::B|}}")
	console.RegisterSemanticTag("Theme_ApplicationVersion", "{{|-|}}")
	console.RegisterSemanticTag("Theme_ApplicationVersionBrackets", "{{|-|}}")
	console.RegisterSemanticTag("Theme_ApplicationVersionSpace", console.ExpandTags("{{_Theme_Screen_}}")+" ")
	console.RegisterSemanticTag("Theme_ApplicationFlags", "{{|-|}}")
	console.RegisterSemanticTag("Theme_ApplicationFlagsBrackets", "{{|-|}}")
	console.RegisterSemanticTag("Theme_ApplicationFlagsSpace", console.ExpandTags("{{_Theme_Screen_}}")+" ")
	console.RegisterSemanticTag("Theme_ApplicationUpdate", "{{|yellow|}}")
	console.RegisterSemanticTag("Theme_ApplicationUpdateBrackets", "{{|-|}}")
	console.RegisterSemanticTag("Theme_Hostname", "{{|::B|}}")

}

// parseColor converts a color name or hex string to a lipgloss.TerminalColor.
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
func parseColor(c string) lipgloss.TerminalColor {
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

func parseTagToColor(tag string) (fg, bg lipgloss.TerminalColor) {
	tag = strings.TrimPrefix(tag, "{{|")
	tag = strings.TrimSuffix(tag, "|}}")
	// Also support legacy brackets for robustness during transition
	tag = strings.Trim(tag, "[]")

	parts := strings.Split(tag, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	}
	return
}

// parseTagWithStyles parses a theme tag and extracts colors and style flags
func parseTagWithStyles(tag string) (fg, bg lipgloss.TerminalColor, styles StyleFlags) {
	tag = strings.TrimPrefix(tag, "{{|")
	tag = strings.TrimSuffix(tag, "|}}")
	tag = strings.Trim(tag, "[]")

	parts := strings.Split(tag, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	}
	// Parse style flags (third part and beyond)
	if len(parts) > 2 {
		flags := parts[2]
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
// It supports chaining (A->B->C) and partial overlays (Base + {{|:green|}}).
func resolveThemeValue(raw string, rawValues map[string]string, visiting map[string]bool) (string, error) {
	// 1. Expand standard semantic tags first (e.g. {{_Notice_}})
	// This is standard single-pass expansion for global system tags.
	// But for THEME tags ({{_ThemeXXX_}}), we need to look them up in our map if possible,
	// because they might be defined LATER in the file (forward reference)
	// or we want the RAW value to overlay on.

	// Actually, console.ExpandTags does a lookup in console.semanticMap.
	// We haven't registered the future theme keys yet!
	// So we must manually parse {{_ThemeX_}} tags here.

	// var resultParts []string // Not used, removing.

	// Simple state machine to parse the string
	// We are looking for {{_ThemeX_}} or {{_X_}} tags.
	// And {{|...|}} style tags.

	// For simplicity, let's use the existing console expansion logic BUT
	// we intercept the unknown tags or specifically look for Theme tags.

	// Hack: We can regex for {{_..._}}
	// But we need to handle "Base" + "Overlay" composition.

	// Strategy:
	// 1. Find all {{_Tag_}} references.
	// 2. Resolve them recursively.
	// 3. Flatten the result into a sequence of style components.
	// 4. Merge them into a single final {{|fg:bg:flags|}} string.

	// Step 1: Split into tokens (text, reference, style)
	// This is complex to write from scratch efficiently.
	// Let's assume the input is mostly "Reference" + "Style".
	// e.g. "{{_ThemeTitle_}}{{|:green|}}"

	// Let's define a composed style.
	var finalFG, finalBG string
	var finalFlags string

	// Helper to merge a style string "ShowMe" logic
	mergeStyle := func(styleStr string) {
		// remove wrappers
		inner := strings.TrimSuffix(strings.TrimPrefix(styleStr, "{{|"), "|}}")
		parts := strings.Split(inner, ":")

		if len(parts) > 0 && parts[0] != "" {
			finalFG = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			finalBG = parts[1]
		}
		if len(parts) > 2 {
			// Merge flags: Upper sets ON, lower sets OFF
			for _, f := range parts[2] {
				flag := string(f)
				// If specific flag (case sensitive)
				// Overwrite existing state for that letter (upper or lower)
				// Actually, we can just append to a "history" and let the final consumer resolve?
				// But we want to produce a CLEAN string like "B" or "BU".

				// If we append, "b" (bold off) after "B" (bold on).
				// Our styles logic handles this! "b" turns off bold.
				// So we can just concatenate flags!
				finalFlags += flag
			}
		}
	}

	// We iterate through the string seeking {{...}}
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

		if strings.HasPrefix(tag, "{{|") {
			// Direct style tag
			mergeStyle(tag)
		} else if strings.HasPrefix(tag, "{{_") {
			// Semantic reference
			refKey := strings.TrimSuffix(strings.TrimPrefix(tag, "{{_"), "_}}")

			// Check if it's a theme key reference (Theme_...)
			if strings.HasPrefix(refKey, "Theme_") {
				targetKey := strings.TrimPrefix(refKey, "Theme_")

				// Resolve it recursively
				resolvedRef, err := resolveThemeValue(rawValues[targetKey], rawValues, visiting)
				if err == nil {
					mergeStyle(resolvedRef)
				} else {
					// referencing non-existent internal key?
					// Maybe it's a global semantic tag (like {{_Notice_}})?
					// Try console lookup by expanding it.
					expanded := console.ExpandTags(tag)
					if expanded != tag && expanded != "" {
						mergeStyle(expanded)
					}
				}
			} else {
				// Regular semantic tag (e.g. {{_Notice_}})
				expanded := console.ExpandTags(tag)
				if expanded != tag && expanded != "" {
					mergeStyle(expanded)
				}
			}
		}

		cur = cur[end:]
	}

	// Construct final string
	// Clean flags? We could normalize (remove B then b).
	// But letting the renderer handle it is robust.
	// Actually, let's just output components.

	// Default empty colors to nothing (inherit? No, theme values are absolute usually)
	// But for "partial" overlays, they rely on the base.

	return fmt.Sprintf("{{|%s:%s:%s|}}", finalFG, finalBG, finalFlags), nil
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

	// 1. Resolve values and register/apply them
	// We need to resolve references (e.g., TitleSuccess -> Title) before parsing colors
	rawValues := tf.Colors
	resolvedValues := make(map[string]string)
	visiting := make(map[string]bool)

	// Maintains consistent registration/mapping logic from INI version
	for key, raw := range rawValues {
		styleValue, err := resolveThemeValue(raw, rawValues, visiting)
		if err != nil {
			// Fallback to raw expansion for robustness
			styleValue = console.ExpandTags(raw)
		}

		resolvedValues[key] = styleValue
		console.RegisterSemanticTag("Theme_"+key, styleValue)

		// Map known keys to Current struct fields
		fg, bg := parseTagToColor(styleValue)
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
			Current.TitleFG, Current.TitleBG, Current.TitleStyles = parseTagWithStyles(styleValue)
		case "TitleHelp":
			Current.TitleHelpFG, Current.TitleHelpBG, Current.TitleHelpStyles = parseTagWithStyles(styleValue)
		case "BoxTitle":
			// Only set if Title wasn't explicitly provided in theme
			if _, titleExists := rawValues["Title"]; !titleExists {
				Current.TitleFG, Current.TitleBG, Current.TitleStyles = parseTagWithStyles(styleValue)
			}
		case "Shadow":
			Current.ShadowColor = fg
		case "ButtonActive":
			fg, bg, styles := parseTagWithStyles(styleValue)
			Current.ButtonActiveFG, Current.ButtonActiveBG = fg, bg
			Current.ButtonActiveStyles = styles
		case "ButtonInactive":
			fg, bg, styles := parseTagWithStyles(styleValue)
			Current.ButtonInactiveFG, Current.ButtonInactiveBG = fg, bg
			Current.ButtonInactiveStyles = styles
		case "ItemSelected":
			Current.ItemSelectedFG, Current.ItemSelectedBG, Current.ItemSelectedStyles = parseTagWithStyles(styleValue)
		case "Item":
			Current.ItemFG, Current.ItemBG, Current.ItemStyles = parseTagWithStyles(styleValue)
		case "Tag":
			Current.TagFG, Current.TagBG, Current.TagStyles = parseTagWithStyles(styleValue)
		case "TagSelected":
			Current.TagSelectedFG, Current.TagSelectedBG, Current.TagSelectedStyles = parseTagWithStyles(styleValue)
		case "TagKey":
			Current.TagKeyFG, Current.TagKeyBG, Current.TagKeyStyles = parseTagWithStyles(styleValue)
		case "TagKeySelected":
			Current.TagKeySelectedFG, Current.TagKeySelectedBG, Current.TagKeySelectedStyles = parseTagWithStyles(styleValue)
		case "Helpline", "ItemHelp", "itemhelp_color":
			Current.HelplineFG, Current.HelplineBG, Current.HelplineStyles = parseTagWithStyles(styleValue)
		case "Program":
			Current.ProgramFG, Current.ProgramBG, Current.ProgramStyles = parseTagWithStyles(styleValue)
		}
	}

	// 2. Re-apply tags based on updated Current
	console.RegisterBaseTags()
	console.BuildColorMap()

	bracketsDef := console.GetColorDefinition("ThemeApplicationVersionBrackets")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")
	bgStr := console.GetColorStr(Current.ScreenBG)
	fgStr := console.GetColorStr(Current.ScreenFG)

	if isReversed {
		console.RegisterSemanticTag("Theme_Reset", "{{|"+bgStr+":"+bgStr+"|}}")
	} else {
		console.RegisterSemanticTag("Theme_Reset", "{{|"+fgStr+":"+bgStr+"|}}")
	}

	return nil
}
