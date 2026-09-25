package classic

import (
	"strconv"
	"strings"
)

// TabStrip is a row of tab labels drawn into a border's title position,
// with overflow arrows when the labels don't all fit. It only tracks labels,
// the active tab, and the scroll position; the owner decides what switching
// tabs means. Render and HitRegions share one layout (Fit), so what's drawn
// and what's clickable never drift apart.
type TabStrip struct {
	// ID prefixes every hit region ID ("<ID>.tab-<n>", "<ID>.tabscroll-left",
	// "<ID>.tabscroll-right").
	ID     string
	Labels []string
	Active int
	// Changed, if set, reports whether tab i has unsaved changes; its label
	// is then drawn between two of the panel's changed indicators, outside
	// the focus indicators.
	Changed func(i int) bool

	scroll int // index of the leftmost visible tab
}

// marker returns the changed marker drawn on both sides of tab i's label,
// or "" when it has no changes.
func (t *TabStrip) marker(i int, ctx StyleContext) string {
	if t.Changed == nil || !t.Changed(i) {
		return ""
	}
	return changedIndicatorChar(ctx.LineCharacters)
}

// styleTag returns tab i's title style tag.
func (t *TabStrip) styleTag(i int) string {
	if i == t.Active {
		return "TitleSubMenuFocused"
	}
	return "TitleSubMenu"
}

// TabStripLayout describes which tabs are visible and where, relative to the
// strip's own start.
type TabStripLayout struct {
	First, Last                   int // inclusive range of visible tab indices
	ShowLeftArrow, ShowRightArrow bool
	TabX                          []int // X offset of each visible tab, one per index in [First, Last]
	TabWidth                      []int // rendered width of each visible tab, one per index in [First, Last]
	ArrowWidth                    int
}

// Fit computes the widest window of whole tabs that fits in availWidth while
// still starting at or before the current scroll position: it grows forward
// from it, then backward with any room left, so extra room always pulls in
// more tabs. The scroll position is normalized to the window's left edge.
func (t *TabStrip) Fit(availWidth int, ctx StyleContext) TabStripLayout {
	const arrowWidth = 1
	n := len(t.Labels)
	if n == 0 {
		return TabStripLayout{ArrowWidth: arrowWidth}
	}
	widths := make([]int, n)
	for i := range t.Labels {
		widths[i] = WidthOfTitleSegment(t.Labels[i], true, ctx) + 2*WidthWithoutZones(t.marker(i, ctx))
	}

	anchor := min(max(t.scroll, 0), n-1)
	rangeWidth := func(lo, hi int) int {
		w := 0
		for i := lo; i <= hi; i++ {
			w += widths[i]
		}
		if lo > 0 {
			w += arrowWidth
		}
		if hi < n-1 {
			w += arrowWidth
		}
		return w
	}

	// Forward first, so the window only reaches back before the scroll
	// position once it runs out of tabs ahead -- otherwise ScrollIntoView's
	// forward steps could be pulled straight back.
	lo, hi := anchor, anchor
	for hi+1 < n && rangeWidth(lo, hi+1) <= availWidth {
		hi++
	}
	for lo-1 >= 0 && rangeWidth(lo-1, hi) <= availWidth {
		lo--
	}
	t.scroll = lo

	layout := TabStripLayout{
		First:          lo,
		Last:           hi,
		ShowLeftArrow:  lo > 0,
		ShowRightArrow: hi < n-1,
		ArrowWidth:     arrowWidth,
	}
	x := 0
	if layout.ShowLeftArrow {
		x = arrowWidth
	}
	for i := lo; i <= hi; i++ {
		layout.TabX = append(layout.TabX, x)
		layout.TabWidth = append(layout.TabWidth, widths[i])
		x += widths[i]
	}
	return layout
}

