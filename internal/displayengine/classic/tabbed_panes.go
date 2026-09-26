package classic

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
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
// its own box, either one at a time (maximized) or side by side or stacked
// (tiled), like the tabbed vars editor. Maximized, the pane's box title is
// the tab strip, with icons for the other two layouts. Tiled, the open panes
// sit inside a middle box titled with the tab strip and carrying those
// icons, and each pane's box is titled with its own label, with a close icon
// while more than one is open. A closed pane reopens from its tab. A tiled
// layout that doesn't fit, or with only one pane open, shows maximized.
//
// Every section is its own Tab stop, and so is the tab strip while tiled
// (Left/Right choose a tab, Space/Enter open it). While maximized, Tab past
// the last stop of one pane opens the next, and Shift+Tab before the first
// opens the previous; Tab entering from outside starts at the first pane,
// Shift+Tab at the last.
type TabbedPanes struct {
	id    string
	Strip *TabStrip

	// MinPaneWidth and MinPaneHeight are the smallest pane size side by
	// side and stacked allow.
	MinPaneWidth, MinPaneHeight int

	// Changed, if set, reports whether pane has unsaved changes; its tab or
	// title then carries the changed marker.
	Changed func(pane int) bool

	panes         []*ContentColumn
	open          []bool // whether each pane shows while tiled
	stripStop     *panesStripStop
	stripFocused  bool        // whether the tab strip holds focus (tiled only)
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
	_ GroupJumper  = (*TabbedPanes)(nil)
	_ GroupJumper  = (*ContentColumn)(nil)
)

// NewTabbedPanes builds a TabbedPanes; labels[i] names panes[i].
func NewTabbedPanes(id string, labels []string, panes []*ContentColumn, layout string) *TabbedPanes {
	t := &TabbedPanes{
		id:            id,
		Strip:         &TabStrip{ID: id + ".tabs", Labels: labels},
		MinPaneWidth:  40,
		MinPaneHeight: 10,
		panes:         panes,
		open:          make([]bool, len(panes)),
		boxWidths:     make([]int, len(panes)),
		boxHeights:    make([]int, len(panes)),
		layout:        normalizePaneLayout(layout),
	}
	for i := range t.open {
		t.open[i] = true
	}
	t.stripStop = &panesStripStop{t: t}
	t.Strip.Changed = t.paneChanged
	t.Strip.ActiveFocus = func() bool { return !t.tiled() || t.stripFocused }
	for i, label := range labels {
		pane := i
		t.labels = append(t.labels, &TabStrip{ID: t.paneID(i), Labels: []string{label},
			Changed: func(int) bool { return t.paneChanged(pane) }})
	}
	t.effective = t.layout
	return t
}

