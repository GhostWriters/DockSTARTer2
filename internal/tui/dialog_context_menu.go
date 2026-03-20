package tui

import (
	"DockSTARTer2/internal/theme"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ContextMenuItem is a single entry in a ContextMenuModel.
type ContextMenuItem struct {
	Label       string            // Displayed text (ignored when IsSeparator is true)
	SubLabel    string            // Optional second line shown below Label (e.g. the actual value)
	Help        string            // Optional help text (shown in helpline when item is focused)
	IsSeparator bool              // When true, renders as a horizontal divider and is not selectable
	IsHeader    bool              // When true, renders Label as a non-selectable title row
	Disabled    bool              // When true, renders dimmed and is not selectable or executable
	Action      tea.Cmd          // Executed when the item is selected; should close the dialog itself
	SubItems    []ContextMenuItem // When non-empty, selecting opens a submenu instead of Action
}

// itemHeight returns the number of display rows an item occupies (2 if it has a SubLabel).
func itemHeight(item ContextMenuItem) int {
	if !item.IsSeparator && !item.IsHeader && item.SubLabel != "" {
		return 2
	}
	return 1
}

// ContextMenuModel is a small positioned popup menu that appears near the cursor.
// It is designed to be shown via ShowDialogMsg so AppModel stacks it on the dialog stack.
//
// Positioning: the model stores the raw right-click coordinates (clickX, clickY).
// IsMaximized() returns true so model_view.go uses DialogMaximized mode, giving
// lx=1, ly=2. The Layers() method compensates with layer.X = menuX-1, layer.Y = menuY-2.
type ContextMenuModel struct {
	items      []ContextMenuItem
	cursor     int // currently highlighted item index
	clickX     int // original right-click screen position
	clickY     int
	screenW    int
	screenH    int

	// Computed positions (set in recalculate)
	menuX int
	menuY int
	menuW int // inner content width (without border)
	menuH int // inner content height (without border)

	offset     int // scroll offset for long menus
	maxVisible int // max items to show at once (default 12)
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
// Returning true makes model_view.go use DialogMaximized positioning:
// lx=EdgeIndent=1, ly=ContentStartY=2. Our Layers() compensates.
func (m *ContextMenuModel) IsMaximized() bool { return true }

// Init implements tea.Model.
func (m *ContextMenuModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *ContextMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.screenW = msg.Width
		m.screenH = msg.Height
		m.recalculate()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, Keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, Keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, Keys.Enter):
			return m, m.executeSelected()
		case key.Matches(msg, Keys.Right):
			// Right arrow opens a submenu if the focused item has one.
			if m.cursor >= 0 && m.cursor < len(m.items) && len(m.items[m.cursor].SubItems) > 0 {
				return m, m.executeSelected()
			}
		case key.Matches(msg, Keys.Esc):
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}

	case LayerHitMsg:
		if msg.ID == "ctxmenu.bg" {
			// Click outside the item list — close
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		// Per-item hit: "ctxmenu.item-N"
		if strings.HasPrefix(msg.ID, "ctxmenu.item-") {
			idxStr := strings.TrimPrefix(msg.ID, "ctxmenu.item-")
			idx := parseIntSafe(idxStr)
			if idx >= 0 && idx < len(m.items) && !m.items[idx].IsSeparator && !m.items[idx].IsHeader && !m.items[idx].Disabled {
				m.cursor = idx
				if msg.Button == tea.MouseLeft {
					return m, m.executeSelected()
				}
			}
		}

	case LayerWheelMsg:
		if msg.Button == tea.MouseWheelDown {
			m.scrollBy(1)
		} else {
			m.scrollBy(-1)
		}

	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelDown {
			m.scrollBy(1)
		} else {
			m.scrollBy(-1)
		}

	case tea.MouseMotionMsg:
		// Update the highlighted item on mouse hover so HelpText() reflects the
		// item under the cursor. model_update.go calls h.HelpText() after every
		// dialog Update(), so the helpline updates automatically.
		idx := m.itemIndexAt(msg.X, msg.Y)
		if idx >= 0 && idx < len(m.items) && !m.items[idx].IsSeparator && !m.items[idx].IsHeader && !m.items[idx].Disabled {
			m.cursor = idx
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
			if m.cursor >= m.offset+m.maxVisible {
				m.offset = m.cursor - m.maxVisible + 1
			}
		}
	}

	return m, nil
}

