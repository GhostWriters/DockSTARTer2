package tui

import (
	"context"
	"fmt"
	"sync"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/logger"

	semstyle "github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

// resolveTintRef reads ref's scheme bytes -- see
// commands.ResolveTintRefData (config.AnsiColors.Tint's doc comment has the
// "file:"/"user:"/"embedded:"/"repo:" prefix convention).
func resolveTintRef(ctx context.Context, ref string) ([]byte, error) {
	data, _, err := commands.ResolveTintRefData(ctx, ref)
	return data, err
}

// tintKeyForConnType is console.TintKeyForConnType -- kept as a local alias
// since every call site in this file already refers to the unqualified
// name.
func tintKeyForConnType(connType string) string {
	return console.TintKeyForConnType(connType)
}

// ansiElements is every element a connType's tint can be independently set
// for -- "menu" (registered under connType's own key, see
// tintKeyForConnType) is the base/implicit element; "programbox"/"cli" each
// register under their own suffixed key too (see
// console.TintKeyForConnTypeElement and RegisterConnTypeTints), each a
// fully independent palette (see config.AnsiColors' own doc comment --
// elements never inherit from one another at render time).
var ansiElements = []string{"menu", "programbox", "cli"}

// RegisterConnTypeTints resolves colors for connType and registers each of
// its elements (see ansiElements) under that element's own semstyle tint
// key (see console.TintKeyForConnTypeElement), so BeginTintForElement
// (which activates a specific element's key directly) always has a real
// registration to find. Re-resolves and re-registers on every call (each
// new session's Init(), typically, or a bare CLI invocation's Execute) so
// a config change (e.g.
// via --theme-tint/--ansi-override/--tint) takes effect for the next
// session/invocation. Does not activate any tint; see ActivateTintFor/
// ActivateTintForElement/BeginTintForElement for that.
func RegisterConnTypeTints(ctx context.Context, connType string, colors config.AnsiColors) {
	for _, element := range ansiElements {
		registerElementTint(ctx, connType, element, colors.Element(element))
	}
}

// registerElementTint resolves and registers one element's own palette --
// see RegisterConnTypeTints, which this factors the per-element work out of.
//
// element.TintEnabled and element.OverrideEnabled (see their doc comments)
// independently gate the SchemeFile-derived part and the 16 explicit
// fields, respectively -- either can be on/off regardless of the other, so
// e.g. disabling the tint (--theme-no-tint) never affects an override
// (--ansi-override), and vice versa.
//
// This replaces the standard ANSI SGR codes a theme's named colors compile
// to with the literal RGB values element resolves to, at the point semstyle
// converts color names to escape codes -- rather than sending the
// connecting terminal a palette override, since not every terminal honors
// one.
func registerElementTint(ctx context.Context, connType, elementName string, element config.AnsiElementColors) {
	RegisterTintKey(ctx, console.TintKeyForConnTypeElement(connType, elementName), element)
}

// RegisterTintKey resolves element's palette and registers it under key
// (see registerElementTint), or unregisters key when element tints nothing.
func RegisterTintKey(ctx context.Context, key string, element config.AnsiElementColors) {
	if !element.OverrideEnabled {
		element = withoutExplicitFields(element)
	}
	variant := ""
	if element.TintEnabled {
		element, variant = resolveAnsiColors(ctx, element)
	}
	palette, empty := colorsToPalette(element)
	palette.Variant = variant
	if empty {
		semstyle.UnregisterTint(key)
	} else {
		semstyle.RegisterTint(key, palette)
	}
	displayengine.InvalidateStyles()
	invalidateShadowCache()
}

// withoutExplicitFields returns element with its 16 explicit color fields
// cleared, keeping TintEnabled/OverrideEnabled/Tint untouched -- used to
// honor OverrideEnabled=false without also losing Tint-derived resolution.
func withoutExplicitFields(element config.AnsiElementColors) config.AnsiElementColors {
	return config.AnsiElementColors{
		TintEnabled:     element.TintEnabled,
		OverrideEnabled: element.OverrideEnabled,
		Tint:            element.Tint,
	}
}

// ActivateTintFor is console.ActivateTintFor -- makes connType's registered
// tint (see RegisterConnTypeTints) the active one for the duration of fn,
// then restores whatever was active before. Wrap every AppModel.Update/View
// call with this so DS2 renders reflect the correct per-connType tint even
// with local/SSH/web sessions running concurrently -- semstyle.RunWithTint's
// own lock serializes them against each other so none sees another's key
// mid-render.
func ActivateTintFor(connType string, fn func()) {
	console.ActivateTintFor(connType, fn)
}

// BeginTintFor is ActivateTintFor split into a begin/restore pair (see
// semstyle.BeginTint) for a bare CLI invocation's startup sequence, which
// has early returns scattered through it (pre-flight checks, re-exec
// paths, etc.) that a single wrapping closure can't cleanly cover. Call
// restore (typically via defer, right where this is called) exactly once,
// after which the whole invocation's console output -- not just command
// dispatch -- reflects connType's tint.
func BeginTintFor(connType string) (restore func()) {
	return beginRenderScope(connType, tintKeyForConnType(connType))
}

// BeginTintForElement is BeginTintFor targeting a specific element (see
// ansiElements) rather than implicitly "menu" -- for a bare CLI
// invocation's "cli" element, which (unlike menu) has no session
// Update/View loop to wrap with ActivateTintForElement, so it needs this
// begin/restore-pair form for the same early-return-scattered-startup
// reason BeginTintFor itself exists.
func BeginTintForElement(connType, element string) (restore func()) {
	return beginRenderScope(connType, console.TintKeyForConnTypeElement(connType, element))
}

// beginRenderScope activates tintKey, connType's theme namespace, and
// connType itself (see console.ActiveConnType) until restore is called.
func beginRenderScope(connType, tintKey string) (restore func()) {
	restoreScope := semstyle.BeginRenderScope(tintKey, console.ThemePrefixForConnType(connType))
	restoreConnType := console.SetActiveConnType(connType)
	return func() {
		restoreConnType()
		restoreScope()
	}
}

// ActivateTintForElement is console.ActivateTintForElement -- makes
// element's registered tint (see RegisterConnTypeTints) active for the
// duration of fn, derived from whichever connType's key is already active
// from the enclosing ActivateTintFor/ActivateSessionRenderContext call.
func ActivateTintForElement(element string, fn func()) {
	console.ActivateTintForElement(element, fn)
}

// ActivateSessionRenderContext is console.ActivateSessionRenderContext --
// nests ActivateTintFor(connType, ...) inside semstyle.RunWithProfile(
// profile, ...) for the duration of fn, so a session's Update/View call
// renders with both its own connType's tint and its own actual color
// profile (see resolveColorProfile) -- one call instead of nesting both
// wrappers at every call site.
func ActivateSessionRenderContext(connType string, profile colorprofile.Profile, fn func()) {
	console.ActivateSessionRenderContext(connType, profile, fn)
}

// DeactivateTint runs fn with no tint active (an empty key, semstyle's
// no-tint sentinel -- not tintKeyForConnType("")), then restores whatever
// was active before, same as ActivateTintFor. For rendering that should
// look like the terminal's own native palette regardless of any
// otherwise-active tint.
//
// Uses semstyle.SetActiveTint/ActiveTintKey directly, not
// semstyle.RunWithTint -- see ActivateTintForElement's doc comment for why:
// this is meant to be callable from inside an already-locked
// Update/View render pass, and RunWithTint's lock isn't reentrant.
func DeactivateTint(fn func()) {
	prev := semstyle.ActiveTintKey()
	semstyle.SetActiveTint("")
	defer semstyle.SetActiveTint(prev)
	fn()
}

// colorsToPalette converts colors' 16 slots (each accepting anything
// semstyle.ToColor understands: hex, an ANSI name, or any broader color name
// tcell resolves) into a semstyle.Palette of literal hex values. empty is
// true if none of the 16 slots were set.
func colorsToPalette(colors config.AnsiElementColors) (palette semstyle.Palette, empty bool) {
	slots := colors.Slots()
	hex := make([]string, len(slots))
	empty = true
	for i, v := range slots {
		if v == "" {
			continue
		}
		empty = false
		r, g, b, _ := semstyle.ToColor(v).RGBA()
		hex[i] = fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	extraHex := func(v string) string {
		if v == "" {
			return ""
		}
		r, g, b, _ := semstyle.ToColor(v).RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	return semstyle.Palette{
		Black: hex[0], Red: hex[1], Green: hex[2], Yellow: hex[3],
		Blue: hex[4], Magenta: hex[5], Cyan: hex[6], White: hex[7],
		BrightBlack: hex[8], BrightRed: hex[9], BrightGreen: hex[10], BrightYellow: hex[11],
		BrightBlue: hex[12], BrightMagenta: hex[13], BrightCyan: hex[14], BrightWhite: hex[15],
		Base01: extraHex(colors.Base01), Base02: extraHex(colors.Base02),
		Base04: extraHex(colors.Base04), Base06: extraHex(colors.Base06),
		Base09: extraHex(colors.Base09), Base0F: extraHex(colors.Base0F),
		Base10: extraHex(colors.Base10), Base11: extraHex(colors.Base11),
	}, empty
}

// tintWarnOnce dedupes resolveAnsiColors' warnings to once per distinct
// broken Tint ref per process run -- RegisterConnTypeTints re-resolves on
// every session Init and bare CLI invocation, and a still-missing file
// would otherwise warn on every single one of those for as long as it
// stays broken.
var (
	tintWarnOnceMu sync.Mutex
	tintWarnedRefs = map[string]bool{}
)

func warnTintOnce(ctx context.Context, ref, format string, args ...any) {
	tintWarnOnceMu.Lock()
	already := tintWarnedRefs[ref]
	tintWarnedRefs[ref] = true
	tintWarnOnceMu.Unlock()
	if !already {
		logger.Warn(ctx, format, args...)
	}
}

// resolveAnsiColors layers colors' own explicit fields over its configured
// Tint (if set), so an individually-set field always wins over the scheme.
// It also returns the scheme's variant ("dark" or "light", empty when the
// scheme doesn't say), which orders the palette's derived backgrounds (see
// semstyle.Palette.Variant).
//
// Any failure to load or parse it warns (once per ref per process run, see
// warnTintOnce) and falls back to colors' own explicit fields (or no tint
// at all if those are empty too) -- --tint itself only ever persists a
// Tint value it already confirmed exists and parses (see HandleTint), so a
// resolution failure here always means the scheme moved, was deleted, or
// otherwise changed out from under an existing, once-valid configuration,
// not an unset default (colors.Tint == "" is handled separately, above,
// and never reaches this far).
func resolveAnsiColors(ctx context.Context, colors config.AnsiElementColors) (config.AnsiElementColors, string) {
	if colors.Tint == "" {
		return colors, ""
	}
	data, err := resolveTintRef(ctx, colors.Tint)
	if err != nil {
		warnTintOnce(ctx, colors.Tint, "ansi_palette: could not load tint %q: %v", colors.Tint, err)
		return colors, ""
	}
	scheme, err := config.ParseBase16Scheme(data)
	if err != nil {
		warnTintOnce(ctx, colors.Tint, "ansi_palette: could not parse tint %q: %v", colors.Tint, err)
		return colors, ""
	}
	meta, _ := config.ParseBase16SchemeMeta(data)
	return colors.WithDefaults(scheme), meta.Variant
}
