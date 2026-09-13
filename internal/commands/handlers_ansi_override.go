package commands

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
)

// ansiColorSlotNames lists the 16 standard ANSI color slot names accepted
// by --ansi-override, in the same order as config.AnsiColors.Slots.
var ansiColorSlotNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow",
	"bright-blue", "bright-magenta", "bright-cyan", "bright-white",
}

// ansiColorSlotField returns a pointer to slot's field on c (case-
// insensitive), or nil if slot isn't one of ansiColorSlotNames.
func ansiColorSlotField(c *config.AnsiColors, slot string) *string {
	switch strings.ToLower(slot) {
	case "black":
		return &c.Black
	case "red":
		return &c.Red
	case "green":
		return &c.Green
	case "yellow":
		return &c.Yellow
	case "blue":
		return &c.Blue
	case "magenta":
		return &c.Magenta
	case "cyan":
		return &c.Cyan
	case "white":
		return &c.White
	case "bright-black":
		return &c.BrightBlack
	case "bright-red":
		return &c.BrightRed
	case "bright-green":
		return &c.BrightGreen
	case "bright-yellow":
		return &c.BrightYellow
	case "bright-blue":
		return &c.BrightBlue
	case "bright-magenta":
		return &c.BrightMagenta
	case "bright-cyan":
		return &c.BrightCyan
	case "bright-white":
		return &c.BrightWhite
	default:
		return nil
	}
}

// HandleAnsiOverride implements --ansi-override <slot> <value> [types],
// setting one of the 16 standard ANSI colors directly on
// ansi_palette.<connType> -- independent of, and layered over, any tint
// configured via --tint-repo/--tint-file (see config.AnsiColors.
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
