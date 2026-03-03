package tui

import (
	"DockSTARTer2/internal/strutil"
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		// Set width to exactly m.Width() so inner text of m.Width()-2 plus 2 chars padding fits perfectly without wrapping
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

	// tagWidth removed as it was unused

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

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))

	// Calculate checkbox/radio width dynamically
	cbWidth := lipgloss.Width(GetPlainText(checkbox))

	// Available width: list width - outer padding(2) - (cbWidth + maxTagLen + 3)
	availableWidth := m.Width() - 2 - (cbWidth + d.maxTagLen + 3)
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
	fmt.Fprint(w, line)

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
	if menuItem.IsCheckbox {
		if ctx.LineCharacters {
			cbGlyph := checkUnselected
			if menuItem.Checked {
				cbGlyph = checkSelected
			}
			// Use tag style for checkbox to match user request
			// Render just the glyph with tagStyle, and add a neutral space after it
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := checkUnselectedAscii
			if menuItem.Checked {
				cbContent = checkSelectedAscii
			}
			// Use tag style for checkbox to match user request
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
		}
	} else if menuItem.IsRadioButton {
		if ctx.LineCharacters {
			cbGlyph := radioUnselected
			if menuItem.Checked {
				cbGlyph = radioSelected
			}
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := radioUnselectedAscii
			if menuItem.Checked {
				cbContent = radioSelectedAscii
			}
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
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

	// tagWidth removed as it was unused

	// Highlighting for gap and description
	// Use itemStyle as base for description so highlight applies, or dialogBG if not selected
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected {
		descStyle = itemStyle
	}
	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
	// Available width: list width - outer padding(2) - (cbWidth + maxTagLen + 3)
	// For checkboxItemDelegate, we assume cbWidth is 2 ([ ]) or 4 ([ ] ) depending on characters
	cbWidth := 2
	if !ctx.LineCharacters {
		cbWidth = 4
	}
	availableWidth := m.Width() - 2 - (cbWidth + d.maxTagLen + 3)
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
	fmt.Fprint(w, line)

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

	// Dialog positioning
	isDialog bool // False if it is a full screen (uses ZScreen), True if a popup modal (uses ZDialog)

	// Unified layout (deterministic sizing)
	layout DialogLayout

	dialogType DialogType

	// Variable height support (for dynamic word wrapping)
	variableHeight bool

	// Memoization for expensive rendering
	lastView       string
	lastWidth      int
	lastHeight     int
	lastIndex      int
	lastFilter     string
	lastActive     bool
	lastLineChars  bool
	lastHitRegions []HitRegion // Cache for variable height hit regions
	viewStartY     int         // Persistent scroll offset for variable height lists

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

// SetDialogType sets the visual style/type of the menu dialog
func (m *MenuModel) SetDialogType(t DialogType) { m.dialogType = t }

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

// SetVariableHeight enables dynamic multiline word wrapping for the list
func (m *MenuModel) SetVariableHeight(enabled bool) {
	m.variableHeight = enabled
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
		if m.items[m.cursor].IsCheckbox || m.items[m.cursor].IsRadioButton {
			m.items[m.cursor].Checked = !m.items[m.cursor].Checked
			m.items[m.cursor].Selected = m.items[m.cursor].Checked
		} else {
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		}
		// Update the list item too
		listItems := make([]list.Item, len(m.items))
		for i, item := range m.items {
			listItems[i] = item
		}
		m.list.SetItems(listItems)
		m.lastView = "" // Invalidate view cache since checked/selected state changed
	}
}

// Init implements tea.Model
func (m *MenuModel) Init() tea.Cmd {
	return nil
}

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ToggleFocusedMsg:
		// Middle click triggers toggle on the currently focused item
		return m.handleSpace()

	case LayerHitMsg:
		// Handle specific item clicks
		if strings.HasPrefix(msg.ID, "item-"+m.id+"-") {
			indexStr := strings.TrimPrefix(msg.ID, "item-"+m.id+"-")
			if idx, err := strconv.Atoi(indexStr); err == nil {
				m.list.Select(idx)
				m.cursor = idx
				menuSelectedIndices[m.id] = idx
				m.focusedItem = FocusList

				// Middle click is handled by AppModel (global Space mapping)
				// We just handle the selection here.
				if msg.Button == tea.MouseMiddle {
					return m, nil
				}

				// For checkboxes/radio buttons, clicking toggles (Space action)
				// For regular items, clicking executes (Enter action)
				if idx >= 0 && idx < len(m.items) {
					item := m.items[idx]
					if item.IsCheckbox || item.IsRadioButton || item.Selectable {
						return m.handleSpace()
					}
				}
				return m.handleEnter()
			}
		}

		// Handle clicks on the menu itself (not a specific item/button)
		if msg.ID == m.id {
			return m, nil
		}

		// Handle button clicks
		switch msg.ID {
		case IDListPanel:
			// Hover moved back over the list — restore list focus so the wheel scrolls items.
			m.focusedItem = FocusList
			return m, nil
		case IDButtonPanel:
			// Hover landed on the button row background — focus the row without executing.
			// Keep whatever button is already highlighted; default to Select when coming from list.
			if m.focusedItem == FocusList {
				m.focusedItem = FocusSelectBtn
			}
			return m, nil
		case "btn-select":
			m.focusedItem = FocusSelectBtn
			return m.handleEnter()
		case "btn-back":
			if m.backAction != nil {
				m.focusedItem = FocusBackBtn
				return m.handleEnter()
			}
		case "btn-exit":
			if m.showExit {
				m.focusedItem = FocusExitBtn
				return m.handleEnter()
			}
		}

		return m, nil

	case LayerWheelMsg, tea.MouseWheelMsg:
		// Handle mouse wheel scrolling (raw or semantic)
		var wheelBtn tea.MouseButton
		var wheelID string
		if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
			wheelBtn = mwMsg.Button
		} else if lwMsg, ok := msg.(LayerWheelMsg); ok {
			wheelBtn = lwMsg.Button
			wheelID = lwMsg.ID
		}

		if wheelBtn != 0 {
			// IDListPanel: scroll the list regardless of button focus state.
			// Mirrors keyboard up/down — button highlight is preserved independently.
			if wheelID == IDListPanel {
				if wheelBtn == tea.MouseWheelUp {
					m.list.CursorUp()
					for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
						m.list.CursorUp()
					}
				} else if wheelBtn == tea.MouseWheelDown {
					m.list.CursorDown()
					for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
						m.list.CursorDown()
					}
				}
				m.cursor = m.list.Index()
				menuSelectedIndices[m.id] = m.cursor
				return m, nil
			}

			// When a button is focused (hover+scroll over button row), shift focus
			// left/right using the clamping helpers — no wrap at either end.
			// subMenuMode menus never render buttons, so always fall through to list scroll.
			if !m.subMenuMode && (m.focusedItem == FocusSelectBtn || m.focusedItem == FocusBackBtn || m.focusedItem == FocusExitBtn) {
				if wheelBtn == tea.MouseWheelUp {
					m.focusedItem = m.prevButtonFocus()
				} else if wheelBtn == tea.MouseWheelDown {
					m.focusedItem = m.nextButtonFocus()
				}
				return m, nil
			}

			if wheelBtn == tea.MouseWheelUp {
				m.list.CursorUp()
				// Skip separators automatically
				for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
					m.list.CursorUp()
				}
			} else if wheelBtn == tea.MouseWheelDown {
				m.list.CursorDown()
				// Skip separators automatically
				for m.list.Index() >= 0 && m.list.Index() < len(m.items) && m.items[m.list.Index()].IsSeparator {
					m.list.CursorDown()
				}
			}
			m.cursor = m.list.Index()
			menuSelectedIndices[m.id] = m.cursor
			return m, nil
		}

	case tea.KeyPressMsg:
		keyMsg := msg
		switch {
		case key.Matches(keyMsg, Keys.Help):
			return m, func() tea.Msg { return ShowDialogMsg{Dialog: NewHelpDialogModel()} }

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
			return m, ConfirmExitAction()

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

	return m, nil
}
func (m *MenuModel) View() tea.View { return tea.View{Content: m.ViewString()} }

