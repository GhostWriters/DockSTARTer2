package commands

import (
	"context"
	"os"
	"strings"

	"DockSTARTer2/internal/paths"
)

// TintEntry is one scheme a tint picker can offer.
type TintEntry struct {
	Source                      string // "embedded", "user", or "repo"
	Slug, Name, Variant, Author string
	HasBase16, HasBase24        bool
}

// Ref returns the config.AnsiElementColors.Tint reference that selects e.
func (e TintEntry) Ref() string { return e.Source + ":" + e.Slug }

// TintRepoCloned reports whether the tinted-theming schemes repo has
// already been downloaded.
func TintRepoCloned() bool {
	_, err := os.Stat(paths.GetTintedThemingSchemesDir())
	return err == nil
}

// DownloadTintRepo downloads the tinted-theming schemes repo if it isn't
// already present.
func DownloadTintRepo(ctx context.Context) error {
	_, err := ensureTintedThemingSchemesRepo(ctx)
	return err
}

// TintCatalog returns every bundled and user scheme, plus the repo's when it
// has already been downloaded (it is never downloaded here), each source
// sorted by slug.
func TintCatalog(ctx context.Context) ([]TintEntry, error) {
	sources := []string{"embedded", "user"}
	if TintRepoCloned() {
		sources = append(sources, "repo")
	}
	var entries []TintEntry
	for _, source := range sources {
		rows, err := tintTableRows(ctx, source, tintFilter{})
		if err != nil {
			return entries, err
		}
		for _, r := range rows {
			entries = append(entries, TintEntry{
				Source: r.Source, Slug: r.Slug, Name: r.Name, Variant: r.Variant, Author: r.Author,
				HasBase16: r.HasBase16, HasBase24: r.HasBase24,
			})
		}
	}
	return entries, nil
}

// TintMatcher parses query the way --tint-list and --tint-table parse their
// search argument and returns whether an entry matches it.
func TintMatcher(query string) (func(TintEntry) bool, error) {
	return TintSearch{Query: query}.Matcher()
}

// TintSearch is a scheme search: Query as --tint-list parses it, narrowed
// to Variant ("light" or "dark") and System ("base16" or "base24") when
// set. Partial matches each term anywhere in a field rather than as whole
// words.
type TintSearch struct {
	Query, Variant, System string
	Partial                bool
}

// Matcher returns whether an entry matches the search.
func (q TintSearch) Matcher() (func(TintEntry) bool, error) {
	f, err := parseTintFilter(q.Query)
	if err != nil {
		return nil, err
	}
	if q.System != "" {
		if f.System != "" && f.System != q.System {
			return func(TintEntry) bool { return false }, nil
		}
		f.System = q.System
	}
	return func(e TintEntry) bool {
		if q.Variant != "" && !strings.EqualFold(e.Variant, q.Variant) {
			return false
		}
		if !f.matchesSystem(e.HasBase16, e.HasBase24) {
			return false
		}
		if q.Partial {
			return f.containsTerms(e.Slug, e.Name, e.Variant, e.Author)
		}
		return f.matchesTerms(e.Slug, e.Name, e.Variant, e.Author)
	}, nil
}
