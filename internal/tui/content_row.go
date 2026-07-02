package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ContentRow groups Content items to be laid out horizontally as one row.
// It implements Content itself, so row-splitting logic never needs to
// special-case "is this a row or a single section" anywhere in
// calculateSectionLayout/viewWithSections/hit-regions/updateSections -- a row
// is just another content kind whose SetSize/ViewString/GetHitRegions fan out
// to its children. Width is split evenly across children (no weighting yet);
// height is the max of the children's natural heights, and every child is
// stretched to that height. Mixing a variable-height child into a row is
// unsupported -- IsVariableHeight always reports false.
type ContentRow struct {
	items    []Content
	widths   []int // last-computed per-item widths, from the most recent SetSize
	height   int
	subFocus int // index into items with row-internal focus; defaults to 0
}

var _ Content = (*ContentRow)(nil)

// NewContentRow groups the given Content items into a single horizontal row.
func NewContentRow(items ...Content) *ContentRow {
	return &ContentRow{items: items}
}

// Init satisfies tea.Model; ContentRow has no independent init behavior of
// its own -- children are initialized by whatever constructs them.
func (r *ContentRow) Init() tea.Cmd { return nil }

// View satisfies tea.Model.
func (r *ContentRow) View() tea.View { return tea.View{Content: r.ViewString()} }

// SubFocusIndex returns the index of the child currently holding
// row-internal focus.
func (r *ContentRow) SubFocusIndex() int {
	if r.subFocus < 0 || r.subFocus >= len(r.items) {
		return 0
	}
	return r.subFocus
}

// SetSubFocusIndex sets which child holds row-internal focus. Called by
// updateSections' Tab/Shift-Tab cycling, which treats each row child as its
// own Tab stop (see NumTabStops) rather than routing Left/Right within a
// row -- Left/Right at the outer level already means "jump to the button
// row," so a row can't also claim it for intra-row navigation.
func (r *ContentRow) SetSubFocusIndex(i int) {
	if i < 0 || i >= len(r.items) {
		i = 0
	}
	r.subFocus = i
}

// NumTabStops returns how many individual Tab stops this row contributes --
// one per child, so Tab visits Font Size and Refresh Rate as separate stops
// even though they render in the same row.
func (r *ContentRow) NumTabStops() int {
	if len(r.items) == 0 {
		return 1
	}
	return len(r.items)
}

// Items returns the row's child Content items, in left-to-right order.
func (r *ContentRow) Items() []Content {
	return r.items
}

// ItemWidth returns the last-assigned width of the child at index i, or 0 if
// out of range or not yet sized.
func (r *ContentRow) ItemWidth(i int) int {
	if i < 0 || i >= len(r.widths) {
		return 0
	}
	return r.widths[i]
}

// splitWidth divides w evenly across n items, distributing the remainder to
// the first items -- the same idiom calculateSectionLayout uses for
// expandable sections.
func splitWidth(w, n int) []int {
	widths := make([]int, n)
	base := w / n
	remainder := w % n
	for i := range widths {
		widths[i] = base
		if remainder > 0 {
			widths[i]++
			remainder--
		}
	}
	return widths
}

// SectionHeight returns the max of the children's natural heights, each
// measured at its post-split width.
func (r *ContentRow) SectionHeight(width int) int {
	if len(r.items) == 0 {
		return 0
	}
	widths := splitWidth(width, len(r.items))
	maxH := 0
	for i, item := range r.items {
		h := item.SectionHeight(widths[i])
		if h > maxH {
			maxH = h
		}
	}
	return maxH
}

// SectionNaturalWidth returns maxWidth unchanged -- a row's width is always
// determined by the outer layout (children split whatever width the row is
// given, per SetSize's equal-split), not by summing children's own natural
// widths, since equal-split sizing doesn't reserve narrower space per child
// today (see the equal-split-only design note in content_row.go/the
// horizontal-layout plan).
func (r *ContentRow) SectionNaturalWidth(maxWidth int) int {
	return maxWidth
}

// SetSize splits width evenly across children and stretches every child to
// height h.
func (r *ContentRow) SetSize(width, height int) {
	r.height = height
	if len(r.items) == 0 {
		r.widths = nil
		return
	}
	r.widths = splitWidth(width, len(r.items))
	for i, item := range r.items {
		item.SetSize(r.widths[i], height)
	}
}

// ViewString joins the children's rendered views left-to-right into a single
// row block.
func (r *ContentRow) ViewString() string {
	if len(r.items) == 0 {
		return ""
	}
	views := make([]string, len(r.items))
	for i, item := range r.items {
		views[i] = item.ViewString()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, views...)
}

// GetHitRegions accumulates a per-child X offset starting at offsetX,
// incrementing by each child's assigned width; all children share the row's
// single offsetY.
func (r *ContentRow) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion
	childX := offsetX
	for i, item := range r.items {
		regions = append(regions, item.GetHitRegions(childX, offsetY)...)
		if i < len(r.widths) {
			childX += r.widths[i]
		}
	}
	return regions
}

