package tui

import (
	"DockSTARTer2/internal/strutil"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// MenuItem defines an item in a menu
type MenuItem struct {
	Tag      string  // Display name (first letter used as shortcut)
	Desc     string  // Description text
	Help     string  // Help line text shown when item is selected
	Shortcut rune    // Keyboard shortcut (usually first letter of Tag)
	Action   tea.Cmd // Command to execute when selected

	// Checklist support
	Selectable bool // Whether this item can be toggled
	Selected   bool // Current selection state

	// Layout support
	IsSeparator bool // Whether this is a non-selectable header/separator

	// Metadata
	IsUserDefined bool              // Whether this is a user-defined app (for color parity)
	Metadata      map[string]string // Optional extra data (e.g. internal app name)
}

// Implement list.Item interface for bubbles/list
func (i MenuItem) FilterValue() string { return i.Tag }
func (i MenuItem) Title() string       { return i.Tag }
func (i MenuItem) Description() string { return i.Desc }

// menuItemDelegate implements list.ItemDelegate for standard navigation menus
type menuItemDelegate struct {
	menuID    string
	maxTagLen int
}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d menuItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	dialogStyle := SemanticStyle("{{|Theme_Dialog|}}")
	dialogBG := dialogStyle.GetBackground()
	isSelected := index == m.Index()

	// Handle separator items
	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = SemanticStyle("{{|Theme_TagKey|}}").Render(menuItem.Tag)
		} else {
			content = strutil.Repeat("─", m.Width()-2)
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	itemStyle := SemanticStyle("{{|Theme_Item|}}")
	tagStyle := SemanticStyle("{{|Theme_Tag|}}")
	keyStyle := SemanticStyle("{{|Theme_TagKey|}}")

	if isSelected {
		itemStyle = SemanticStyle("{{|Theme_ItemSelected|}}")
		tagStyle = SemanticStyle("{{|Theme_TagSelected|}}")
		keyStyle = SemanticStyle("{{|Theme_TagKeySelected|}}")
	} else {
		itemStyle = itemStyle.Background(dialogBG)
		tagStyle = tagStyle.Background(dialogBG)
		keyStyle = keyStyle.Background(dialogBG)
	}

	// Render tag with first-letter highlighting
	tag := menuItem.Tag
	var tagStr string
	if len(tag) > 0 {
		letterIdx := 0
		if strings.HasPrefix(tag, "[") && len(tag) > 1 {
			letterIdx = 1
		}
		prefix := tag[:letterIdx]
		firstLetter := string(tag[letterIdx])
		rest := tag[letterIdx+1:]
		tagStr = tagStyle.Render(prefix) + keyStyle.Render(firstLetter) + tagStyle.Render(rest)
	}

	tagWidth := lipgloss.Width(menuItem.Tag)
	paddingSpaces := strutil.Repeat(" ", d.maxTagLen-tagWidth+2)

	availableWidth := m.Width() - (d.maxTagLen + 2) - 2
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, itemStyle)
	descStr = itemStyle.MaxWidth(availableWidth).Height(1).Render(descStr)
	descLine := strings.Split(descStr, "\n")[0]

	line := tagStr + neutralStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
	line = lineStyle.Render(line)

	zoneID := fmt.Sprintf("item-%s-%d", d.menuID, index)
	fmt.Fprint(w, zone.Mark(zoneID, line))
}

// checkboxItemDelegate implements specialized styling for app selection screens
type checkboxItemDelegate struct {
	menuID    string
	maxTagLen int
}

