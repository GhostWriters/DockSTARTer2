package classic

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

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

// TestContentColumnRoutesContainerHitToItsChild checks that a click on a
// nested child's own chrome (here a TabbedPanes tab) reaches that child and
// moves focus to it, even while a sibling holds focus.
func TestContentColumnRoutesContainerHitToItsChild(t *testing.T) {
	panes := NewTabbedPanes("p", []string{"One", "Two"}, []*ContentColumn{
		NewContentColumn(NewMenuModel("one", "", "", nil)),
		NewContentColumn(NewMenuModel("two", "", "", nil)),
	}, PaneLayoutMaximized)
	outer := NewContentColumn(panes, NewMenuModel("options", "", "", nil))
	outer.SetSubFocusIndex(panes.NumTabStops())
	outer.SetSubFocused(true)
	if leaf := outer.Items()[outer.SubFocusIndex()]; leaf.ID() != "options" {
		t.Fatalf("focused leaf = %q, want %q", leaf.ID(), "options")
	}

	outer.Update(LayerHitMsg{ID: panes.Strip.ID + ".tab-1", Button: tea.MouseLeft})

	if panes.Active() != 1 {
		t.Errorf("shown pane = %d, want 1", panes.Active())
	}
	if leaf := outer.Items()[outer.SubFocusIndex()]; leaf.ID() != "two" {
		t.Errorf("focused leaf = %q, want %q", leaf.ID(), "two")
	}
}
