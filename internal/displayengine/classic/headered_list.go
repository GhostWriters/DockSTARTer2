package classic

import tea "charm.land/bubbletea/v2"

// HeaderedList is a submenu list with a section drawn inside its border
// above its rows (see MenuModel.SetHeader), e.g. a search box over the list
// it filters. It has a Tab stop for each, and routes focus, clicks, and keys
// to whichever holds focus, like a two-item ContentColumn; the list draws,
// sizes, and places both.
type HeaderedList struct {
	*ContentColumn
	list    *MenuModel
	focused bool
}

var (
	_ Content      = (*HeaderedList)(nil)
	_ SubFocusable = (*HeaderedList)(nil)
)

// NewHeaderedList draws header inside list's border above its rows.
func NewHeaderedList(header Content, list *MenuModel) *HeaderedList {
	list.SetHeader(header)
	return &HeaderedList{ContentColumn: NewContentColumn(header, list), list: list}
}

func (h *HeaderedList) SetSize(width, height int)          { h.list.SetSize(width, height) }
func (h *HeaderedList) ViewString() string                 { return h.list.ViewString() }
func (h *HeaderedList) View() tea.View                     { return tea.View{Content: h.ViewString()} }
func (h *HeaderedList) SectionHeight(width int) int        { return h.list.SectionHeight(width) }
func (h *HeaderedList) SectionNaturalWidth(w int) int      { return h.list.SectionNaturalWidth(w) }
func (h *HeaderedList) IsVariableHeight() bool             { return h.list.IsVariableHeight() }
func (h *HeaderedList) Height() int                        { return h.list.Height() }
func (h *HeaderedList) ID() string                         { return h.list.ID() }
func (h *HeaderedList) ScrollID() string                   { return h.list.ScrollID() }
func (h *HeaderedList) GetHitRegions(x, y int) []HitRegion { return h.list.GetHitRegions(x, y) }

// SetSubFocused focuses whichever part holds focus; the list's border
// shows focus for either, since the header sits inside it.
func (h *HeaderedList) SetSubFocused(focused bool) tea.Cmd {
	h.focused = focused
	cmd := h.ContentColumn.SetSubFocused(focused)
	h.syncFrame()
	return cmd
}

// syncFrame draws the list's border focused while the header holds focus.
func (h *HeaderedList) syncFrame() {
	h.list.SetFrameFocused(h.focused && h.SubFocusIndex() == 0)
}

// Update routes msg to whichever part holds focus, except the list's own
// scrollbar messages, which always go to the list.
func (h *HeaderedList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case ScrollDoneMsg:
		if m.ID == h.list.ScrollID() {
			_, cmd := h.list.Update(msg)
			return h, cmd
		}
	case DragDoneMsg:
		if m.ID == h.list.ScrollID() {
			_, cmd := h.list.Update(msg)
			return h, cmd
		}
	}
	_, cmd := h.ContentColumn.Update(msg)
	h.syncFrame()
	return h, cmd
}