func (d checkboxItemDelegate) Height() int                             { return 1 }
func (d checkboxItemDelegate) Spacing() int                            { return 0 }
func (d checkboxItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d checkboxItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	dialogStyle := SemanticStyle("{{|Theme_Dialog|}}")
	dialogBG := dialogStyle.GetBackground()
	isSelected := index == m.Index()

	// Handle separator items
	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = SemanticStyle("{{|Theme_TagKey|}}").Render(menuItem.Tag)
		} else {
			content = strutil.Repeat("─", ma	x(0, m.Width()-2))
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)

	itemStyle := SemanticStyle("{{|Theme_Item|}}").Background(dialogBG)
	tagStyle := SemanticStyle("{{|Theme_Tag|}}").Background(dialogBG)
	keyStyle := SemanticStyle("{{|Theme_TagKey|}}").Background(dialogBG)

	if isSelected {
		itemStyle = SemanticStyle("{{|Theme_ItemSelected|}}")
		tagStyle = SemanticStyle("{{|Theme_TagSelected|}}")
		keyStyle = SemanticStyle("{{|Theme_TagKeySelected|}}")
	}

	// Render checkbox for selectable items
	var checkbox string
	if menuItem.Selectable {
		cbContent := " "
		if menuItem.Selected {
			cbContent = "*"
		}

		// Use tag style for checkbox to match user request
		if isSelected {
			checkbox = tagStyle.Render("["+cbContent+"]") + neutralStyle.Render(" ")
		} else {
			checkbox = neutralStyle.Render("[") + tagStyle.Render(cbContent) + neutralStyle.Render("] ")
		}
	}

	var tagStr string
	tag := menuItem.Tag
	if len(tag) > 0 {
		if isSelected {
			tagStr = tagStyle.Render(tag)
		} else {
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(tag) > 1 {
				letterIdx = 1
			}
			prefix := tag[:letterIdx]
			firstLetter := string(tag[letterIdx])
			rest := tag[letterIdx+1:]
			tagStr = tagStyle.Render(prefix) + keyStyle.Render(firstLetter) + tagStyle.Render(rest)
		}
	}

	tagWidth := 0
	if menuItem.Selectable {
		tagWidth += 4
	}
	tagWidth += lipgloss.Width(menuItem.Tag)
	paddingSpaces := strutil.Repeat(" ", d.maxTagLen-tagWidth+2)

	availableWidth := m.Width() - (d.maxTagLen + 2) - 2
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, itemStyle)
	descStr = itemStyle.MaxWidth(availableWidth).Height(1).Render(descStr)
	descLine := strings.Split(descStr, "\n")[0]

	line := checkbox + tagStr + neutralStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
	line = lineStyle.Render(line)

	zoneID := fmt.Sprintf("item-%s-%d", d.menuID, index)
	fmt.Fprint(w, zone.Mark(zoneID, line))
}

// MenuModel represents a selectable menu
type MenuModel struct {
	id       string // Unique identifier for selection persistence
	title    string // Menu title
	subtitle string // Optional subtitle/description
	items    []MenuItem
	cursor   int // Current selection
	width    int
	height   int

	// Focus state
	focused     bool
	focusedItem FocusItem // Which element has focus

	// Back action (nil if no back button)
	backAction tea.Cmd

	// Bubbles list model
	list      list.Model
	maximized bool // Whether to maximize the dialog to fill available space
	showExit  bool // Whether to show Exit button (default true for main menus)

	// Key override actions
	escAction   tea.Cmd
	enterAction tea.Cmd
	spaceAction tea.Cmd

	// Custom button labels
	selectLabel string
	backLabel   string
	exitLabel   string

	// Checkbox mode (for app selection)
	checkboxMode bool
}

// FocusItem represents which UI element has focus
type FocusItem int

const (
	FocusList FocusItem = iota
	FocusSelectBtn
	FocusBackBtn
	FocusExitBtn
)

// menuSelectedIndices persists menu selection across visits
var menuSelectedIndices = make(map[string]int)

// NewMenuModel creates a new menu model
func NewMenuModel(id, title, subtitle string, items []MenuItem, backAction tea.Cmd) MenuModel {
	// Set default shortcuts from first letter of Tag
	for i := range items {
		if items[i].Shortcut == 0 && len(items[i].Tag) > 0 {
			items[i].Shortcut = []rune(items[i].Tag)[0]
		}
	}

	// Restore previous selection
	cursor := 0
	if idx, ok := menuSelectedIndices[id]; ok && idx >= 0 && idx < len(items) {
		cursor = idx
	}

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	// Calculate max tag and desc length for sizing
	// Use lipgloss.Width() instead of len() for proper terminal width
	maxTagLen := 0
	maxDescLen := 0
	for _, item := range items {
		tagWidth := lipgloss.Width(item.Tag)
		descWidth := lipgloss.Width(item.Desc)
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}

	// Calculate initial width based on actual content
	// Width = tag + spacing(2) + desc + margins(2)
	initialWidth := maxTagLen + 2 + maxDescLen + 2

	// Create bubbles list with CUSTOM delegate (Phase 2 - custom styling!)
	// Size based on actual number of items for dynamic sizing
	delegate := menuItemDelegate{menuID: id, maxTagLen: maxTagLen}

	// Calculate proper height based on delegate metrics
	// Total height = (items * itemHeight) + ((items - 1) * spacing)
	itemHeight := delegate.Height()
	spacing := delegate.Spacing()
	totalItemHeight := len(items) * itemHeight
	if len(items) > 1 && spacing > 0 {
		totalItemHeight += (len(items) - 1) * spacing
	}
	// Try exact height with no buffer now that pagination is disabled
	initialHeight := totalItemHeight

	l := list.New(listItems, delegate, initialWidth, initialHeight)
	// Don't set l.Title - we render title in border instead
	l.SetShowTitle(false) // Disable list's built-in title rendering
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false) // Disable pagination indicators

	// Set list background to match dialog background (not black!)
	styles := GetStyles()
	dialogBg := styles.Dialog.GetBackground()
	l.Styles.NoItems = l.Styles.NoItems.Background(dialogBg)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Background(dialogBg)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Background(dialogBg)

	// Set initial cursor position
	if cursor > 0 && cursor < len(items) {
		l.Select(cursor)
	}

	return MenuModel{
		id:          id,
		title:       title,
		subtitle:    subtitle,
		items:       items,
		cursor:      cursor,
		backAction:  backAction,
		focused:     true,
		focusedItem: FocusList,
		list:        l,
		showExit:    true, // Default to showing Exit button
	}
}

