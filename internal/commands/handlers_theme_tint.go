package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

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

	logger.Notice(ctx, "Applied %s color palette:", tintedThemingLink())
	logger.Notice(ctx, "\t{{|Var|}}%s:{{[-]}}", strings.Join(connTypes, ", "))
	printTintDetails(ctx, ref, "\t\t")
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

// tintListSources is the source prefixes --tint-list/--tint-table accept in
// their optional filter argument, in the order an unfiltered ("list every
// source") call shows them -- "repo:"/"user:"/"embedded:" matches
// canonicalTintRef's own prefix vocabulary; "file:" is excluded, since a
// single file isn't a listable source.
var tintListSources = []string{"repo", "user", "embedded"}

// parseTintSources parses --tint-list/--tint-table's optional source-filter
// argument: a comma-separated list of "repo:"/"user:"/"embedded:" prefixes
// -- the trailing ":" is required so looksLikeTintSourceArg can tell a
// source filter apart from a search term regardless of which of the two
// optional args order it arrives in. "all:" is also accepted, explicitly
// requesting the same set an omitted/empty argument already defaults to
// (tintListSources) -- e.g. to combine with a search term ("--tint-table
// all: dark") without the source itself doing any narrowing.
func parseTintSources(s string) ([]string, error) {
	if s == "" {
		return tintListSources, nil
	}
	var sources []string
	all := false
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		src, ok := strings.CutSuffix(part, ":")
		if !ok || (src != "all" && !slices.Contains(tintListSources, src)) {
			return nil, fmt.Errorf("unknown tint source %q (valid: repo:, user:, embedded:, all:, or a comma-separated list)", part)
		}
		if src == "all" {
			all = true
		} else if !slices.Contains(sources, src) {
			sources = append(sources, src)
		}
	}
	if all {
		return tintListSources, nil
	}
	return sources, nil
}

// looksLikeTintSourceArg reports whether s parses as a source-filter
// argument (see parseTintSources) -- every comma-separated part ends with
// ":". Used to tell --tint-list/--tint-table's source and search args apart
// regardless of how many there are or what order they're given in.
func looksLikeTintSourceArg(s string) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, ",") {
		if !strings.HasSuffix(strings.TrimSpace(part), ":") {
			return false
		}
	}
	return true
}

// splitTintArgs sorts --tint-list/--tint-table's args (any number, order-
// independent) into (sourceArg, searchArg): every arg that
// looksLikeTintSourceArg is joined with "," into the combined source
// filter, and every other arg is joined with "," into the combined search
// term list -- so "repo: user: ayu dark" and "repo:,user: ayu,dark" (or any
// mix of the two) parse identically.
func splitTintArgs(args []string) (sourceArg, searchArg string) {
	var sourceParts, searchParts []string
	for _, arg := range args {
		if looksLikeTintSourceArg(arg) {
			sourceParts = append(sourceParts, arg)
		} else if arg != "" {
			searchParts = append(searchParts, arg)
		}
	}
	return strings.Join(sourceParts, ","), strings.Join(searchParts, ",")
}

// tintFilter is --tint-list/--tint-table's optional search argument: a
// comma-separated list of search terms, all of which must match (AND) --
// e.g. "ayu,dark" or "Kempson,dark" finds a scheme only if every term is
// found somewhere. A bare "base16"/"base24" term narrows to just that
// system (same vocabulary --tint's "repo:" reference accepts as a
// "base16-"/"base24-" prefix) instead of counting as a word to search for.
// An empty tintFilter matches everything.
type tintFilter struct {
	System string   // "", "base16", or "base24"
	Terms  []string // search terms (whole-word, case-insensitive -- see tintSearchWords), AND'd together
}

