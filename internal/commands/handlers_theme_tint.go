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
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
)

// parseConnTypeList parses a --tint/--theme-tint/--theme-no-tint command's
// connection-type argument: "all", a single conn type, or a comma-separated
// list of them.
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
// enabled/disabled -- --tint only picks which scheme is configured, not
// whether it's applied; use --theme-tint/--theme-no-tint for that.
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
// ensureTintedThemingSchemesRepo) -- the shared lookup behind the
// "repo:<name>" Tint reference (see ResolveTintRefData and internal/tui's
// resolveTintRef), and its default (a bare, unprefixed name). name may
// carry an explicit "base16-"/"base24-" prefix (tinty's own scheme-ID
// convention) to force which subfolder to read from; without one, prefers
// base24 -- same scheme, but with real distinct bright colors (see
// ParseBase16Scheme's doc comment) instead of base16's fallback of reusing
// the normal color -- falling back to base16 when a scheme has no base24
// counterpart (see repoSchemeSubfolders).
func ResolveRepoTintData(ctx context.Context, name string) ([]byte, error) {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		return nil, err
	}
	slug, subs := repoSchemeSubfolders(name)
	for _, sub := range subs {
		data, err := os.ReadFile(filepath.Join(repoDir, sub, slug+".yaml"))
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("scheme %q not found -- check the name against %s or %s", name,
		"https://github.com/tinted-theming/schemes/tree/spec-0.11/base24",
		"https://github.com/tinted-theming/schemes/tree/spec-0.11/base16")
}

// ResolveTintRefData reads ref's scheme bytes -- see config.AnsiColors.Tint's
// doc comment for the "file:"/"user:"/"embedded:"/"repo:" prefix convention.
// A bare, unprefixed name means "repo:<name>": most schemes come from
// tinted-theming/schemes, DS2's own bundled set is small and rarely what a
// bare name means. desc is a short human-readable label for error/notice
// messages, distinct from ref itself (a plain path or name reads oddly
// prefixed with its own "file:"/"repo:" tag).
func ResolveTintRefData(ctx context.Context, ref string) (data []byte, desc string, err error) {
	switch {
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimPrefix(ref, "file:")
		data, err = os.ReadFile(path)
		return data, "'" + path + "'", err
	case strings.HasPrefix(ref, "user:"):
		name := strings.TrimPrefix(ref, "user:")
		data, err = os.ReadFile(filepath.Join(paths.GetTintsDir(), name+".yaml"))
		return data, "'" + name + "' (user)", err
	case strings.HasPrefix(ref, "embedded:"):
		name := strings.TrimPrefix(ref, "embedded:")
		data, err = assets.GetTintTheme(name)
		if err != nil {
			names, _ := assets.ListTintThemes()
			err = fmt.Errorf("no bundled scheme named %q (available: %s)", name, strings.Join(names, ", "))
		}
		return data, "'" + name + "' (embedded)", err
	case strings.HasPrefix(ref, "repo:"):
		name := strings.TrimPrefix(ref, "repo:")
		data, err = ResolveRepoTintData(ctx, name)
		return data, "'" + name + "'", err
	default:
		data, err = ResolveRepoTintData(ctx, ref)
		return data, "'" + ref + "'", err
	}
}

// canonicalTintRef normalizes ref to always carry an explicit prefix (a bare
// name becomes "repo:<name>", per ResolveTintRefData's doc comment), so
// what's persisted to ansi_palette.<connType>.tint is never ambiguous even
// if the default source were to change later. A "file:" path is also
// resolved to absolute -- same reasoning as HandleTheme's own "file:"
// handling: a relative path is only meaningful relative to wherever this
// command happened to run, and would silently break the next time DS2
// resolves the config from a different working directory (e.g. as a
// daemon).
func canonicalTintRef(ref string) string {
	if path, ok := strings.CutPrefix(ref, "file:"); ok {
		if abs, err := filepath.Abs(path); err == nil {
			return "file:" + abs
		}
		return ref
	}
	for _, p := range []string{"user:", "embedded:", "repo:"} {
		if strings.HasPrefix(ref, p) {
			return ref
		}
	}
	return "repo:" + ref
}

// HandleTintListEmbedded implements --tint-list-embedded, listing the
// base16 scheme names bundled with DS2 (usable with --tint's "embedded:"
// reference).
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
// --tint's "repo:" reference, and its default) -- cloning it on first use,
// same as ResolveRepoTintData itself. A name present in either base24/ or
// base16/ is listed once; ResolveRepoTintData resolves it from whichever of
// the two actually has it, preferring base24.
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

// tintStatusMeta is ref's own scheme metadata for --tint's status display,
// broken into its separate fields rather than one collapsed line -- the raw
// reference itself tells the user little for "user:"/"embedded:"/"repo:"
// names, and nothing extra for "file:" beyond the path they already
// configured. Unreadable is set (and Name left as ref) if the source can't
// be read or parsed, so a broken/missing one is still visible rather than
// silently blank.
type tintStatusMeta struct {
	Name, Slug, Author, Variant string
	Unreadable                  bool
}

