package tui

import tea "charm.land/bubbletea/v2"

// Sizer reports how many rows a Content would occupy at a given width,
// without actually resizing it. calculateSectionLayout's Pass 1 uses this to
// measure every fixed-height section before distributing remaining height to
// expandable ones.
type Sizer interface {
	SectionHeight(width int) int
}

// Content is a sizeable, renderable, focusable unit that can be placed inside
// a dialog's content area -- either directly (one section = one row) or
// grouped into a ContentRow for horizontal placement. *MenuModel implements
// this today for all existing kinds (plain list, checklist, flow-grid,
// sinput); ContentRow implements it by fanning out to whichever child has
// row-internal focus, so calculateSectionLayout/viewWithSections/hit-regions/
// updateSections never need to special-case "is this a row or a single
// section" -- a row is just another Content.
type Content interface {
	Sizer
	SetSize(width, height int)
	ViewString() string
	GetHitRegions(offsetX, offsetY int) []HitRegion
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	SetSubFocused(focused bool) tea.Cmd
	SetIsDialog(isDialog bool)
	SetLockedByOthers(locked bool)
	IsVariableHeight() bool
	Height() int
	ID() string
	// ScrollID returns the scrollbar ID this Content owns for ScrollDoneMsg
	// routing, or "" if it has none (e.g. a ContentRow with no single
	// scrollbar of its own).
	ScrollID() string
	// MatchesID reports whether the given hit-region/message ID belongs to
	// this Content (or, for a ContentRow, to any of its children). Used by
	// updateSections to find which section a LayerHitMsg/LayerWheelMsg
	// targets -- a *MenuModel checks strings.Contains(msgID, m.ID()); a
	// ContentRow delegates to each child so a hit inside any single child's
	// rendered region is still recognized as belonging to the row.
	MatchesID(msgID string) bool
	// WantsHorizontalKeys reports whether this Content consumes Left/Right
	// key presses itself (e.g. a sinput text field moving its cursor) rather
	// than letting updateSections' outer Left/Right-jumps-to-buttons
	// shortcut claim them. A *MenuModel reports true when it has a
	// contentRenderer (the sinput kind); a ContentRow delegates to whichever
	// child currently holds row-internal focus.
	WantsHorizontalKeys() bool
}

var _ Content = (*MenuModel)(nil)