// SetFocused sets whether this menu's dialog border is rendered as focused (thick)
// or unfocused (normal). Called by AppModel when the log panel takes focus.
func (m *MenuModel) SetFocused(f bool) {
	m.focused = f
}

// SetMaximized sets whether the menu should expand to fill available space
func (m *MenuModel) SetMaximized(maximized bool) {
	m.maximized = maximized
}

// SetShowExit sets whether to show the Exit button
func (m *MenuModel) SetShowExit(show bool) {
	m.showExit = show
}

// IsMaximized returns whether the menu is maximized
func (m MenuModel) IsMaximized() bool {
	return m.maximized
}

// HasDialog returns whether the menu has an active dialog overlay
func (m MenuModel) HasDialog() bool {
	return false // Menus don't have nested dialogs
}

// SetCheckboxMode enables checkbox rendering for app selection
func (m *MenuModel) SetCheckboxMode(enabled bool) {
	m.checkboxMode = enabled
	if enabled {
		// Switch to checkbox delegate
		maxTagLen := 0
		for _, item := range m.items {
			tagWidth := lipgloss.Width(item.Tag)
			if tagWidth > maxTagLen {
				maxTagLen = tagWidth
			}
		}
		m.list.SetDelegate(checkboxItemDelegate{menuID: m.id, maxTagLen: maxTagLen})
	} else {
		// Switch back to standard delegate
		maxTagLen := 0
		for _, item := range m.items {
			tagWidth := lipgloss.Width(item.Tag)
			if tagWidth > maxTagLen {
				maxTagLen = tagWidth
			}
		}
		m.list.SetDelegate(menuItemDelegate{menuID: m.id, maxTagLen: maxTagLen})
	}
}

// SetEscAction sets a custom action for the Escape key
func (m *MenuModel) SetEscAction(action tea.Cmd) {
	m.escAction = action
}

// SetEnterAction sets a custom action for the Enter key
func (m *MenuModel) SetEnterAction(action tea.Cmd) {
	m.enterAction = action
}

// SetSpaceAction sets a custom action for the Space key
func (m *MenuModel) SetSpaceAction(action tea.Cmd) {
	m.spaceAction = action
}

// SetButtonLabels sets custom labels for the buttons
func (m *MenuModel) SetButtonLabels(selectLabel, backLabel, exitLabel string) {
	m.selectLabel = selectLabel
	m.backLabel = backLabel
	m.exitLabel = exitLabel
}

// ToggleSelectedItem toggles the selected state of the current item (for checkbox mode)
func (m *MenuModel) ToggleSelectedItem() {
	if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Selectable {
		m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		// Update the list item too
		listItems := make([]list.Item, len(m.items))
		for i, item := range m.items {
			listItems[i] = item
		}
		m.list.SetItems(listItems)
	}
}

// GetItems returns the current menu items (for reading selection state)
func (m MenuModel) GetItems() []MenuItem {
	return m.items
}