// describeTintRef reads ref's (see config.AnsiColors.Tint's doc comment)
// base16 scheme metadata for --tint's status display. Slug is
// "<system>-<slug>" (e.g. "base16-gruvbox-dark") -- tinty's own scheme-ID
// convention (e.g. "tinty apply base16-mocha"), which is also the form
// --tint's "repo:" reference accepts to force a subfolder (see
// repoSchemeSubfolders). Built from the scheme file's own "system" field
// (see config.ParseBase16SchemeMeta) rather than from ref, so it's correct
// even for a "file:"/"user:" source where ref itself carries no base16/
// base24 information at all. Name is the scheme's free-text display name
// (e.g. "Gruvbox Dark"), separate from this machine-readable identifier.
func describeTintRef(ctx context.Context, ref string) tintStatusMeta {
	data, _, err := ResolveTintRefData(ctx, ref)
	if err != nil {
		return tintStatusMeta{Name: ref, Unreadable: true}
	}
	meta, err := config.ParseBase16SchemeMeta(data)
	if err != nil || meta.Name == "" {
		return tintStatusMeta{Name: ref}
	}
	slug := meta.Slug
	if meta.System != "" && slug != "" {
		slug = meta.System + "-" + slug
	}
	return tintStatusMeta{Name: meta.Name, Slug: slug, Author: meta.Author, Variant: meta.Variant}
}

// formatEnabledState returns "enabled"/"disabled" as semstyle tag markup,
// styled with the theme's own Yes/No semantic tags (the same ones a y/n
// prompt answer uses) rather than plain text.
func formatEnabledState(enabled bool) string {
	if enabled {
		return "{{|Yes|}}enabled{{[-]}}"
	}
	return "{{|No|}}disabled{{[-]}}"
}

// formatTintRefSource returns ref (see config.AnsiColors.Tint's doc comment
// for the prefix convention) as hyperlinked semstyle tag markup for --tint's
// status display, when there's somewhere sensible to point it:
//   - "file:<path>" links to the file itself.
//   - "user:<name>" links to the file under the user tint folder, displayed
//     as "user:<name>" the same way console.FormatUserFilePath renders any
//     other user-folder reference.
//   - "repo:<name>" links to the file's canonical location on GitHub
//     (see RepoTintSourceURL) rather than DS2's own local clone -- that
//     clone is an implementation-detail cache, not somewhere a user has
//     reason to open.
//   - "embedded:<name>" has no real, independently-meaningful location to
//     link to (it's compiled into the binary), so it's left as plain text.
func formatTintRefSource(ctx context.Context, ref string) string {
	switch {
	case strings.HasPrefix(ref, "file:"):
		return console.FormatFilePath(strings.TrimPrefix(ref, "file:"))
	case strings.HasPrefix(ref, "user:"):
		name := strings.TrimPrefix(ref, "user:")
		return console.FormatUserFilePath(paths.GetTintsDir(), filepath.Join(paths.GetTintsDir(), name+".yaml"))
	case strings.HasPrefix(ref, "repo:"):
		name := strings.TrimPrefix(ref, "repo:")
		if url, ok := RepoTintSourceURL(ctx, name); ok {
			return console.FormatLink("Var", ref, url)
		}
		return "{{|Var|}}" + ref + "{{[-]}}"
	default:
		return "{{|Var|}}" + ref + "{{[-]}}"
	}
}

// HandleTint implements --tint: no args prints each connection type's
// current tint state (enabled/disabled and scheme, if any) plus whether
// ANSI color overrides are enabled and any that are explicitly set via
// --ansi-override -- a separate mechanism from the tint (see
// HandleAnsiOverride/HandleThemeAnsiOverrideOnOff), shown here too since
// together they determine what actually renders for that connType. <ref>
// [types] sets the tint instead -- see config.AnsiColors.Tint's doc comment
// for the "file:"/"user:"/"embedded:"/"repo:" reference syntax (a bare name
// means "repo:<name>"); types is optional, omitted means "all". <ref>
// "" (an explicitly empty argument) or "none:" clears the tint instead of
// setting one -- not bare "none" (no colon), which stays a valid, if
// unlikely, "repo:none" scheme name instead of being reserved as a keyword.
func HandleTint(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		return handleTintStatus(ctx)
	}

	arg := group.Args[0]
	typesArg := ""
	if len(group.Args) > 1 {
		typesArg = group.Args[1]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	if arg == "" || arg == "none:" {
		conf := config.LoadAppConfig()
		setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
			c.Tint = ""
		})
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Error(ctx, "Failed to save tint setting: %v", err)
			return err
		}
		logger.Notice(ctx, "ANSI palette tint cleared for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
		return nil
	}

	ref := canonicalTintRef(arg)
	data, desc, err := ResolveTintRefData(ctx, ref)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	return applyTintRef(ctx, connTypes, data, ref, desc)
}

func handleTintStatus(ctx context.Context) error {
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
		state := formatEnabledState(row.c.TintEnabled)
		overrideState := formatEnabledState(row.c.OverrideEnabled)
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
		if row.c.Tint == "" {
			logger.Notice(ctx, "\t\t{{|Var|}}(none){{[-]}}")
		} else {
			logger.Notice(ctx, "\t\tSource:  %s", formatTintRefSource(ctx, row.c.Tint))
			meta := describeTintRef(ctx, row.c.Tint)
			if meta.Unreadable {
				logger.Notice(ctx, "\t\t(unreadable)")
			} else {
				logger.Notice(ctx, "\t\tScheme:  {{|Var|}}%s{{[-]}}", meta.Name)
				if meta.Slug != "" {
					logger.Notice(ctx, "\t\tSlug:    {{|Var|}}%s{{[-]}}", meta.Slug)
				}
				if meta.Author != "" {
					logger.Notice(ctx, "\t\tAuthor:  {{|Var|}}%s{{[-]}}", meta.Author)
				}
				if meta.Variant != "" {
					logger.Notice(ctx, "\t\tVariant: {{|Var|}}%s{{[-]}}", meta.Variant)
				}
			}
		}
		logger.Notice(ctx, "\tOverrides (%s):", overrideState)
		logger.Notice(ctx, "\t\t{{|Var|}}%s{{[-]}}", overridesDesc)
	}
	return nil
}
