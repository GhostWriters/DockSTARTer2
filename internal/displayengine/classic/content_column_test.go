package classic

import "testing"

// TestContentColumnSubFocusFollowsNestedChild checks that a column reports
// the stop a nested child moved focus to on its own, not the one it last
// set itself.
func TestContentColumnSubFocusFollowsNestedChild(t *testing.T) {
	inner := NewContentColumn(NewMenuModel("a", "", "", nil), NewMenuModel("b", "", "", nil))
	outer := NewContentColumn(NewMenuModel("first", "", "", nil), inner)
	outer.SetSubFocusIndex(1)

	inner.SetSubFocusIndex(1)

	if got := outer.SubFocusIndex(); got != 2 {
		t.Fatalf("SubFocusIndex() = %d, want 2", got)
	}
	if leaf := outer.Items()[outer.SubFocusIndex()]; leaf.ID() != "b" {
		t.Fatalf("focused leaf = %q, want %q", leaf.ID(), "b")
	}
}
