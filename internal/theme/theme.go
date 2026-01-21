package theme

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/tslocum/cview"
	"github.com/gdamore/tcell/v3"
)

// colorToHexMap maps tcell colors to cview-safe hex strings
var colorToHexMap = map[tcell.Color]string{
	tcell.ColorBlack:   "#000000",
	tcell.ColorMaroon:  "#800000",
	tcell.ColorGreen:   "#008000",
	tcell.ColorOlive:   "#808000",
	tcell.ColorNavy:    "#000080",
	tcell.ColorPurple:  "#800080",
	tcell.ColorTeal:    "#008080",
	tcell.ColorSilver:  "#c0c0c0",
	tcell.ColorGray:    "#808080",
	tcell.ColorRed:     "#ff0000",
	tcell.ColorLime:    "#00ff00",
	tcell.ColorYellow:  "#ffff00",
	tcell.ColorBlue:    "#0000ff",
	tcell.ColorFuchsia: "#ff00ff",
	tcell.ColorAqua:    "#00ffff",
	tcell.ColorWhite:   "#ffffff",
}

func GetColorStr(c tcell.Color) string {
	if hex, ok := colorToHexMap[c]; ok {
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
	"TitleSuccess":               "Notice",
	"TitleError":                 "Error",
	"TitleWarning":               "Warn",
	"Heading":                    "Info",
	"Highlight":                  "UserCommand",
	"LineComment":                "Trace",
	"CommandLine":                "RunningCommand",
	"Var":                        "Var",
}

// ThemeConfig holds colors derived from .dialogrc and theme.ini
type ThemeConfig struct {
	ScreenFG         tcell.Color
	ScreenBG         tcell.Color
	DialogFG         tcell.Color
	DialogBG         tcell.Color
	BorderFG         tcell.Color
	BorderBG         tcell.Color
	Border2FG        tcell.Color
	Border2BG        tcell.Color
	TitleFG          tcell.Color
	TitleBG          tcell.Color
	ShadowColor      tcell.Color
	ButtonActiveFG   tcell.Color
	ButtonActiveBG   tcell.Color
	ButtonInactiveFG tcell.Color
	ButtonInactiveBG tcell.Color
	ItemSelectedFG   tcell.Color
	ItemSelectedBG   tcell.Color
	ItemFG           tcell.Color
	ItemBG           tcell.Color
	TagFG            tcell.Color
	TagBG            tcell.Color
	TagKeyFG         tcell.Color
	TagKeySelectedFG tcell.Color
}

// Current holds the active theme configuration
var Current ThemeConfig

// Load theme by name
func Load(themeName string) error {
	// Initialize with defaults first
	Default()

	themesDir := paths.GetThemesDir()
	themePath := filepath.Join(themesDir, themeName)

	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		// If theme doesn't exist, we just stay with defaults
		return nil
	}

	// Parse theme.ini (Overrides defaults)
	tiPath := filepath.Join(themePath, "theme.ini")
	if _, err := os.Stat(tiPath); err == nil {
		_ = parseThemeINI(tiPath)
	}

	return nil
}

// Apply updates the global console.Colors with theme-specific tags
func Apply() {
	// 1. Register component tags from Current config (Defaults)
	// This maps the struct fields (colors) to tags like [_ThemeScreen_]
	updateTagsFromCurrent()

	// 2. Map theme.ini fields to themed tags (NOT overwriting global console.Colors)
	// This ensures that when Translate("[_Notice_]") is called in TUI, it uses the theme color.
	for themeKey, consoleField := range themeToConsoleMap {
		themeTag := "[_Theme" + themeKey + "_]"
		// Eagerly resolve the theme tag to its actual cview tag
		resolved := console.Translate(themeTag)
		console.RegisterColor("_"+consoleField+"_", resolved)
	}

	// Re-register tags in console to reflect changes
	console.RegisterBaseTags()
	console.BuildColorMap()

	// Register _ThemeReset_ AFTER all other tags are loaded
	// We check if "VersionBrackets" uses Reverse. If so, we must invert our colors
	// because the Reverse flag is sticky and we cannot use [-] to clear it (causes black flash).
	bracketsDef := console.GetColorDefinition("_ThemeApplicationVersionBrackets_")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")

	bgStr := GetColorStr(Current.ScreenBG)
	fgStr := GetColorStr(Current.ScreenFG)

	if isReversed {
		// Reverse is active: Swapping FG/BG will result in Correct orientation after rendering
		// BUT we must ensure the logical BACKGROUND is the ScreenBG so that subsequent tags (like "T:")
		// inherit the stable background.
		// Solution: Set BOTH to ScreenBG. Results in invisible text for the gap (Silver on Silver),
		// but ensures the Next tag sees "BG=Silver", so "Blue on Silver" reversed becomes "Silver on Blue".
		console.RegisterColor("_ThemeReset_", "["+bgStr+":"+bgStr+"]")
	} else {
		// Normal: Set FG/BG normally.
		console.RegisterColor("_ThemeReset_", "["+fgStr+":"+bgStr+"]")
	}

	// 3. Update global cview styles to match theme globals
	updateStyles()
}

