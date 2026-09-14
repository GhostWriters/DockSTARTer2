package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"DockSTARTer2/internal/assets"
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

// applyTintRef validates data as a base16 scheme, then points
// ansi_palette.<connType>.tint at ref (e.g. "embedded:ansi", "repo:dracula",
// "file:/path/to/scheme.yaml") for each of connTypes. Does not touch
// enabled/disabled -- --tint-repo/-embedded/-file only pick which scheme is
// configured, not whether it's applied; use --theme-tint/--theme-no-tint for
// that.
func applyTintRef(ctx context.Context, connTypes []string, data []byte, ref, source string) error {
	if _, err := config.ParseBase16Scheme(data); err != nil {
		return fmt.Errorf("%s does not look like a valid base16 scheme: %w", source, err)
	}

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		c.Tint = ref
	})
	if err := config.SaveAppConfig(conf); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	desc := source
	if d := describeSchemeData(data); d != "" {
		desc = d
	}
	logger.Notice(ctx, "ANSI palette tint set to %s for: {{|Var|}}%s{{[-]}}", desc, strings.Join(connTypes, ", "))
	return nil
}

// ResolveRepoTintData reads a named scheme's bytes from a local clone of
// tinted-theming/schemes (cloned on first use, see
// ensureTintedThemingSchemesRepo) -- the shared lookup behind --tint-repo
// and the "repo:<name>" Tint reference (see internal/tui's resolveTintRef).
// Prefers base24 -- same scheme, but with real distinct bright colors (see
// ParseBase16Scheme's doc comment) instead of base16's fallback of reusing
// the normal color. Not every scheme has a base24 counterpart, so falls
// back to base16 when it doesn't.
func ResolveRepoTintData(ctx context.Context, name string) ([]byte, error) {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(repoDir, "base24", name+".yaml"))
	if err == nil {
		return data, nil
	}
	data, err = os.ReadFile(filepath.Join(repoDir, "base16", name+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("scheme %q not found -- check the name against %s or %s", name,
			"https://github.com/tinted-theming/schemes/tree/spec-0.11/base24",
			"https://github.com/tinted-theming/schemes/tree/spec-0.11/base16")
	}
	return data, nil
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

	data, err := ResolveRepoTintData(ctx, schemeName)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	return applyTintRef(ctx, connTypes, data, "repo:"+schemeName, "'"+schemeName+"'")
}

