package tui

import (
	"DockSTARTer2/internal/strutil"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
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
	IsUserDefined bool              // Whether this is a user-defined app (for coloring)
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
			content = RenderThemeText(menuItem.Tag, SemanticStyle("{{|Theme_TagKey|}}"))
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
			content = RenderThemeText(menuItem.Tag, SemanticStyle("{{|Theme_TagKey|}}"))
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
	list        list.Model
	maximized   bool // Whether to maximize the dialog to fill available space
	showExit    bool // Whether to show Exit button (default true for main menus)
	showButtons bool // Whether to show any buttons (default true)

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
	isDialog bool // True when used as a modal dialog — raises hit-region Z priority above screen regions

	// Unified layout (deterministic sizing)
	layout DialogLayout

	dialogType DialogType

	// Variable height support (for dynamic word wrapping)
	variableHeight bool                                      // Allow list to expand naturally up to layout limits
	interceptor    func(tea.Msg, *MenuModel) (tea.Cmd, bool) // Optional custom message handler

	// Memoization for expensive rendering
	lastView   string
	cacheValid bool // Indicates if lastView is up-to-date with current state

	// Memoization specifically for the variable-height list (separated to avoid border recursion loops)
	lastListView   string
	lastWidth      int
	lastHeight     int
	lastIndex      int
	lastFilter     string
	lastActive     bool
	lastLineChars  bool
	lastHitRegions []HitRegion // Cache for variable height hit regions
	viewStartY     int         // Persistent scroll offset for variable height lists
	lastScrollTotal int        // Total content height from last renderVariableHeightList (for scrollbar)

	menuName string // Name used for --menu or -M to return to this screen

	// Content sections: sub-menus rendered stacked inside the outer border.
	// When present, replaces the standard list+inner-border rendering.
	contentSections []*MenuModel

	// Scrollbar interaction state
	sbInfo     ScrollbarInfo // geometry from last render (set by menu_render.go)
	sbAbsTopY  int           // absolute screen Y of scrollbar column top (set by GetHitRegions)
	sbDragging bool          // true while the user is dragging the scrollbar thumb
}

// IsScrollbarDragging reports whether the menu is currently processing a scrollbar thumb drag.
// AppModel uses this interface to give the active screen drag priority.
func (m *MenuModel) IsScrollbarDragging() bool {
	return m.sbDragging
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
func NewMenuModel(id, title, subtitle string, items []MenuItem, backAction tea.Cmd) *MenuModel {
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

	return &MenuModel{
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
		showButtons: true, // Default to show buttons
	}
}

// Title returns the menu title
func (m *MenuModel) Title() string {
	return m.title
}

// Subtitle returns the menu subtitle
func (m *MenuModel) Subtitle() string {
	return m.subtitle
}

// SetTitle sets the menu title
func (m *MenuModel) SetTitle(title string) { m.title = title }

// ID returns the unique identifier for this menu
func (m *MenuModel) ID() string { return m.id }

// SetDialogType sets the visual style/type of the menu dialog
func (m *MenuModel) SetDialogType(t DialogType) { m.dialogType = t }

// MenuName returns the name used for --menu or -M to return to this screen
func (m *MenuModel) MenuName() string {
	return m.menuName
}

// SetMenuName sets the persistent menu name
func (m *MenuModel) SetMenuName(name string) {
	m.menuName = name
}

// AddContentSection appends a sub-menu as a stacked section rendered inside this menu's border.
// When sections are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section *MenuModel) {
	m.contentSections = append(m.contentSections, section)
}

// SetFocusedItem explicitly sets which UI element has focus (list or a button).
func (m *MenuModel) SetFocusedItem(item FocusItem) {
	m.focusedItem = item
}

// GetButtonHeight returns the current button row height (1 = flat, 3 = bordered).
func (m *MenuModel) GetButtonHeight() int {
	return m.layout.ButtonHeight
}

