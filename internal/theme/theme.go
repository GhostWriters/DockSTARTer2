package theme

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	TitleBold            bool
	TitleUnderline       bool
	ShadowColor          lipgloss.TerminalColor
	ButtonActiveFG       lipgloss.TerminalColor
	ButtonActiveBG       lipgloss.TerminalColor
	ButtonActiveStyles   StyleFlags
	ButtonInactiveFG     lipgloss.TerminalColor
	ButtonInactiveBG     lipgloss.TerminalColor
	ButtonInactiveStyles StyleFlags
	ItemSelectedFG       lipgloss.TerminalColor
	ItemSelectedBG       lipgloss.TerminalColor
	ItemFG               lipgloss.TerminalColor
	ItemBG               lipgloss.TerminalColor
	TagFG                lipgloss.TerminalColor
	TagBG                lipgloss.TerminalColor
	TagKeyFG             lipgloss.TerminalColor
	TagKeySelectedFG     lipgloss.TerminalColor
	HelplineFG           lipgloss.TerminalColor
	HelplineBG           lipgloss.TerminalColor
	ProgramFG            lipgloss.TerminalColor
	ProgramBG            lipgloss.TerminalColor
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
	_ = parseThemeINI(themePath)

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
		console.RegisterSemanticTag("ThemeReset", "{{|"+bgStr+":"+bgStr+"|}}")
	} else {
		// Normal: Set FG/BG normally.
		console.RegisterSemanticTag("ThemeReset", "{{|"+fgStr+":"+bgStr+"|}}")
	}
}

func updateTagsFromCurrent() {
	regComp := func(name string, fg, bg lipgloss.TerminalColor) {
		fgName := console.GetColorStr(fg)
		bgName := console.GetColorStr(bg)
		tag := "{{|" + fgName + ":" + bgName + "|}}"
		console.RegisterSemanticTag("Theme"+name, tag)
	}

	regComp("Screen", Current.ScreenFG, Current.ScreenBG)
	regComp("Dialog", Current.DialogFG, Current.DialogBG)
	regComp("Border", Current.BorderFG, Current.BorderBG)
	regComp("Border2", Current.Border2FG, Current.Border2BG)

	// Note: Title here refers to the mapped _ThemeTitle_ tag, which is usually the Main Menu text.
	// If theme.ini overrides it with a "Title" key, that will take precedence.
	regComp("Title", Current.TitleFG, Current.TitleBG)

	regComp("ButtonActive", Current.ButtonActiveFG, Current.ButtonActiveBG)
	regComp("ButtonInactive", Current.ButtonInactiveFG, Current.ButtonInactiveBG)
	regComp("ItemSelected", Current.ItemSelectedFG, Current.ItemSelectedBG)
	regComp("Item", Current.ItemFG, Current.ItemBG)
	regComp("Tag", Current.TagFG, Current.TagBG)

	console.RegisterSemanticTag("ThemeTagKey", "{{|"+console.GetColorStr(Current.TagKeyFG)+"|}}")
	console.RegisterSemanticTag("ThemeTagKeySelected", "{{|"+console.GetColorStr(Current.TagKeySelectedFG)+"|}}")
	console.RegisterSemanticTag("ThemeShadow", "{{|"+console.GetColorStr(Current.ShadowColor)+"|}}")

	regComp("Helpline", Current.HelplineFG, Current.HelplineBG)
	regComp("ItemHelp", Current.HelplineFG, Current.HelplineBG)
	regComp("Program", Current.ProgramFG, Current.ProgramBG)
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
		TagBG:            parseColor("blue"),
		TagKeyFG:         parseColor("red"),
		TagKeySelectedFG: parseColor("black"),
		HelplineFG:       parseColor("black"),
		HelplineBG:       parseColor("cyan"),
		ProgramFG:        parseColor("bright-white"),
		ProgramBG:        parseColor("black"),
	}
	Apply()

	// Register basic theme fallbacks to prevent literal tags if theme files fail to load
	console.RegisterSemanticTag("ThemeApplicationName", "{{|::B|}}")
	console.RegisterSemanticTag("ThemeApplicationVersion", "{{|-|}}")
	console.RegisterSemanticTag("ThemeApplicationVersionBrackets", "{{|-|}}")
	console.RegisterSemanticTag("ThemeApplicationVersionSpace", console.ExpandTags("{{_ThemeScreen_}}")+" ")
	console.RegisterSemanticTag("ThemeApplicationFlags", "{{|-|}}")
	console.RegisterSemanticTag("ThemeApplicationFlagsBrackets", "{{|-|}}")
	console.RegisterSemanticTag("ThemeApplicationFlagsSpace", console.ExpandTags("{{_ThemeScreen_}}")+" ")
	console.RegisterSemanticTag("ThemeApplicationUpdate", "{{|yellow|}}")
	console.RegisterSemanticTag("ThemeApplicationUpdateBrackets", "{{|-|}}")
	console.RegisterSemanticTag("ThemeHostname", "{{|::B|}}")

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
		flags := strings.ToLower(parts[2])
		styles.Bold = strings.Contains(flags, "b")
		styles.Underline = strings.Contains(flags, "u")
		styles.Italic = strings.Contains(flags, "i")
		styles.Blink = strings.Contains(flags, "l")
		styles.Dim = strings.Contains(flags, "d")
		styles.Reverse = strings.Contains(flags, "r")
		styles.Strikethrough = strings.Contains(flags, "s")
		styles.HighIntensity = strings.Contains(flags, "h")
	}
	return
}

