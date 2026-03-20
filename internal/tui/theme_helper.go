package tui

import (
	"image/color"
	"regexp"
	"strings"
	"sync"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

var (
	semanticStyleCache = make(map[string]lipgloss.Style)
	renderCache        = make(map[string]string)
	cacheMu            sync.RWMutex
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

// Color parsing now uses tcell/v3/colors for RGB conversion via console.GetHexForColor().
// This ensures all colors are resolved to RGB/hex values, allowing proper color profile
// downgrading for terminals with limited color support.

// themeTagRegex matches any tag using current delimiters
var themeTagRegex = console.GetDelimitedRegex()

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
	// Create a cache key from the text, the style, and the prefix
	cacheKey := "theme|" + text + "|" + resetStyle.String() + "|" + ctx.Prefix

	cacheMu.RLock()
	if cached, ok := renderCache[cacheKey]; ok {
		cacheMu.RUnlock()
		return cached
	}
	cacheMu.RUnlock()

	// Ensure the starting text has the correct background/foreground/attributes
	getCodes := func(s lipgloss.Style) string {
		rendered := s.Render("_")
		return strings.Split(rendered, "_")[0]
	}

	// Resolve tags to ANSI using the context's prefix and the isolated theme map
	rendered := console.ToThemeANSIWithPrefix(text, ctx.Prefix)

	// Combine components and ensure reset at end
	result := getCodes(resetStyle) + rendered + console.CodeReset

	// Prevent embedded resets from clearing container background or attributes
	final := MaintainBackground(result, resetStyle)

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

	// Ensure the starting text has the correct background/foreground/attributes
	getCodes := func(s lipgloss.Style) string {
		rendered := s.Render("_")
		return strings.Split(rendered, "_")[0]
	}

	// Resolve tags to ANSI using MUST use the console registry
	rendered := console.ToConsoleANSI(text)

	// Combine components and ensure reset at end
	result := getCodes(resetStyle) + rendered + console.CodeReset

	// Prevent embedded resets from clearing container background or attributes
	final := MaintainBackground(result, resetStyle)

	cacheMu.Lock()
	renderCache[cacheKey] = final
	cacheMu.Unlock()

	return final
}

// ApplyStyleCode applies tview-style color codes (fg:bg:flags) to a lipgloss style.
func ApplyStyleCode(style lipgloss.Style, resetStyle lipgloss.Style, styleCode string) lipgloss.Style {
	return theme.ApplyStyleCode(style, resetStyle, styleCode)
}

// ApplyTagsToStyle translates any {{...}} tags and applies them to the given style.
func ApplyTagsToStyle(text string, style lipgloss.Style, resetStyle lipgloss.Style) lipgloss.Style {
	return theme.ApplyTagsToStyle(text, style, resetStyle)
}

// ParseColor is a wrapper around console.ParseColor for TUI use.
func ParseColor(name string) color.Color {
	return theme.ParseColor(name)
}

// brightenColor delegates to theme.BrightenColor.
func brightenColor(c color.Color) color.Color {
	return theme.BrightenColor(c)
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
			translated := console.Translate(console.WrapSemantic(tagContent))
			// Recurse into translated (recursive check)
			return GetInitialStyle(translated, base)
		} else if direct != "" {
			colorCode := strings.Trim(direct, "|")
			return ApplyStyleCode(base, base, colorCode)
		}
	}
	return base
}

// MaintainBackground replaces ANSI resets (\x1b[0m, \x1b[m, \x1b[39m, \x1b[49m) with the reset followed by the parent style's codes.
// This prevents content-level resets from "bleeding" to the terminal default background or clearing attributes.
// For partial resets (FG-only or BG-only), only the affected channel is restored so that
// the other channel's color (set by the content) is not overwritten.
func MaintainBackground(text string, style lipgloss.Style) string {
	// Extract the full ANSI state from a dummy render
	getANSI := func(s lipgloss.Style) string {
		rendered := s.Render("_")
		return strings.Split(rendered, "_")[0]
	}

	fullCode := getANSI(style)
	if fullCode == "" {
		return text
	}

	// Per-channel codes: injecting only the affected channel after a partial reset
	// prevents overwriting colors set by the content before the reset.
	fgCode := getANSI(lipgloss.NewStyle().Foreground(style.GetForeground()))
	bgCode := getANSI(lipgloss.NewStyle().Background(style.GetBackground()))

	// Regex to find:
	// 1. Full reset: \x1b[0m or \x1b[m
	// 2. FG reset: \x1b[39m
	// 3. BG reset: \x1b[49m
	re := regexp.MustCompile(`\x1b\[(?:0|39|49)?m`)

	return re.ReplaceAllStringFunc(text, func(match string) string {
		switch match {
		case "\x1b[39m":
			return match + fgCode
		case "\x1b[49m":
			return match + bgCode
		default: // \x1b[0m or \x1b[m — full reset, restore everything
			return match + fullCode
		}
	})
}

// GetPlainText strips all {{...}} theme tags from text
func GetPlainText(text string) string {
	return themeTagRegex.ReplaceAllString(text, "")
}