// paneChanged reports whether pane has unsaved changes (see Changed).
func (t *TabbedPanes) paneChanged(pane int) bool {
	return t.Changed != nil && t.Changed(pane)
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

// tiled reports whether the panes show side by side or stacked.
func (t *TabbedPanes) tiled() bool { return t.effective != PaneLayoutMaximized }

// openPanes returns the panes open while tiled, in order.
func (t *TabbedPanes) openPanes() []int {
	var open []int
	for i, o := range t.open {
		if o {
			open = append(open, i)
		}
	}
	return open
}

// ClosePane closes pane while tiled, keeping at least one open; focus in it
// moves to the next open pane. The returned PaneLayoutMsg asks the owner to
// lay out again.
func (t *TabbedPanes) ClosePane(pane int) tea.Cmd {
	if !t.tiled() || pane < 0 || pane >= len(t.panes) || !t.open[pane] || len(t.openPanes()) < 2 {
		return nil
	}
	t.open[pane] = false
	if p, _ := t.locate(t.SubFocusIndex()); p == pane {
		open := t.openPanes()
		next := open[0]
		for _, o := range open {
			if o > pane {
				next = o
				break
			}
		}
		if j, ok := t.panes[next].NextFocusableSub(-1); ok {
			t.SetSubFocusIndex(t.base(next) + j)
		}
	}
	return t.relayout()
}

// CloseFocused closes the pane holding focus (see ClosePane).
func (t *TabbedPanes) CloseFocused() tea.Cmd {
	p, _ := t.locate(t.SubFocusIndex())
	return t.ClosePane(p)
}

// openPane reopens pane and focuses it.
func (t *TabbedPanes) openPane(pane int) tea.Cmd {
	if pane < 0 || pane >= len(t.panes) {
		return nil
	}
	t.open[pane] = true
	t.stripFocused = false
	return tea.Batch(t.ShowPane(pane), t.relayout())
}

// relayout asks the owner to lay out again (see PaneLayoutMsg).
func (t *TabbedPanes) relayout() tea.Cmd {
	layout := t.layout
	return func() tea.Msg { return PaneLayoutMsg{ID: t.id, Layout: layout, Pane: -1} }
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
	if !t.tiled() {
		return []int{t.Strip.Active}
	}
	return t.openPanes()
}

// closeWidget returns pane's close icon.
func (t *TabbedPanes) closeWidget() WidgetDef {
	return WidgetDef{ID: IDTitleWidgetClose, Label: "Close", HelpText: "Close this pane (reopen it from its tab).",
		Glyph: closeWidget, GlyphAscii: closeWidgetAscii, IconName: "Exit"}
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

// effectiveFor returns the layout shown at width x height: maximized when a
// tiled layout doesn't fit its open panes, or only one is open.
func (t *TabbedPanes) effectiveFor(width, height int) string {
	n := len(t.openPanes())
	if n < 2 {
		return PaneLayoutMaximized
	}
	switch t.layout {
	case PaneLayoutSideBySide:
		if (width-2)/n < t.MinPaneWidth {
			return PaneLayoutMaximized
		}
	case PaneLayoutStacked:
		if height > 0 && (height-2)/n < t.MinPaneHeight {
			return PaneLayoutMaximized
		}
	}
	return t.layout
}

// ---- Tab stops ----
//
// Stop 0 is the tab strip, only focusable while tiled; the panes' stops
// follow, so the count never changes with the layout.

func (t *TabbedPanes) base(pane int) int {
	b := 1
	for i := 0; i < pane && i < len(t.panes); i++ {
		b += t.panes[i].NumTabStops()
	}
	return b
}

// locate returns the pane holding stop and the stop's index within it; pane
// is -1 for the strip or an out-of-range stop.
func (t *TabbedPanes) locate(stop int) (pane, local int) {
	if stop < 1 {
		return -1, 0
	}
	stop--
	for i, p := range t.panes {
		n := p.NumTabStops()
		if stop < n {
			return i, stop
		}
		stop -= n
	}
	return -1, 0
}

// paneStops reports whether pane's stops are reachable now.
func (t *TabbedPanes) paneStops(pane int) bool {
	if t.tiled() {
		return t.open[pane]
	}
	return true
}

func (t *TabbedPanes) NumTabStops() int {
	total := 1
	for _, p := range t.panes {
		total += p.NumTabStops()
	}
	return total
}

func (t *TabbedPanes) SubFocusIndex() int {
	if t.subFocus < 0 || t.subFocus >= t.NumTabStops() {
		return 1
	}
	return t.subFocus
}

// SetSubFocusIndex focuses Tab stop i, showing its pane while maximized.
func (t *TabbedPanes) SetSubFocusIndex(i int) {
	if i < 0 || i >= t.NumTabStops() {
		i = 1
	}
	t.subFocus = i
	t.stripFocused = i == 0 && t.tiled()
	pane, local := t.locate(i)
	if pane < 0 {
		return
	}
	t.panes[pane].SetSubFocusIndex(local)
	if pane != t.Strip.Active {
		t.Strip.Active = pane
		if !t.tiled() {
			t.SetSize(t.width, t.height)
		}
	}
}

// NextFocusableSub enters from before at the strip while tiled, otherwise at
// the first pane; PrevFocusableSub enters from after at the last pane.
func (t *TabbedPanes) NextFocusableSub(from int) (int, bool) {
	start := 0
	if pane, local := t.locate(from); pane >= 0 {
		if j, ok := t.panes[pane].NextFocusableSub(local); ok {
			return t.base(pane) + j, true
		}
		start = pane + 1
	} else if from != 0 && t.tiled() {
		return 0, true
	}
	for p := start; p < len(t.panes); p++ {
		if !t.paneStops(p) {
			continue
		}
		if j, ok := t.panes[p].NextFocusableSub(-1); ok {
			return t.base(p) + j, true
		}
	}
	return -1, false
}

func (t *TabbedPanes) PrevFocusableSub(from int) (int, bool) {
	start := len(t.panes) - 1
	if from == 0 {
		return -1, false
	}
	if pane, local := t.locate(from); pane >= 0 {
		if j, ok := t.panes[pane].PrevFocusableSub(local); ok {
			return t.base(pane) + j, true
		}
		start = pane - 1
	}
	for p := start; p >= 0; p-- {
		if !t.paneStops(p) {
			continue
		}
		if j, ok := t.panes[p].PrevFocusableSub(t.panes[p].NumTabStops()); ok {
			return t.base(p) + j, true
		}
	}
	if t.tiled() {
		return 0, true
	}
	return -1, false
}

// GroupStop implements GroupJumper: the tab strip (while tiled) and each
// visible pane are groups.
func (t *TabbedPanes) GroupStop(from, dir int) (int, bool) {
	// Group positions: -1 for the strip, otherwise the pane index.
	var groups []int
	if t.tiled() {
		groups = append(groups, -1)
	}
	groups = append(groups, t.visible()...)
	cur := len(t.panes)
	if from < 0 {
		cur = -2
	} else if from == 0 {
		cur = -1
	} else if p, _ := t.locate(from); p >= 0 {
		cur = p
	}
	first := func(g int) (int, bool) {
		if g == -1 {
			return 0, true
		}
		if j, ok := t.panes[g].NextFocusableSub(-1); ok {
			return t.base(g) + j, true
		}
		return -1, false
	}
	if dir < 0 {
		for k := len(groups) - 1; k >= 0; k-- {
			if groups[k] < cur {
				if i, ok := first(groups[k]); ok {
					return i, true
				}
			}
		}
		return -1, false
	}
	for _, g := range groups {
		if g > cur {
			if i, ok := first(g); ok {
				return i, true
			}
		}
	}
	return -1, false
}

func (t *TabbedPanes) Items() []Content {
	leaves := []Content{t.stripStop}
	for _, p := range t.panes {
		leaves = append(leaves, p.Items()...)
	}
	return leaves
}

// ---- Content ----

func (t *TabbedPanes) Init() tea.Cmd  { return nil }
func (t *TabbedPanes) View() tea.View { return tea.View{Content: t.ViewString()} }

func (t *TabbedPanes) SectionHeight(width int) int {
	open := t.openPanes()
	switch t.effectiveFor(width, 0) {
	case PaneLayoutSideBySide:
		widths := SplitWidth(width-2, len(open))
		h := 0
		for k, p := range open {
			h = max(h, t.panes[p].SectionHeight(max(widths[k]-2, 1))+2)
		}
		return h + 2
	case PaneLayoutStacked:
		h := 0
		for _, p := range open {
			h += t.panes[p].SectionHeight(max(width-4, 1)) + 2
		}
		return h + 2
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
	if !t.tiled() {
		t.stripFocused = false
	}
	for i := range t.panes {
		t.boxWidths[i], t.boxHeights[i] = width, height
	}
	if t.tiled() {
		// Inside the middle box.
		open := t.openPanes()
		for _, p := range open {
			t.boxWidths[p], t.boxHeights[p] = width-2, height-2
		}
		if t.effective == PaneLayoutSideBySide {
			for k, w := range SplitWidth(width-2, len(open)) {
				t.boxWidths[open[k]] = w
			}
		} else {
			for k, h := range SplitWidth(height-2, len(open)) {
				t.boxHeights[open[k]] = h
			}
		}
	}
	for i, p := range t.panes {
		p.SetSize(max(t.boxWidths[i]-2, 1), max(t.boxHeights[i]-2, 1))
	}
}

// paneWidgets returns pane's box icons: the layout icons while maximized,
// its close icon while tiled with more than one pane open.
func (t *TabbedPanes) paneWidgets(pane int) []WidgetDef {
	if !t.tiled() {
		return t.layoutWidgets(pane)
	}
	if len(t.openPanes()) > 1 {
		return []WidgetDef{t.closeWidget()}
	}
	return nil
}

// middleID prefixes the middle box's icon hit region IDs.
func (t *TabbedPanes) middleID() string { return t.id + ".middle" }

// renderPane draws pane inside its box.
func (t *TabbedPanes) renderPane(pane int) string {
	ctx := GetActiveContext()
	w, h := t.boxWidths[pane], t.boxHeights[pane]
	widgets := t.paneWidgets(pane)
	focused := t.paneFocused(pane)
	strip := t.paneStrip(pane)
	_, avail := strip.TitlePlacement(max(w-2, 1), ctx.SubmenuTitleAlign, widgets, ctx)
	return RenderBorderedBoxCtx(strip.Render(avail, focused, ctx), t.panes[pane].ViewString(), max(w-2, 1), h,
		focused, false, true, ctx.SubmenuTitleAlign, "RAW", ctx, TitleBarState{Show: len(widgets) > 0, Widgets: widgets})
}

// middleWidgets returns the middle box's layout icons, maximizing the pane
// holding focus.
func (t *TabbedPanes) middleWidgets() []WidgetDef {
	pane, _ := t.locate(t.SubFocusIndex())
	if pane < 0 {
		pane = t.Strip.Active
	}
	return t.layoutWidgets(pane)
}

func (t *TabbedPanes) ViewString() string {
	var views []string
	for _, i := range t.visible() {
		views = append(views, t.renderPane(i))
	}
	if !t.tiled() {
		return lipgloss.JoinVertical(lipgloss.Left, views...)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, views...)
	if t.effective == PaneLayoutSideBySide {
		body = lipgloss.JoinHorizontal(lipgloss.Top, views...)
	}
	ctx := GetActiveContext()
	widgets := t.middleWidgets()
	focused := t.focused
	_, avail := t.Strip.TitlePlacement(max(t.width-2, 1), ctx.SubmenuTitleAlign, widgets, ctx)
	return RenderBorderedBoxCtx(t.Strip.Render(avail, focused, ctx), body, max(t.width-2, 1), t.height,
		focused, false, true, ctx.SubmenuTitleAlign, "RAW", ctx, TitleBarState{Show: true, Widgets: widgets})
}

func (t *TabbedPanes) GetHitRegions(offsetX, offsetY int) []HitRegion {
	ctx := GetActiveContext()
	var regions []HitRegion
	x, y := offsetX, offsetY
	if t.tiled() {
		widgets := t.middleWidgets()
		sx, avail := t.Strip.TitlePlacement(max(t.width-2, 1), ctx.SubmenuTitleAlign, widgets, ctx)
		regions = append(regions, t.Strip.HitRegions(x+sx, y, avail, ZDialog+10, ctx, nil)...)
		regions = append(regions, TitleBarHitRegionsFor(t.middleID(), x, y, t.width, false, widgets, ZDialog)...)
		x, y = x+1, y+1
	}
	for _, i := range t.visible() {
		w := t.boxWidths[i]
		regions = append(regions, t.panes[i].GetHitRegions(x+1, y+1)...)
		widgets := t.paneWidgets(i)
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
	case tea.KeyPressMsg:
		if t.stripFocused {
			n := len(t.panes)
			switch {
			case key.Matches(m, Keys.Left):
				t.Strip.Active = (t.Strip.Active - 1 + n) % n
			case key.Matches(m, Keys.Right):
				t.Strip.Active = (t.Strip.Active + 1) % n
			case key.Matches(m, Keys.Space), key.Matches(m, Keys.Enter):
				return t, t.openPane(t.Strip.Active)
			}
			return t, nil
		}
	case LayerHitMsg:
		if i, ok := t.Strip.TabFromID(m.ID); ok {
			if !t.open[i] {
				return t, t.openPane(i)
			}
			return t, t.ShowPane(i)
		}
		if layout, ok := t.iconSuffixLayout(t.middleID(), m.ID); ok {
			pane, _ := t.locate(t.SubFocusIndex())
			return t, func() tea.Msg { return PaneLayoutMsg{ID: t.id, Layout: layout, Pane: pane} }
		}
		for i, label := range t.labels {
			if label.OwnsID(m.ID) {
				return t, t.ShowPane(i)
			}
			if m.ID == t.paneID(i)+"."+IDTitleWidgetClose {
				return t, t.ClosePane(i)
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
	return t.iconSuffixLayout(t.paneID(pane), id)
}

// iconSuffixLayout returns the layout the icon with hit region id, under
// prefix, selects.
func (t *TabbedPanes) iconSuffixLayout(prefix, id string) (string, bool) {
	suffix, ok := strings.CutPrefix(id, prefix+".")
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
	t.stripFocused = focused && t.tiled() && t.SubFocusIndex() == 0
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
	if t.Strip.OwnsID(id) || strings.HasPrefix(id, t.id+".pane") || strings.HasPrefix(id, t.middleID()) {
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

func (t *TabbedPanes) WantsHorizontalKeys() bool {
	return t.stripFocused || t.focusedPane().WantsHorizontalKeys()
}
func (t *TabbedPanes) WantsAllMessages() bool { return t.focusedPane().WantsAllMessages() }

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

// panesStripStop is the tab strip's Tab stop (see TabbedPanes); the strip
// itself is drawn in the middle box's title.
type panesStripStop struct{ t *TabbedPanes }

var _ Content = (*panesStripStop)(nil)

func (p *panesStripStop) SectionHeight(int) int                { return 0 }
func (p *panesStripStop) SectionNaturalWidth(maxWidth int) int { return maxWidth }
func (p *panesStripStop) SetSize(int, int)                     {}
func (p *panesStripStop) ViewString() string                   { return "" }
func (p *panesStripStop) GetHitRegions(int, int) []HitRegion   { return nil }
func (p *panesStripStop) Update(tea.Msg) (tea.Model, tea.Cmd)  { return p, nil }
func (p *panesStripStop) View() tea.View                       { return tea.View{} }
func (p *panesStripStop) Init() tea.Cmd                        { return nil }
func (p *panesStripStop) SetSubFocused(bool) tea.Cmd           { return nil }
func (p *panesStripStop) SetIsDialog(bool)                     {}
func (p *panesStripStop) SetLockedByOthers(bool)               {}
func (p *panesStripStop) IsVariableHeight() bool               { return false }
func (p *panesStripStop) Height() int                          { return 0 }
func (p *panesStripStop) ID() string                           { return p.t.Strip.ID }
func (p *panesStripStop) ScrollID() string                     { return "" }
func (p *panesStripStop) MatchesID(string) bool                { return false }
func (p *panesStripStop) WantsHorizontalKeys() bool            { return true }
func (p *panesStripStop) WantsAllMessages() bool               { return false }
func (p *panesStripStop) Focusable() bool                      { return p.t.tiled() }
func (p *panesStripStop) AbsorbMessage(tea.Msg) tea.Cmd        { return nil }
func (p *panesStripStop) IsProcessing() bool                   { return false }