// Init implements tea.Model
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model (Phase 1: delegate to bubbles/list)
func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window size first — delegate to SetSize (single source of truth)
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.SetSize(wsMsg.Width, wsMsg.Height)
	}

	// Handle mouse events using BubbleZones
	if mouseMsg, ok := msg.(tea.MouseClickMsg); ok && mouseMsg.Button == tea.MouseLeft {
		// Check each zone to see if the click is within bounds
		// Menu item zones - clicking executes immediately (same as clicking Select)
		for i := 0; i < len(m.items); i++ {
			// Skip separator items
			if m.items[i].IsSeparator {
				continue
			}

			zoneID := fmt.Sprintf("item-%s-%d", m.id, i)
			if zoneInfo := zone.Get(zoneID); zoneInfo != nil {
				if zoneInfo.InBounds(mouseMsg) {
					// Select the item
					m.list.Select(i)
					m.cursor = i
					menuSelectedIndices[m.id] = i
					m.focusedItem = FocusList

					// In checkbox mode, toggle selection instead of executing action
					if m.checkboxMode {
						m.ToggleSelectedItem()
						return m, nil
					}

					return m.handleEnter()
				}
			}
		}

		// Button zones
		if zoneInfo := zone.Get("btn-select"); zoneInfo != nil {
			if zoneInfo.InBounds(mouseMsg) {
				m.focusedItem = FocusSelectBtn
				return m.handleEnter()
			}
		}

		if m.backAction != nil {
			if zoneInfo := zone.Get("btn-back"); zoneInfo != nil {
				if zoneInfo.InBounds(mouseMsg) {
					m.focusedItem = FocusBackBtn
					return m.handleEnter()
				}
			}
		}

		if zoneInfo := zone.Get("btn-exit"); zoneInfo != nil {
			if zoneInfo.InBounds(mouseMsg) {
				m.focusedItem = FocusExitBtn
				return m.handleEnter()
			}
		}
		return m, nil
	}

	// Handle mouse wheel scrolling
	if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if mwMsg.Button == tea.MouseWheelUp {
			m.focusedItem = FocusList
			m.list.CursorUp()
			for m.items[m.list.Index()].IsSeparator {
				m.list.CursorUp()
				if m.list.Index() == 0 && m.items[0].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil
		}
		if mwMsg.Button == tea.MouseWheelDown {
			m.focusedItem = FocusList
			m.list.CursorDown()
			for m.items[m.list.Index()].IsSeparator {
				m.list.CursorDown()
				if m.list.Index() == len(m.items)-1 && m.items[len(m.items)-1].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil
		}
	}

	// Handle key events
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		// Help dialog (takes priority so ? doesn't get eaten by list)
		case key.Matches(keyMsg, Keys.Help):
			return m, func() tea.Msg { return ShowDialogMsg{Dialog: newHelpDialogModel()} }

		// Tab / ShiftTab: switch between screen-level elements
		// (e.g., menu dialog ↔ header version widget in the future)
		// A whole dialog/window is one screen element; buttons/list within it are not.
		// Does nothing until multi-element screens are implemented.
		case key.Matches(keyMsg, Keys.Tab), key.Matches(keyMsg, Keys.ShiftTab):
			return m, nil

		// Up / Down: navigate the list (and return focus to the list from buttons)
		case key.Matches(keyMsg, Keys.Up):
			m.focusedItem = FocusList
			m.list.CursorUp()
			// Skip separators automatically
			for m.items[m.list.Index()].IsSeparator {
				m.list.CursorUp()
				// Safety: if we hit top and it's a separator (unlikely with header), stop or wrap?
				// Simple safety: if index is 0 and it's a separator, try going down instead?
				// For now, assume top item isn't a separator or just let bubbles handle bounds.
				if m.list.Index() == 0 && m.items[0].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.Down):
			m.focusedItem = FocusList
			m.list.CursorDown()
			// Skip separators automatically
			for m.items[m.list.Index()].IsSeparator {
				m.list.CursorDown()
				if m.list.Index() == len(m.items)-1 && m.items[len(m.items)-1].IsSeparator {
					// If last item is separator, go back up one?
					// Or just let it be.
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		// Right: move to next button (from list → first button; wraps within button row)
		case key.Matches(keyMsg, Keys.Right):
			m.focusedItem = m.nextButtonFocus()
			return m, nil

		// Left: move to prev button (from list → last button; wraps within button row)
		case key.Matches(keyMsg, Keys.Left):
			m.focusedItem = m.prevButtonFocus()
			return m, nil

		// Enter: select/confirm current focused element
		case key.Matches(keyMsg, Keys.Enter):
			return m.handleEnter()

		// Space: select/toggle current focused element
		case key.Matches(keyMsg, Keys.Space):
			if m.focusedItem == FocusList {
				if m.checkboxMode {
					m.ToggleSelectedItem()
					return m, nil
				}
				if m.spaceAction != nil {
					return m, m.spaceAction
				}
			}
			// Fallback to select button if focused
			if m.focusedItem == FocusSelectBtn {
				return m.handleEnter()
			}
			return m, nil

		// Esc: back if available, else exit
		case key.Matches(keyMsg, Keys.Esc):
			if m.backAction != nil {
				return m, m.backAction
			}
			return m, tea.Quit

		// Dynamic Hotkeys
		default:
			// In v2, KeyPressMsg has Text field directly
			if keyMsg.Text != "" {
				keyRune := strings.ToLower(keyMsg.Text)

				// 1. Check Menu Items first (priority)
				for i, item := range m.items {
					// Handle tagged tags like [F] properly
					tag := strings.Trim(item.Tag, "[]")
					if len(tag) > 0 {
						firstChar := strings.ToLower(string(tag[0]))
						if firstChar == keyRune {
							m.list.Select(i)
							m.cursor = i
							menuSelectedIndices[m.id] = i
							m.focusedItem = FocusList
							return m.handleEnter()
						}
					}
				}

				// 2. Check Buttons (if no item matched)
				// Determine available buttons using shared helper
				buttons := m.getButtonSpecs()

				if idx, found := CheckButtonHotkeys(keyMsg, buttons); found {
					// Map index back to FocusItem
					// Use zone IDs or index to map, assuming standard order
					// But since we use dynamic buttons, we should map based on the button's intended action
					// Or just map index if we know the order: Select, [Back], [Exit]
					// Helper: map button text/zone to FocusItem?
					// Simpler: iterate known types and check if active in specs?
					// Because getButtonSpecs builds in order: Select, Back, Exit.

					// Re-derive focus based on index loop
					focusMap := []FocusItem{FocusSelectBtn}
					if m.backAction != nil {
						focusMap = append(focusMap, FocusBackBtn)
					}
					if m.showExit {
						focusMap = append(focusMap, FocusExitBtn)
					}

					if idx < len(focusMap) {
						m.focusedItem = focusMap[idx]
					}
					return m.handleEnter()
				}
			}
		}
	}

	// Delegate to list only when list has focus
	if m.focusedItem == FocusList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Sync cursor with list index for helpline updates
		m.cursor = m.list.Index()
		menuSelectedIndices[m.id] = m.cursor
		return m, cmd
	}

	return m, nil
}