func parseThemeINI(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Define Bash variable expansion to map to cview-style tags (Legacy support mostly removed)
	replacer := strings.NewReplacer(
		"${1}", version.ApplicationName,
	)

	// Track whether Title was explicitly set (for BoxTitle fallback)
	titleWasSet := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")

		// Expand app name if present
		expanded := replacer.Replace(val)

		// Convert {{|code|}} format to tview [code] format
		styleValue := console.ExpandTags(expanded)

		// Register "Theme"+Key exclusively for themed tags (prevents collision with system semantic tags)
		console.RegisterSemanticTag("Theme"+key, styleValue)

		// 2. Map known keys to Current struct fields
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
		case "Title": // Menu title with style flags (underline, bold, etc.)
			var styles StyleFlags
			fg, bg, styles = parseTagWithStyles(styleValue)
			Current.TitleFG, Current.TitleBG = fg, bg
			Current.TitleBold, Current.TitleUnderline = styles.Bold, styles.Underline
			titleWasSet = true
		case "BoxTitle": // Fallback from .dialogrc (no styles)
			// Only set if Title wasn't explicitly provided in theme
			if !titleWasSet {
				Current.TitleFG, Current.TitleBG = fg, bg
				// BoxTitle doesn't have style flags, leave Bold/Underline as false
			}
		case "Shadow":
			// Shadow is usually just BG
			// But tag might be [black:black:b]
			// We take the BG? Or FG? Usually same.
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
			Current.ItemSelectedFG, Current.ItemSelectedBG = fg, bg
		case "Item":
			Current.ItemFG, Current.ItemBG = fg, bg
		case "Tag":
			Current.TagFG, Current.TagBG = fg, bg
		case "TagKey":
			Current.TagKeyFG = fg
		case "TagKeySelected":
			Current.TagKeySelectedFG = fg
		case "Helpline", "ItemHelp", "itemhelp_color":
			Current.HelplineFG, Current.HelplineBG = fg, bg
		case "Program":
			Current.ProgramFG, Current.ProgramBG = fg, bg
		}
	}

	// 3. Re-apply tags based on updated Current
	// Note: We do NOT call updateTagsFromCurrent() because we want to keep the specific tags registered above.
	// We MUST ensure base tags and color map are built before we finalize mappings.
	console.RegisterBaseTags()
	console.BuildColorMap()

	bracketsDef := console.GetColorDefinition("ThemeApplicationVersionBrackets")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")
	bgStr := console.GetColorStr(Current.ScreenBG)
	fgStr := console.GetColorStr(Current.ScreenFG)

	if isReversed {
		console.RegisterSemanticTag("ThemeReset", "{{|"+bgStr+":"+bgStr+"|}}")
	} else {
		console.RegisterSemanticTag("ThemeReset", "{{|"+fgStr+":"+bgStr+"|}}")
	}

	return nil
}
