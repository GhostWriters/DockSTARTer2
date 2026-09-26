package classic

import tea "charm.land/bubbletea/v2"

// HeaderedList is a submenu list with a section drawn inside its border
// above its rows (see MenuModel.SetHeader), e.g. a search box over the list
// it filters, or below them sharing its border (see MenuModel.SetFooter).
// It has a Tab stop for each, in the order drawn, and routes focus, clicks,
// and keys to whichever holds focus, like a two-item ContentColumn; the list
// draws, sizes, and places both. The section can be hidden (see
// SetSectionShown), keeping its Tab stop but skipping it.
type HeaderedList struct {
	*ContentColumn
	list    *MenuModel
	slot    *sectionSlot
	bottom  bool
	focused bool
}

var (
	_ Content      = (*HeaderedList)(nil)
	_ SubFocusable = (*HeaderedList)(nil)
)

// NewHeaderedList draws header inside list's border above its rows.
func NewHeaderedList(header Content, list *MenuModel) *HeaderedList {
	slot := &sectionSlot{Content: header, shown: true}
	list.SetHeader(header)
	return &HeaderedList{ContentColumn: NewContentColumn(slot, list), list: list, slot: slot}
}

// NewFooteredList draws footer below list's rows, sharing its border (see
// MenuModel.SetFooter).
func NewFooteredList(footer Content, list *MenuModel) *HeaderedList {
	slot := &sectionSlot{Content: footer, shown: true}
	list.SetFooter(footer)
	return &HeaderedList{ContentColumn: NewContentColumn(list, slot), list: list, slot: slot, bottom: true}
}

// SetSectionShown shows or hides the header or footer.
func (h *HeaderedList) SetSectionShown(shown bool) {
	if h.slot.shown == shown {
		return
	}
	h.slot.shown = shown
	switch {
	case !shown:
		h.list.SetHeader(nil)
	case h.bottom:
		h.list.SetFooter(h.slot.Content)
	default:
		h.list.SetHeader(h.slot.Content)
	}
}

// SectionShown reports whether the header or footer shows.
func (h *HeaderedList) SectionShown() bool { return h.slot.shown }

// sectionIndex is the header or footer's Tab stop.
func (h *HeaderedList) sectionIndex() int {
	if h.bottom {
		return 1
	}
	return 0
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

// syncFrame draws the list's border focused while the section holds focus,
// and a footer's while the list does, since they share edges.
func (h *HeaderedList) syncFrame() {
	onSection := h.SubFocusIndex() == h.sectionIndex()
	h.list.SetFrameFocused(h.focused && onSection)
	if f, ok := h.slot.Content.(interface{ SetFrameFocused(bool) }); ok && h.bottom {
		f.SetFrameFocused(h.focused && !onSection)
	}
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

// sectionSlot holds a HeaderedList's header or footer, which Tab and
// clicks skip while hidden.
type sectionSlot struct {
	Content
	shown bool
}

var _ ContentWrapper = (*sectionSlot)(nil)

func (s *sectionSlot) Unwrap() Content { return s.Content }
func (s *sectionSlot) Focusable() bool { return s.shown && s.Content.Focusable() }
func (s *sectionSlot) MatchesID(id string) bool {
	return s.shown && s.Content.MatchesID(id)
}

func (s *sectionSlot) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.Content.Update(msg)
	if c, ok := updated.(Content); ok {
		s.Content = c
	}
	return s, cmd
}

func (s *sectionSlot) Init() tea.Cmd  { return nil }
func (s *sectionSlot) View() tea.View { return tea.View{Content: s.ViewString()} }