// ScrollIntoView moves the scroll position as little as possible so the
// active tab is visible at availWidth.
func (t *TabStrip) ScrollIntoView(availWidth int, ctx StyleContext) {
	if len(t.Labels) == 0 {
		return
	}
	layout := t.Fit(availWidth, ctx)
	if t.Active < layout.First {
		t.scroll = t.Active
		return
	}
	for t.Active > layout.Last && t.scroll < t.Active {
		t.scroll++
		layout = t.Fit(availWidth, ctx)
	}
}

// ScrollBy shifts the visible window by delta tabs, clamped to the ends.
func (t *TabStrip) ScrollBy(delta int) {
	t.scroll = min(max(t.scroll+delta, 0), max(len(t.Labels)-1, 0))
}

// Render returns the strip's segments for use as a pre-rendered border
// title (titleTag "RAW"). focused dims or highlights the strip along with
// its border; the active tab is always marked.
func (t *TabStrip) Render(availWidth int, focused bool, ctx StyleContext) string {
	layout := t.Fit(availWidth, ctx)
	leftArrow, rightArrow := "<", ">"
	if ctx.LineCharacters {
		leftArrow, rightArrow = "◂", "▸"
	}
	var segs []string
	if layout.ShowLeftArrow {
		segs = append(segs, leftArrow)
	}
	for i := layout.First; i <= layout.Last; i++ {
		styleTag := t.styleTag(i)
		segs = append(segs, RenderMarkedTitleSegmentCtx(t.Labels[i], t.marker(i, ctx), focused, i == t.Active, true, styleTag, ctx))
	}
	if layout.ShowRightArrow {
		segs = append(segs, rightArrow)
	}
	return strings.Join(segs, "")
}

// HitRegions returns one region per visible tab and arrow, with the strip's
// first column at (x, y). help, if non-nil, supplies each tab's help.
func (t *TabStrip) HitRegions(x, y, availWidth, zOrder int, ctx StyleContext, help func(i int) *HelpContext) []HitRegion {
	layout := t.Fit(availWidth, ctx)
	var regions []HitRegion
	if layout.ShowLeftArrow {
		regions = append(regions, HitRegion{
			ID: t.ID + ".tabscroll-left", X: x, Y: y, Width: layout.ArrowWidth, Height: 1, ZOrder: zOrder,
			Label: "Scroll tabs left",
		})
	}
	for n, i := 0, layout.First; i <= layout.Last; n, i = n+1, i+1 {
		r := HitRegion{
			ID: t.ID + ".tab-" + strconv.Itoa(i), X: x + layout.TabX[n], Y: y, Width: layout.TabWidth[n], Height: 1, ZOrder: zOrder,
			Label: t.Labels[i],
		}
		if help != nil {
			r.Help = help(i)
		}
		regions = append(regions, r)
	}
	if layout.ShowRightArrow {
		last := len(layout.TabX) - 1
		regions = append(regions, HitRegion{
			ID: t.ID + ".tabscroll-right", X: x + layout.TabX[last] + layout.TabWidth[last], Y: y, Width: layout.ArrowWidth, Height: 1, ZOrder: zOrder,
			Label: "Scroll tabs right",
		})
	}
	return regions
}

// TabFromID returns the tab index a hit region ID from HitRegions names.
func (t *TabStrip) TabFromID(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, t.ID+".tab-")
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(rest)
	if err != nil || i < 0 || i >= len(t.Labels) {
		return 0, false
	}
	return i, true
}

// ScrollFromID returns the scroll direction (-1 or 1) an arrow hit region ID
// from HitRegions names.
func (t *TabStrip) ScrollFromID(id string) (int, bool) {
	switch id {
	case t.ID + ".tabscroll-left":
		return -1, true
	case t.ID + ".tabscroll-right":
		return 1, true
	}
	return 0, false
}

// OwnsID reports whether id is one of this strip's hit region IDs.
func (t *TabStrip) OwnsID(id string) bool {
	return strings.HasPrefix(id, t.ID+".tab")
}
