package tui

import (
	"context"
	"fmt"
	"sync"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"

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

// tintKeyForConnType returns the semstyle tint registration key for
// connType ("local", "ssh", or "web") -- kept separate per connType so
// local/SSH/web sessions never clobber each other's tint (see
// registerConnTypeTints and activateTintFor).
func tintKeyForConnType(connType string) string {
	return "ds2-tint-" + connType
}

// RegisterConnTypeTints resolves colors for connType and registers it under
// that connType's own semstyle tint key (see tintKeyForConnType).
// Re-resolves and re-registers on every call (each new session's Init(),
// typically, or a bare CLI invocation's Execute) so a config change (e.g.
// via --theme-tint/--ansi-override) takes effect for the next
// session/invocation, same as before this connType-scoped tint mechanism
// existed. Does not activate the tint; see ActivateTintFor for that.
//
// colors.TintEnabled and colors.OverrideEnabled (see their doc comments)
// independently gate the SchemeFile-derived part and the 16 explicit
// fields, respectively -- either can be on/off regardless of the other, so
// e.g. disabling the tint (--theme-no-tint) never affects an override
// (--ansi-override), and vice versa.
//
// This replaces the standard ANSI SGR codes a theme's named colors compile
// to with the literal RGB values colors resolves to, at the point semstyle
// converts color names to escape codes -- rather than sending the
// connecting terminal a palette override, since not every terminal honors
// one.
func RegisterConnTypeTints(ctx context.Context, connType string, colors config.AnsiColors) {
	key := tintKeyForConnType(connType)
	if !colors.OverrideEnabled {
		colors = withoutExplicitFields(colors)
	}
	if colors.TintEnabled {
		colors = resolveAnsiColors(ctx, colors)
	}
	palette, empty := colorsToPalette(colors)
	if empty {
		semstyle.UnregisterTint(key)
	} else {
		semstyle.RegisterTint(key, palette)
	}
	theme.ClearSemanticCache()
}

// withoutExplicitFields returns colors with its 16 explicit color fields
// cleared, keeping TintEnabled/OverrideEnabled/Tint untouched -- used to
// honor OverrideEnabled=false without also losing Tint-derived resolution.
func withoutExplicitFields(colors config.AnsiColors) config.AnsiColors {
	return config.AnsiColors{
		TintEnabled:     colors.TintEnabled,
		OverrideEnabled: colors.OverrideEnabled,
		Tint:            colors.Tint,
	}
}

// ActivateTintFor makes connType's registered tint (see
// RegisterConnTypeTints) the active one for the duration of fn (see
// semstyle.RunWithTint), then restores whatever was active before. Wrap
// every AppModel.Update/View call with this so DS2 renders reflect the
// correct per-connType tint even with local/SSH/web sessions running
// concurrently -- semstyle.RunWithTint's own lock serializes them against
// each other so none sees another's key mid-render.
func ActivateTintFor(connType string, fn func()) {
	semstyle.RunWithTint(tintKeyForConnType(connType), fn)
}

// BeginTintFor is ActivateTintFor split into a begin/restore pair (see
// semstyle.BeginTint) for a bare CLI invocation's startup sequence, which
// has early returns scattered through it (pre-flight checks, re-exec
// paths, etc.) that a single wrapping closure can't cleanly cover. Call
// restore (typically via defer, right where this is called) exactly once,
// after which the whole invocation's console output -- not just command
// dispatch -- reflects connType's tint.
func BeginTintFor(connType string) (restore func()) {
	return semstyle.BeginTint(tintKeyForConnType(connType))
}

// ActivateSessionRenderContext nests ActivateTintFor(connType, ...) inside
// semstyle.RunWithProfile(profile, ...) for the duration of fn, so a
// session's Update/View call renders with both its own connType's tint and
// its own actual color profile (see resolveColorProfile) -- one call
// instead of nesting both wrappers at every call site.
func ActivateSessionRenderContext(connType string, profile colorprofile.Profile, fn func()) {
	semstyle.RunWithProfile(profile, func() {
		ActivateTintFor(connType, fn)
	})
}

// DeactivateTint runs fn with no tint active (an empty key, semstyle's
// no-tint sentinel -- not tintKeyForConnType("")), then restores whatever
// was active before, same as ActivateTintFor. For rendering that should
// look like the terminal's own native palette even while nested inside an
// otherwise-tinted render pass (e.g. a ProgramBox's streamed command
// output when config.AnsiPaletteConfig.ApplyToProgramBox is off).
func DeactivateTint(fn func()) {
	semstyle.RunWithTint("", fn)
}

// colorsToPalette converts colors' 16 slots (each accepting anything
// semstyle.ToColor understands: hex, an ANSI name, or any broader color name
// tcell resolves) into a semstyle.Palette of literal hex values. empty is
// true if none of the 16 slots were set.
func colorsToPalette(colors config.AnsiColors) (palette semstyle.Palette, empty bool) {
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
//
// Any failure to load or parse it warns (once per ref per process run, see
// warnTintOnce) and falls back to colors' own explicit fields (or no tint
// at all if those are empty too) -- --tint itself only ever persists a
// Tint value it already confirmed exists and parses (see HandleTint), so a
// resolution failure here always means the scheme moved, was deleted, or
// otherwise changed out from under an existing, once-valid configuration,
// not an unset default (colors.Tint == "" is handled separately, above,
// and never reaches this far).
func resolveAnsiColors(ctx context.Context, colors config.AnsiColors) config.AnsiColors {
	if colors.Tint == "" {
		return colors
	}
	data, err := resolveTintRef(ctx, colors.Tint)
	if err != nil {
		warnTintOnce(ctx, colors.Tint, "ansi_palette: could not load tint %q: %v", colors.Tint, err)
		return colors
	}
	scheme, err := config.ParseBase16Scheme(data)
	if err != nil {
		warnTintOnce(ctx, colors.Tint, "ansi_palette: could not parse tint %q: %v", colors.Tint, err)
		return colors
	}
	return colors.WithDefaults(scheme)
}
