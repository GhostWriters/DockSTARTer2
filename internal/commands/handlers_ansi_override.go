package commands

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
)

// ansiColorSlotNames lists the 16 standard ANSI color slots' canonical
// (base16/base24 slot) names, in the same order as config.AnsiColors.Slots
// -- used for --tint's status display and ansiColorSlotField's error
// message. --ansi-override itself also accepts each slot's classic ANSI
// name (see ansiColorSlotField); this list intentionally only shows one
// name per slot so the display doesn't repeat itself.
var ansiColorSlotNames = []string{
	"base00", "base08", "base0b", "base0a", "base0d", "base0e", "base0c", "base05",
	"base03", "base12", "base14", "base13", "base16", "base17", "base15", "base07",
	"base01", "base02", "base04", "base06", "base09", "base0f", "base10", "base11",
}

// ansiColorSlotField returns a pointer to slot's field on c, accepting
// either its base16/base24 slot name (config.AnsiColors' own canonical
// field names, matching a scheme file's own "palette:" keys) or its classic
// ANSI name (matching semstyle's own color-name aliasing, see
// ansiColorIndex's doc comment in the semstyle module) -- both spellings
// are exactly the same color, never two separate ones. Case-insensitive.
// Returns nil if slot matches neither.
func ansiColorSlotField(c *config.AnsiColors, slot string) *string {
	switch strings.ToLower(slot) {
	case "black", "base00":
		return &c.Base00
	case "red", "base08":
		return &c.Base08
	case "green", "base0b":
		return &c.Base0B
	case "yellow", "base0a":
		return &c.Base0A
	case "blue", "base0d":
		return &c.Base0D
	case "magenta", "base0e":
		return &c.Base0E
	case "cyan", "base0c":
		return &c.Base0C
	case "white", "base05":
		return &c.Base05
	case "bright-black", "base03":
		return &c.Base03
	case "bright-red", "base12":
		return &c.Base12
	case "bright-green", "base14":
		return &c.Base14
	case "bright-yellow", "base13":
		return &c.Base13
	case "bright-blue", "base16":
		return &c.Base16
	case "bright-magenta", "base17":
		return &c.Base17
	case "bright-cyan", "base15":
		return &c.Base15
	case "bright-white", "base07":
		return &c.Base07
	case "base01":
		return &c.Base01
	case "base02":
		return &c.Base02
	case "base04":
		return &c.Base04
	case "base06":
		return &c.Base06
	case "base09":
		return &c.Base09
	case "base0f":
		return &c.Base0F
	case "base10":
		return &c.Base10
	case "base11":
		return &c.Base11
	default:
		return nil
	}
}

// HandleAnsiOverride implements --ansi-override <slot> <value> [types],
// setting one of the 16 standard ANSI colors directly on
// ansi_palette.<connType> -- independent of, and layered over, any tint
// configured via --tint (see config.AnsiColors.
// WithDefaults). value "none" (or an empty string) clears the override for
// that slot, falling back to any tint's own value. types is optional --
// omitted means "all". See HandleThemeAnsiOverrideOnOff for the bulk
// on/off switch covering all 16 slots at once.
func HandleAnsiOverride(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 2 {
		logger.Error(ctx, "Usage: --ansi-override <slot> <value> [local|ssh|web|all|a,b,c]")
		return fmt.Errorf("missing arguments")
	}
	slot := group.Args[0]
	value := group.Args[1]
	if strings.EqualFold(value, "none") {
		value = ""
	}
	typesArg := ""
	if len(group.Args) > 2 {
		typesArg = group.Args[2]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	if ansiColorSlotField(&config.AnsiColors{}, slot) == nil {
		err := fmt.Errorf("unknown color slot %q (valid: %s)", slot, strings.Join(ansiColorSlotNames, ", "))
		logger.Error(ctx, "%v", err)
		return err
	}

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		*ansiColorSlotField(c, slot) = value
	})
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save ANSI color override: %v", err)
		return err
	}

	if value == "" {
		logger.Notice(ctx, "ANSI color override for {{|Var|}}%s{{[-]}} cleared for: {{|Var|}}%s{{[-]}}", slot, strings.Join(connTypes, ", "))
	} else {
		logger.Notice(ctx, "ANSI color override for {{|Var|}}%s{{[-]}} set to {{|Var|}}%s{{[-]}} for: {{|Var|}}%s{{[-]}}", slot, value, strings.Join(connTypes, ", "))
	}
	return nil
}

// HandleThemeAnsiOverrideOnOff implements --theme-ansi-override [types] and
// --theme-no-ansi-override [types], toggling whether all 16 explicit
// ansi_palette fields are applied as a group, without discarding any of
// them (config.AnsiColors.OverrideEnabled) -- independent of the tint's own
// --theme-tint/--theme-no-tint switch. A single slot can still be cleared
// individually regardless of this (--ansi-override <slot> none). types is
// optional -- omitted means "all".
func HandleThemeAnsiOverrideOnOff(ctx context.Context, group *CommandGroup) error {
	typesArg := ""
	if len(group.Args) > 0 {
		typesArg = group.Args[0]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	enabled := group.Command == "--theme-ansi-override"

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		c.OverrideEnabled = enabled
	})
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save ANSI override setting: %v", err)
		return err
	}

	if enabled {
		logger.Notice(ctx, "ANSI color overrides enabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	} else {
		logger.Notice(ctx, "ANSI color overrides disabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	}
	return nil
}
