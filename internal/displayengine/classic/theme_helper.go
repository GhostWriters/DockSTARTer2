package classic

import (
	"image/color"
	"strings"
	"sync"

	"DockSTARTer2/internal/theme"
	semstyle "github.com/GhostWriters/semstyle/lg"

	"charm.land/lipgloss/v2"
)

var (
	renderCache = make(map[string]string)
	cacheMu     sync.RWMutex
)

// ClearSemanticCache clears both the theme-level style cache and the TUI render cache.
func ClearSemanticCache() {
	theme.ClearSemanticCache()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	renderCache = make(map[string]string)
}

// ClearSemanticCachePrefix removes render cache and style cache entries whose key
// contains the given prefix string.
func ClearSemanticCachePrefix(prefix string) {
	theme.ClearSemanticCachePrefix(prefix)
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for k := range renderCache {
		if strings.Contains(k, prefix) {
			delete(renderCache, k)
		}
	}
	// Also clear the semantic style cache in the theme package
	theme.ClearSemanticCachePrefix(prefix)
}

// SemanticStyle translates a semantic tag or direct style code strictly using the theme registry.
func SemanticStyle(tag string) lipgloss.Style {
	return theme.ThemeSemanticStyle(tag)
}

// SemanticRawStyle translates a raw semantic name strictly using the theme registry.
func SemanticRawStyle(name string) lipgloss.Style {
	return theme.ThemeSemanticRawStyle(name)
}

// TextCursorColor returns the foreground color defined by the TextCursor theme entry.
// Used to set the cursor color on textinput and enveditor models.
func TextCursorColor() color.Color {
	return SemanticRawStyle("TextCursor").GetForeground()
}

// TagBracketGlyphs returns the open/close glyphs used for focused-row bracket
// indicators (App Select, Menu Brackets, and now line-number brackets),
// following the active ui.line_characters setting.
func TagBracketGlyphs() (open, closeCh string) {
	return bracketGlyphs(GetActiveContext())
}

// TagOverride is one entry in a ResolveThemeOverrides result: the resolved
// style and flags an option wants a tag to use instead of its plain
// theme-defined value.
type TagOverride struct {
	Style lipgloss.Style
	Flags semstyle.StyleFlags
}

// ResolveDisabledStyle returns tag's own "<tag>Disabled" variant if the
// active theme explicitly defines one (semstyle.GetRawTagCode returns ""
// for an unregistered tag, the signal used here), otherwise tag's own
// enabled style with Dim applied on top. This lets themes author an
// explicit disabled color only where they actually want one, with every
// other disable-able element getting a sensible dimmed-enabled-color
// default for free instead of requiring a hand-authored *Disabled tag for
// every themeable element.
func ResolveDisabledStyle(tag string) (style lipgloss.Style, flags semstyle.StyleFlags) {
	disabledTag := tag + "Disabled"
	if semstyle.GetRawTagCode(disabledTag) != "" {
		return SemanticRawStyle(disabledTag), semstyle.CodeToFlags(semstyle.GetRawTagCode(disabledTag))
	}
	// Only override Bold specifically -- Bold+Dim together render as still-
	// bright on many terminals (Bold effectively cancels Dim's visual
	// effect), but every other attribute the base tag has (Underline,
	// Italic, etc.) is kept rather than reset wholesale.
	flags = semstyle.CodeToFlags(semstyle.GetRawTagCode(tag))
	flags.Bold = false
	flags.Dim = true
	return flags.Apply(SemanticRawStyle(tag)), flags
}

// DisabledTagRef returns a semantic tag reference usable as a titleTag or
// embedded directly in a "{{|...|}}" string: tag's own "<tag>Disabled" name
// if the active theme explicitly defines one, else an inline
// "<tag>:::bD" override reference. The trailing "bD" (bold off, dim on) is
// not a leading-dash reset -- semstyle's nested-tag resolver (parse.go's
// resolveThemeValue) merges non-reset flag overrides onto the referenced
// tag's own resolved flags rather than replacing them, so this preserves
// every other attribute (Underline, Italic, etc.) the base tag carries,
// exactly like ResolveDisabledStyle's Go-level fallback -- just expressed as
// text so it also works at call sites that embed the tag name literally
// (e.g. panel_render.go's "{{|"+inputTitleTag+"|}}Command{{[-]}}") where a
// Go-resolved lipgloss.Style can't reach the embedded string at all.
func DisabledTagRef(tag string) string {
	disabledTag := tag + "Disabled"
	if semstyle.GetRawTagCode(disabledTag) != "" {
		return disabledTag
	}
	return tag + ":::bD"
}

