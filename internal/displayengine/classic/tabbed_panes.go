package classic

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Pane layouts, matching the vars editor's tab_layout values.
const (
	PaneLayoutMaximized  = "maximized"
	PaneLayoutSideBySide = "sidebyside"
	PaneLayoutStacked    = "stacked"
)

// PaneLayoutMsg asks the TabbedPanes with ID to switch to Layout. For
// PaneLayoutMaximized, Pane is the pane to show. Sent when a layout icon is
// clicked; the owner routes it to SetLayout and lays itself out again.
type PaneLayoutMsg struct {
	ID     string
	Layout string
	Pane   int
}

// TabbedPanes shows several panes, each a column of sections drawn inside
// its own box, either one at a time behind a tab strip (maximized), side by
// side, or stacked. A pane box's title is the tab strip while maximized, or
// the pane's own label otherwise, with icons for the other two layouts. A
// layout that doesn't fit falls back to maximized.
//
// Every section is its own Tab stop. While maximized, Tab past the last
// stop of one pane opens the next, and Shift+Tab before the first opens the
// previous; entering from outside lands in the shown pane.
type TabbedPanes struct {
	id    string
	Strip *TabStrip

	// MinPaneWidth and MinPaneHeight are the smallest pane size side by
	// side and stacked allow.
	MinPaneWidth, MinPaneHeight int

	panes         []*ContentColumn
	labels        []*TabStrip // one single-label strip per pane, its title outside maximized
	boxWidths     []int       // each pane box's width, from the last SetSize
	boxHeights    []int       // each pane box's height, from the last SetSize
	layout        string
	effective     string
	width, height int
	subFocus      int
	focused       bool
}

var (
	_ Content      = (*TabbedPanes)(nil)
	_ SubFocusable = (*TabbedPanes)(nil)
)

// NewTabbedPanes builds a TabbedPanes; labels[i] names panes[i].
func NewTabbedPanes(id string, labels []string, panes []*ContentColumn, layout string) *TabbedPanes {
	t := &TabbedPanes{
		id:            id,
		Strip:         &TabStrip{ID: id + ".tabs", Labels: labels},
		MinPaneWidth:  40,
		MinPaneHeight: 10,
		panes:         panes,
		boxWidths:     make([]int, len(panes)),
		boxHeights:    make([]int, len(panes)),
		layout:        normalizePaneLayout(layout),
	}
	for i, label := range labels {
		t.labels = append(t.labels, &TabStrip{ID: t.paneID(i), Labels: []string{label}})
	}
	t.effective = t.layout
	return t
}

// paneID prefixes pane's title strip and icon hit region IDs.
func (t *TabbedPanes) paneID(pane int) string {
	return t.id + ".pane" + strconv.Itoa(pane)
}

// paneStrip returns pane's box title: the tab strip while maximized, its
// own label otherwise.
func (t *TabbedPanes) paneStrip(pane int) *TabStrip {
	if t.effective == PaneLayoutMaximized {
		return t.Strip
	}
	return t.labels[pane]
}

// paneFocused reports whether focus is inside pane.
func (t *TabbedPanes) paneFocused(pane int) bool {
	p, _ := t.locate(t.SubFocusIndex())
	return t.focused && p == pane
}

func normalizePaneLayout(layout string) string {
	switch layout {
	case PaneLayoutSideBySide, PaneLayoutStacked:
		return layout
	}
	return PaneLayoutMaximized
}

// Layout returns the requested layout; EffectiveLayout the one shown.
func (t *TabbedPanes) Layout() string          { return t.layout }
func (t *TabbedPanes) EffectiveLayout() string { return t.effective }

// Active returns the pane shown while maximized.
func (t *TabbedPanes) Active() int { return t.Strip.Active }

