package classic

import (
	"strings"
	"testing"
)

func newTestStrip(changed ...int) *TabStrip {
	t := &TabStrip{ID: "strip", Labels: []string{"Local (Current)", "SSH Server", "Web Server", "Fourth", "Fifth Tab"}}
	if len(changed) > 0 {
		t.Changed = func(i int) bool {
			for _, c := range changed {
				if c == i {
					return true
				}
			}
			return false
		}
	}
	return t
}

// TestTabStripRenderMatchesLayout checks that what Render draws is exactly
// as wide as the layout Fit reports, for every width, active tab, and set of
// changed tabs -- the property that keeps HitRegions lined up with the
// drawn tabs.
func TestTabStripRenderMatchesLayout(t *testing.T) {
	ctx := StyleContext{LineCharacters: true, DrawBorders: true}
	for _, changed := range [][]int{nil, {0}, {1, 3}, {0, 1, 2, 3, 4}} {
		for active := range 5 {
			for width := 12; width <= 90; width++ {
				strip := newTestStrip(changed...)
				strip.Active = active
				strip.ScrollIntoView(width, ctx)
				layout := strip.Fit(width, ctx)

				want := layout.TabX[len(layout.TabX)-1] + layout.TabWidth[len(layout.TabWidth)-1]
				if layout.ShowRightArrow {
					want += layout.ArrowWidth
				}
				got := WidthWithoutZones(strip.Render(width, false, ctx))
				if got != want {
					t.Errorf("changed=%v active=%d width=%d: rendered width %d, layout width %d", changed, active, width, got, want)
				}
				if want > width && layout.First != layout.Last {
					t.Errorf("changed=%v active=%d width=%d: layout width %d overflows", changed, active, width, want)
				}
				if active < layout.First || active > layout.Last {
					t.Errorf("changed=%v active=%d width=%d: active tab not visible (showing %d-%d)", changed, active, width, layout.First, layout.Last)
				}
			}
		}
	}
}

// TestTabStripChangedMarkerWidth checks that a changed tab is exactly two
// marker characters wider, and an unchanged one reserves no space for it.
func TestTabStripChangedMarkerWidth(t *testing.T) {
	ctx := StyleContext{LineCharacters: true, DrawBorders: true}
	plain := newTestStrip().Fit(200, ctx)
	marked := newTestStrip(1).Fit(200, ctx)
	for i := range plain.TabWidth {
		want := plain.TabWidth[i]
		if i == 1 {
			want += 2
		}
		if marked.TabWidth[i] != want {
			t.Errorf("tab %d width = %d, want %d", i, marked.TabWidth[i], want)
		}
	}
}

// TestTabStripHitRegionsMatchLayout checks that each tab and arrow region
// sits where Fit places it, offset by the strip's origin.
func TestTabStripHitRegionsMatchLayout(t *testing.T) {
	ctx := StyleContext{LineCharacters: true, DrawBorders: true}
	strip := newTestStrip(2)
	strip.Active = 2
	const width, x, y = 30, 7, 3
	strip.ScrollIntoView(width, ctx)
	layout := strip.Fit(width, ctx)

	var tabs []HitRegion
	var left, right bool
	for _, r := range strip.HitRegions(x, y, width, 5, ctx, nil) {
		if r.Y != y || r.Height != 1 {
			t.Errorf("%s: at y=%d height=%d, want y=%d height=1", r.ID, r.Y, r.Height, y)
		}
		switch {
		case strings.HasSuffix(r.ID, ".tabscroll-left"):
			left = true
			if r.X != x {
				t.Errorf("left arrow at x=%d, want %d", r.X, x)
			}
		case strings.HasSuffix(r.ID, ".tabscroll-right"):
			right = true
		default:
			tabs = append(tabs, r)
		}
	}
	if left != layout.ShowLeftArrow || right != layout.ShowRightArrow {
		t.Errorf("arrows left=%v right=%v, layout says %v/%v", left, right, layout.ShowLeftArrow, layout.ShowRightArrow)
	}
	if len(tabs) != layout.Last-layout.First+1 {
		t.Fatalf("%d tab regions, want %d", len(tabs), layout.Last-layout.First+1)
	}
	for n, r := range tabs {
		i, ok := strip.TabFromID(r.ID)
		if !ok || i != layout.First+n {
			t.Errorf("region %q: tab %d ok=%v, want %d", r.ID, i, ok, layout.First+n)
		}
		if r.X != x+layout.TabX[n] || r.Width != layout.TabWidth[n] {
			t.Errorf("tab %d: x=%d width=%d, want x=%d width=%d", i, r.X, r.Width, x+layout.TabX[n], layout.TabWidth[n])
		}
	}
}

func TestTabStripIDs(t *testing.T) {
	strip := newTestStrip()
	if i, ok := strip.TabFromID("strip.tab-3"); !ok || i != 3 {
		t.Errorf("TabFromID(strip.tab-3) = %d, %v", i, ok)
	}
	for _, id := range []string{"strip.tab-9", "strip.tab--1", "strip.tab-x", "other.tab-1", "strip.tabscroll-left"} {
		if _, ok := strip.TabFromID(id); ok {
			t.Errorf("TabFromID(%q) accepted", id)
		}
	}
	if d, ok := strip.ScrollFromID("strip.tabscroll-left"); !ok || d != -1 {
		t.Errorf("ScrollFromID(left) = %d, %v", d, ok)
	}
	if d, ok := strip.ScrollFromID("strip.tabscroll-right"); !ok || d != 1 {
		t.Errorf("ScrollFromID(right) = %d, %v", d, ok)
	}
	if !strip.OwnsID("strip.tab-0") || !strip.OwnsID("strip.tabscroll-right") || strip.OwnsID("other.tab-0") {
		t.Errorf("OwnsID mismatch")
	}
}