// nextFocus cycles focus forward through all focus areas (list → buttons).
// Used for future Tab/window-cycling logic.
func (m MenuModel) nextFocus() FocusItem {
	switch m.focusedItem {
	case FocusList:
		return FocusSelectBtn
	case FocusSelectBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		if m.showExit {
			return FocusExitBtn
		}
		// If no back and no exit, cycle back to list?
		return FocusList
	case FocusBackBtn:
		if m.showExit {
			return FocusExitBtn
		}
		return FocusList
	case FocusExitBtn:
		return FocusList
	}
	return FocusList
}

// prevFocus cycles focus backward through all focus areas.
// Used for future ShiftTab/window-cycling logic.
func (m MenuModel) prevFocus() FocusItem {
	switch m.focusedItem {
	case FocusList:
		if m.showExit {
			return FocusExitBtn
		}
		if m.backAction != nil {
			return FocusBackBtn
		}
		return FocusSelectBtn
	case FocusSelectBtn:
		return FocusList
	case FocusBackBtn:
		return FocusSelectBtn
	case FocusExitBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		return FocusSelectBtn
	}
	return FocusList
}

// nextButtonFocus cycles the Right arrow through buttons only.
// From the list or last button, moves to the first button (Select).
func (m MenuModel) nextButtonFocus() FocusItem {
	switch m.focusedItem {
	case FocusList, FocusExitBtn:
		return FocusSelectBtn
	case FocusSelectBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		if m.showExit {
			return FocusExitBtn
		}
		return FocusSelectBtn // wrap around if only one button
	case FocusBackBtn:
		if m.showExit {
			return FocusExitBtn
		}
		return FocusSelectBtn
	}
	return FocusSelectBtn
}

// prevButtonFocus cycles the Left arrow through buttons only.
// From the list or first button, moves to the last button (Exit).
func (m MenuModel) prevButtonFocus() FocusItem {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		if m.showExit {
			return FocusExitBtn
		}
		if m.backAction != nil {
			return FocusBackBtn
		}
		return FocusSelectBtn
	case FocusExitBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		return FocusSelectBtn
	case FocusBackBtn:
		return FocusSelectBtn
	}
	return FocusExitBtn
}

func (m MenuModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		// Get selected item from bubbles list
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.Action != nil {
				// Update cursor for persistence
				m.cursor = m.list.Index()
				menuSelectedIndices[m.id] = m.cursor
				return m, item.Action
			}
		}
	case FocusBackBtn:
		if m.backAction != nil {
			return m, m.backAction
		}
	case FocusExitBtn:
		return m, tea.Quit
	}

	// Fallback to model-level enter action if item had no action
	if m.enterAction != nil {
		return m, m.enterAction
	}

	return m, nil
}

// ViewString renders the menu content as a string (for compositing)
func (m MenuModel) ViewString() string {
	styles := GetStyles()

	// Get list view and apply background color
	listView := m.list.View()
	// Wrap with dialog background to eliminate black space
	listView = lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Render(listView)

	// Wrap list in its own border (no padding, items have their own margins)
	listStyle := styles.Dialog.
		Padding(0, 0)
	listStyle = ApplyStraightBorder(listStyle, styles.LineCharacters)
	borderedList := listStyle.Render(listView)

	// Calculate the target width for all content
	// This is the width of the bordered list
	targetWidth := lipgloss.Width(borderedList)

	// Render buttons to match the same bordered width
	// Account for the padding (2) that renderButtonBox will add
	buttonInnerWidth := targetWidth - 2
	buttonRow := m.renderSimpleButtons(buttonInnerWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, buttonInnerWidth)

	// Add equal margins around both boxes for spacing
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	paddedList := marginStyle.Render(borderedList)
	paddedListWidth := lipgloss.Width(paddedList)

	// Ensure button box has same width as list for proper vertical alignment
	paddedButtons := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Width(paddedListWidth).
		Padding(0, 1).
		Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (left-aligned, matching padded width)
	if m.subtitle != "" {
		paddedWidth := lipgloss.Width(paddedList)

		subtitleStyle := styles.Dialog.
			Width(paddedWidth).
			Padding(0, 1).
			Align(lipgloss.Left)

		// Use RenderThemeText for proper inline tag handling (not just leading tags)
		subStr := RenderThemeText(m.subtitle, subtitleStyle)

		subtitle := subtitleStyle.Render(subStr)
		innerParts = append(innerParts, subtitle)
	}

	// Add list box and button box
	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts - they should all have the same width now
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Wrap in bordered dialog with title embedded in border
	var dialog string
	if m.title != "" {
		dialog = m.renderBorderWithTitle(content, targetWidth)
	} else {
		// No title: focused uses thick border, background uses normal border
		dialogStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		if m.focused {
			dialogStyle = ApplyThickBorder(dialogStyle, styles.LineCharacters)
		} else {
			dialogStyle = ApplyStraightBorder(dialogStyle, styles.LineCharacters)
		}
		dialog = dialogStyle.Render(content)
	}

	// Add shadow
	dialog = AddShadow(dialog)

	return dialog
}

