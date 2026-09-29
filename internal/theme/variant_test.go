package theme

import (
	"testing"

	semtheme "github.com/GhostWriters/semstyle/theme"
)

func TestVariant(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantVariant  string
		wantDetected bool
	}{
		{"declared", "[metadata]\nvariant = \"light\"\n[styles]\nDialog = \"{{[white:black]}}\"\n", VariantLight, false},
		{"tint slots", "[styles]\nDialog = \"{{[base05:base00]}}\"\n", VariantTinted, true},
		{"dark background", "[styles]\nDialog = \"{{[white:blue]}}\"\n", VariantDark, true},
		{"light background", "[styles]\nDialog = \"{{[black:cyan]}}\"\n", VariantLight, true},
		{"hex background", "[styles]\nDialog = \"{{[#000000:#f0f0f0]}}\"\n", VariantLight, true},
		{"reverse swaps", "[styles]\nDialog = \"{{[white:black:R]}}\"\n", VariantLight, true},
		{"terminal background", "[styles]\nDialog = \"{{[-:-]}}\"\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf, err := semtheme.Parse([]byte(tt.data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			v, detected := Variant(tf)
			if v != tt.wantVariant || detected != tt.wantDetected {
				t.Errorf("Variant() = %q, %v; want %q, %v", v, detected, tt.wantVariant, tt.wantDetected)
			}
		})
	}
}