// HandleThemeTintEmbedded implements --tint-embedded <name> [types],
// applying one of DS2's own bundled schemes (see assets.GetTintTheme) as an
// ANSI palette tint -- kept as its own command rather than folded into
// --tint-repo's lookup so a bundled name never silently shadows (or gets
// shadowed by) a same-named scheme in the real tinted-theming/schemes repo.
// If tinted-theming ever publishes an equivalent scheme upstream, removing
// the bundled file here just makes this command 404 for that name, with
// --tint-repo picking it up from the real repo instead. types is optional
// -- omitted means "all".
func HandleThemeTintEmbedded(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 1 {
		logger.Error(ctx, "Usage: --tint-embedded <name> [local|ssh|web|all|a,b,c]")
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

	data, err := assets.GetTintTheme(schemeName)
	if err != nil {
		names, _ := assets.ListTintThemes()
		err := fmt.Errorf("no bundled scheme named %q (available: %s)", schemeName, strings.Join(names, ", "))
		logger.Error(ctx, "%v", err)
		return err
	}

	return applyTintRef(ctx, connTypes, data, "embedded:"+schemeName, "'"+schemeName+"' (embedded)")
}

// HandleThemeTintUser implements --tint-user <name> [types], applying a
// user-supplied scheme YAML file from paths.GetTintsDir() (see
// GetTintsDir's doc comment) as an ANSI palette tint. types is optional --
// omitted means "all".
func HandleThemeTintUser(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 1 {
		logger.Error(ctx, "Usage: --tint-user <name> [local|ssh|web|all|a,b,c]")
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

	path := filepath.Join(paths.GetTintsDir(), schemeName+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error(ctx, "Reading '{{|File|}}%s{{[-]}}': %v", path, err)
		return err
	}

	return applyTintRef(ctx, connTypes, data, "user:"+schemeName, "'"+schemeName+"' (user)")
}

// HandleTintListEmbedded implements --tint-list-embedded, listing the
// base16 scheme names bundled with DS2 (usable with --tint-embedded).
func HandleTintListEmbedded(ctx context.Context, _ *CommandGroup) error {
	names, err := assets.ListTintThemes()
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

// HandleTintListRepo implements --tint-list-repo, listing the scheme names
// available from the cloned tinted-theming/schemes repo (usable with
// --tint-repo) -- cloning it on first use, same as --tint-repo itself. A
// name present in either base24/ or base16/ is listed once; --tint-repo
// itself resolves it from whichever of the two actually has it, preferring
// base24 (see HandleThemeTintRepo).
func HandleTintListRepo(ctx context.Context, _ *CommandGroup) error {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	seen := make(map[string]bool)
	var names []string
	for _, sub := range []string{"base24", "base16"} {
		entries, err := os.ReadDir(filepath.Join(repoDir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".yaml")
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		err := fmt.Errorf("no schemes found in %q or %q", filepath.Join(repoDir, "base24"), filepath.Join(repoDir, "base16"))
		logger.Error(ctx, "%v", err)
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
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

	return applyTintRef(ctx, connTypes, data, "file:"+path, "'"+path+"'")
}

// HandleThemeTintOnOff implements --theme-tint [types] and --theme-no-tint
// [types], toggling whether an already-configured tint (Tint) is
// applied, without discarding it. Does not affect any --ansi-override
// values, which are a separate mechanism (see HandleThemeAnsiOverrideOnOff
// for their own on/off switch). types is optional on both -- omitted means
// "all".
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

	enabled := group.Command == "--theme-tint"

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		c.TintEnabled = enabled
	})
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save tint setting: %v", err)
		return err
	}

	if enabled {
		logger.Notice(ctx, "ANSI palette tint enabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	} else {
		logger.Notice(ctx, "ANSI palette tint disabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	}
	return nil
}

// HandleThemeCLITintOnOff implements --theme-cli-tint and
// --theme-no-cli-tint, toggling whether a bare, non-interactive CLI
// invocation also renders with local's tint (config.AnsiPaletteConfig.
// ApplyToCLI), in addition to the interactive local TUI (which always
// does). Takes no connType argument -- a bare CLI invocation is always
// local (see cmd.Execute's only caller, main.go).
func HandleThemeCLITintOnOff(ctx context.Context, group *CommandGroup) error {
	enabled := group.Command == "--theme-cli-tint"

	conf := config.LoadAppConfig()
	conf.AnsiColors.ApplyToCLI = enabled
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save CLI tint setting: %v", err)
		return err
	}

	if enabled {
		logger.Notice(ctx, "ANSI palette tint enabled for non-interactive CLI output.")
	} else {
		logger.Notice(ctx, "ANSI palette tint disabled for non-interactive CLI output (the interactive TUI is unaffected).")
	}
	return nil
}

// HandleThemeProgramBoxTintOnOff implements --theme-programbox-tint and
// --theme-no-programbox-tint, toggling whether a ProgramBox dialog's
// streamed command output renders with the session's tint
// (config.AnsiPaletteConfig.ApplyToProgramBox) or with the terminal's own
// native palette instead. Takes no connType argument -- it affects every
// connType's ProgramBox dialogs the same way.
func HandleThemeProgramBoxTintOnOff(ctx context.Context, group *CommandGroup) error {
	enabled := group.Command == "--theme-programbox-tint"

	conf := config.LoadAppConfig()
	conf.AnsiColors.ApplyToProgramBox = enabled
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save ProgramBox tint setting: %v", err)
		return err
	}

	if enabled {
		logger.Notice(ctx, "ANSI palette tint enabled for ProgramBox output.")
	} else {
		logger.Notice(ctx, "ANSI palette tint disabled for ProgramBox output (it will render with the terminal's own palette).")
	}
	return nil
}

// describeSchemeData formats a base16 scheme's own name/author/variant
// metadata (e.g. "ANSI by DockSTARTer2 (dark)") for display, or "" if data
// doesn't parse or has no name.
func describeSchemeData(data []byte) string {
	meta, err := config.ParseBase16SchemeMeta(data)
	if err != nil || meta.Name == "" {
		return ""
	}
	desc := meta.Name
	if meta.Author != "" {
		desc += " by " + meta.Author
	}
	if meta.Variant != "" {
		desc += " (" + meta.Variant + ")"
	}
	return desc
}

// describeTintRef reads ref's (see config.AnsiColors.Tint's doc comment)
// base16 scheme metadata for --tint's status display -- the raw reference
// itself tells the user little for "user:"/"embedded:"/"repo:" names, and
// nothing extra for "file:" beyond the path they already configured. Falls
// back to ref itself if the source can't be read or parsed, so a
// broken/missing one is still visible rather than silently blank.
func describeTintRef(ctx context.Context, ref string) string {
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(ref, "file:"):
		data, err = os.ReadFile(strings.TrimPrefix(ref, "file:"))
	case strings.HasPrefix(ref, "user:"):
		data, err = os.ReadFile(filepath.Join(paths.GetTintsDir(), strings.TrimPrefix(ref, "user:")+".yaml"))
	case strings.HasPrefix(ref, "embedded:"):
		data, err = assets.GetTintTheme(strings.TrimPrefix(ref, "embedded:"))
	case strings.HasPrefix(ref, "repo:"):
		data, err = ResolveRepoTintData(ctx, strings.TrimPrefix(ref, "repo:"))
	default:
		data, err = assets.GetTintTheme(ref)
	}
	if err != nil {
		return ref + " (unreadable)"
	}
	if desc := describeSchemeData(data); desc != "" {
		return desc
	}
	return ref
}

// HandleTintStatus implements --tint, printing each connection type's
// current tint state (enabled/disabled and scheme_file, if any) plus
// whether ANSI color overrides are enabled and any that are explicitly set
// via --ansi-override -- a separate mechanism from the tint (see
// HandleAnsiOverride/HandleThemeAnsiOverrideOnOff), shown here too since
// together they determine what actually renders for that connType.
func HandleTintStatus(ctx context.Context, _ *CommandGroup) error {
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
		state := "disabled"
		if row.c.TintEnabled {
			state = "enabled"
		}
		scheme := "(none)"
		if row.c.Tint != "" {
			scheme = describeTintRef(ctx, row.c.Tint)
		}

		overrideState := "disabled"
		if row.c.OverrideEnabled {
			overrideState = "enabled"
		}
		var overrides []string
		for _, slot := range ansiColorSlotNames {
			if v := *ansiColorSlotField(&row.c, slot); v != "" {
				overrides = append(overrides, slot+"="+v)
			}
		}
		overridesDesc := "(none set)"
		if len(overrides) > 0 {
			overridesDesc = strings.Join(overrides, ", ")
		}

		logger.Notice(ctx, "{{|Var|}}%s:{{[-]}}", row.label)
		logger.Notice(ctx, "\tTint (%s):", state)
		logger.Notice(ctx, "\t\t{{|Var|}}%s{{[-]}}", scheme)
		logger.Notice(ctx, "\tOverrides (%s):", overrideState)
		logger.Notice(ctx, "\t\t{{|Var|}}%s{{[-]}}", overridesDesc)
	}
	return nil
}
