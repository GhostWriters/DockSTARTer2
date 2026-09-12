package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"

	semstyle "github.com/GhostWriters/semstyle"
)

// buildAnsiPaletteOSC returns an OSC 4 sequence overriding the ANSI palette
// slots resolved from colors (its own explicit fields layered over its
// SchemeFile, if set), or "" if none are set. Each value accepts anything
// semstyle.ToColor understands: hex, an ANSI name, or any broader color
// name tcell resolves.
//
// Sent once at session start (AppModel.Init) so a theme's named ANSI colors
// (which compile to plain ANSI SGR codes, not literal RGB) render as
// intended regardless of the connecting terminal's own default palette --
// most relevant for the web frontend, whose client has no user-configured
// palette the way a real local/SSH terminal does.
func buildAnsiPaletteOSC(ctx context.Context, colors config.AnsiColors) string {
	if colors.Disabled {
		return ""
	}
	colors = resolveAnsiColors(ctx, colors)
	var b strings.Builder
	for i, v := range colors.Slots() {
		if v == "" {
			continue
		}
		r, g, bl, _ := semstyle.ToColor(v).RGBA()
		fmt.Fprintf(&b, ";%d;rgb:%02x/%02x/%02x", i, r>>8, g>>8, bl>>8)
	}
	if b.Len() == 0 {
		return ""
	}
	return "\x1b]4" + b.String() + "\x07"
}

// resolveAnsiColors layers colors' own explicit fields over its SchemeFile
// (if set), so an individually-set field always wins over the scheme.
//
// A SchemeFile that simply doesn't exist is silently ignored -- falls back
// to colors' own explicit fields (or the terminal's own default palette if
// those are empty too), same as if SchemeFile were never set. Anything
// else wrong with it (permission denied, malformed YAML) warns, since that
// points at a real problem with a file the user did configure, not just an
// unset/cleared default.
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
