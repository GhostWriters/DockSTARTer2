package classic

import (
	"testing"
	"time"

	"DockSTARTer2/internal/console"
)

// A list menu held as a content section, as most screens build their item
// list, advances its item spinner through the outer menu's tick.
func TestAdvanceSpinnersReachesContentSections(t *testing.T) {
	ct := console.ActiveConnType()
	prev := console.DisplayFor(ct)
	console.SetConnTypeDisplay(ct, console.ConnTypeDisplay{Spinner: true, SpinnerSpeed: 100, RefreshRate: 100})
	defer console.SetConnTypeDisplay(ct, prev)

	inner := NewMenuModel("inner", "", "", []MenuItem{{Tag: "Item", Selectable: true}})
	outer := NewMenuModel("outer", "Outer", "", nil)
	outer.AddContentSection(inner)

	inner.processingItemIdx = 0
	inner.titleSpinner.Start()

	t0 := time.Now()
	outer.AdvanceSpinners(t0)
	start := inner.titleSpinner.frame
	if !outer.AdvanceSpinners(t0.Add(time.Second)) {
		t.Fatal("outer.AdvanceSpinners reported no change")
	}
	if inner.titleSpinner.frame == start {
		t.Errorf("inner spinner frame stayed %d; want it advanced", start)
	}
}
