package classic

// RepeatedStops presents a column's Tab stops once per tab of a tab strip
// whose tabs all show that same column (e.g. element tabs over one set of
// controls): Tab past the last stop of one tab shows the next tab and goes
// on at its first stop, and Shift+Tab the reverse. Tab entering from before
// starts at the first tab; Shift+Tab entering from after, at the last.
type RepeatedStops struct {
	*ContentColumn

	// Count returns how many tabs there are, Current which is shown, and
	// Show switches to one.
	Count   func() int
	Current func() int
	Show    func(tab int)
}

var _ SubFocusable = (*RepeatedStops)(nil)

// NewRepeatedStops wraps column; see RepeatedStops.
func NewRepeatedStops(column *ContentColumn, count, current func() int, show func(tab int)) *RepeatedStops {
	return &RepeatedStops{ContentColumn: column, Count: count, Current: current, Show: show}
}

func (r *RepeatedStops) stops() int { return r.ContentColumn.NumTabStops() }

func (r *RepeatedStops) NumTabStops() int { return r.stops() * max(r.Count(), 1) }

func (r *RepeatedStops) SubFocusIndex() int {
	return r.Current()*r.stops() + r.ContentColumn.SubFocusIndex()
}

// SetSubFocusIndex focuses stop i, showing its tab first.
func (r *RepeatedStops) SetSubFocusIndex(i int) {
	n := r.stops()
	if i < 0 || i >= r.NumTabStops() {
		i = r.Current() * n
	}
	if tab := i / n; tab != r.Current() {
		r.Show(tab)
	}
	r.ContentColumn.SetSubFocusIndex(i % n)
}

func (r *RepeatedStops) NextFocusableSub(from int) (int, bool) {
	n, tab := r.stops(), 0
	if from >= 0 {
		tab = from / n
		if j, ok := r.ContentColumn.NextFocusableSub(from % n); ok {
			return tab*n + j, true
		}
		tab++
	}
	for ; tab < r.Count(); tab++ {
		if j, ok := r.ContentColumn.NextFocusableSub(-1); ok {
			return tab*n + j, true
		}
	}
	return -1, false
}

func (r *RepeatedStops) PrevFocusableSub(from int) (int, bool) {
	n, tab := r.stops(), r.Count()-1
	if from < r.NumTabStops() {
		tab = from / n
		if j, ok := r.ContentColumn.PrevFocusableSub(from % n); ok {
			return tab*n + j, true
		}
		tab--
	}
	for ; tab >= 0; tab-- {
		if j, ok := r.ContentColumn.PrevFocusableSub(n); ok {
			return tab*n + j, true
		}
	}
	return -1, false
}

// GroupStop implements GroupJumper: the repeated stops form no groups of
// their own.
func (r *RepeatedStops) GroupStop(int, int) (int, bool) { return -1, false }

// Items repeats the column's Items once per tab; only the shown tab's
// match hit region IDs, so a click never switches tabs.
func (r *RepeatedStops) Items() []Content {
	inner := r.ContentColumn.Items()
	var leaves []Content
	for tab := range max(r.Count(), 1) {
		for _, c := range inner {
			if tab == r.Current() {
				leaves = append(leaves, c)
			} else {
				leaves = append(leaves, unmatchedStop{c})
			}
		}
	}
	return leaves
}

// unmatchedStop is a Tab stop of a tab not shown, which no hit region
// belongs to.
type unmatchedStop struct{ Content }

func (unmatchedStop) MatchesID(string) bool { return false }
