package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
)

// parseConnTypeList parses a --tint-repo/--tint-file/--theme-tint/
// --theme-no-tint command's connection-type argument: "all", a single
// conn type, or a comma-separated list of them.
func parseConnTypeList(s string) ([]string, error) {
	if s == "all" {
		return []string{"local", "ssh", "web"}, nil
	}
	var types []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "local", "ssh", "web":
			if !slices.Contains(types, part) {
				types = append(types, part)
			}
		default:
			return nil, fmt.Errorf("unknown connection type %q (valid: local, ssh, web, all, or a comma-separated list)", part)
		}
	}
	if len(types) == 0 {
		return nil, fmt.Errorf("no connection type given (valid: local, ssh, web, all, or a comma-separated list)")
	}
	return types, nil
}

// parseOptionalConnTypeList is parseConnTypeList, but an empty string (the
// argument wasn't given at all) means "all" -- used by --theme-tint /
// --theme-no-tint, whose connection-type argument is optional.
func parseOptionalConnTypeList(s string) ([]string, error) {
	if s == "" {
		s = "all"
	}
	return parseConnTypeList(s)
}

// tintStateFile returns the path a connType's downloaded/copied scheme file
// is stored at -- one persistent copy per connection type in DS2's state
// dir, independent of wherever the original source (a URL or another local
// path) came from.
func tintStateFile(connType string) string {
	return filepath.Join(paths.GetStateDir(), "ansi_tint_"+connType+".yaml")
}

// setAnsiColorsField applies fn to the AnsiColors for each of connTypes on
// conf, in place.
func setAnsiColorsField(conf *config.AppConfig, connTypes []string, fn func(*config.AnsiColors)) {
	for _, ct := range connTypes {
		switch ct {
		case "local":
			fn(&conf.AnsiColors.Local)
		case "ssh":
			fn(&conf.AnsiColors.SSH)
		case "web":
			fn(&conf.AnsiColors.Web)
		}
	}
}

// applyTint validates data as a base16 scheme, then for each of connTypes:
// copies it to that connection type's state file and points
// ansi_palette.<connType>.scheme_file at it. Does not touch disabled --
// --tint-repo/-file only pick which scheme is configured, not
// whether it's applied; use --theme-tint/--theme-no-tint for that.
func applyTint(ctx context.Context, connTypes []string, data []byte, source string) error {
	if _, err := config.ParseBase16Scheme(data); err != nil {
		return fmt.Errorf("%s does not look like a valid base16 scheme: %w", source, err)
	}

	if err := os.MkdirAll(paths.GetStateDir(), 0700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	conf := config.LoadAppConfig()
	for _, ct := range connTypes {
		dest := tintStateFile(ct)
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return fmt.Errorf("writing tint file for %s: %w", ct, err)
		}
		setAnsiColorsField(&conf, []string{ct}, func(c *config.AnsiColors) {
			c.SchemeFile = tintStateFile(ct)
		})
	}

	if err := config.SaveAppConfig(conf); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	logger.Notice(ctx, "ANSI palette tint set from %s for: {{|Var|}}%s{{[-]}}", source, strings.Join(connTypes, ", "))
	return nil
}

// HandleThemeTintRepo implements --tint-repo <scheme-name> [types],
// resolving a named tinted-theming base16 scheme
// (github.com/tinted-theming/schemes) from a local clone of that repo
// (cloned on first use, see ensureTintedThemingSchemesRepo) and applying
// it as an ANSI palette tint for the given connection type(s). types is
// optional -- omitted means "all".
func HandleThemeTintRepo(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 1 {
		logger.Error(ctx, "Usage: --tint-repo <scheme-name> [local|ssh|web|all|a,b,c]")
		return fmt.Errorf("missing arguments")
	}
	schemeName := group.Args[0]
	typesArg := ""
	if len(group.Args) > 1 {
		typesArg = group.Args[1]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	data, err := os.ReadFile(filepath.Join(repoDir, "base16", schemeName+".yaml"))
	if err != nil {
		err := fmt.Errorf("scheme %q not found -- check the name against %s", schemeName, "https://github.com/tinted-theming/schemes/tree/spec-0.11/base16")
		logger.Error(ctx, "%v", err)
		return err
	}

	return applyTint(ctx, connTypes, data, "'"+schemeName+"'")
}

// HandleThemeTintFile implements --tint-file <path> [types], applying
// a local base16 scheme YAML file as an ANSI palette tint for the given
// connection type(s). types is optional -- omitted means "all".
func HandleThemeTintFile(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 1 {
		logger.Error(ctx, "Usage: --tint-file <path> [local|ssh|web|all|a,b,c]")
		return fmt.Errorf("missing arguments")
	}
	path := group.Args[0]
	typesArg := ""
	if len(group.Args) > 1 {
		typesArg = group.Args[1]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error(ctx, "Reading '{{|File|}}%s{{[-]}}': %v", path, err)
		return err
	}

	return applyTint(ctx, connTypes, data, "'"+path+"'")
}

// HandleThemeTintOnOff implements --theme-tint [types] and --theme-no-tint
// [types], toggling whether an already-configured tint (SchemeFile/the 16
// explicit fields) is applied, without discarding any of it. types is
// optional on both -- omitted means "all".
func HandleThemeTintOnOff(ctx context.Context, group *CommandGroup) error {
	typesArg := ""
	if len(group.Args) > 0 {
		typesArg = group.Args[0]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	disabled := group.Command == "--theme-no-tint"

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		c.Disabled = disabled
	})
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save tint setting: %v", err)
		return err
	}

	if disabled {
		logger.Notice(ctx, "ANSI palette tint disabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	} else {
		logger.Notice(ctx, "ANSI palette tint enabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	}
	return nil
}

// HandleTintStatus implements --tint, printing each connection type's
// current tint state (enabled/disabled and scheme_file, if any).
func HandleTintStatus(_ context.Context, _ *CommandGroup) error {
	conf := config.LoadAppConfig()
	rows := []struct {
		label string
		c     config.AnsiColors
	}{
		{"local", conf.AnsiColors.Local},
		{"ssh", conf.AnsiColors.SSH},
		{"web", conf.AnsiColors.Web},
	}
	for _, row := range rows {
		state := "enabled"
		if row.c.Disabled {
			state = "disabled"
		}
		scheme := row.c.SchemeFile
		if scheme == "" {
			scheme = "(none)"
		}
		fmt.Printf("%-6s %-9s scheme_file: %s\n", row.label+":", state, scheme)
	}
	return nil
}
