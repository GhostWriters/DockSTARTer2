package theme

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"codeberg.org/tslocum/cview"
	"github.com/gdamore/tcell/v3"
)

// GetColorStr is moved to console package for reuse

func GetColorStr(c tcell.Color) string {
	if hex, ok := console.ColorToHexMap[strings.ToLower(c.Name())]; ok {
		return hex
	}
	name := strings.ToLower(c.Name())
	if name == "" {
		return fmt.Sprintf("#%06x", c.Hex())
	}
	return name
}

// Mapping from theme.ini keys to console.AppColors field names
var themeToConsoleMap = map[string]string{
	"ApplicationName":            "ApplicationName",
	"ApplicationVersion":         "Version",
	"ApplicationVersionBrackets": "VersionBrackets",
	"ApplicationVersionSpace":    "VersionSpace",
	"ApplicationFlagsBrackets":   "FlagsBrackets",
	"ApplicationFlagsSpace":      "FlagsSpace",
	"ApplicationUpdate":          "Update",
	"ApplicationUpdateBrackets":  "UpdateBrackets",
	"Hostname":                   "Hostname",
	"Title":                      "Theme",
	"Heading":                    "Info",
	"Highlight":                  "UserCommand",
	"LineComment":                "Trace",
	"Var":                        "Var",
}

// StyleFlags holds ANSI style modifiers
type StyleFlags struct {
	Bold          bool
	Underline     bool
	Italic        bool
	Blink         bool
	Dim           bool
	Reverse       bool
	Strikethrough bool
}

// ThemeConfig holds colors derived from .dialogrc and theme.ini
type ThemeConfig struct {
	ScreenFG             tcell.Color
	ScreenBG             tcell.Color
	DialogFG             tcell.Color
	DialogBG             tcell.Color
	BorderFG             tcell.Color
	BorderBG             tcell.Color
	Border2FG            tcell.Color
	Border2BG            tcell.Color
	TitleFG              tcell.Color
	TitleBG              tcell.Color
	TitleBold            bool
	TitleUnderline       bool
	ShadowColor          tcell.Color
	ButtonActiveFG       tcell.Color
	ButtonActiveBG       tcell.Color
	ButtonActiveStyles   StyleFlags
	ButtonInactiveFG     tcell.Color
	ButtonInactiveBG     tcell.Color
	ButtonInactiveStyles StyleFlags
	ItemSelectedFG       tcell.Color
	ItemSelectedBG       tcell.Color
	ItemFG               tcell.Color
	ItemBG               tcell.Color
	TagFG                tcell.Color
	TagBG                tcell.Color
	TagKeyFG             tcell.Color
	TagKeySelectedFG     tcell.Color
	ItemHelpFG           tcell.Color
	ItemHelpBG           tcell.Color
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

	// 2. Map theme.ini fields to themed tags (NOT overwriting global console.Colors)
	// This ensures that when Translate("{{_Notice_}}") is called in TUI, it uses the theme color.
	for themeKey, consoleField := range themeToConsoleMap {
		themeTag := "{{_Theme" + themeKey + "_}}"
		// Eagerly resolve the theme tag to its actual cview tag
		resolved := console.Translate(themeTag)
		console.RegisterSemanticTag(consoleField, resolved)
	}

	// 3. Update global cview styles to match theme globals

	// Register ThemeReset AFTER all other tags are loaded
	// We check if "VersionBrackets" uses Reverse. If so, we must invert our colors
	// because the Reverse flag is sticky and we cannot use {{_-_}} to clear it (causes black flash).
	bracketsDef := console.GetColorDefinition("ThemeApplicationVersionBrackets")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")

	bgStr := GetColorStr(Current.ScreenBG)
	fgStr := GetColorStr(Current.ScreenFG)

	if isReversed {
		// Reverse is active: Swapping FG/BG will result in Correct orientation after rendering
		// BUT we must ensure the logical BACKGROUND is the ScreenBG so that subsequent tags (like "T:")
		// inherit the stable background.
		// Solution: Set BOTH to ScreenBG. Results in invisible text for the gap (Silver on Silver),
		// but ensures the Next tag sees "BG=Silver", so "Blue on Silver" reversed becomes "Silver on Blue".
		console.RegisterSemanticTag("ThemeReset", "["+bgStr+":"+bgStr+"]")
	} else {
		// Normal: Set FG/BG normally.
		console.RegisterSemanticTag("ThemeReset", "["+fgStr+":"+bgStr+"]")
	}

	// 3. Update global cview styles to match theme globals
	updateStyles()
}

