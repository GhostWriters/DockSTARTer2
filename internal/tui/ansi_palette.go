package tui

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/config"

	semstyle "github.com/GhostWriters/semstyle"
)

// buildAnsiPaletteOSC returns an OSC 4 sequence overriding the ANSI palette
// slots configured in colors, or "" if none are set. Each value accepts
// anything semstyle.ToColor understands: hex, an ANSI name, or any broader
// color name tcell resolves.
//
// Sent once at session start (AppModel.Init) so a theme's named ANSI colors
// (which compile to plain ANSI SGR codes, not literal RGB) render as
// intended regardless of the connecting terminal's own default palette --
// most relevant for the web frontend, whose client has no user-configured
// palette the way a real local/SSH terminal does.
func buildAnsiPaletteOSC(colors config.AnsiColors) string {
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