// View implements tea.Model
func (m MenuModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// getButtonSpecs returns the current button configuration based on state
func (m MenuModel) getButtonSpecs() []ButtonSpec {
	var specs []ButtonSpec

	// Select Button
	label := m.selectLabel
	if label == "" {
		label = "Select"
	}
	specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusSelectBtn, ZoneID: "btn-select"})

	// Back Button
	if m.backAction != nil {
		label := m.backLabel
		if label == "" {
			label = "Back"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusBackBtn, ZoneID: "btn-back"})
	}

	// Exit Button
	if m.showExit {
		label := m.exitLabel
		if label == "" {
			label = "Exit"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusExitBtn, ZoneID: "btn-exit"})
	}

	return specs
}

// renderSimpleButtons creates a button row with evenly spaced sections
func (m MenuModel) renderSimpleButtons(contentWidth int) string {
	// Build button specs with focus state and explicit zone IDs
	specs := m.getButtonSpecs()

	return RenderCenteredButtons(contentWidth, specs...)
}

/* OLD CUSTOM RENDERING - Kept for reference (Phase 2 will add back custom styling)
func (m MenuModel) viewOld() string {
	styles := GetStyles()

	// Calculate dimensions
	maxTagLen := 0
	maxDescLen := 0
	for _, item := range m.items {
		tagWidth := lipgloss.Width(item.Tag)
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		descWidth := lipgloss.Width(item.Desc)
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}

	colPadding := 2
	contentWidth := maxTagLen + colPadding + maxDescLen + 4

	// Ensure minimum width for title
	titleWidth := lipgloss.Width(m.title) + 4
	if titleWidth > contentWidth {
		contentWidth = titleWidth
	}

	// Calculate minimum width for buttons
	// Button texts: "<Select>" (8), "<Back>" (6), "<Exit>" (6)
	buttonTexts := []string{"<Select>", "<Exit>"}
	if m.backAction != nil {
		buttonTexts = []string{"<Select>", "<Back>", "<Exit>"}
	}

	// Calculate minimum width per button section (button text + some padding)
	minButtonSectionWidth := 12 // Minimum space per button for readability
	totalButtonWidth := len(buttonTexts) * minButtonSectionWidth

	// Account for button box padding and border (4 chars total)
	minButtonBoxWidth := totalButtonWidth

	if minButtonBoxWidth > contentWidth {
		contentWidth = minButtonBoxWidth
	}

	// Constrain to terminal width (leave space for outer borders, padding, and shadow)
	// Outer dialog has: padding(2) + border(2) + shadow(2) = 6 extra chars
	// List/button boxes inside have: padding(2) + border(2) = 4 extra chars
	// Total overhead: ~10 chars
	maxAvailableWidth := m.width - 10
	if maxAvailableWidth < 30 {
		maxAvailableWidth = 30 // Absolute minimum
	}
	if contentWidth > maxAvailableWidth {
		contentWidth = maxAvailableWidth
		// Button text will be clipped if sections are too narrow
	}

	// Build the menu content
	var b strings.Builder

	// Menu items
	for i, item := range m.items {
		isSelected := i == m.cursor

		// Render tag with highlighted first letter
		tagRunes := []rune(item.Tag)
		var tagStr string
		if len(tagRunes) > 0 {
			firstLetter := string(tagRunes[0])
			rest := string(tagRunes[1:])

			if isSelected {
				// Selected: use selected colors
				keyStyle := styles.TagKeySelected
				restStyle := styles.ItemSelected
				tagStr = keyStyle.Render(firstLetter) + restStyle.Render(rest)
			} else {
				// Normal: highlight first letter
				keyStyle := styles.TagKey
				restStyle := styles.TagNormal
				tagStr = keyStyle.Render(firstLetter) + restStyle.Render(rest)
			}
		}

		// Pad tag to align descriptions
		tagWidth := lipgloss.Width(item.Tag)
		padding := strutil.Repeat(" ", maxTagLen-tagWidth+colPadding)

		// Render description
		var descStr string
		if isSelected {
			descStr = styles.ItemSelected.Render(padding + item.Desc)
		} else {
			descStr = styles.ItemNormal.Render(padding + item.Desc)
		}

		// Combine tag and description
		line := tagStr + descStr

		// Apply full-line background for selected item
		if isSelected {
			line = styles.ItemSelected.Width(contentWidth).Render(line)
		} else {
			line = styles.ItemNormal.Width(contentWidth).Render(line)
		}

		b.WriteString(line)
		if i < len(m.items)-1 {
			b.WriteString("\n")
		}
	}

	// Create list content (no zones yet - we'll add them after full render)
	listContent := b.String()

	// Apply padding and border to list
	listStyle := styles.Dialog.
		Padding(0, 1)
	listStyle = Apply3DBorder(listStyle)
	paddedList := listStyle.Render(listContent)

	// Create buttons in a full bordered box matching menu width
	listWidth := lipgloss.Width(paddedList)

	// listWidth includes padding(2) + border(2) = 4 extra characters
	// To match the total width, buttonBox needs the same total width
	// renderButtonBox adds padding(2) + border(2), so we need inner content width = listWidth - 4
	innerWidth := listWidth - 4
	buttons := m.renderButtons(innerWidth)
	buttonBox := m.renderButtonBox(buttons, innerWidth)

	// Wrap in dialog frame (button box has its own border)
	view := m.renderDialog(paddedList, buttonBox, listWidth)

	// NOW add zones to the fully rendered output
	view = m.addZonesToRenderedDialog(view)

	// Scan zones for mouse support (zone manager tracks positions)
	return m.zoneManager.Scan(view)
}
*/

