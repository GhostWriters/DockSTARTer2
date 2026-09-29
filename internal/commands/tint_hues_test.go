package commands

import (
	"slices"
	"testing"
)

func TestBackgroundHues(t *testing.T) {
	tests := []struct {
		name           string
		base00, base01 string
		want           []string
	}{
		{"neutral", "#1d1f21", "#282a2e", []string{"gray"}},
		{"muted lean", "#282a36", "#282a36", []string{"gray", "blue", "purple"}},
		{"saturated", "#002b36", "#073642", []string{"cyan", "blue"}},
		{"saturated green", "#001100", "#003300", []string{"green"}},
		{"no background", "", "", nil},
	}
	for _, tt := range tests {
		if got := backgroundHues(tt.base00, tt.base01); !slices.Equal(got, tt.want) {
			t.Errorf("%s: backgroundHues(%q, %q) = %v, want %v", tt.name, tt.base00, tt.base01, got, tt.want)
		}
	}
}

func TestListedTintHues(t *testing.T) {
	tests := []struct {
		slug string
		want []string
		ok   bool
	}{
		{"dracula", []string{"gray", "purple"}, true},
		{"rose-pine-moon", []string{"gray", "purple", "pink"}, true}, // pattern
		{"rose-pine-dawn", []string{"pink", "orange"}, true},         // exact beats pattern
		{"gruvbox-material-dark-hard", []string{"gray", "yellow", "orange"}, true},
		{"nord", nil, false},
	}
	for _, tt := range tests {
		got, ok := listedTintHues(tt.slug)
		if ok != tt.ok || !slices.Equal(got, tt.want) {
			t.Errorf("listedTintHues(%q) = %v, %v; want %v, %v", tt.slug, got, ok, tt.want, tt.ok)
		}
	}
}