// itemIndexAt returns the absolute item index at screen coordinates (x, y),
// or -1 if the coordinates are outside the menu content area.
func (m *ContextMenuModel) itemIndexAt(x, y int) int {
	// Content rows begin at menuY+1 (inside the top border).
	rowY := m.menuY + 1
	absIdx := m.offset
	for _, item := range m.visibleItems() {
		h := itemHeight(item)
		if y >= rowY && y < rowY+h && x >= m.menuX+1 && x < m.menuX+m.menuW+3 {
			return absIdx
		}
		rowY += h
		absIdx++
	}
	return -1
}

// ViewString renders the context menu as a string.
func (m *ContextMenuModel) ViewString() string {
	if m.menuW <= 0 {
		return ""
	}

	ctx := GetActiveContext()
	bgStyle := theme.ThemeSemanticStyle("{{|Dialog|}}")
	normalStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	selectedStyle := theme.ThemeSemanticStyle("{{|ItemSelected|}}")
	subLabelStyle := theme.ThemeSemanticStyle("{{|HelpItem|}}")
	disabledStyle := normalStyle.Faint(true)

	// Compute which items are visible
	visible := m.visibleItems()

	var lines []string
	absIdx := m.offset
	for _, item := range visible {
		if item.IsSeparator {
			sepChar := "─"
			if !ctx.LineCharacters {
				sepChar = "-"
			}
			sep := bgStyle.Render(" " + strings.Repeat(sepChar, m.menuW) + " ")
			lines = append(lines, sep)
			absIdx++
			continue
		}
		if item.IsHeader {
			headerStyle := theme.ThemeSemanticStyle("{{|EnvBuiltin|}}").Bold(true)
			lbl := item.Label
			if lipgloss.Width(lbl) > m.menuW {
				lbl = TruncateRight(lbl, m.menuW)
			}
			pad := m.menuW - lipgloss.Width(lbl)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, bgStyle.Render(" ")+headerStyle.Render(lbl)+bgStyle.Render(strings.Repeat(" ", pad)+" "))
			absIdx++
			continue
		}

		label := item.Label
		if len(item.SubItems) > 0 {
			label += " ▶"
		}
		// Truncate if needed
		if lipgloss.Width(label) > m.menuW {
			label = TruncateRight(label, m.menuW)
		}
		// Pad to full width
		pad := m.menuW - lipgloss.Width(label)
		if pad < 0 {
			pad = 0
		}
		line := " " + label + strings.Repeat(" ", pad) + " "

		if item.Disabled {
			lines = append(lines, MaintainBackground(disabledStyle.Render(line), bgStyle))
		} else if absIdx == m.cursor {
			lines = append(lines, MaintainBackground(selectedStyle.Render(line), selectedStyle))
			if item.SubLabel != "" {
				sl := item.SubLabel
				if lipgloss.Width(sl) > m.menuW {
					sl = TruncateRight(sl, m.menuW)
				}
				slPad := m.menuW - lipgloss.Width(sl)
				if slPad < 0 {
					slPad = 0
				}
				lines = append(lines, MaintainBackground(selectedStyle.Render(" "+sl+strings.Repeat(" ", slPad)+" "), selectedStyle))
			}
		} else {
			lines = append(lines, MaintainBackground(normalStyle.Render(line), bgStyle))
			if item.SubLabel != "" {
				sl := item.SubLabel
				if lipgloss.Width(sl) > m.menuW {
					sl = TruncateRight(sl, m.menuW)
				}
				slPad := m.menuW - lipgloss.Width(sl)
				if slPad < 0 {
					slPad = 0
				}
				lines = append(lines, MaintainBackground(subLabelStyle.Background(bgStyle.GetBackground()).Render(" "+sl+strings.Repeat(" ", slPad)+" "), bgStyle))
			}
		}
		absIdx++
	}

	content := strings.Join(lines, "\n")

	// Draw border using the same ctx-driven approach as other dialogs
	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.NormalBorder()
	} else {
		border = AsciiBorder
	}
	borderBG := bgStyle.GetBackground()

	boxStyle := lipgloss.NewStyle().
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG).
		Background(borderBG)

	return boxStyle.Render(content)
}