func (m MenuModel) renderButtons(contentWidth int) string {
	styles := GetStyles()

	// Select button
	selectStyle := styles.ButtonInactive
	if m.focusedItem == FocusSelectBtn {
		selectStyle = styles.ButtonActive
	}
	selectBtn := selectStyle.Render("<Select>")

	// Back button (optional)
	var backBtn string
	if m.backAction != nil {
		backStyle := styles.ButtonInactive
		if m.focusedItem == FocusBackBtn {
			backStyle = styles.ButtonActive
		}
		backBtn = backStyle.Render("<Back>")
	}

	// Exit button
	exitStyle := styles.ButtonInactive
	if m.focusedItem == FocusExitBtn {
		exitStyle = styles.ButtonActive
	}
	exitBtn := exitStyle.Render("<Exit>")

	// Collect all buttons
	var buttonStrs []string
	buttonStrs = append(buttonStrs, selectBtn)
	if m.backAction != nil {
		buttonStrs = append(buttonStrs, backBtn)
	}
	buttonStrs = append(buttonStrs, exitBtn)

	// Divide available width into equal sections (one per button)
	numButtons := len(buttonStrs)
	sectionWidth := contentWidth / numButtons

	// Center each button in its section
	var sections []string
	for _, btn := range buttonStrs {
		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(styles.Dialog.GetBackground()).
			Render(btn)
		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
}

func (m MenuModel) renderButtonBox(buttons string, contentWidth int) string {
	styles := GetStyles()

	// Center buttons in content width
	centeredButtons := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Background(styles.Dialog.GetBackground()).
		Render(buttons)

	// Add padding for spacing (no border since buttons have their own)
	boxStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	return boxStyle.Render(centeredButtons)
}

func (m MenuModel) renderDialog(menuContent, buttonBox string, listWidth int) string {
	styles := GetStyles()

	// Build inner content parts
	var innerParts []string

	// Subtitle (left-aligned, matching content width)
	if m.subtitle != "" {
		subtitle := styles.Dialog.
			Width(listWidth).
			Padding(0, 1).
			Align(lipgloss.Left).
			Render(m.subtitle)
		innerParts = append(innerParts, subtitle)
	}

	// Add list box (already has its own border)
	innerParts = append(innerParts, menuContent)

	// Add button box (already has its own border)
	innerParts = append(innerParts, buttonBox)

	// Join all parts
	innerContent := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Add padding inside the outer border
	paddedContent := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1).
		Render(innerContent)

	// Wrap with outer border
	var dialogBox string

	if m.title != "" {
		dialogBox = m.renderBorderWithTitle(paddedContent, listWidth)
	} else {
		// No title, use standard border
		boxStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		boxStyle = Apply3DBorder(boxStyle)
		dialogBox = boxStyle.Render(paddedContent)
	}

	// Add shadow effect
	dialogBox = AddShadow(dialogBox)

	// Just return the dialogBox - centering and backdrop are handled by AppModel.View
	return dialogBox
}

