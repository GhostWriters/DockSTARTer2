package tui

import (
	"DockSTARTer2/internal/console"
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
	Tag         string  // Display name (first letter used as shortcut)
	Desc        string  // Description text
	Help        string  // Help line text shown when item is selected
	Shortcut    rune    // Keyboard shortcut (usually first letter of Tag)
	Action      tea.Cmd // Command to execute when selected (Enter)
	SpaceAction tea.Cmd // Command to execute when Space is pressed

	// Checklist support
	Selectable    bool // Whether this item can be toggled
	Selected      bool // Current selection state
	IsCheckbox    bool // Whether this is a checkbox [ ] / [x]
	IsRadioButton bool // Whether this is a radio button ( ) / (*)
	Checked       bool // Current checkbox/radio state

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

// calculateMaxTagLength returns the maximum visual width of item tags
func calculateMaxTagLength(items []MenuItem) int {
	maxTagLen := 0
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
	}
	return maxTagLen
}

// calculateMaxTagAndDescLength returns the maximum visual width of item tags and descriptions
func calculateMaxTagAndDescLength(items []MenuItem) (maxTagLen, maxDescLen int) {
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		descWidth := lipgloss.Width(GetPlainText(item.Desc))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}
	return
}

// menuItemDelegate implements list.ItemDelegate for standard navigation menus
type menuItemDelegate struct {
	menuID    string
	maxTagLen int
	focused   bool
	flowMode  bool
}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d menuItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
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
	}

	// Render tag with first-letter highlighting (if no semantic tags present)
	tag := menuItem.Tag
	var tagStr string
	if len(tag) > 0 {
		// If tag already contains theme tags, render it normally (highlights might be ruined)
		if strings.Contains(tag, "{{") {
			tagStr = RenderThemeText(tag, tagStyle)
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

	tagWidth := lipgloss.Width(GetPlainText(tag))

	// Checkbox visual [ ] or [x] / Radio visual ( ) or (*)
	checkbox := ""
	if menuItem.IsRadioButton {
		var cb string
		if ctx.LineCharacters {
			cb = radioUnselected
			if menuItem.Checked {
				cb = radioSelected
			}
		} else {
			cb = radioUnselectedAscii
			if menuItem.Checked {
				cb = radioSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsCheckbox {
		var cb string
		if ctx.LineCharacters {
			cb = checkUnselected
			if menuItem.Checked {
				cb = checkSelected
			}
		} else {
			cb = checkUnselectedAscii
			if menuItem.Checked {
				cb = checkSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	}

	// Highlighting for gap and description
	// Use itemStyle as base for description so highlight applies, or dialogBG if not selected
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected {
		descStyle = itemStyle
	}
	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", d.maxTagLen-tagWidth+2)

	// Calculate checkbox/radio width dynamically
	cbWidth := lipgloss.Width(GetPlainText(checkbox))

	availableWidth := m.Width() - (d.maxTagLen + 2) - 2 - cbWidth
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, descStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
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
	focused   bool
	flowMode  bool
}

func (d checkboxItemDelegate) Height() int                             { return 1 }
func (d checkboxItemDelegate) Spacing() int                            { return 0 }
func (d checkboxItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d checkboxItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	// Handle separator items
	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = SemanticStyle("{{|Theme_TagKey|}}").Render(menuItem.Tag)
		} else {
			content = strutil.Repeat("─", max(0, m.Width()-2))
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
	}

	// Render checkbox for selectable items
	var checkbox string
	if menuItem.Selectable {
		if ctx.LineCharacters {
			cbGlyph := checkUnselected
			if menuItem.Selected {
				cbGlyph = checkSelected
			}
			// Use tag style for checkbox to match user request
			// Render just the glyph with tagStyle, and add a neutral space after it
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := checkUnselectedAscii
			if menuItem.Selected {
				cbContent = checkSelectedAscii
			}
			// Use tag style for checkbox to match user request
			checkbox = tagStyle.Render(cbContent)
		}
	}

	var tagStr string
	tag := menuItem.Tag
	if len(tag) > 0 {
		if isSelected {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			if strings.Contains(tag, "{{") {
				tagStr = RenderThemeText(tag, tagStyle)
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
	}

	tagWidth := lipgloss.Width(GetPlainText(checkbox)) + lipgloss.Width(GetPlainText(tag))

	// Highlighting for gap and description
	// Use itemStyle as base for description so highlight applies, or dialogBG if not selected
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected {
		descStyle = itemStyle
	}
	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", d.maxTagLen-tagWidth+2)
	availableWidth := m.Width() - (d.maxTagLen + 2) - 2
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, descStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
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

	// Sub-menu mode (for consolidated screens)
	subMenuMode bool
	focusedSub  bool // If false, use normal borders. If true, use thick borders.

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
	flowMode     bool // Whether to layout items horizontally instead of vertically

	// Unified layout (deterministic sizing)
	layout DialogLayout
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
	} else {
		// New: Auto-focus the currently selected radio option if no persistent session
		for i, item := range items {
			if item.IsRadioButton && item.Checked {
				cursor = i
				break
			}
		}
	}

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	// Calculate max tag and desc length for sizing
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(items)

	// Calculate initial width based on actual content
	// Width = tag + spacing(2) + desc + margins(2)
	initialWidth := maxTagLen + 2 + maxDescLen + 2

	// Create bubbles list

	// Size based on actual number of items for dynamic sizing
	delegate := menuItemDelegate{menuID: id, maxTagLen: maxTagLen, focused: true}

	// Calculate height
	itemHeight := delegate.Height()
	spacing := delegate.Spacing()
	totalItemHeight := len(items) * itemHeight
	if len(items) > 1 && spacing > 0 {
		totalItemHeight += (len(items) - 1) * spacing
	}
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
		focusedItem: FocusSelectBtn,
		list:        l,
		showExit:    true, // Default to show Exit button
	}
}

// SetFocused sets whether this menu's dialog border is rendered as focused (thick)
// or unfocused (normal). Called by AppModel when the log panel takes focus.
func (m *MenuModel) SetFocused(f bool) {
	m.focused = f
	m.updateDelegate()
}

// SetMaximized sets whether the menu should expand to fill available space
func (m *MenuModel) SetMaximized(maximized bool) {
	m.maximized = maximized
	m.calculateLayout()
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

// SetSubMenuMode sets whether the menu acts as a sub-component with simpler borders
func (m *MenuModel) SetSubMenuMode(enabled bool) {
	m.subMenuMode = enabled
}

// SetSubFocused sets the focus state specifically for sub-menu mode (thick vs normal border)
func (m *MenuModel) SetSubFocused(focused bool) {
	m.focusedSub = focused
	m.updateDelegate()
}

// IsActive returns whether this menu actually has focus (accounting for subMenuMode)
func (m *MenuModel) IsActive() bool {
	if m.subMenuMode {
		return m.focusedSub
	}
	return m.focused
}

// updateDelegate refreshes the list delegate with the current focus state
func (m *MenuModel) updateDelegate() {
	focused := m.IsActive()
	maxTagLen := calculateMaxTagLength(m.items)
	if m.checkboxMode {
		m.list.SetDelegate(checkboxItemDelegate{menuID: m.id, maxTagLen: maxTagLen, focused: focused, flowMode: m.flowMode})
	} else {
		m.list.SetDelegate(menuItemDelegate{menuID: m.id, maxTagLen: maxTagLen, focused: focused, flowMode: m.flowMode})
	}
}

// SetCheckboxMode enables checkbox rendering for app selection
func (m *MenuModel) SetCheckboxMode(enabled bool) {
	m.checkboxMode = enabled
	m.updateDelegate()
}

// Index returns the current cursor index
func (m *MenuModel) Index() int {
	return m.cursor
}

// Select programmatically sets the cursor index
func (m *MenuModel) Select(index int) {
	if index >= 0 && index < len(m.items) {
		m.cursor = index
		m.list.Select(index)
		menuSelectedIndices[m.id] = index
	}
}

// GetItems returns the current list of MenuItems
func (m *MenuModel) GetItems() []MenuItem {
	return m.items
}

// SetItems updates the menu items and refreshes the bubbles list
func (m *MenuModel) SetItems(items []MenuItem) {
	m.items = items

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)

	// Update delegate with new max tag length and focus
	m.updateDelegate()
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

// Init implements tea.Model
func (m *MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
					m.focusedItem = FocusSelectBtn

					// In checkbox mode, toggle selection instead of executing action
					if m.checkboxMode {
						m.ToggleSelectedItem()
						return m, nil
					}

					// For individual checkbox items, treat click as Space (toggle)
					if m.items[i].IsCheckbox && m.items[i].Selectable {
						return m.handleSpace()
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

	// Handle MouseMiddle (Space-like, non-pointing)
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseMiddle {
		return m.handleSpace()
	}

	// Handle mouse wheel scrolling
	if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if mwMsg.Button == tea.MouseWheelUp {
			m.list.CursorUp()
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
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
			m.list.CursorDown()
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
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

		// Up / Down: navigate the list (independent of button focus)
		case key.Matches(keyMsg, Keys.Up):
			m.list.CursorUp()
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
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
			m.list.CursorDown()
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
				m.list.CursorDown()
				if m.list.Index() == len(m.items)-1 && m.items[len(m.items)-1].IsSeparator {
					break
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.PageUp):
			pageHeight := m.list.Height()
			if pageHeight < 1 {
				pageHeight = 5 // Fallback
			}
			newIndex := m.list.Index() - pageHeight
			if newIndex < 0 {
				newIndex = 0
			}
			m.list.Select(newIndex)
			// Skip separators automatically (moving down to find first selectable)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageUp):
			pageHeight := m.list.Height() / 2
			if pageHeight < 1 {
				pageHeight = 1
			}
			newIndex := m.list.Index() - pageHeight
			if newIndex < 0 {
				newIndex = 0
			}
			m.list.Select(newIndex)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.PageDown):
			pageHeight := m.list.Height()
			if pageHeight < 1 {
				pageHeight = 5 // Fallback
			}
			newIndex := m.list.Index() + pageHeight
			if newIndex >= len(m.items) {
				newIndex = len(m.items) - 1
			}
			m.list.Select(newIndex)
			// Skip separators automatically (moving up to find first selectable)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.HalfPageDown):
			pageHeight := m.list.Height() / 2
			if pageHeight < 1 {
				pageHeight = 1
			}
			newIndex := m.list.Index() + pageHeight
			if newIndex >= len(m.items) {
				newIndex = len(m.items) - 1
			}
			m.list.Select(newIndex)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.Home):
			m.list.Select(0)
			// Skip separators automatically
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() < len(m.items)-1 {
				m.list.CursorDown()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		case key.Matches(keyMsg, Keys.End):
			m.list.Select(len(m.items) - 1)
			// Skip separators automatically (moving up)
			for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator && m.list.Index() > 0 {
				m.list.CursorUp()
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil

		// Right: move to next button (wraps within button row)
		case key.Matches(keyMsg, Keys.Right):
			m.focusedItem = m.nextButtonFocus()
			return m, nil

		// Left: move to prev button (wraps within button row)
		case key.Matches(keyMsg, Keys.Left):
			m.focusedItem = m.prevButtonFocus()
			return m, nil

		// Enter: select/confirm current focused element
		case key.Matches(keyMsg, Keys.Enter):
			return m.handleEnter()

		// Space: select/toggle current focused element
		case key.Matches(keyMsg, Keys.Space):
			return m.handleSpace()

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

	// Delegate to list only when list has focus (Deprecated/Removed logic)
	// But we still need to update the list model if it has internal logic (like spinners, though we turned them off)
	// Since we handle navigation manually, we might not need this, but for safety:
	// m.list, cmd = m.list.Update(msg)
	// Actually, let's remove the conditional since FocusList is gone.
	// But bubbles/list update handles keys too... strictly strictly speaking we should be careful.
	// Since handle keys manually, let's just NOT call list.Update for keys.
	// For other messages (like window size, which we handled), maybe?
	// Let's just remove the FocusList check and Update list only if strictly needed?
	// For now, removing the block entirely as we handle everything manually.

	return m, nil
}

// nextFocus cycles focus forward through all focus areas (list → buttons).
// Used for future Tab/window-cycling logic.
func (m *MenuModel) nextFocus() FocusItem {
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
func (m *MenuModel) prevFocus() FocusItem {
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
func (m *MenuModel) nextButtonFocus() FocusItem {
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
func (m *MenuModel) prevButtonFocus() FocusItem {
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

func (m *MenuModel) handleEnter() (tea.Model, tea.Cmd) {
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
func (m *MenuModel) handleSpace() (tea.Model, tea.Cmd) {
	if m.checkboxMode {
		m.ToggleSelectedItem()
		return m, nil
	}

	// Always prioritize checkbox toggle if item is one
	selectedItem := m.list.SelectedItem()
	if item, ok := selectedItem.(MenuItem); ok {
		if item.IsCheckbox && item.Selectable {
			item.Checked = !item.Checked
			// Update the item in our internal list too so state persists
			idx := m.list.Index()
			if idx >= 0 && idx < len(m.items) {
				m.items[idx].Checked = item.Checked
				// Update list.Model internal items to reflect changes immediately
				m.list.SetItem(idx, item)
			}

			if item.SpaceAction != nil {
				return m, item.SpaceAction
			}
			return m, nil
		}
	}

	if m.focusedItem == FocusList {
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.SpaceAction != nil {
				return m, item.SpaceAction
			}
			// Fallback: Space on a list item can also trigger its primary Action if no SpaceAction defined
			if item.Action != nil {
				return m, item.Action
			}
		}
	}

	if m.spaceAction != nil {
		return m, m.spaceAction
	}
	// Fallback to select button if focused
	if m.focusedItem == FocusSelectBtn {
		return m.handleEnter()
	}
	return m, nil
}

// ViewString renders the menu content as a string (for compositing)
func (m *MenuModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	// In Sub-menu mode, we render a simpler view without the global backdrop logic
	if m.subMenuMode {
		return m.viewSubMenu()
	}

	if m.flowMode {
		return m.renderFlow()
	}

	styles := GetStyles()

	// Get list view and apply background color
	listView := m.list.View()
	// Wrap with dialog background to eliminate black space
	listViewStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground())
	if m.maximized {
		// Only force height when maximized — ensures list fills the full dialog.
		listViewStyle = listViewStyle.Height(m.list.Height())
	}
	listView = listViewStyle.Render(listView)

	// Wrap list in its own border (no padding, items have their own margins).
	// Use rounded border when focused for higher visual fidelity.
	listStyle := styles.Dialog.
		Padding(0, 0)
	listStyle = ApplyInnerBorder(listStyle, m.focused, styles.LineCharacters)
	borderedList := listStyle.Render(listView)

	// Calculate widths from known dimensions, not measured content
	// For maximized: use m.width - outer border (2) for the content width
	// For non-maximized: use list width + inner padding (2) for consistency
	listWidth := m.list.Width()

	var outerContentWidth int
	if m.maximized {
		// Outer dialog fills available space: content = total - outer border
		outerContentWidth = m.width - 2
	} else {
		// Non-maximized: outer content = bordered list (listWidth + 2) + padding (2)
		outerContentWidth = listWidth + 2 + 2
	}

	// Inner components should fit within outerContentWidth - padding (2)
	innerWidth := outerContentWidth - 2
	// bordered list width = innerWidth, so list content = innerWidth - 2
	targetWidth := innerWidth

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

	// Ensure button box has same width as list for proper vertical alignment
	paddedButtons := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Width(outerContentWidth).
		Padding(0, 1).
		Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (left-aligned, matching outer content width)
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(outerContentWidth).
			Padding(0, 1).
			Align(lipgloss.Left)

		// Use RenderThemeText for proper inline tag handling (not just leading tags)
		subStr := RenderThemeText(m.subtitle, subtitleStyle)

		subtitle := subtitleStyle.Render(subStr)
		innerParts = append(innerParts, subtitle)
	}

	// Add list box and button box with NO gaps to maximize list space
	// JoinVertical will stack them tightly
	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts and standardize TrimRight to prevent implicit gaps
	for i, part := range innerParts {
		innerParts[i] = strings.TrimRight(part, "\n")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Force total content height to match the calculated budget (Total - Outer Borders - Shadow)
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - DialogBorderHeight - m.layout.ShadowHeight
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(styles.Dialog.GetBackground()).
				Render(content)
		}
	}
	// Determine target height for the outer dialog
	targetHeight := 0
	if m.maximized {
		targetHeight = m.height
	}

	// Wrap in bordered dialog with title embedded in border

	// Wrap in bordered dialog with title embedded in border
	// Use outerContentWidth - the known content width for the outer dialog
	var dialog string
	if m.title != "" {
		dialog = m.renderBorderWithTitle(content, outerContentWidth, targetHeight, m.focused, false)
	} else {
		// No title: use focus-aware inner rounded border
		dialogStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		dialogStyle = ApplyInnerBorder(dialogStyle, m.focused, styles.LineCharacters)
		if targetHeight > 0 {
			dialogStyle = dialogStyle.Height(targetHeight - DialogBorderHeight)
		}
		dialog = dialogStyle.Render(content)
	}

	// Add shadow
	dialog = AddShadow(dialog)

	return dialog
}

// View implements tea.Model
func (m *MenuModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// getButtonSpecs returns the current button configuration based on state
func (m *MenuModel) getButtonSpecs() []ButtonSpec {
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
func (m *MenuModel) renderSimpleButtons(contentWidth int) string {
	// Build button specs with focus state and explicit zone IDs
	specs := m.getButtonSpecs()

	return RenderCenteredButtons(contentWidth, specs...)
}

func (m *MenuModel) renderButtons(contentWidth int) string {
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

func (m *MenuModel) renderButtonBox(buttons string, contentWidth int) string {
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

func (m *MenuModel) renderDialog(menuContent, buttonBox string, listWidth int) string {
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

	// Join all parts with careful newline trimming to prevent extra gaps
	for i, part := range innerParts {
		innerParts[i] = strings.TrimRight(part, "\n")
	}
	innerContent := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Add padding inside the outer border
	paddedContent := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1).
		Render(innerContent)

	// Outer content width = listWidth + padding (2)
	outerContentWidth := listWidth + 2

	// Wrap with outer border
	var dialogBox string

	if m.title != "" {
		dialogBox = m.renderBorderWithTitle(paddedContent, outerContentWidth, 0, m.focused, false)
	} else {
		// No title, use standard border
		boxStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)

		if !styles.DrawBorders {
			boxStyle = boxStyle.Border(lipgloss.HiddenBorder())
		} else {
			boxStyle = Apply3DBorder(boxStyle)
		}
		dialogBox = boxStyle.Render(paddedContent)
	}

	// Add shadow effect
	dialogBox = AddShadow(dialogBox)

	// Just return the dialogBox - centering and backdrop are handled by AppModel.View
	return dialogBox
}

func (m *MenuModel) renderBorderWithTitle(content string, contentWidth int, targetHeight int, focused bool, rounded bool) string {
	return RenderBorderedBoxCtx(m.title, content, contentWidth, targetHeight, focused, rounded, GetActiveContext())
}

// SetSize updates the menu dimensions and resizes the list
func (m *MenuModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// If in flow mode, calculate height based on content
	if m.flowMode {
		flowLines := m.GetFlowHeight(width)
		// +2 for top/bottom borders
		m.layout.ViewportHeight = flowLines
		m.layout.Height = flowLines + 2
	} else {
		m.calculateLayout()
	}
}

func (m *MenuModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// 1. Subtitle Height
	subtitleHeight := 0
	if m.subtitle != "" {
		subtitleHeight = lipgloss.Height(m.subtitle)
	}

	// 2. Button and Shadow Heights
	layout := GetLayout()
	buttonHeight := DialogButtonHeight
	shadowHeight := 0
	hasShadow := currentConfig.UI.Shadow
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 3. Vertical Budgeting Logic
	var listHeight, overhead int
	var maxListHeight int
	if m.subMenuMode {
		// Sub-menu overhead is just the subtitle and its own borders (2)
		overhead = subtitleHeight + layout.BorderHeight()
		maxListHeight = m.height - overhead
	} else {
		// Full dialog overhead: borders, subtitle, buttons, shadow
		// Account for internal gaps: 1 after subtitle, 1 before buttons
		internalOverhead := subtitleHeight
		if m.subtitle != "" {
			internalOverhead += 1 // Gap after subtitle
		}

		maxListHeight = layout.DialogContentHeight(m.height, internalOverhead, true, hasShadow)

		// Subtract 1 more for the gap before buttons (not covered by DialogButtonHeight)
		maxListHeight -= 1

		// Total overhead = total height - available list height
		overhead = m.height - maxListHeight
	}

	if maxListHeight < 3 {
		maxListHeight = 3
	}

	// 4. Calculate intrinsic list height based on items
	itemHeight := 1
	spacing := 0
	totalItemHeight := len(m.items) * itemHeight
	if len(m.items) > 1 && spacing > 0 {
		totalItemHeight += (len(m.items) - 1) * spacing
	}

	// Final list height is whichever is smaller: intrinsic or maximum
	listHeight = totalItemHeight
	if m.maximized || listHeight > maxListHeight {
		listHeight = maxListHeight
	}

	// 5. Calculate list width based on content
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(m.items)
	// Width = tag + spacing(2) + desc + margins(2) + buffer(4)
	listWidth := maxTagLen + 2 + maxDescLen + 2 + 4

	// Constrain width to fit within terminal dialog area using Layout helpers
	var maxListWidth int
	if m.subMenuMode {
		// Submenu: just has its own border, content fills the rest
		maxListWidth, _ = layout.InnerContentSize(m.width, m.height)
	} else {
		// Full dialog: outer border + inner list border + padding
		// Total overhead = outer border (2) + inner border (2) + padding (2) = 6
		maxListWidth = m.width - 6
	}
	if maxListWidth < 34 {
		maxListWidth = 34
	}

	// If maximized, fill the space. Otherwise, ensure minimum for buttons.
	if m.maximized {
		listWidth = maxListWidth
	} else {
		if listWidth < 34 {
			listWidth = 34
		}
		if listWidth > maxListWidth {
			listWidth = maxListWidth
		}
	}

	m.layout = DialogLayout{
		Width:          m.width,
		Height:         m.height,
		HeaderHeight:   subtitleHeight,
		ViewportHeight: listHeight,
		ButtonHeight:   buttonHeight,
		ShadowHeight:   shadowHeight,
		Overhead:       overhead,
	}

	m.list.SetSize(listWidth, listHeight)
}

// renderFlow renders items in a horizontal flow layout for compact menus
func (m *MenuModel) renderFlow() string {
	ctx := GetActiveContext()
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	// Use Layout helpers for consistent border calculations
	layout := GetLayout()
	maxWidth, _ := layout.InnerContentSize(m.width, m.height)
	// Subtract 2 for internal 1-char margin on each side (matching standard list menus)
	if maxWidth > 2 {
		maxWidth -= 2
	}

	var lines []string
	var currentLine []string
	currentLineWidth := 0
	itemSpacing := 3

	for i, item := range m.items {
		isSelected := i == m.cursor && m.IsActive()

		tagStyle := SemanticStyle("{{|Theme_Tag|}}").Background(dialogBG)
		keyStyle := SemanticStyle("{{|Theme_TagKey|}}").Background(dialogBG)

		if isSelected {
			tagStyle = SemanticStyle("{{|Theme_TagSelected|}}")
			keyStyle = SemanticStyle("{{|Theme_TagKeySelected|}}")
		}

		// Checkbox/Radio visual
		prefix := ""
		if item.IsRadioButton {
			var cb string
			if ctx.LineCharacters {
				cb = radioUnselected + " "
				if item.Checked {
					cb = radioSelected + " "
				}
			} else {
				cb = radioUnselectedAscii
				if item.Checked {
					cb = radioSelectedAscii
				}
			}
			prefix = tagStyle.Render(cb)
		} else if item.IsCheckbox {
			var cb string
			if ctx.LineCharacters {
				cb = checkUnselected + " "
				if item.Checked {
					cb = checkSelected + " "
				}
			} else {
				cb = checkUnselectedAscii
				if item.Checked {
					cb = checkSelectedAscii
				}
			}
			prefix = tagStyle.Render(cb)
		}

		// Tag with first-letter shortcut
		tag := item.Tag
		tagStr := ""
		if len(tag) > 0 {
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(tag) > 1 {
				letterIdx = 1
			}
			p := tag[:letterIdx]
			f := string(tag[letterIdx])
			r := tag[letterIdx+1:]
			tagStr = tagStyle.Render(p) + keyStyle.Render(f) + tagStyle.Render(r)
		}

		itemContent := prefix + tagStr

		// For non-checkbox/non-radio items (e.g. dropdowns), append
		// the Desc inline so the current value is visible without clicking.
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			desc := item.Desc
			if isSelected {
				// Strip OptionValue tag so the value inherits selection colors (e.g. red background)
				desc = strings.ReplaceAll(desc, "{{|Theme_OptionValue|}}", "")
			}
			// Include leading space in RenderThemeText so it gets the correct background
			itemContent += RenderThemeText(" "+desc, tagStyle)
		}

		// Mark zone for click detection
		zoneID := fmt.Sprintf("item-%s-%d", m.id, i)
		itemContent = zone.Mark(zoneID, itemContent)

		// Hard reset after each element to ensure background colors (like selection)
		// don't bleed into the itemSpacing gaps.
		itemContent += console.CodeReset

		itemWidth := lipgloss.Width(GetPlainText(itemContent))

		// Check if we need to wrap
		if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
			lines = append(lines, strings.Join(currentLine, strutil.Repeat(" ", itemSpacing)))
			currentLine = []string{itemContent}
			currentLineWidth = itemWidth
		} else {
			currentLine = append(currentLine, itemContent)
			if currentLineWidth > 0 {
				currentLineWidth += itemSpacing
			}
			currentLineWidth += itemWidth
		}
	}

	// Add final line
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, strutil.Repeat(" ", itemSpacing)))
	}

	// Apply 1-char side margins to match MenuItemDelegate.Render
	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(maxWidth + 2)
	for i, line := range lines {
		lines[i] = lineStyle.Render(line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *MenuModel) viewSubMenu() string {
	styles := GetStyles()
	var content string
	if m.flowMode {
		content = m.renderFlow()
	} else {
		content = m.list.View()
	}

	// Inner content with maintained background
	inner := MaintainBackground(content, styles.Dialog)

	// Use Layout helpers for consistent border calculations
	layout := GetLayout()
	innerWidth, _ := layout.InnerContentSize(m.width, m.height)

	// When in flow mode, the height is strictly determined by the content + borders
	targetHeight := m.height
	if m.flowMode {
		_, targetHeight = layout.OuterTotalSize(0, m.layout.ViewportHeight)
	}
	return m.renderBorderWithTitle(inner, innerWidth, targetHeight, m.focusedSub, true)
}

// GetFlowHeight calculates required lines for horizontal layout given the available width
func (m *MenuModel) GetFlowHeight(width int) int {
	if len(m.items) == 0 {
		return 0
	}

	ctx := GetActiveContext()

	maxWidth := width
	// Subtract 2 for borders and 2 for internal 1-char margins (matching standard list menus)
	if maxWidth > 4 {
		maxWidth -= 4
	}

	lines := 1
	currentLineWidth := 0
	itemSpacing := 3

	for _, item := range m.items {
		// Dynamic width calculation
		cbWidth := 0
		if item.IsRadioButton || item.IsCheckbox {
			glyph := ""
			if ctx.LineCharacters {
				if item.IsRadioButton {
					glyph = radioUnselected + " "
				} else {
					glyph = checkUnselected + " "
				}
			} else {
				if item.IsRadioButton {
					glyph = radioUnselectedAscii
				} else {
					glyph = checkUnselectedAscii
				}
			}
			cbWidth = lipgloss.Width(glyph)
		}

		itemWidth := cbWidth + lipgloss.Width(GetPlainText(item.Tag))

		// For non-checkbox/non-radio items, include the Desc width
		// to match renderFlow which appends Desc inline
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			itemWidth += 1 + lipgloss.Width(GetPlainText(item.Desc))
		}

		if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
			lines++
			currentLineWidth = itemWidth
		} else {
			if currentLineWidth > 0 {
				currentLineWidth += itemSpacing
			}
			currentLineWidth += itemWidth
		}
	}

	return lines
}

// SetFlowMode toggles horizontal flow layout
func (m *MenuModel) SetFlowMode(flow bool) {
	m.flowMode = flow
}

// SetHeaderVisibility toggles background/title for sub-menus
func (m *MenuModel) SetHeaderVisibility(visible bool) {
	m.list.SetShowTitle(visible)
}

// Title returns the menu title
func (m *MenuModel) Title() string {
	return m.title
}

// HelpText returns the current item's help text
func (m *MenuModel) HelpText() string {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].Help
	}
	return ""
}

// Cursor returns the current selection index
func (m *MenuModel) Cursor() int {
	return m.cursor
}
