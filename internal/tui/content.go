package tui

import tea "charm.land/bubbletea/v2"

// Sizer reports how many rows a Content would occupy at a given width,
// without actually resizing it. calculateSectionLayout's Pass 1 uses this to
// measure every fixed-height section before distributing remaining height to
// expandable ones.
type Sizer interface {
	SectionHeight(width int) int
	// SectionNaturalWidth returns how wide this Content would naturally like
	// to be, given at most maxWidth to work with -- used by
	// calculateSectionLayout when the outer MenuModel is not maximized, to
	// shrink to intrinsic content width the same way it already shrinks to
	// intrinsic content height. Most kinds (flow-grid, sinput, plain-text)
	// have no narrower natural width than whatever they're given, so they
	// simply return maxWidth; the plain-list kind measures from its items'
	// Tag/Desc text (mirroring the plain-list calculateLayout path's own
	// natural-width formula).
	SectionNaturalWidth(maxWidth int) int
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
	// WantsAllMessages reports whether this Content must see every message
	// type reaching updateSections, not just the ones its switch has an
	// explicit case for (Tab/Shift-Tab, LayerHitMsg, wheel/drag/motion/
	// release, ToggleFocusedMsg, sinput clipboard messages). A *MenuModel
	// reports true only when it wraps a variable-height contentRenderer
	// section whose inner widget needs raw message types beyond those
	// explicit cases (e.g. a streaming viewport section forwarding
	// PgUp/PgDn and other leftover key messages to its own inner model).
	// Every other kind reports false, preserving existing behavior exactly.
	// A ContentRow reports true if any child does.
	WantsAllMessages() bool
	// Focusable reports whether this Content can receive Tab-cycled focus at
	// all. A *MenuModel reports false only for the plain-text kind (a
	// read-only display line, e.g. a subtitle section, that Tab/Shift-Tab
	// must skip automatically); every other kind reports true. A ContentRow
	// reports true if any child is focusable.
	Focusable() bool
	// AbsorbMessage lets a Content section observe its own deferred-action
	// messages (button-row clicks and list-item Action clicks scheduled via
	// deferAction) without participating in general Update() dispatch.
	// Returns nil if msg doesn't belong to this section. updateSections calls
	// this for every content section on every message -- not just the
	// focused one -- so a section's deferred action still fires correctly
	// after focus has moved elsewhere by the time the tick arrives (e.g. a
	// list item's click schedules a menuDeferredActionMsg scoped to that
	// section's own instanceID one tick later; nothing else would route it
	// back once the section is no longer focused). A ContentRow delegates to
	// each child.
	AbsorbMessage(msg tea.Msg) tea.Cmd
	// IsProcessing reports whether this section has an in-flight item/button
	// action (spinner visible). updateSections uses this to detect when a
	// section just started processing an item click, so the outer dialog's
	// own Select-role button can spin too -- mirroring the pre-migration
	// single-MenuModel behavior where the button row and the item spinner
	// were the same object. A ContentRow reports true if any child is
	// processing.
	IsProcessing() bool
}

var _ Content = (*MenuModel)(nil)
