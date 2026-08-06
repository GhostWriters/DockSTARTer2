package classic

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// itemHeight returns the number of display rows an item occupies. Matches
// ViewString exactly: disabled items never render their SubLabel line, even
// when SubLabel is set.
func itemHeight(item ContextMenuItem) int {
	if !item.Disabled && !item.IsSeparator && !item.IsHeader && item.SubLabel != "" {
		return 2
	}
	return 1
}

// ContextMenuModel is a small positioned popup menu that appears near the cursor.
// It is designed to be shown via ShowDialogMsg so AppModel stacks it on the dialog stack.
//
// Positioning: the model stores the raw right-click coordinates (clickX, clickY).
// IsMaximized() returns true so model_view.go uses DialogMaximized mode, giving
// lx=EdgeIndent(1), ly=ContentStartY(1)=3. The Layers() method compensates with
// layer.X = menuX-lx, layer.Y = menuY-ly so the menu lands at exactly (menuX, menuY).
type ContextMenuModel struct {
	items   []ContextMenuItem
	cursor  int // currently highlighted item index
	clickX  int // original right-click screen position
	clickY  int
	screenW int
	screenH int

	// Computed positions (set in recalculate)
	menuX int
	menuY int
	menuW int // inner content width (without border)
	menuH int // inner content height (without border)

	offset     int // scroll offset for long menus
	maxVisible int // max items to show at once (default 12)

	subMenu  *ContextMenuModel // open submenu, managed internally
	isClosed bool             // set when submenu wants to close (signals parent to nil it)
}

// NewContextMenuModel creates a context menu positioned near (clickX, clickY).
// screenW and screenH are the full terminal dimensions.
func NewContextMenuModel(clickX, clickY, screenW, screenH int, items []ContextMenuItem) *ContextMenuModel {
	m := &ContextMenuModel{
		items:      items,
		cursor:     0,
		clickX:     clickX,
		clickY:     clickY,
		screenW:    screenW,
		screenH:    screenH,
		maxVisible: 12,
	}
	m.cursor = m.firstSelectable()
	m.recalculate()
	return m
}

// IsMaximized satisfies the interface checked by model_view.go.
// Returning true makes model_view.go use DialogMaximized positioning.
// Our Layers() compensates using the same layout values.
func (m *ContextMenuModel) IsMaximized() bool { return true }

// Init implements tea.Model.
func (m *ContextMenuModel) Init() tea.Cmd { return nil }

// HelpText returns the help text for the currently focused item (for the helpline).
func (m *ContextMenuModel) HelpText() string {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].Help
	}
	return ""
}

// --- internal helpers ---

func (m *ContextMenuModel) recalculate() {
	// Compute content width = max label/sublabel length, capped to screen.
	// Items with SubItems get a " ▶" suffix (2 extra columns).
	maxW := 0
	for _, item := range m.items {
		if !item.IsSeparator {
			w := lipgloss.Width(item.Label)
			if len(item.SubItems) > 0 {
				w += 2 // " ▶"
			}
			if w > maxW {
				maxW = w
			}
			if sw := lipgloss.Width(item.SubLabel); sw > maxW {
				maxW = sw
			}
		}
	}
	if maxW < 8 {
		maxW = 8
	}
	// Box total width = content + 2 padding + 2 border
	totalBoxW := maxW + 4
	// Cap to screen
	maxAllowedW := m.screenW - 4
	if maxAllowedW < 12 {
		maxAllowedW = 12
	}
	if totalBoxW > maxAllowedW {
		totalBoxW = maxAllowedW
		maxW = totalBoxW - 4
	}
	m.menuW = maxW

	// Visible item count (capped to maxVisible items).
	visibleItems := len(m.items)
	if visibleItems > m.maxVisible {
		visibleItems = m.maxVisible
	}

	// Total display rows = sum of itemHeight for each visible item.
	visibleRows := 0
	for i, item := range m.items {
		if i >= visibleItems {
			break
		}
		visibleRows += itemHeight(item)
	}
	m.menuH = visibleRows

	// Total box height = rows + 2 border
	totalBoxH := visibleRows + 2

	// Compute position: prefer right/below click
	x := m.clickX + 1
	y := m.clickY

	// Clamp to screen edges
	if x+totalBoxW > m.screenW-1 {
		x = m.screenW - 1 - totalBoxW
	}
	if y+totalBoxH > m.screenH-1 {
		y = m.screenH - 1 - totalBoxH
	}
	if x < 1 {
		x = 1
	}
	if y < 1 {
		y = 1
	}

	m.menuX = x
	m.menuY = y
}

func (m *ContextMenuModel) firstSelectable() int {
	for i, item := range m.items {
		if !item.IsSeparator && !item.IsHeader && !item.Disabled {
			return i
		}
	}
	return 0
}