// SetLayout switches to layout; for PaneLayoutMaximized, pane (when valid)
// becomes the shown pane.
func (t *TabbedPanes) SetLayout(layout string, pane int) tea.Cmd {
	t.layout = normalizePaneLayout(layout)
	if t.layout == PaneLayoutMaximized && pane >= 0 && pane < len(t.panes) {
		t.Strip.Active = pane
		if p, _ := t.locate(t.subFocus); p != pane {
			if j, ok := t.panes[pane].NextFocusableSub(-1); ok {
				t.subFocus = t.base(pane) + j
			}
		}
	}
	t.SetSize(t.width, t.height)
	return t.SetSubFocused(t.focused)
}

// CycleLayout moves to the next layout: maximized, side by side, stacked.
func (t *TabbedPanes) CycleLayout() tea.Cmd {
	next := PaneLayoutSideBySide
	switch t.layout {
	case PaneLayoutSideBySide:
		next = PaneLayoutStacked
	case PaneLayoutStacked:
		next = PaneLayoutMaximized
	}
	return t.SetLayout(next, -1)
}

// ShowPane shows pane (while maximized) and focuses its first Tab stop.
func (t *TabbedPanes) ShowPane(pane int) tea.Cmd {
	if pane < 0 || pane >= len(t.panes) {
		return nil
	}
	if j, ok := t.panes[pane].NextFocusableSub(-1); ok {
		t.SetSubFocusIndex(t.base(pane) + j)
	}
	t.Strip.Active = pane
	t.SetSize(t.width, t.height)
	return t.SetSubFocused(t.focused)
}

// visible returns the panes currently drawn.
func (t *TabbedPanes) visible() []int {
	if t.effective == PaneLayoutMaximized {
		return []int{t.Strip.Active}
	}
	all := make([]int, len(t.panes))
	for i := range all {
		all[i] = i
	}
	return all
}

// layoutWidgets returns pane's icons for the two layouts not requested now.
func (t *TabbedPanes) layoutWidgets(pane int) []WidgetDef {
	widget := func(id, label, help, glyph, ascii, icon, layout string) WidgetDef {
		return WidgetDef{ID: id, Label: label, HelpText: help, Glyph: glyph, GlyphAscii: ascii, IconName: icon,
			Action: func() tea.Cmd {
				return func() tea.Msg { return PaneLayoutMsg{ID: t.id, Layout: layout, Pane: pane} }
			}}
	}
	sideBySide := widget(IDTitleWidgetSideBySide, "Side by side", "Show the panes side by side.", "▥", "|", "SideBySide", PaneLayoutSideBySide)
	stacked := widget(IDTitleWidgetStacked, "Stacked", "Show the panes stacked vertically.", "▤", "-", "Stacked", PaneLayoutStacked)
	maximize := widget(IDTitleWidgetMaximize, "Maximize", "Show only this pane, with tabs to switch.", "□", "+", "Maximize", PaneLayoutMaximized)
	switch t.layout {
	case PaneLayoutSideBySide:
		return []WidgetDef{stacked, maximize}
	case PaneLayoutStacked:
		return []WidgetDef{sideBySide, maximize}
	default:
		return []WidgetDef{sideBySide, stacked}
	}
}

// effectiveFor returns the layout shown at width x height.
func (t *TabbedPanes) effectiveFor(width, height int) string {
	n := max(len(t.panes), 1)
	switch t.layout {
	case PaneLayoutSideBySide:
		if width/n < t.MinPaneWidth {
			return PaneLayoutMaximized
		}
	case PaneLayoutStacked:
		if height > 0 && height/n < t.MinPaneHeight {
			return PaneLayoutMaximized
		}
	}
	return t.layout
}

// ---- Tab stops ----

func (t *TabbedPanes) base(pane int) int {
	b := 0
	for i := 0; i < pane && i < len(t.panes); i++ {
		b += t.panes[i].NumTabStops()
	}
	return b
}

func (t *TabbedPanes) locate(stop int) (pane, local int) {
	if stop < 0 {
		return -1, 0
	}
	for i, p := range t.panes {
		n := p.NumTabStops()
		if stop < n {
			return i, stop
		}
		stop -= n
	}
	return -1, 0
}

