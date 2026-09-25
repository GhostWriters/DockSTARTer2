package commands

import (
	"context"
	"os"

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
	f, err := parseTintFilter(query)
	if err != nil {
		return nil, err
	}
	return func(e TintEntry) bool {
		return f.matchesSystem(e.HasBase16, e.HasBase24) && f.matchesTerms(e.Slug, e.Name, e.Variant, e.Author)
	}, nil
}