// ResolveThemeOverrides reads the untouched <prefix>Border/<prefix>Border2
// (and their BorderDisabled/Border2Disabled counterparts) semantic tags and
// computes what each should resolve to given the Border Color mode (1 =
// Border wins, 2 = Border2 wins, anything else = each tag keeps its own
// theme-defined value), returning the result as a small derived map keyed
// by unprefixed tag name ("Border", "Border2", "BorderDisabled",
// "Border2Disabled") instead of writing back into the shared semstyle
// registry. An earlier version of this logic overwrote one tag's raw code
// with the other's directly in the registry, which corrupted later lookups
// once a tag's original theme value had been overwritten (e.g. mode 1 then
// mode 2 would read the mode-1-corrupted Border2 instead of the theme's
// real value) -- computing a fresh map every call avoids that entirely.
// The disabled variants merge under the same mode as their enabled
// counterparts, so a disabled border reflects the same Border Color choice
// instead of always following the theme's own disabled colors regardless
// of the setting.
//
// Border Color is the only option resolved today; this is the shape to
// extend if more options need the same kind of tag-merging behavior later.
// prefix matches the namespace used elsewhere for non-active-theme
// registrations (e.g. "Preview_" for the Appearance Settings mockup); pass
// "" for the main active theme.
func ResolveThemeOverrides(borderColorMode int, prefix string) map[string]TagOverride {
	p := strings.TrimSuffix(prefix, "_")
	borderTag, border2Tag := "Border", "Border2"
	if p != "" {
		borderTag, border2Tag = p+"_Border", p+"_Border2"
	}
	borderDisabledStyle, borderDisabledFlags := ResolveDisabledStyle(borderTag)
	border2DisabledStyle, border2DisabledFlags := ResolveDisabledStyle(border2Tag)
	overrides := map[string]TagOverride{
		"Border":          {SemanticRawStyle(borderTag), semstyle.CodeToFlags(semstyle.GetRawTagCode(borderTag))},
		"Border2":         {SemanticRawStyle(border2Tag), semstyle.CodeToFlags(semstyle.GetRawTagCode(border2Tag))},
		"BorderDisabled":  {borderDisabledStyle, borderDisabledFlags},
		"Border2Disabled": {border2DisabledStyle, border2DisabledFlags},
	}
	switch borderColorMode {
	case 1:
		overrides["Border2"] = overrides["Border"]
		overrides["Border2Disabled"] = overrides["BorderDisabled"]
	case 2:
		overrides["Border"] = overrides["Border2"]
		overrides["BorderDisabled"] = overrides["Border2Disabled"]
	}
	return overrides
}

// Color parsing now uses tcell/v3/colors for RGB conversion via semstyle.GetHexForColor().
// This ensures all colors are resolved to RGB/hex values, allowing proper color profile
// downgrading for terminals with limited color support.

// themeTagRegex matches any tag using current delimiters
var themeTagRegex = semstyle.GetDelimitedRegex()

// RenderThemeText takes text with {{...}} theme tags and returns lipgloss-styled text
// defaultStyle is used for reset state and unstyled text
func RenderThemeText(text string, defaultStyle ...lipgloss.Style) string {
	ctx := GetActiveContext()
	if len(defaultStyle) > 0 {
		ctx.Dialog = defaultStyle[0]
	}
	return RenderThemeTextCtx(text, ctx)
}

// RenderConsoleText takes text with {{...}} console tags and returns lipgloss-styled text using the console registry.
func RenderConsoleText(text string, defaultStyle ...lipgloss.Style) string {
	ctx := GetActiveContext()
	if len(defaultStyle) > 0 {
		ctx.Dialog = defaultStyle[0]
	}
	return RenderConsoleTextCtx(text, ctx)
}

// RenderThemeTextCtx renders themed text using properties from a specific context
func RenderThemeTextCtx(text string, ctx StyleContext) string {
	if text == "" {
		return ""
	}

	resetStyle := ctx.Dialog
	cacheKey := "theme|" + text + "|" + resetStyle.String() + "|" + ctx.Prefix

	cacheMu.RLock()
	if cached, ok := renderCache[cacheKey]; ok {
		cacheMu.RUnlock()
		return cached
	}
	cacheMu.RUnlock()

	final := semstyle.ToANSIOnBackground(text, resetStyle, ctx.Prefix)

	cacheMu.Lock()
	renderCache[cacheKey] = final
	cacheMu.Unlock()

	return final
}

// RenderConsoleTextCtx renders console text using properties from a specific context and the console registry.
func RenderConsoleTextCtx(text string, ctx StyleContext) string {
	if text == "" {
		return ""
	}

	resetStyle := ctx.Dialog
	// Create a cache key from the text AND the style
	cacheKey := "console|" + text + "|" + resetStyle.String()

	cacheMu.RLock()
	if cached, ok := renderCache[cacheKey]; ok {
		cacheMu.RUnlock()
		return cached
	}
	cacheMu.RUnlock()

	final := semstyle.ToANSIOnBackground(text, resetStyle)

	cacheMu.Lock()
	renderCache[cacheKey] = final
	cacheMu.Unlock()

	return final
}

// CodeToStyle applies tview-style color codes (fg:bg:flags) to a lipgloss style.
func CodeToStyle(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	return semstyle.CodeToStyle(styleCode, style, resetStyle)
}

// ToStyle translates any {{...}} tags and applies them to the given style.
func ToStyle(text string, style lipgloss.Style, resetStyle lipgloss.Style) lipgloss.Style {
	return semstyle.ToStyle(semstyle.Default, text, style, resetStyle)
}

// ParseColor is a wrapper around console.ParseColor for TUI use.
func ParseColor(name string) color.Color {
	return semstyle.ToColor(name)
}

// GetInitialStyle peeks at the first theme tag in text and returns a style derived from it.
// Useful for setting container backgrounds to match themed content.
func GetInitialStyle(text string, base lipgloss.Style) lipgloss.Style {
	match := themeTagRegex.FindStringSubmatch(text)
	if len(match) > 2 {
		semantic := match[1]
		direct := match[2]

		if semantic != "" {
			tagContent := semantic
			translated := semstyle.ToTags(semstyle.WrapSemantic(tagContent))
			// Recurse into translated (recursive check)
			return GetInitialStyle(translated, base)
		} else if direct != "" {
			colorCode := strings.Trim(direct, "|")
			return CodeToStyle(base, base, colorCode)
		}
	}
	return base
}

func MaintainBackground(text string, style lipgloss.Style) string {
	return semstyle.MaintainBackground(text, style)
}

// GetPlainText strips all {{...}} theme tags from text
func GetPlainText(text string) string {
	return semstyle.StripTags(text)
}