func (t *TabbedPanes) NumTabStops() int {
	total := 0
	for _, p := range t.panes {
		total += p.NumTabStops()
	}
	return max(total, 1)
}

func (t *TabbedPanes) SubFocusIndex() int {
	if t.subFocus < 0 || t.subFocus >= t.NumTabStops() {
		return 0
	}
	return t.subFocus
}

// SetSubFocusIndex focuses Tab stop i, showing its pane while maximized.
func (t *TabbedPanes) SetSubFocusIndex(i int) {
	if i < 0 || i >= t.NumTabStops() {
		i = 0
	}
	t.subFocus = i
	pane, local := t.locate(i)
	if pane < 0 {
		return
	}
	t.panes[pane].SetSubFocusIndex(local)
	if t.effective == PaneLayoutMaximized && pane != t.Strip.Active {
		t.Strip.Active = pane
		t.SetSize(t.width, t.height)
	}
}

func (t *TabbedPanes) NextFocusableSub(from int) (int, bool) {
	start := 0
	if pane, local := t.locate(from); pane >= 0 {
		if j, ok := t.panes[pane].NextFocusableSub(local); ok {
			return t.base(pane) + j, true
		}
		start = pane + 1
	} else if t.effective == PaneLayoutMaximized {
		if j, ok := t.panes[t.Strip.Active].NextFocusableSub(-1); ok {
			return t.base(t.Strip.Active) + j, true
		}
		return -1, false
	}
	for p := start; p < len(t.panes); p++ {
		if j, ok := t.panes[p].NextFocusableSub(-1); ok {
			return t.base(p) + j, true
		}
	}
	return -1, false
}

func (t *TabbedPanes) PrevFocusableSub(from int) (int, bool) {
	start := len(t.panes) - 1
	if pane, local := t.locate(from); pane >= 0 {
		if j, ok := t.panes[pane].PrevFocusableSub(local); ok {
			return t.base(pane) + j, true
		}
		start = pane - 1
	} else if t.effective == PaneLayoutMaximized {
		p := t.panes[t.Strip.Active]
		if j, ok := p.PrevFocusableSub(p.NumTabStops()); ok {
			return t.base(t.Strip.Active) + j, true
		}
		return -1, false
	}
	for p := start; p >= 0; p-- {
		if j, ok := t.panes[p].PrevFocusableSub(t.panes[p].NumTabStops()); ok {
			return t.base(p) + j, true
		}
	}
	return -1, false
}

func (t *TabbedPanes) Items() []Content {
	var leaves []Content
	for _, p := range t.panes {
		leaves = append(leaves, p.Items()...)
	}
	return leaves
}

// ---- Content ----

func (t *TabbedPanes) Init() tea.Cmd  { return nil }
func (t *TabbedPanes) View() tea.View { return tea.View{Content: t.ViewString()} }

func (t *TabbedPanes) SectionHeight(width int) int {
	switch t.effectiveFor(width, 0) {
	case PaneLayoutSideBySide:
		widths := SplitWidth(width, len(t.panes))
		h := 0
		for i, p := range t.panes {
			h = max(h, p.SectionHeight(max(widths[i]-2, 1))+2)
		}
		return h
	case PaneLayoutStacked:
		h := 0
		for _, p := range t.panes {
			h += p.SectionHeight(max(width-2, 1)) + 2
		}
		return h
	}
	return t.panes[t.Strip.Active].SectionHeight(max(width-2, 1)) + 2
}

func (t *TabbedPanes) SectionNaturalWidth(maxWidth int) int {
	w := 0
	for _, p := range t.panes {
		w = max(w, p.SectionNaturalWidth(max(maxWidth-2, 1))+2)
	}
	if t.effectiveFor(maxWidth, 0) == PaneLayoutSideBySide {
		w = min(w*len(t.panes), maxWidth)
	}
	return w
}