// Update routes messages to the currently sub-focused child. LayerHitMsg/
// LayerWheelMsg matching a specific child's ID moves sub-focus to that child
// before routing (so clicking directly on a non-focused child in the row
// focuses it, independent of Tab-driven sub-focus).
func (r *ContentRow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(r.items) == 0 {
		return r, nil
	}
	switch msg := msg.(type) {
	case LayerHitMsg:
		for i, item := range r.items {
			if item.MatchesID(msg.ID) {
				r.subFocus = i
				break
			}
		}
	case LayerWheelMsg:
		for i, item := range r.items {
			if item.MatchesID(msg.ID) {
				r.subFocus = i
				break
			}
		}
	}
	if r.subFocus < 0 || r.subFocus >= len(r.items) {
		r.subFocus = 0
	}
	updated, cmd := r.items[r.subFocus].Update(msg)
	if c, ok := updated.(Content); ok {
		r.items[r.subFocus] = c
	}
	return r, cmd
}

// SetSubFocused propagates sub-focus to the child currently holding
// row-internal focus (defaulting to the first child), and explicitly
// unfocuses every other child so at most one child in the row ever renders
// as focused.
func (r *ContentRow) SetSubFocused(focused bool) tea.Cmd {
	if len(r.items) == 0 {
		return nil
	}
	if r.subFocus < 0 || r.subFocus >= len(r.items) {
		r.subFocus = 0
	}
	var cmd tea.Cmd
	for i, item := range r.items {
		if i == r.subFocus {
			cmd = item.SetSubFocused(focused)
		} else {
			item.SetSubFocused(false)
		}
	}
	return cmd
}

// SetIsDialog propagates to every child.
func (r *ContentRow) SetIsDialog(isDialog bool) {
	for _, item := range r.items {
		item.SetIsDialog(isDialog)
	}
}

// SetLockedByOthers propagates to every child.
func (r *ContentRow) SetLockedByOthers(locked bool) {
	for _, item := range r.items {
		item.SetLockedByOthers(locked)
	}
}

// IsVariableHeight always returns false -- mixing a variable-height child
// into a row is unsupported/undefined (not needed by any current use case).
func (r *ContentRow) IsVariableHeight() bool { return false }

// Height returns the row's last-assigned height.
func (r *ContentRow) Height() int { return r.height }

// ID returns a synthetic row ID formed from the joined child IDs. Not used
// for hit-region substring matching (see MatchesID) -- only for logging/
// identification purposes.
func (r *ContentRow) ID() string {
	ids := make([]string, len(r.items))
	for i, item := range r.items {
		ids[i] = item.ID()
	}
	return "row-" + strings.Join(ids, "-")
}

// ScrollID returns "" -- a row has no single scrollbar of its own; each
// child manages its own if it has one.
func (r *ContentRow) ScrollID() string { return "" }

// MatchesID reports whether msgID belongs to any child of this row.
func (r *ContentRow) MatchesID(msgID string) bool {
	for _, item := range r.items {
		if item.MatchesID(msgID) {
			return true
		}
	}
	return false
}

// AbsorbMessage delegates to each child, returning the first non-nil cmd.
func (r *ContentRow) AbsorbMessage(msg tea.Msg) tea.Cmd {
	for _, item := range r.items {
		if cmd := item.AbsorbMessage(msg); cmd != nil {
			return cmd
		}
	}
	return nil
}

// ClearProcessingState propagates to every child that supports it, so
// MenuModel.ClearProcessingState's generic interface-assertion recursion
// reaches into a row's children too.
func (r *ContentRow) ClearProcessingState() {
	for _, item := range r.items {
		if cp, ok := item.(interface{ ClearProcessingState() }); ok {
			cp.ClearProcessingState()
		}
	}
}

// IsProcessing reports true if any child is processing.
func (r *ContentRow) IsProcessing() bool {
	for _, item := range r.items {
		if item.IsProcessing() {
			return true
		}
	}
	return false
}

// IsScrollbarDragging reports true if any child is dragging its scrollbar.
func (r *ContentRow) IsScrollbarDragging() bool {
	for _, item := range r.items {
		if d, ok := item.(interface{ IsScrollbarDragging() bool }); ok && d.IsScrollbarDragging() {
			return true
		}
	}
	return false
}

// WantsHorizontalKeys delegates to whichever child currently holds
// row-internal focus.
func (r *ContentRow) WantsHorizontalKeys() bool {
	if len(r.items) == 0 {
		return false
	}
	return r.items[r.SubFocusIndex()].WantsHorizontalKeys()
}

// Focusable reports true if any child is focusable.
func (r *ContentRow) Focusable() bool {
	for _, item := range r.items {
		if item.Focusable() {
			return true
		}
	}
	return false
}