// View implements tea.Model.
func (m *ContextMenuModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView.
// model_view.go adds lx=1, ly=2 to our layer X/Y (DialogMaximized offsets).
// We compensate so the menu lands at exactly (menuX, menuY) in screen coordinates.
func (m *ContextMenuModel) Layers() []*lipgloss.Layer {
	content := m.ViewString()
	if content == "" {
		return nil
	}
	// lx=EdgeIndent=1, ly=ContentStartY=headerH(1)+SeparatorHeight(1)=2 for DialogMaximized
	const lx, ly = 1, 2
	layerX := m.menuX - lx
	layerY := m.menuY - ly
	return []*lipgloss.Layer{
		lipgloss.NewLayer(content).X(layerX).Y(layerY).Z(ZScreen).ID("Dialog.ContextMenu"),
	}
}

// GetHitRegions implements the hit-region interface so mouse events route correctly.
func (m *ContextMenuModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	if m.menuW <= 0 {
		return nil
	}
	var regions []HitRegion

	// The full box (border included) as a background catch-all
	totalW := m.menuW + 2 + 2 // content + 2 padding + 2 border
	totalH := m.menuH + 2     // menuH is already in display rows
	regions = append(regions, HitRegion{
		ID:     "ctxmenu.bg",
		X:      m.menuX,
		Y:      m.menuY,
		Width:  totalW,
		Height: totalH,
		ZOrder: ZDialog + 5,
		Label:  "Context Menu",
	})

	// Per-item rows (inside border, starting at menuY+1)
	visible := m.visibleItems()
	absIdx := m.offset
	rowY := m.menuY + 1
	for _, item := range visible {
		h := itemHeight(item)
		if !item.IsSeparator && !item.IsHeader {
			regions = append(regions, HitRegion{
				ID:     "ctxmenu.item-" + itoa(absIdx),
				X:      m.menuX + 1,
				Y:      rowY,
				Width:  m.menuW + 2, // content + 2 padding spaces
				Height: h,
				ZOrder: ZDialog + 10,
				Label:  item.Label,
			})
		}
		absIdx++
		rowY += h
	}
	return regions
}

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
	// Adjust scroll offset to keep cursor visible
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.maxVisible {
		m.offset = m.cursor - m.maxVisible + 1
	}
}

func (m *ContextMenuModel) scrollBy(delta int) {
	m.offset += delta
	if m.offset < 0 {
		m.offset = 0
	}
	maxOff := len(m.items) - m.maxVisible
	if maxOff < 0 {
		maxOff = 0
	}
	if m.offset > maxOff {
		m.offset = maxOff
	}
	// Keep cursor within visible range
	isUnselectable := func(i int) bool {
		return m.items[i].IsSeparator || m.items[i].IsHeader || m.items[i].Disabled
	}
	if m.cursor < m.offset {
		m.cursor = m.offset
		for m.cursor < len(m.items) && isUnselectable(m.cursor) {
			m.cursor++
		}
	}
	if m.cursor >= m.offset+m.maxVisible {
		m.cursor = m.offset + m.maxVisible - 1
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

	// Item with sub-items: open a child context menu positioned to the right.
	if len(item.SubItems) > 0 {
		subItems := item.SubItems
		// Position submenu to the right of this menu, aligned with the selected row.
		subClickX := m.menuX + m.menuW + 3 // right edge of parent box
		subClickY := m.menuY + (m.cursor - m.offset)
		sw, sh := m.screenW, m.screenH
		return func() tea.Msg {
			return ShowDialogMsg{Dialog: NewContextMenuModel(subClickX, subClickY, sw, sh, subItems)}
		}
	}

	if action := item.Action; action != nil {
		return action
	}
	return func() tea.Msg { return CloseDialogMsg{} }
}

func (m *ContextMenuModel) visibleItems() []ContextMenuItem {
	end := m.offset + m.maxVisible
	if end > len(m.items) {
		end = len(m.items)
	}
	if m.offset >= len(m.items) {
		return nil
	}
	return m.items[m.offset:end]
}

func (m *ContextMenuModel) visibleCount() int {
	return len(m.visibleItems())
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