// View implements tea.Model and ScreenModel
func (m *MenuModel) View() tea.View {
	return tea.View{Content: m.ViewString()}
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

// SetShowButtons sets whether to show the button row at all
func (m *MenuModel) SetShowButtons(show bool) {
	m.showButtons = show
	m.calculateLayout() // Layout needs recalculation when buttons are toggled
}

// IsMaximized returns whether the menu is maximized
func (m MenuModel) IsMaximized() bool {
	return m.maximized
}

// HasDialog returns whether the menu has an active dialog overlay
func (m MenuModel) HasDialog() bool {
	return false // Menus don't have nested dialogs
}

// SetSubMenuMode enables a compact mode for menus inside other screens/containers
func (m *MenuModel) SetSubMenuMode(v bool) {
	m.subMenuMode = v
	if v {
		m.showButtons = false
	}
	m.calculateLayout()
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

// SetVariableHeight allows the list viewport to expand instead of forcing pagination
func (m *MenuModel) SetVariableHeight(variable bool) {
	m.variableHeight = variable
}

// SetUpdateInterceptor allows setting a custom handler that runs before normal message processing
func (m *MenuModel) SetUpdateInterceptor(interceptor func(tea.Msg, *MenuModel) (tea.Cmd, bool)) {
	m.interceptor = interceptor
}

// Index returns the current cursor index
func (m *MenuModel) Index() int {
	return m.cursor
}

// FocusedItem returns the currently focused UI element
func (m *MenuModel) FocusedItem() FocusItem {
	return m.focusedItem
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

// GetInnerContentWidth returns the width of the space inside the outer borders
func (m *MenuModel) GetInnerContentWidth() int {
	layout := GetLayout()
	if m.subMenuMode {
		return m.width - layout.BorderWidth()
	}

	var contentWidth int
	if m.maximized {
		contentWidth, _ = layout.InnerContentSize(m.width, m.height)
	} else {
		contentWidth = m.list.Width() + layout.BorderWidth() + 2
		maxWidth, _ := layout.InnerContentSize(m.width, m.height)
		if contentWidth > maxWidth {
			contentWidth = maxWidth
		}
	}
	return contentWidth
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

// HelpContext implements HelpContextProvider.
func (m *MenuModel) HelpContext(contentWidth int) HelpContext {
	itemHelp := ""
	itemTitle := "Help"
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.items) {
		itemHelp = m.items[idx].Help
		if m.items[idx].Tag != "" {
			itemTitle = m.items[idx].Tag
		}
	}

	return HelpContext{
		ScreenName: m.title,
		PageTitle:  "Description",
		PageText:   m.subtitle,
		ItemTitle:  itemTitle,
		ItemText:   itemHelp,
	}
}

// showContextMenu returns a command to show the context menu for the item at the given index.
func (m *MenuModel) showContextMenu(idx int, x, y int) tea.Cmd {
	var tag, desc string
	var hCtx *HelpContext

	if idx >= 0 && idx < len(m.items) {
		item := m.items[idx]
		tag = item.Tag
		desc = item.Desc
		hCtx = &HelpContext{
			ScreenName: m.title,
			PageTitle:  "Description",
			PageText:   m.subtitle,
			ItemTitle:  tag,
			ItemText:   item.Help,
		}
	}

	var items []ContextMenuItem
	if tag != "" {
		items = append(items, ContextMenuItem{IsHeader: true, Label: tag})
		items = append(items, ContextMenuItem{IsSeparator: true})
	}
	var clipItems []ContextMenuItem

	if tag != "" {
		t := tag
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Title",
			Help:  "Copy the item's title (tag) to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(t)
				return CloseDialogMsg{}
			},
		})
	}
	if desc != "" {
		d := desc
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Description",
			Help:  "Copy the item's description to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(d)
				return CloseDialogMsg{}
			},
		})
	}

	items = AppendContextMenuTail(items, clipItems, hCtx)

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}

// Init implements tea.Model
func (m *MenuModel) Init() tea.Cmd {
	return nil
}
