package screens

import (
	"testing"

	"DockSTARTer2/internal/displayengine"
)

func TestTintCursorFor(t *testing.T) {
	row := func(tag, ref string) displayengine.MenuItem {
		return displayengine.MenuItem{Tag: tag, Metadata: map[string]string{"config_value": ref}}
	}
	sep := displayengine.MenuItem{Tag: "Repo", IsSeparator: true}
	// The applied scheme's two rows merged into one, moving later rows up.
	items := []displayengine.MenuItem{
		row("None", ""), sep,
		row("dracula", "repo:base24-dracula"),
		row("nord", "repo:base16-nord"),
	}
	tests := []struct {
		name    string
		focused displayengine.MenuItem
		cursor  int
		want    int
	}{
		{"same tint", row("nord", "repo:base16-nord"), 4, 3},
		{"same scheme, merged row", row("dracula", "repo:base16-dracula"), 3, 2},
		{"gone", row("gruvbox", "repo:base24-gruvbox"), 3, 3},
		{"no ref", displayengine.MenuItem{Tag: "x"}, 1, 1},
	}
	for _, tt := range tests {
		if got := tintCursorFor(items, tt.focused, tt.cursor); got != tt.want {
			t.Errorf("%s: tintCursorFor = %d; want %d", tt.name, got, tt.want)
		}
	}
}