// nextButtonFocus, prevButtonFocus → menu_buttons.go
// ViewString, Layers, renderBorderWithTitle, viewSubMenu → menu_render.go
// getButtonSpecs, renderSimpleButtons, renderButtons, renderButtonBox → menu_buttons.go
// GetHitRegions → menu_hit_regions.go
// renderFlow, GetFlowHeight → menu_render_flow.go
// renderVariableHeightList → menu_render_list.go

func (m *MenuModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		// 1. Try list item action first (for navigation menus)
		// This is the primary function for navigation menus, and also applies
		// if "Select" is used as a proxy for Enter on the list.
		selectedItem := m.list.SelectedItem()
		if item, ok := selectedItem.(MenuItem); ok {
			if item.Action != nil {
				// Update cursor for persistence
				m.cursor = m.list.Index()
				menuSelectedIndices[m.id] = m.cursor
				return m, item.Action
			}

			// In checkbox mode, Enter on a list item toggles its state.
			// Enter on the "Select" (Done) button should NOT toggle; it should fall through to enterAction.
			if m.checkboxMode && m.focusedItem == FocusList {
				m.ToggleSelectedItem()
				return m, nil
			}
		}

		// 2. Fall back to model-level enter action (for "Done" buttons on selection screens)
		if m.enterAction != nil {
			return m, m.enterAction
		}

	case FocusBackBtn:
		if m.backAction != nil {
			return m, m.backAction
		}
	case FocusExitBtn:
		return m, ConfirmExitAction()
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
			item.Selected = item.Checked
			// Update the item in our internal list too so state persists
			idx := m.list.Index()
			if idx >= 0 && idx < len(m.items) {
				m.items[idx].Checked = item.Checked
				m.items[idx].Selected = item.Selected
				// Update list.Model internal items to reflect changes immediately
				m.list.SetItem(idx, item)
			}
			m.lastView = "" // Invalidate cache

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
			// Items with only Action (e.g. dropdown selectors) have no SpaceAction.
			// Fall through to Enter so middle-clicking over the panel activates them.
			if item.Action != nil {
				return m.handleEnter()
			}
		}
	}

	if m.spaceAction != nil {
		return m, m.spaceAction
	}
	// Fallback: activate whichever button is currently focused
	if m.focusedItem == FocusSelectBtn || m.focusedItem == FocusBackBtn || m.focusedItem == FocusExitBtn {
		return m.handleEnter()
	}
	return m, nil
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