func (t *TabbedPanes) SetSize(width, height int) {
	t.width, t.height = width, height
	t.effective = t.effectiveFor(width, height)
	for i := range t.panes {
		t.boxWidths[i], t.boxHeights[i] = width, height
	}
	switch t.effective {
	case PaneLayoutSideBySide:
		copy(t.boxWidths, SplitWidth(width, len(t.panes)))
	case PaneLayoutStacked:
		copy(t.boxHeights, SplitWidth(height, len(t.panes)))
	}
	for i, p := range t.panes {
		p.SetSize(max(t.boxWidths[i]-2, 1), max(t.boxHeights[i]-2, 1))
	}
}

// renderPane draws pane inside its box.
func (t *TabbedPanes) renderPane(pane int) string {
	ctx := GetActiveContext()
	w, h := t.boxWidths[pane], t.boxHeights[pane]
	widgets := t.layoutWidgets(pane)
	focused := t.paneFocused(pane)
	strip := t.paneStrip(pane)
	_, avail := strip.TitlePlacement(max(w-2, 1), ctx.SubmenuTitleAlign, widgets, ctx)
	return RenderBorderedBoxCtx(strip.Render(avail, focused, ctx), t.panes[pane].ViewString(), max(w-2, 1), h,
		focused, false, true, ctx.SubmenuTitleAlign, "RAW", ctx, TitleBarState{Show: true, Widgets: widgets})
}

