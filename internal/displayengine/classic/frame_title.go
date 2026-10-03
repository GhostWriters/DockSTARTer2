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
	widgets = append(m.titleIconDefs(), ft.Widgets()...)
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

// SetTitleIcons draws icons at the right of this submenu's title, before an
// enclosing frame's widgets (see SetFrameTitle), with hit region IDs under
// TitleIconsID. icons is read each time the menu draws, so an icon can come
// and go.
func (m *MenuModel) SetTitleIcons(icons func() []WidgetDef) {
	m.titleIcons = icons
	m.InvalidateCache()
}

// TitleIconsID prefixes the title icons' hit region IDs.
func (m *MenuModel) TitleIconsID() string { return m.id + ".icons" }

func (m *MenuModel) titleIconDefs() []WidgetDef {
	if m.titleIcons == nil {
		return nil
	}
	return m.titleIcons()
}

// ownTitleIcons returns the title icons a submenu drawing its own title
// shows as its only widgets, or nil.
func (m *MenuModel) ownTitleIcons() []WidgetDef {
	if m.frameTitle != nil || !m.subMenuMode || m.submenuWidgets || m.title == "" {
		return nil
	}
	return m.titleIconDefs()
}

// titleIconsStamp describes the title icons' current drawing, for the view
// cache (see viewStamp).
func (m *MenuModel) titleIconsStamp() string {
	s := ""
	for _, w := range m.titleIconDefs() {
		s += fmt.Sprint("|", w.ID, w.Disabled)
	}
	return s
}

// titleWidgetRegions returns the hit regions of widgets drawn at the right
// end of the top border: the title icons, then the rest with IDs under
// restID.
func (m *MenuModel) titleWidgetRegions(restID string, offsetX, offsetY, dialogWidth int, widgets []WidgetDef, baseZ int, ctx StyleContext) []HitRegion {
	width := WidthWithoutZones(BuildDialogTitleWidgets(false, "", "", widgets, ctx))
	if width == 0 {
		return nil
	}
	const endPad = 1
	x := offsetX + dialogWidth - 1 - endPad - width
	icons := min(len(m.titleIconDefs()), len(widgets))
	regions := TitleBarWidgetRegions(m.TitleIconsID(), widgets[:icons], x, offsetY, baseZ)
	return append(regions, TitleBarWidgetRegions(restID, widgets[icons:], x+4*icons, offsetY, baseZ)...)
}

// listHeaderHeight returns the rows above the list taken by a header
// headerHeight tall; a footer takes none.
func (m *MenuModel) listHeaderHeight(headerHeight int) int {
	if m.headerBottom {
		return 0
	}
	return headerHeight
}