// Width returns the menu's width
func (m *MenuModel) Width() int {
	return m.width
}

// Height returns the menu's height
func (m *MenuModel) Height() int {
	return m.height
}

func (m *MenuModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// 1. Calculate list width first — subtitle height measurement depends on it.
	layout := GetLayout()
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

	// 2. Subtitle Height — measured at the actual render width (listWidth + 4) so
	// word-wrap lines match what ViewString() produces. Using lipgloss.Height(m.subtitle)
	// only counts explicit '\n' and underestimates at narrow terminal widths.
	subtitleHeight := 0
	if m.subtitle != "" {
		styles := GetStyles()
		outerContentWidth := listWidth + 4
		subtitleStyle := styles.Dialog.Width(outerContentWidth).Padding(0, 1)
		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		subtitleHeight = lipgloss.Height(subtitleStyle.Render(subStr))
	}

	// 3. Button and Shadow Heights
	buttonHeight := DialogButtonHeight
	shadowHeight := 0
	hasShadow := currentConfig.UI.Shadow
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 4. Vertical Budgeting Logic
	var listHeight, overhead int
	var maxListHeight int
	if m.subMenuMode {
		// Sub-menu overhead is just the subtitle and its own borders (2)
		overhead = subtitleHeight + layout.BorderHeight()
		maxListHeight = m.height - overhead
	} else {
		// Full dialog overhead: borders, subtitle, buttons, shadow
		// Vertical budgeting uses DialogContentHeight which handles gaps
		maxListHeight = layout.DialogContentHeight(m.height, subtitleHeight, true, hasShadow)
		// Account for inner border around the list (Top + Bottom = 2 lines)
		maxListHeight -= layout.BorderHeight()
		overhead = m.height - maxListHeight
	}

	if maxListHeight < 3 {
		maxListHeight = 3
	}

	// 5. Calculate intrinsic list height based on items
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