func updateTagsFromCurrent() {
	regComp := func(name string, fg, bg tcell.Color) {
		fgName := GetColorStr(fg)
		bgName := GetColorStr(bg)
		tag := "[" + fgName + ":" + bgName + "]"
		console.RegisterColor("_Theme"+name+"_", tag)
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

	console.RegisterColor("_ThemeTagKey_", "["+GetColorStr(Current.TagKeyFG)+"]")
	console.RegisterColor("_ThemeTagKeySelected_", "["+GetColorStr(Current.TagKeySelectedFG)+"]")
	console.RegisterColor("_ThemeShadow_", "["+GetColorStr(Current.ShadowColor)+"]")
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
	}
	Apply()

	// Register basic theme fallbacks to prevent literal tags if theme files fail to load
	console.RegisterColor("_ThemeApplicationName_", "[::b]")
	console.RegisterColor("_ThemeApplicationVersion_", "[-]")
	console.RegisterColor("_ThemeApplicationVersionBrackets_", "[-]")
	console.RegisterColor("_ThemeApplicationVersionSpace_", "[_ThemeScreen_] ")
	console.RegisterColor("_ThemeApplicationFlags_", "[-]")
	console.RegisterColor("_ThemeApplicationFlagsBrackets_", "[-]")
	console.RegisterColor("_ThemeApplicationFlagsSpace_", "[_ThemeScreen_] ")
	console.RegisterColor("_ThemeApplicationUpdate_", "[yellow]")
	console.RegisterColor("_ThemeApplicationUpdateBrackets_", "[-]")
	console.RegisterColor("_ThemeHostname_", "[::b]")
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

		// 1. Directly register the value as the tag (Legacy behavior + Custom overrides)
		console.RegisterColor("_Theme"+key+"_", expanded)

		// 2. Map known keys to Current struct fields
		fg, bg := parseTagToColor(expanded)
		switch key {
		case "Screen":
			Current.ScreenFG, Current.ScreenBG = fg, bg
		case "Dialog":
			Current.DialogFG, Current.DialogBG = fg, bg
		case "Border":
			Current.BorderFG, Current.BorderBG = fg, bg
		case "Border2":
			Current.Border2FG, Current.Border2BG = fg, bg
		case "BoxTitle": // Specific key for Border Title
			Current.TitleFG, Current.TitleBG = fg, bg
		case "Shadow":
			// Shadow is usually just BG
			// But tag might be [black:black:b]
			// We take the BG? Or FG? Usually same.
			Current.ShadowColor = fg
		case "ButtonActive":
			Current.ButtonActiveFG, Current.ButtonActiveBG = fg, bg
		case "ButtonInactive":
			Current.ButtonInactiveFG, Current.ButtonInactiveBG = fg, bg
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
		}
	}

	// 3. Re-apply styles and tags based on updated Current
	// Note: We do NOT call updateTagsFromCurrent() because we want to keep the specific tags registered above.
	// However, we MUST update global cview styles.
	updateStyles()

	// We also need to re-run the complex logic in Apply() regarding _ThemeReset_ and standard mappings
	// Apply() calls updateTagsFromCurrent() which overwrites tags.
	// But parseThemeINI just overwrote them with EXACT values from INI.
	// If INI has "Screen", parseThemeINI registers _ThemeScreen_.
	// If INI forces "Title" (Content), it registers _ThemeTitle_.
	// Apply's mapping loop registers maps like Title->Theme.

	// Let's call the REST of Apply's logic.
	// Actually, we can just duplicate the needed parts to avoid circular overwrite.

	// Map theme.ini fields to themed tags
	for themeKey, consoleField := range themeToConsoleMap {
		themeTag := "[_Theme" + themeKey + "_]"
		resolved := console.Translate(themeTag)
		console.RegisterColor("_"+consoleField+"_", resolved)
	}

	console.RegisterBaseTags()
	console.BuildColorMap()

	bracketsDef := console.GetColorDefinition("_ThemeApplicationVersionBrackets_")
	isReversed := strings.Contains(bracketsDef, ":r") || strings.Contains(bracketsDef, "reverse")
	bgStr := GetColorStr(Current.ScreenBG)
	fgStr := GetColorStr(Current.ScreenFG)

	if isReversed {
		console.RegisterColor("_ThemeReset_", "["+bgStr+":"+bgStr+"]")
	} else {
		console.RegisterColor("_ThemeReset_", "["+fgStr+":"+bgStr+"]")
	}

	return nil
}