func (m MenuModel) renderBorderWithTitle(content string, contentWidth int) string {
	styles := GetStyles()
	// Focused dialogs use thick border, background dialogs use normal border
	var border lipgloss.Border
	if styles.LineCharacters {
		if m.focused {
			border = lipgloss.ThickBorder()
		} else {
			border = lipgloss.NormalBorder()
		}
	} else {
		if m.focused {
			border = thickAsciiBorder
		} else {
			border = asciiBorder
		}
	}

	// Style definitions
	borderBG := styles.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(borderBG)
	titleStyle := styles.DialogTitle.
		Background(borderBG)

	// Parse color tags from title and update style
	var title string
	title, titleStyle = ParseTitleTags(m.title, titleStyle)

	// Get actual content width
	lines := strings.Split(content, "\n")
	actualWidth := 0
	if len(lines) > 0 {
		actualWidth = lipgloss.Width(lines[0])
	}

	// Build top border with title using T connectors
	// Format: ────┤ Title ├──── (normal) or ━━━━┫ Title ┣━━━━ (thick/focused)
	// Spaces are rendered with border style, not title style
	var leftT, rightT string
	if styles.LineCharacters {
		if m.focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if m.focused {
			leftT = "H" // thick ASCII T-connector
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}
	// Total title section width: leftT + space + title + space + rightT
	titleSectionLen := 1 + 1 + lipgloss.Width(title) + 1 + 1
	leftPad := (actualWidth - titleSectionLen) / 2
	rightPad := actualWidth - titleSectionLen - leftPad

	var result strings.Builder

	// Top border
	result.WriteString(borderStyleLight.Render(border.TopLeft))
	result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, leftPad)))
	result.WriteString(borderStyleLight.Render(leftT))
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(titleStyle.Render(title))
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(borderStyleLight.Render(rightT))
	result.WriteString(borderStyleLight.Render(strutil.Repeat(border.Top, rightPad)))
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	// Content lines with left/right borders
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))

		// Pad line to actualWidth with styled padding to prevent black splotches
		textWidth := lipgloss.Width(line)
		padding := ""
		if textWidth < actualWidth {
			padding = lipgloss.NewStyle().Background(borderBG).Render(strutil.Repeat(" ", actualWidth-textWidth))
		}

		// Use MaintainBackground to handle internal color resets within the line
		fullLine := MaintainBackground(line+padding, styles.Dialog)
		result.WriteString(fullLine)

		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	// Bottom border
	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strutil.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}

// SetSize updates the menu dimensions and resizes the list
func (m *MenuModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Recalculate list dimensions (same logic as WindowSizeMsg handler)
	maxTagLen := 0
	maxDescLen := 0
	for _, item := range m.items {
		tagWidth := lipgloss.Width(item.Tag)
		descWidth := lipgloss.Width(item.Desc)
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}
	// Width = tag + spacing(2) + desc + margins(2) + buffer(4)
	listWidth := maxTagLen + 2 + maxDescLen + 2 + 4

	// Constrain width to fit within terminal
	// Account for: outer border (2) + inner border (2) + margins (6) + shadow (2 if enabled)
	if m.width > 0 {
		widthOverhead := 10 // outer border (2) + inner border (2) + margins (6)
		if currentConfig.UI.Shadow {
			widthOverhead += 2
		}
		maxListWidth := m.width - widthOverhead
		if maxListWidth < 30 {
			maxListWidth = 30
		}
		if listWidth > maxListWidth {
			listWidth = maxListWidth
		}
	}

	// Calculate list height based on items
	itemHeight := 1
	spacing := 0
	totalItemHeight := len(m.items) * itemHeight
	if len(m.items) > 1 && spacing > 0 {
		totalItemHeight += (len(m.items) - 1) * spacing
	}
	listHeight := totalItemHeight

	// When maximized, constrain list height to fit within available space
	if m.maximized && m.height > 0 {
		// Window fills contentAreaHeight (m.height). Calculate list height from that.
		subtitleHeight := 0
		if m.subtitle != "" {
			subtitleHeight = lipgloss.Height(m.subtitle)
		}
		shadowHeight := 0
		if currentConfig.UI.Shadow {
			shadowHeight = 1
		}
		// Fixed elements: outer border (2) + inner border (2) + buttons (3: border + label + border) + shadow + subtitle
		fixedElements := 2 + 2 + 3 + shadowHeight + subtitleHeight
		maxListHeight := m.height - fixedElements
		if maxListHeight < 5 {
			maxListHeight = 5
		}
		if listHeight > maxListHeight {
			listHeight = maxListHeight
		}
	}

	m.list.SetSize(listWidth, listHeight)
}

// Title returns the menu title
func (m MenuModel) Title() string {
	return m.title
}

// HelpText returns the current item's help text
func (m MenuModel) HelpText() string {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].Help
	}
	return ""
}

// Cursor returns the current selection index
func (m MenuModel) Cursor() int {
	return m.cursor
}