// parseTintFilter parses s (e.g. "dark", "ayu,base24") into a tintFilter --
// each comma-separated part is classified independently, so a bare
// "base16"/"base24" narrows System regardless of its position among the
// other terms. Errors if both "base16" and "base24" are given -- a scheme
// can't be both, so silently keeping only the last one would hide what's
// really a contradictory, always-empty search rather than accept it.
func parseTintFilter(s string) (tintFilter, error) {
	var f tintFilter
	for _, term := range strings.Split(s, ",") {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "base16" || term == "base24" {
			if f.System != "" && f.System != term {
				return tintFilter{}, fmt.Errorf("search terms %q and %q are contradictory -- a scheme can't be both", f.System, term)
			}
			f.System = term
			continue
		}
		if term != "" {
			f.Terms = append(f.Terms, term)
		}
	}
	return f, nil
}

// tintSearchWords splits s into lowercase words on runs of anything that
// isn't a letter or digit (spaces, "-", ",", parens, etc.), so search
// matches whole words rather than any substring -- otherwise a term like
// "ayu" would false-positive match inside an unrelated author name like
// "Mayush".
func tintSearchWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// containsWordSequence reports whether seq appears as a contiguous,
// in-order run within words (both already lowercased).
func containsWordSequence(words, seq []string) bool {
	if len(seq) == 0 {
		return true
	}
	for start := 0; start+len(seq) <= len(words); start++ {
		match := true
		for j, w := range seq {
			if words[start+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// matchesTerms reports whether every one of f's search terms is found in
// at least one of fields. A single-word term matches if that word appears
// anywhere in the field (see tintSearchWords); a term that's itself
// multiple words (e.g. "ayu-dark", from a term with an internal "-" or
// space) matches only where those words appear together, in order, in the
// *same* field -- so "ayu-dark" finds the scheme literally named that,
// rather than any scheme with "ayu" and "dark" somewhere unrelated (that
// looser search is still available by giving "ayu" and "dark" as separate
// terms, e.g. "ayu,dark"). True (vacuously) when f has no terms.
func (f tintFilter) matchesTerms(fields ...string) bool {
	if len(f.Terms) == 0 {
		return true
	}
	var fieldWords [][]string
	for _, field := range fields {
		fieldWords = append(fieldWords, tintSearchWords(field))
	}
	for _, term := range f.Terms {
		termWords := tintSearchWords(term)
		found := false
		for _, words := range fieldWords {
			if containsWordSequence(words, termWords) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// matchesSystem reports whether f's optional system constraint is satisfied
// by a source available in hasBase16/hasBase24.
func (f tintFilter) matchesSystem(hasBase16, hasBase24 bool) bool {
	switch f.System {
	case "base16":
		return hasBase16
	case "base24":
		return hasBase24
	default:
		return true
	}
}

// matchesLabel reports whether f matches label (a tintListLabels entry,
// "<system>-<slug>" or a bare slug for a source with no system prefix) --
// searched by slug alone, since a label carries no other metadata.
func (f tintFilter) matchesLabel(label string) bool {
	system, slug := "", label
	if v, ok := strings.CutPrefix(label, "base24-"); ok {
		system, slug = "base24", v
	} else if v, ok := strings.CutPrefix(label, "base16-"); ok {
		system, slug = "base16", v
	}
	if f.System != "" && f.System != system {
		return false
	}
	return f.matchesTerms(slug)
}

// tintFileLabel returns name as "<system>-<name>" using data's own declared
// "system:" field, or the bare name if it can't be parsed.
func tintFileLabel(name string, data []byte) string {
	meta, err := config.ParseBase16SchemeMeta(data)
	if err != nil || meta.System == "" {
		return name
	}
	return meta.System + "-" + name
}

// embeddedTintLabel is tintFileLabel for one of DS2's own bundled schemes.
func embeddedTintLabel(name string) string {
	data, err := assets.GetTintTheme(name)
	if err != nil {
		return name
	}
	return tintFileLabel(name, data)
}

// dirTintFileLabels returns every *.yaml file directly under dir as
// tintFileLabel(name, data) -- for a flat scheme folder (the user tint
// folder) with no base16/base24 subfolder split. A missing dir is not an
// error (no user schemes yet).
func dirTintFileLabels(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var labels []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		label := name
		if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			label = tintFileLabel(name, data)
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// repoTintFileLabels returns every scheme file in the cloned
// tinted-theming/schemes repo as "<system>-<slug>" -- --tint's own
// scheme-ID form to force a subfolder (see repoSchemeSubfolders). A slug
// present in both base24/ and base16/ yields two labels, since both are
// real, distinct files.
func repoTintFileLabels(ctx context.Context) ([]string, error) {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		return nil, err
	}
	var labels []string
	for _, sub := range []string{"base24", "base16"} {
		entries, err := os.ReadDir(filepath.Join(repoDir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".yaml")
			labels = append(labels, sub+"-"+slug)
		}
	}
	return labels, nil
}

// tintListLabels returns source's file-level labels (see
// repoTintFileLabels/dirTintFileLabels/embeddedTintLabel) for --tint-list.
func tintListLabels(ctx context.Context, source string) ([]string, error) {
	switch source {
	case "repo":
		return repoTintFileLabels(ctx)
	case "user":
		return dirTintFileLabels(paths.GetTintsDir())
	case "embedded":
		names, err := assets.ListTintThemes()
		if err != nil {
			return nil, err
		}
		labels := make([]string, 0, len(names))
		for _, name := range names {
			labels = append(labels, embeddedTintLabel(name))
		}
		return labels, nil
	default:
		return nil, fmt.Errorf("unknown tint source %q", source)
	}
}

// HandleTintList implements --tint-list, taking any number of optional
// args in any order (see splitTintArgs): a "<repo:|user:|embedded:|all:>[,...]"
// source filter (every source when omitted -- see parseTintSources) and a
// search term (see parseTintFilter). Lists scheme names as
// "<system>-<name>", narrowed to slugs containing every search term (see
// tintFilter.matchesTerms; a label carries no other metadata to search by,
// unlike --tint-table's rows) -- a bare "base16"/"base24" term narrows by
// system instead. When more than one source is listed, each label is
// further prefixed "<source>:" (e.g. "repo:base16-mocha") to stay
// unambiguous; a single source's output has no such prefix.
func HandleTintList(ctx context.Context, group *CommandGroup) error {
	sourceArg, filterArg := splitTintArgs(group.Args)
	sources, err := parseTintSources(sourceArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	filter, err := parseTintFilter(filterArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	var labels []string
	for _, source := range sources {
		sourceLabels, err := tintListLabels(ctx, source)
		if err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		for _, label := range sourceLabels {
			if !filter.matchesLabel(label) {
				continue
			}
			if len(sources) > 1 {
				label = source + ":" + label
			}
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		err := fmt.Errorf("no schemes found for source(s): %s", strings.Join(sources, ", "))
		logger.Error(ctx, "%v", err)
		return err
	}
	slices.Sort(labels)
	for _, label := range labels {
		fmt.Println(label)
	}
	return nil
}

// tintTableRow is one --tint-table row: a slug-level view (unlike
// tintListLabels' file-level one) -- a repo slug present in both base16/
// and base24/ is one row with both availability flags set, not two rows.
type tintTableRow struct {
	Source               string
	Slug, Name, Variant  string
	HasBase16, HasBase24 bool
}

// repoTintTableRows builds one tintTableRow per distinct repo slug matching
// filter, its base16/base24 availability flags reflecting which
// subfolder(s) actually have that slug, and Name/Variant read from
// whichever format is preferred (base24, falling back to base16 -- see
// ParseBase16Scheme's doc comment). filter.Terms are matched against the
// slug, Name, Variant, and Author (see config.Base16SchemeMeta) -- Author
// isn't kept on the row (see tintTableRow's doc comment), just checked here
// while it's already in scope from parsing the scheme's metadata.
func repoTintTableRows(ctx context.Context, filter tintFilter) ([]tintTableRow, error) {
	repoDir, err := ensureTintedThemingSchemesRepo(ctx)
	if err != nil {
		return nil, err
	}
	availability := make(map[string]struct{ base16, base24 bool })
	for _, sub := range []string{"base16", "base24"} {
		entries, err := os.ReadDir(filepath.Join(repoDir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".yaml")
			a := availability[slug]
			if sub == "base16" {
				a.base16 = true
			} else {
				a.base24 = true
			}
			availability[slug] = a
		}
	}
	slugs := make([]string, 0, len(availability))
	for slug := range availability {
		slugs = append(slugs, slug)
	}
	slices.Sort(slugs)

	var rows []tintTableRow
	for _, slug := range slugs {
		a := availability[slug]
		if !filter.matchesSystem(a.base16, a.base24) {
			continue
		}
		row := tintTableRow{Source: "repo", Slug: slug, HasBase16: a.base16, HasBase24: a.base24}
		author := ""
		if data, err := ResolveRepoTintData(ctx, slug); err == nil {
			if meta, err := config.ParseBase16SchemeMeta(data); err == nil {
				row.Name, row.Variant, author = meta.Name, meta.Variant, meta.Author
			}
		}
		if filter.matchesTerms(row.Slug, row.Name, row.Variant, author) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// dirTintTableRows builds one tintTableRow per *.yaml file directly under
// dir matching filter (a flat scheme folder, so exactly one of
// HasBase16/HasBase24 is set, from that file's own declared "system:"
// field), sorted by slug. See repoTintTableRows's doc comment for how
// filter.Terms are matched against metadata not kept on the row. A missing
// dir is not an error (no user schemes yet).
func dirTintTableRows(dir, source string, filter tintFilter) ([]tintTableRow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rows []tintTableRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		row := tintTableRow{Source: source, Slug: strings.TrimSuffix(e.Name(), ".yaml")}
		author := ""
		if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			if meta, err := config.ParseBase16SchemeMeta(data); err == nil {
				row.Name, row.Variant, author = meta.Name, meta.Variant, meta.Author
				if meta.System == "base24" {
					row.HasBase24 = true
				} else {
					row.HasBase16 = true
				}
			}
		}
		if filter.matchesSystem(row.HasBase16, row.HasBase24) && filter.matchesTerms(row.Slug, row.Name, row.Variant, author) {
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, func(a, b tintTableRow) int { return strings.Compare(a.Slug, b.Slug) })
	return rows, nil
}

// embeddedTintTableRows is repoTintTableRows/dirTintTableRows for DS2's own
// bundled schemes -- a flat set (exactly one of HasBase16/HasBase24 set,
// from each file's own declared "system:" field), read from the embedded
// filesystem rather than a real directory.
func embeddedTintTableRows(filter tintFilter) ([]tintTableRow, error) {
	names, err := assets.ListTintThemes()
	if err != nil {
		return nil, err
	}
	var rows []tintTableRow
	for _, name := range names {
		row := tintTableRow{Source: "embedded", Slug: name}
		author := ""
		if data, err := assets.GetTintTheme(name); err == nil {
			if meta, err := config.ParseBase16SchemeMeta(data); err == nil {
				row.Name, row.Variant, author = meta.Name, meta.Variant, meta.Author
				if meta.System == "base24" {
					row.HasBase24 = true
				} else {
					row.HasBase16 = true
				}
			}
		}
		if filter.matchesSystem(row.HasBase16, row.HasBase24) && filter.matchesTerms(row.Slug, row.Name, row.Variant, author) {
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, func(a, b tintTableRow) int { return strings.Compare(a.Slug, b.Slug) })
	return rows, nil
}

// tintTableRows returns source's tintTableRow slice, matching filter, for
// --tint-table.
func tintTableRows(ctx context.Context, source string, filter tintFilter) ([]tintTableRow, error) {
	switch source {
	case "repo":
		return repoTintTableRows(ctx, filter)
	case "user":
		return dirTintTableRows(paths.GetTintsDir(), "user", filter)
	case "embedded":
		return embeddedTintTableRows(filter)
	default:
		return nil, fmt.Errorf("unknown tint source %q", source)
	}
}

// HandleTintTable implements --tint-table, taking any number of optional
// args in any order (see splitTintArgs): a "<repo:|user:|embedded:|all:>[,...]"
// source filter (every source when omitted -- see parseTintSources) and a
// search term (see parseTintFilter, tintFilter.matchesTerms). Shows a
// Slug/Scheme/Variant/base16/base24 table (Author is omitted as a column --
// its GitHub-profile links push most rows well past a normal terminal
// width -- but is still searchable, since search checks every field a
// scheme's metadata has, not just what's displayed). A Source column is
// added only when more than one source is selected; a single source's
// table has no such column.
func HandleTintTable(ctx context.Context, group *CommandGroup) error {
	sourceArg, filterArg := splitTintArgs(group.Args)
	sources, err := parseTintSources(sourceArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	filter, err := parseTintFilter(filterArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	var rows []tintTableRow
	for _, source := range sources {
		sourceRows, err := tintTableRows(ctx, source, filter)
		if err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		rows = append(rows, sourceRows...)
	}
	if len(rows) == 0 {
		err := fmt.Errorf("no schemes found for source(s): %s", strings.Join(sources, ", "))
		logger.Error(ctx, "%v", err)
		return err
	}

	multi := len(sources) > 1
	if multi {
		slices.SortFunc(rows, func(a, b tintTableRow) int {
			if c := strings.Compare(a.Source, b.Source); c != 0 {
				return c
			}
			return strings.Compare(a.Slug, b.Slug)
		})
	}

	headers := []string{"Slug", "Scheme", "Variant", "base16", "base24"}
	if multi {
		headers = append([]string{"Source"}, headers...)
	}
	var data []string
	for _, row := range rows {
		col16, col24 := "", ""
		if row.HasBase16 {
			col16 = "base16"
		}
		if row.HasBase24 {
			col24 = "base24"
		}
		if multi {
			data = append(data, row.Source, row.Slug, row.Name, row.Variant, col16, col24)
		} else {
			data = append(data, row.Slug, row.Name, row.Variant, col16, col24)
		}
	}

	console.PrintTableCtx(ctx, headers, data, true)
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

// printTintDetails prints ref's Source/Scheme/Slug/Author/Variant lines (or
// an "(unreadable)" note), each prefixed with indent.
func printTintDetails(ctx context.Context, ref, indent string) {
	logger.Notice(ctx, "%sSource:  %s", indent, formatTintRefSource(ctx, ref))
	meta := describeTintRef(ctx, ref)
	if meta.Unreadable {
		logger.Notice(ctx, "%s(unreadable)", indent)
		return
	}
	if meta.Slug != "" {
		logger.Notice(ctx, "%sSlug:    {{|Var|}}%s{{[-]}}", indent, meta.Slug)
	}
	logger.Notice(ctx, "%sScheme:  {{|Var|}}%s{{[-]}}", indent, meta.Name)
	if meta.Author != "" {
		logger.Notice(ctx, "%sAuthor:  {{|Var|}}%s{{[-]}}", indent, meta.Author)
	}
	if meta.Variant != "" {
		logger.Notice(ctx, "%sVariant: {{|Var|}}%s{{[-]}}", indent, meta.Variant)
	}
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

// tintedThemingLink returns "tinted-theming" as an ApplicationName-styled
// hyperlink to the schemes repo, for display anywhere DS2 refers to the
// project by name.
func tintedThemingLink() string {
	return console.FormatLink("ApplicationName", "tinted-theming", "https://github.com/"+tintedThemingSchemesRepo)
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
			printTintDetails(ctx, row.c.Tint, "\t\t")
		}
		logger.Notice(ctx, "\tOverrides (%s):", overrideState)
		logger.Notice(ctx, "\t\t{{|Var|}}%s{{[-]}}", overridesDesc)
	}
	return nil
}
