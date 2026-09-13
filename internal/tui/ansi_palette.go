package tui

import (
	"context"
	"fmt"
	"os"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"

	semstyle "github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

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
// cleared, keeping TintEnabled/OverrideEnabled/SchemeFile untouched -- used
// to honor OverrideEnabled=false without also losing SchemeFile-derived
// resolution.
func withoutExplicitFields(colors config.AnsiColors) config.AnsiColors {
	return config.AnsiColors{
		TintEnabled:     colors.TintEnabled,
		OverrideEnabled: colors.OverrideEnabled,
		SchemeFile:      colors.SchemeFile,
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
	return semstyle.Palette{
		Black: hex[0], Red: hex[1], Green: hex[2], Yellow: hex[3],
		Blue: hex[4], Magenta: hex[5], Cyan: hex[6], White: hex[7],
		BrightBlack: hex[8], BrightRed: hex[9], BrightGreen: hex[10], BrightYellow: hex[11],
		BrightBlue: hex[12], BrightMagenta: hex[13], BrightCyan: hex[14], BrightWhite: hex[15],
	}, empty
}

// resolveAnsiColors layers colors' own explicit fields over its SchemeFile
// (if set), so an individually-set field always wins over the scheme.
//
// A SchemeFile that simply doesn't exist is silently ignored -- falls back
// to colors' own explicit fields (or no tint at all if those are empty too),
// same as if SchemeFile were never set. Anything else wrong with it
// (permission denied, malformed YAML) warns, since that points at a real
// problem with a file the user did configure, not just an unset/cleared
// default.
func resolveAnsiColors(ctx context.Context, colors config.AnsiColors) config.AnsiColors {
	if colors.SchemeFile == "" {
		return colors
	}
	scheme, err := config.LoadBase16Scheme(colors.SchemeFile)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn(ctx, "ansi_palette: could not load scheme_file %q: %v", colors.SchemeFile, err)
		}
		return colors
	}
	return colors.WithDefaults(scheme)
}
