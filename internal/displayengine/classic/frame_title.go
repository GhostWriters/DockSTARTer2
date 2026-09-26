package classic

import "fmt"

// FrameTitle is an enclosing frame's title drawn in a submenu's own top
// border, so the menu's border is that frame (see SetFrameTitle): the
// frame's title, then the menu's own title controls, then the frame's
// widgets. Every field is read each time the menu draws.
type FrameTitle struct {
	// Render draws the title within avail columns; HitRegions returns its
	// hit regions with its first column at (x, y).
	Render     func(avail int, focused bool, ctx StyleContext) string
	HitRegions func(x, y, avail int, ctx StyleContext) []HitRegion
	// Widgets are the frame's title bar widgets, with hit region IDs under
	// WidgetID.
	Widgets  func() []WidgetDef
	WidgetID string
}

// SetFrameTitle makes the menu's border an enclosing frame's, drawing ft in
// place of its own title; nil restores its own.
func (m *MenuModel) SetFrameTitle(ft *FrameTitle) {
	if m.frameTitle != ft {
		m.frameTitle = ft
		m.InvalidateCache()
	}
}

// FrameHost returns the menu an enclosing frame can draw its title in
// instead of a border of its own (see SetFrameTitle): a bordered list
// submenu, or nil for any other kind.
func (m *MenuModel) FrameHost() *MenuModel {
	if !m.subMenuMode || m.borderless || m.isPlainTextKind || m.ContentRenderer != nil || len(m.contentSections) > 0 {
		return nil
	}
	return m
}

// FrameHost returns the list, whose border the header sits inside.
func (h *HeaderedList) FrameHost() *MenuModel { return h.list.FrameHost() }

// FrameHoster is Content whose border an enclosing frame can draw its title
// in (see MenuModel.FrameHost).
type FrameHoster interface {
	FrameHost() *MenuModel
}

// frameTitleLayout places the frame title in a border contentWidth wide:
// its x within the border, the columns it may use, and its widgets.
func (m *MenuModel) frameTitleLayout(contentWidth int, ctx StyleContext) (x, avail int, widgets []WidgetDef) {
	ft := m.frameTitle
	widgets = ft.Widgets()
	controls := 0
	for i, p := range m.titleControlPieces(ctx, false) {
		controls += WidthWithoutZones(p)
		if i > 0 {
			controls++ // the border line between controls
		}
	}
	if controls > 0 && len(widgets) > 0 {
		controls++ // the border line before the widgets
	}
	align := ctx.SubmenuTitleAlign
	avail = max(MaxRawTitleWidth(max(contentWidth, 1), false, align, widgets, ctx)-controls, 1)
	x = 1
	if align != "left" {
		x += max((contentWidth-WidthWithoutZones(ft.Render(avail, false, ctx)))/2, 0)
	}
	return x, avail, widgets
}

// frameTitleStamp describes the frame title's current drawing, for the
// view cache (see viewStamp).
func (m *MenuModel) frameTitleStamp(ctx StyleContext) string {
	if m.frameTitle == nil {
		return ""
	}
	_, avail, widgets := m.frameTitleLayout(m.width-GetLayout().BorderWidth(), ctx)
	s := m.frameTitle.Render(avail, m.focusedSub || m.frameFocused, ctx)
	for _, w := range widgets {
		s += fmt.Sprint("|", w.ID)
	}
	return s
}

// listHeaderHeight returns the rows above the list taken by a header
// headerHeight tall; a footer takes none.
func (m *MenuModel) listHeaderHeight(headerHeight int) int {
	if m.headerBottom {
		return 0
	}
	return headerHeight
}
