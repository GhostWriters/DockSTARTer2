package tui

import (
	"testing"

	keybind "charm.land/bubbles/v2/key"
)

func TestSplitTallColumns(t *testing.T) {
	col := func(n int) []keybind.Binding { return make([]keybind.Binding, n) }
	got := splitTallColumns([][]keybind.Binding{col(3), col(2), col(7)})
	var lens []int
	for _, c := range got {
		lens = append(lens, len(c))
	}
	want := []int{3, 2, 3, 3, 1}
	if len(lens) != len(want) {
		t.Fatalf("column heights = %v; want %v", lens, want)
	}
	for i := range want {
		if lens[i] != want[i] {
			t.Fatalf("column heights = %v; want %v", lens, want)
		}
	}
}