func (t *TabbedPanes) ViewString() string {
	var views []string
	for _, i := range t.visible() {
		views = append(views, t.renderPane(i))
	}
	if t.effective == PaneLayoutSideBySide {
		return lipgloss.JoinHorizontal(lipgloss.Top, views...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func (t *TabbedPanes) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	var regions []HitRegion
	x, y := offsetX, offsetY
	for _, i := range t.visible() {
		w := t.boxWidths[i]
		regions = append(regions, t.panes[i].GetHitRegions(x+1, y+1)...)
		widgets := t.layoutWidgets(i)
		strip := t.paneStrip(i)
		sx, avail := strip.TitlePlacement(max(w-2, 1), ctx.SubmenuTitleAlign, widgets, ctx)
		regions = append(regions, strip.HitRegions(x+sx, y, avail, ZDialog+10, ctx, nil)...)
		regions = append(regions, TitleBarHitRegionsFor(t.paneID(i), x, y, w, false, widgets, ZDialog)...)
		switch t.effective {
		case PaneLayoutSideBySide:
			x += w
		case PaneLayoutStacked:
			y += t.boxHeights[i]
		}
	}
	return regions
}

// Update routes msg to the pane holding focus. A click on a tab shows that
// pane; a click or wheel inside a visible pane focuses it first.
func (t *TabbedPanes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var focusCmd tea.Cmd
	switch m := msg.(type) {
	case LayerHitMsg:
		if i, ok := t.Strip.TabFromID(m.ID); ok {
			return t, t.ShowPane(i)
		}
		for i, label := range t.labels {
			if label.OwnsID(m.ID) {
				return t, t.ShowPane(i)
			}
			if layout, ok := t.iconLayout(i, m.ID); ok {
				return t, func() tea.Msg { return PaneLayoutMsg{ID: t.id, Layout: layout, Pane: i} }
			}
		}
		if d, ok := t.Strip.ScrollFromID(m.ID); ok {
			t.Strip.ScrollBy(d)
			return t, nil
		}
		focusCmd = t.focusHit(m.ID)
	case LayerWheelMsg:
		focusCmd = t.focusHit(m.ID)
	}
	pane, _ := t.locate(t.SubFocusIndex())
	if pane < 0 {
		return t, focusCmd
	}
	_, cmd := t.panes[pane].Update(msg)
	return t, tea.Batch(focusCmd, cmd)
}

// iconLayout returns the layout pane's icon with hit region id selects.
func (t *TabbedPanes) iconLayout(pane int, id string) (string, bool) {
	suffix, ok := strings.CutPrefix(id, t.paneID(pane)+".")
	if !ok {
		return "", false
	}
	switch suffix {
	case IDTitleWidgetSideBySide:
		return PaneLayoutSideBySide, true
	case IDTitleWidgetStacked:
		return PaneLayoutStacked, true
	case IDTitleWidgetMaximize:
		return PaneLayoutMaximized, true
	}
	return "", false
}

// focusHit focuses the Tab stop in a visible pane whose Content id belongs
// to, if any.
func (t *TabbedPanes) focusHit(id string) tea.Cmd {
	for _, p := range t.visible() {
		for j, leaf := range t.panes[p].Items() {
			if leaf.MatchesID(id) {
				t.SetSubFocusIndex(t.base(p) + j)
				return t.SetSubFocused(true)
			}
		}
	}
	return nil
}

func (t *TabbedPanes) SetSubFocused(focused bool) tea.Cmd {
	t.focused = focused
	pane, _ := t.locate(t.SubFocusIndex())
	var cmd tea.Cmd
	for i, p := range t.panes {
		if i == pane {
			cmd = p.SetSubFocused(focused)
		} else {
			p.SetSubFocused(false)
		}
	}
	return cmd
}

func (t *TabbedPanes) SetIsDialog(v bool) {
	for _, p := range t.panes {
		p.SetIsDialog(v)
	}
}

func (t *TabbedPanes) SetLockedByOthers(v bool) {
	for _, p := range t.panes {
		p.SetLockedByOthers(v)
	}
}

func (t *TabbedPanes) IsVariableHeight() bool {
	for _, p := range t.panes {
		if p.IsVariableHeight() {
			return true
		}
	}
	return false
}

func (t *TabbedPanes) Height() int { return t.height }

func (t *TabbedPanes) ID() string { return t.id }

func (t *TabbedPanes) ScrollID() string { return "" }

// MatchesID reports whether id belongs to the tab strip, a pane box's title
// or icons, or a visible pane.
func (t *TabbedPanes) MatchesID(id string) bool {
	if t.Strip.OwnsID(id) || strings.HasPrefix(id, t.id+".pane") {
		return true
	}
	for _, p := range t.visible() {
		if t.panes[p].MatchesID(id) {
			return true
		}
	}
	return false
}

func (t *TabbedPanes) focusedPane() *ContentColumn {
	pane, _ := t.locate(t.SubFocusIndex())
	if pane < 0 {
		return t.panes[t.Strip.Active]
	}
	return t.panes[pane]
}

func (t *TabbedPanes) WantsHorizontalKeys() bool { return t.focusedPane().WantsHorizontalKeys() }
func (t *TabbedPanes) WantsAllMessages() bool    { return t.focusedPane().WantsAllMessages() }

func (t *TabbedPanes) Focusable() bool {
	for _, p := range t.panes {
		if p.Focusable() {
			return true
		}
	}
	return false
}

func (t *TabbedPanes) AbsorbMessage(msg tea.Msg) tea.Cmd {
	for _, p := range t.panes {
		if cmd := p.AbsorbMessage(msg); cmd != nil {
			return cmd
		}
	}
	return nil
}

func (t *TabbedPanes) IsProcessing() bool {
	for _, p := range t.panes {
		if p.IsProcessing() {
			return true
		}
	}
	return false
}

func (t *TabbedPanes) ClearProcessingState() {
	for _, p := range t.panes {
		p.ClearProcessingState()
	}
}

func (t *TabbedPanes) IsScrollbarDragging() bool {
	for _, p := range t.panes {
		if p.IsScrollbarDragging() {
			return true
		}
	}
	return false
}

// OwnsLayoutMsg reports whether msg is for this TabbedPanes.
func (t *TabbedPanes) OwnsLayoutMsg(msg PaneLayoutMsg) bool {
	return msg.ID == t.id
}
