package classic

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ContentColumn groups Content items to be stacked vertically as one unit --
// the vertical counterpart to ContentRow, for pairing with it inside a
// ContentRow (e.g. a column of stacked sections beside a single section).
// Width is passed through unchanged to every child; fixed-height children
// keep their natural height and expandable children split whatever height
// remains (see SetSize).
type ContentColumn struct {
	items    []Content
	heights  []int // last-computed per-item heights, from the most recent SetSize
	width    int
	subFocus int // index into items with column-internal focus; defaults to 0
}

var _ Content = (*ContentColumn)(nil)

// NewContentColumn groups the given Content items into a single vertical column.
func NewContentColumn(items ...Content) *ContentColumn {
	return &ContentColumn{items: items}
}

// Init satisfies tea.Model; ContentColumn has no independent init behavior of
// its own -- children are initialized by whatever constructs them.
func (c *ContentColumn) Init() tea.Cmd { return nil }

// View satisfies tea.Model.
func (c *ContentColumn) View() tea.View { return tea.View{Content: c.ViewString()} }

// SubFocusIndex returns the index of the child currently holding
// column-internal focus.
func (c *ContentColumn) SubFocusIndex() int {
	if c.subFocus < 0 || c.subFocus >= len(c.items) {
		return 0
	}
	return c.subFocus
}

// SetSubFocusIndex sets which child holds column-internal focus.
func (c *ContentColumn) SetSubFocusIndex(i int) {
	if i < 0 || i >= len(c.items) {
		i = 0
	}
	c.subFocus = i
}

// NumTabStops returns how many individual Tab stops this column contributes --
// one per child, mirroring ContentRow's convention.
func (c *ContentColumn) NumTabStops() int {
	if len(c.items) == 0 {
		return 1
	}
	return len(c.items)
}

// NextFocusableSub returns the next child index after from that is
// Focusable(), or (-1, false) if none remains.
func (c *ContentColumn) NextFocusableSub(from int) (int, bool) {
	for i := from + 1; i < len(c.items); i++ {
		if c.items[i].Focusable() {
			return i, true
		}
	}
	return -1, false
}

// PrevFocusableSub returns the previous child index before from that is
// Focusable(), or (-1, false) if none remains.
func (c *ContentColumn) PrevFocusableSub(from int) (int, bool) {
	for i := from - 1; i >= 0; i-- {
		if c.items[i].Focusable() {
			return i, true
		}
	}
	return -1, false
}

// Items returns the column's child Content items, in top-to-bottom order.
func (c *ContentColumn) Items() []Content {
	return c.items
}

// SectionHeight returns the sum of the children's natural heights, each
// measured at the column's full width.
// SectionHeight returns the sum of fixed children's natural heights plus the
// sum of expandable children's own natural heights (mirroring
// calculateSectionLayout's fixedTotal + expandableNaturalTotal) -- i.e. the
// column's natural, unstretched total height.
func (c *ContentColumn) SectionHeight(width int) int {
	total := 0
	for _, item := range c.items {
		total += item.SectionHeight(width)
	}
	return total
}

// SectionNaturalWidth returns the max of the children's natural widths, since
// the column itself is only as wide as its widest child needs to be.
func (c *ContentColumn) SectionNaturalWidth(maxWidth int) int {
	natural := 0
	for _, item := range c.items {
		if w := item.SectionNaturalWidth(maxWidth); w > natural {
			natural = w
		}
	}
	if natural == 0 || natural > maxWidth {
		return maxWidth
	}
	return natural
}

// SetSize passes width through unchanged to every child. Fixed-height
// children (IsVariableHeight() == false) get their own natural height;
// remaining height is split evenly across expandable children (mirroring
// calculateSectionLayout's Pass 1/Pass 2) -- e.g. a column of a fixed header,
// an expandable list, and a fixed footer gives the list whatever height is
// left over, not an even three-way split. A column with no expandable child
// just gives every child its own natural height, same as SectionHeight's sum.
func (c *ContentColumn) SetSize(width, height int) {
	c.width = width
	if len(c.items) == 0 {
		c.heights = nil
		return
	}

	natural := make([]int, len(c.items))
	fixedTotal := 0
	var expandable []int
	for i, item := range c.items {
		natural[i] = item.SectionHeight(width)
		if item.IsVariableHeight() {
			expandable = append(expandable, i)
			continue
		}
		fixedTotal += natural[i]
	}

	c.heights = make([]int, len(c.items))
	if len(expandable) == 0 {
		copy(c.heights, natural)
	} else {
		remaining := height - fixedTotal
		if remaining < 0 {
			remaining = 0
		}
		perExpandable := SplitWidth(remaining, len(expandable))
		ei := 0
		for i := range c.items {
			if c.items[i].IsVariableHeight() {
				c.heights[i] = perExpandable[ei]
				ei++
			} else {
				c.heights[i] = natural[i]
			}
		}
	}

	for i, item := range c.items {
		item.SetSize(width, c.heights[i])
	}
}