func updateTagsFromCurrent() {
	regComp := func(name string, fg, bg tcell.Color) {
		fgName := GetColorStr(fg)
		bgName := GetColorStr(bg)
		tag := "[" + fgName + ":" + bgName + "]"
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

	console.RegisterSemanticTag("ThemeTagKey", "["+GetColorStr(Current.TagKeyFG)+"]")
	console.RegisterSemanticTag("ThemeTagKeySelected", "["+GetColorStr(Current.TagKeySelectedFG)+"]")
	console.RegisterSemanticTag("ThemeShadow", "["+GetColorStr(Current.ShadowColor)+"]")
}

func updateStyles() {
	cview.Styles.PrimitiveBackgroundColor = Current.ScreenBG
	cview.Styles.PrimaryTextColor = Current.ScreenFG
	cview.Styles.BorderColor = Current.BorderFG
	cview.Styles.TitleColor = Current.TitleFG
}

// Default initializes the Current ThemeConfig with standard DockSTARTer colors (Classic)
func Default() {
	Current = ThemeConfig{
		ScreenFG:         tcell.ColorBlack,
		ScreenBG:         tcell.ColorSilver, // Light background
		DialogFG:         tcell.ColorBlack,
		DialogBG:         tcell.ColorTeal,
		BorderFG:         tcell.ColorWhite,
		BorderBG:         tcell.ColorTeal,
		Border2FG:        tcell.ColorBlack, // Default contrast is black on teal
		Border2BG:        tcell.ColorTeal,
		TitleFG:          tcell.ColorBlack,
		TitleBG:          tcell.ColorTeal,
		ShadowColor:      tcell.ColorBlack,
		ButtonActiveFG:   tcell.ColorWhite,
		ButtonActiveBG:   tcell.ColorMaroon,
		ButtonInactiveFG: tcell.ColorBlack,
		ButtonInactiveBG: tcell.ColorTeal,
		ItemSelectedFG:   tcell.ColorBlack,
		ItemSelectedBG:   tcell.ColorMaroon,
		ItemFG:           tcell.ColorBlack,
		ItemBG:           tcell.ColorTeal,
		TagFG:            tcell.ColorBlack,
		TagBG:            tcell.ColorTeal,
		TagKeyFG:         tcell.ColorMaroon,
		TagKeySelectedFG: tcell.ColorBlack,
		ItemHelpFG:       tcell.ColorBlack,
		ItemHelpBG:       tcell.ColorTeal,
	}
	Apply()

	// Register basic theme fallbacks to prevent literal tags if theme files fail to load
	console.RegisterSemanticTag("ThemeApplicationName", "[::b]")
	console.RegisterSemanticTag("ThemeApplicationVersion", "[-]")
	console.RegisterSemanticTag("ThemeApplicationVersionBrackets", "[-]")
	console.RegisterSemanticTag("ThemeApplicationVersionSpace", console.ToTview("{{_ThemeScreen_}}")+" ")
	console.RegisterSemanticTag("ThemeApplicationFlags", "[-]")
	console.RegisterSemanticTag("ThemeApplicationFlagsBrackets", "[-]")
	console.RegisterSemanticTag("ThemeApplicationFlagsSpace", console.ToTview("{{_ThemeScreen_}}")+" ")
	console.RegisterSemanticTag("ThemeApplicationUpdate", "[yellow]")
	console.RegisterSemanticTag("ThemeApplicationUpdateBrackets", "[-]")
	console.RegisterSemanticTag("ThemeHostname", "[::b]")
}

func parseColor(c string) tcell.Color {
	c = strings.ToUpper(strings.TrimSpace(c))
	switch c {
	case "BLACK":
		return tcell.ColorBlack
	case "RED":
		return tcell.ColorMaroon
	case "GREEN":
		return tcell.ColorGreen
	case "YELLOW":
		return tcell.ColorOlive
	case "BLUE":
		return tcell.ColorNavy
	case "MAGENTA":
		return tcell.ColorPurple
	case "CYAN":
		return tcell.ColorTeal
	case "WHITE":
		return tcell.ColorWhite
	case "SILVER":
		return tcell.ColorSilver
	case "GRAY", "GREY":
		return tcell.ColorGray
	default:
		return tcell.ColorDefault
	}
}

func parseTagToColor(tag string) (fg, bg tcell.Color) {
	tag = strings.Trim(tag, "[]")
	parts := strings.Split(tag, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	} else {
		bg = tcell.ColorDefault
	}
	return
}

// parseTagWithStyles parses a theme tag and extracts colors and style flags
func parseTagWithStyles(tag string) (fg, bg tcell.Color, styles StyleFlags) {
	tag = strings.Trim(tag, "[]")
	parts := strings.Split(tag, ":")
	if len(parts) > 0 {
		fg = parseColor(parts[0])
	}
	if len(parts) > 1 {
		bg = parseColor(parts[1])
	} else {
		bg = tcell.ColorDefault
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
		tviewValue := console.ToTview(expanded)

		// 1. Register the tview-format value as the tag
		console.RegisterSemanticTag("Theme"+key, tviewValue)

		// 2. Update global console.Colors if this key is in the map
		if fieldName, ok := themeToConsoleMap[key]; ok {
			// Update global console.Colors
			f := reflect.ValueOf(&console.Colors).Elem().FieldByName(fieldName)
			if f.IsValid() && f.CanSet() {
				f.SetString(tviewValue)
			}
		}

		// 3. Map known keys to Current struct fields
		fg, bg := parseTagToColor(tviewValue)
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
			fg, bg, styles = parseTagWithStyles(tviewValue)
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
			fg, bg, styles := parseTagWithStyles(tviewValue)
			Current.ButtonActiveFG, Current.ButtonActiveBG = fg, bg
			Current.ButtonActiveStyles = styles
		case "ButtonInactive":
			fg, bg, styles := parseTagWithStyles(tviewValue)
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
		case "ItemHelp":
			Current.ItemHelpFG, Current.ItemHelpBG = fg, bg
		}
	}

	// 3. Re-apply styles and tags based on updated Current
	// Note: We do NOT call updateTagsFromCurrent() because we want to keep the specific tags registered above.
	// We MUST ensure base tags and color map are built before we finalize mappings.
	console.RegisterBaseTags()
	console.BuildColorMap()

	// Map theme.ini fields to themed tags
	for themeKey, consoleField := range themeToConsoleMap {
		themeTag := "{{_Theme" + themeKey + "_}}"
		resolved := console.Translate(themeTag)
		console.RegisterSemanticTag(consoleField, resolved)
	}

	// Update global cview styles.
	updateStyles()

	bracketsDef := console.GetColorDefinition("ThemeApplicationVersionBrackets")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")
	bgStr := GetColorStr(Current.ScreenBG)
	fgStr := GetColorStr(Current.ScreenFG)

	if isReversed {
		console.RegisterSemanticTag("ThemeReset", "["+bgStr+":"+bgStr+"]")
	} else {
		console.RegisterSemanticTag("ThemeReset", "["+fgStr+":"+bgStr+"]")
	}

	return nil
}
