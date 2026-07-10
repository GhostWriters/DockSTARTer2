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