// ViewString joins the children's rendered views top-to-bottom into a single
// column block.
func (c *ContentColumn) ViewString() string {
	if len(c.items) == 0 {
		return ""
	}
	views := make([]string, len(c.items))
	for i, item := range c.items {
		views[i] = item.ViewString()
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

// GetHitRegions accumulates a per-child Y offset starting at offsetY,
// incrementing by each child's assigned height; all children share the
// column's single offsetX.
func (c *ContentColumn) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion
	childY := offsetY
	for i, item := range c.items {
		regions = append(regions, item.GetHitRegions(offsetX, childY)...)
		if i < len(c.heights) {
			childY += c.heights[i]
		}
	}
	return regions
}

// Update routes messages to the currently sub-focused child, mirroring
// ContentRow.Update.
func (c *ContentColumn) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(c.items) == 0 {
		return c, nil
	}
	focusCmd := resolveSubFocusHit(c.items, &c.subFocus, msg, c.SetSubFocused)
	if c.subFocus < 0 || c.subFocus >= len(c.items) {
		c.subFocus = 0
	}
	updated, cmd := c.items[c.subFocus].Update(msg)
	if updatedContent, ok := updated.(Content); ok {
		c.items[c.subFocus] = updatedContent
	}
	return c, tea.Batch(focusCmd, cmd)
}

// SetSubFocused propagates sub-focus to the child currently holding
// column-internal focus, unfocusing every other child.
func (c *ContentColumn) SetSubFocused(focused bool) tea.Cmd {
	if len(c.items) == 0 {
		return nil
	}
	if c.subFocus < 0 || c.subFocus >= len(c.items) {
		c.subFocus = 0
	}
	var cmd tea.Cmd
	for i, item := range c.items {
		if i == c.subFocus {
			cmd = item.SetSubFocused(focused)
		} else {
			item.SetSubFocused(false)
		}
	}
	return cmd
}

// SetIsDialog propagates to every child.
func (c *ContentColumn) SetIsDialog(isDialog bool) {
	for _, item := range c.items {
		item.SetIsDialog(isDialog)
	}
}

// SetLockedByOthers propagates to every child.
func (c *ContentColumn) SetLockedByOthers(locked bool) {
	for _, item := range c.items {
		item.SetLockedByOthers(locked)
	}
}

// IsVariableHeight reports true if any child is variable-height, so a
// ContentColumn nested inside another expandable-aware container (a row, or
// an outer MenuModel's own section list) is itself treated as expandable.
func (c *ContentColumn) IsVariableHeight() bool {
	for _, item := range c.items {
		if item.IsVariableHeight() {
			return true
		}
	}
	return false
}

// Height returns the sum of the column's last-assigned child heights.
func (c *ContentColumn) Height() int {
	total := 0
	for _, h := range c.heights {
		total += h
	}
	return total
}

// ID returns a synthetic column ID formed from the joined child IDs. Not
// used for hit-region substring matching (see MatchesID) -- only for
// logging/identification purposes.
func (c *ContentColumn) ID() string {
	ids := make([]string, len(c.items))
	for i, item := range c.items {
		ids[i] = item.ID()
	}
	return "column-" + strings.Join(ids, "-")
}

// ScrollID returns "" -- a column has no single scrollbar of its own; each
// child manages its own if it has one.
func (c *ContentColumn) ScrollID() string { return "" }

// MatchesID reports whether msgID belongs to any child of this column.
func (c *ContentColumn) MatchesID(msgID string) bool {
	for _, item := range c.items {
		if item.MatchesID(msgID) {
			return true
		}
	}
	return false
}

// AbsorbMessage delegates to each child, returning the first non-nil cmd.
func (c *ContentColumn) AbsorbMessage(msg tea.Msg) tea.Cmd {
	for _, item := range c.items {
		if cmd := item.AbsorbMessage(msg); cmd != nil {
			return cmd
		}
	}
	return nil
}

// ClearProcessingState propagates to every child that supports it.
func (c *ContentColumn) ClearProcessingState() {
	for _, item := range c.items {
		if cp, ok := item.(interface{ ClearProcessingState() }); ok {
			cp.ClearProcessingState()
		}
	}
}

// IsProcessing reports true if any child is processing.
func (c *ContentColumn) IsProcessing() bool {
	for _, item := range c.items {
		if item.IsProcessing() {
			return true
		}
	}
	return false
}

// IsScrollbarDragging reports true if any child is dragging its scrollbar.
func (c *ContentColumn) IsScrollbarDragging() bool {
	for _, item := range c.items {
		if d, ok := item.(interface{ IsScrollbarDragging() bool }); ok && d.IsScrollbarDragging() {
			return true
		}
	}
	return false
}

// WantsHorizontalKeys delegates to whichever child currently holds
// column-internal focus.
func (c *ContentColumn) WantsHorizontalKeys() bool {
	if len(c.items) == 0 {
		return false
	}
	return c.items[c.SubFocusIndex()].WantsHorizontalKeys()
}

// WantsAllMessages delegates to whichever child currently holds
// column-internal focus.
func (c *ContentColumn) WantsAllMessages() bool {
	if len(c.items) == 0 {
		return false
	}
	return c.items[c.SubFocusIndex()].WantsAllMessages()
}

// Focusable reports true if any child is focusable.
func (c *ContentColumn) Focusable() bool {
	for _, item := range c.items {
		if item.Focusable() {
			return true
		}
	}
	return false
}
