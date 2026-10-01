package commands

import "testing"

func TestTintSearchMatcher(t *testing.T) {
	dracula := TintEntry{Slug: "dracula", Name: "Dracula", Variant: "dark", Author: "clach04", HasBase16: true, HasBase24: true}
	alucard := TintEntry{Slug: "alucard", Name: "Alucard", Variant: "light", Author: "clach04", Description: "Alucard Classic - Dracula light", HasBase24: true}
	light := TintEntry{Slug: "github", Name: "Github", Variant: "light", Author: "Defman21", HasBase16: true}
	tests := []struct {
		name   string
		search TintSearch
		entry  TintEntry
		want   bool
	}{
		{"whole word", TintSearch{Query: "dracula"}, dracula, true},
		{"partial off", TintSearch{Query: "drac"}, dracula, false},
		{"partial on", TintSearch{Query: "drac", Partial: true}, dracula, true},
		{"partial every term", TintSearch{Query: "drac, zzz", Partial: true}, dracula, false},
		{"description word", TintSearch{Query: "dracula"}, alucard, true},
		{"description partial", TintSearch{Query: "drac", Partial: true}, alucard, true},
		{"description partial off", TintSearch{Query: "drac"}, alucard, false},
		{"description with variant", TintSearch{Query: "dracula", Variant: "dark"}, alucard, false},
		{"variant match", TintSearch{Variant: "dark"}, dracula, true},
		{"variant mismatch", TintSearch{Variant: "light"}, dracula, false},
		{"system base24", TintSearch{System: "base24"}, dracula, true},
		{"system base24 missing", TintSearch{System: "base24"}, light, false},
		{"system contradicts query", TintSearch{Query: "base16", System: "base24"}, dracula, false},
		{"system agrees with query", TintSearch{Query: "base16", System: "base16"}, light, true},
	}
	for _, tt := range tests {
		match, err := tt.search.Matcher()
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := match(tt.entry); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTintEntrySourceURL(t *testing.T) {
	cases := []struct {
		e    TintEntry
		want string
	}{
		{TintEntry{Source: "repo", Slug: "dracula", HasBase16: true, HasBase24: true}, tintSchemeURL("base24", "dracula")},
		{TintEntry{Source: "repo", Slug: "mocha", HasBase16: true}, tintSchemeURL("base16", "mocha")},
		{TintEntry{Source: "user", Slug: "mine", HasBase16: true}, ""},
		{TintEntry{Source: "embedded", Slug: "ansi", HasBase16: true}, ""},
	}
	for _, c := range cases {
		if got := c.e.SourceURL(); got != c.want {
			t.Errorf("%s:%s SourceURL() = %q, want %q", c.e.Source, c.e.Slug, got, c.want)
		}
	}
}
