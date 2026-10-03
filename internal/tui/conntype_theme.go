package tui

import (
	"context"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
)

// connTypeThemeNamespace is the semstyle theme namespace connType's own theme
// registers under when it differs from the unprefixed (local) theme.
func connTypeThemeNamespace(connType string) string {
	return "ct-" + connType + "_"
}

// RegisterConnTypeTheme registers connType's theme under its own namespace
// (see connTypeThemeNamespace) when it differs from the unprefixed theme, and
// records which namespace connType renders with (see
// console.ThemePrefixForConnType).
func RegisterConnTypeTheme(ctx context.Context, connType string, conf config.AppConfig) {
	name := conf.Appearance.ForConnType(connType).Theme
	prefix := ""
	if connType != "local" && name != conf.Appearance.Local.Theme {
		prefix = connTypeThemeNamespace(connType)
		if _, err := theme.Load(name, prefix); err != nil {
			logger.Warn(ctx, "Could not load theme %q for %s sessions: %v", name, connType, err)
			prefix = ""
		}
	}
	console.SetThemePrefixForConnType(connType, prefix)
	displayengine.InvalidateStyles()
	invalidateShadowCache()
}
