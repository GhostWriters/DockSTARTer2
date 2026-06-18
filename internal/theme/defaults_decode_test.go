package theme

import (
	"testing"

	"DockSTARTer2/internal/semstyle/theme"
)

// TestFileDefaultsDecode verifies the opaque [defaults] table round-trips into the typed
// ThemeDefaults across bool, int, and string fields (guards against int64/int mismatches
// from the TOML decoder feeding mapstructure).
func TestFileDefaultsDecode(t *testing.T) {
	data := []byte(`
[metadata]
name = "t"
[defaults]
borders = true
spinner = false
shadow_level = 3
border_color = 5
dialog_title_align = "center"
panel_remote = "log"
[styles]
Foo = "{{[red]}}"
`)
	tf, err := semtheme.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d, err := FileDefaults(tf)
	if err != nil {
		t.Fatalf("FileDefaults: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil defaults")
	}
	if d.Borders == nil || !*d.Borders {
		t.Errorf("Borders = %v, want true", d.Borders)
	}
	if d.Spinner == nil || *d.Spinner {
		t.Errorf("Spinner = %v, want false", d.Spinner)
	}
	if d.ShadowLevel == nil || *d.ShadowLevel != 3 {
		t.Errorf("ShadowLevel = %v, want 3", d.ShadowLevel)
	}
	if d.BorderColor == nil || *d.BorderColor != 5 {
		t.Errorf("BorderColor = %v, want 5", d.BorderColor)
	}
	if d.DialogTitleAlign == nil || *d.DialogTitleAlign != "center" {
		t.Errorf("DialogTitleAlign = %v, want center", d.DialogTitleAlign)
	}
	if d.PanelRemote == nil || *d.PanelRemote != "log" {
		t.Errorf("PanelRemote = %v, want log", d.PanelRemote)
	}
	// Fields not present must stay nil (unset).
	if d.LargeButtons != nil {
		t.Errorf("LargeButtons = %v, want nil (unset)", d.LargeButtons)
	}
}