func (m *ContextMenuModel) moveCursor(delta int) {
	next := m.cursor + delta
	// Skip separators, headers, and disabled items
	for next >= 0 && next < len(m.items) && (m.items[next].IsSeparator || m.items[next].IsHeader || m.items[next].Disabled) {
		next += delta
	}
	if next < 0 || next >= len(m.items) {
		return
	}
	m.cursor = next
	// Adjust scroll offset (relative to scrollable items, i.e. after pinned header)
	pinned := m.pinnedCount()
	scrollCursor := m.cursor - pinned
	scrollVisible := m.maxVisible - pinned
	if scrollCursor < m.offset {
		m.offset = scrollCursor
	}
	if scrollCursor >= m.offset+scrollVisible {
		m.offset = scrollCursor - scrollVisible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *ContextMenuModel) scrollBy(delta int) {
	pinned := m.pinnedCount()
	scrollable := len(m.items) - pinned
	scrollVisible := m.maxVisible - pinned
	m.offset += delta
	if m.offset < 0 {
		m.offset = 0
	}
	maxOff := scrollable - scrollVisible
	if maxOff < 0 {
		maxOff = 0
	}
	if m.offset > maxOff {
		m.offset = maxOff
	}
	// Keep cursor within visible scrollable range
	isUnselectable := func(i int) bool {
		return m.items[i].IsSeparator || m.items[i].IsHeader || m.items[i].Disabled
	}
	scrollCursor := m.cursor - pinned
	if scrollCursor < m.offset {
		m.cursor = m.offset + pinned
		for m.cursor < len(m.items) && isUnselectable(m.cursor) {
			m.cursor++
		}
	}
	if scrollCursor >= m.offset+scrollVisible {
		m.cursor = m.offset + scrollVisible - 1 + pinned
		for m.cursor >= 0 && isUnselectable(m.cursor) {
			m.cursor--
		}
	}
}

func (m *ContextMenuModel) executeSelected() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return func() tea.Msg { return CloseDialogMsg{} }
	}
	item := m.items[m.cursor]
	if item.Disabled {
		return nil
	}

	// Item with sub-items: open submenu internally so parent stays visible.
	if len(item.SubItems) > 0 {
		subClickX := m.menuX + m.menuW + 1 // right edge of parent box, overlapping by 2 chars
		// Count display rows from top of menu to the selected item
		rowOffset := 0
		for i := m.offset; i < m.cursor; i++ {
			rowOffset += itemHeight(m.items[i])
		}
		subClickY := m.menuY + rowOffset
		m.subMenu = NewContextMenuModel(subClickX, subClickY, m.screenW, m.screenH, item.SubItems)
		return nil
	}

	if action := item.Action; action != nil {
		return action
	}
	return func() tea.Msg { return CloseDialogMsg{} }
}

// pinnedCount returns the number of leading header/separator items that are
// always shown at the top regardless of scroll offset.
func (m *ContextMenuModel) pinnedCount() int {
	for i, item := range m.items {
		if !item.IsHeader && !item.IsSeparator {
			return i
		}
	}
	return len(m.items)
}

func (m *ContextMenuModel) visibleItems() []ContextMenuItem {
	pinned := m.pinnedCount()
	scrollable := m.items[pinned:]
	end := m.offset + m.maxVisible - pinned
	if end > len(scrollable) {
		end = len(scrollable)
	}
	if m.offset > len(scrollable) {
		return m.items[:pinned]
	}
	return append(m.items[:pinned:pinned], scrollable[m.offset:end]...)
}

// parseIntSafe parses an integer string, returning -1 on failure.
func parseIntSafe(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// itoa converts an int to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// AppendContextMenuTail appends a separator, a Clipboard submenu (if clipItems is non-empty),
// another separator, and a Help item to the given items slice.
// If hCtx is provided, it will be used when triggering the Help dialog.
func AppendContextMenuTail(items []ContextMenuItem, clipItems []ContextMenuItem, hCtx *HelpContext) []ContextMenuItem {
	// Add separator before tail items if list is not empty and last item isn't a separator
	if len(items) > 0 && !items[len(items)-1].IsSeparator {
		items = append(items, ContextMenuItem{IsSeparator: true})
	}

	// Clipboard Submenu
	if len(clipItems) > 0 {
		items = append(items, ContextMenuItem{
			Label:    "Clipboard",
			Help:     "Access clipboard operations.",
			SubItems: clipItems,
		})
	}

	// Another separator before Help (if we added Clipboard or had prior items)
	if len(items) > 0 && !items[len(items)-1].IsSeparator {
		items = append(items, ContextMenuItem{IsSeparator: true})
	}

	// Help Item
	items = append(items, ContextMenuItem{
		Label: "Help",
		Help:  "Display keyboard shortcuts and context information (F1 or ?).",
		Action: func() tea.Msg {
			return TriggerHelpMsg{CapturedContext: hCtx}
		},
	})

	return items
}
