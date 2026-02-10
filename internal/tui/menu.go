package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// MenuItem defines an item in a menu
type MenuItem struct {
	Tag      string  // Display name (first letter used as shortcut)
	Desc     string  // Description text
	Help     string  // Help line text shown when item is selected
	Shortcut rune    // Keyboard shortcut (usually first letter of Tag)
	Action   tea.Cmd // Command to execute when selected
}

// Implement list.Item interface for bubbles/list
func (i MenuItem) FilterValue() string { return i.Tag }
func (i MenuItem) Title() string       { return i.Tag }
func (i MenuItem) Description() string { return i.Desc }

// customDelegate implements list.ItemDelegate with our custom two-column styling
type customDelegate struct {
	maxTagLen int
}

func (d customDelegate) Height() int                             { return 1 }
func (d customDelegate) Spacing() int                            { return 0 }
func (d customDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d customDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	styles := GetStyles()
	isSelected := index == m.Index()

	// Render tag with first-letter highlighting
	tag := menuItem.Tag
	var tagStr string
	if len(tag) > 0 {
		// Find the index of the first letter (skip brackets)
		letterIdx := 0
		if strings.HasPrefix(tag, "[") && len(tag) > 1 {
			letterIdx = 1
		}

		prefix := tag[:letterIdx]
		firstLetter := string(tag[letterIdx])
		rest := tag[letterIdx+1:]

		var keyStyle, restStyle lipgloss.Style
		if isSelected {
			keyStyle = styles.TagKeySelected
			restStyle = styles.TagSelected
		} else {
			keyStyle = styles.TagKey
			restStyle = styles.TagNormal
		}

		tagStr = restStyle.Render(prefix) + keyStyle.Render(firstLetter) + restStyle.Render(rest)
	}

	// Pad tag to align descriptions
	// Use lipgloss.Width() for proper terminal width measurement
	tagWidth := lipgloss.Width(menuItem.Tag)
	paddingSpaces := strings.Repeat(" ", d.maxTagLen-tagWidth+2) // 2 for column spacing

	// Render padding with dialog background (not black/transparent)
	paddingStyle := lipgloss.NewStyle().Background(styles.Dialog.GetBackground())
	padding := paddingStyle.Render(paddingSpaces)

	// Render description (padding OUTSIDE style to create separate highlight boxes)
	var descStr string
	if isSelected {
		descStr = padding + styles.ItemSelected.Render(menuItem.Desc)
	} else {
		descStr = padding + styles.ItemNormal.Render(menuItem.Desc)
	}

	// Combine tag and description
	line := tagStr + descStr

	// Apply dialog background and padding to fill list width
	lineStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1). // Add 1 space margin on left and right
		Width(m.Width())
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

	// Back action (nil if no back button)
	backAction tea.Cmd

	// Bubbles list model
	list list.Model
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
	delegate := customDelegate{maxTagLen: maxTagLen}

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
	}
}

// Init implements tea.Model
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model (Phase 1: delegate to bubbles/list)
func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window size first
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height

		// Calculate list width based on actual content
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

		// Set list height based on actual number of items (dynamic sizing!)
		// Calculate proper height based on delegate metrics
		// customDelegate has Height=1 and Spacing=0
		itemHeight := 1
		spacing := 0
		totalItemHeight := len(m.items) * itemHeight
		if len(m.items) > 1 && spacing > 0 {
			totalItemHeight += (len(m.items) - 1) * spacing
		}
		// Try exact height with no buffer now that pagination is disabled
		listHeight := totalItemHeight

		m.list.SetSize(listWidth, listHeight)
	}

	// Handle mouse events using BubbleZones
	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		// Only handle left mouse button press
		if mouseMsg.Action == tea.MouseActionPress && mouseMsg.Button == tea.MouseButtonLeft {
			// Check each zone to see if the click is within bounds
			// Menu item zones - clicking executes immediately (same as clicking Select)
			for i := 0; i < len(m.items); i++ {
				zoneID := fmt.Sprintf("item-%d", i)
				if zoneInfo := zone.Get(zoneID); zoneInfo != nil {
					if zoneInfo.InBounds(mouseMsg) {
						// Select and execute the clicked item
						m.list.Select(i)
						m.cursor = i
						menuSelectedIndices[m.id] = i
						m.focusedItem = FocusList
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
		}
		return m, nil
	}

	// Handle key events
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
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
		case key.Matches(keyMsg, Keys.Up), key.Matches(keyMsg, Keys.Down):
			m.focusedItem = FocusList
			// Falls through to list.Update below so the cursor also moves

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

		// Esc: back if available, else exit
		case key.Matches(keyMsg, Keys.Esc):
			if m.backAction != nil {
				return m, m.backAction
			}
			return m, tea.Quit

		// Dynamic Hotkeys
		default:
			if keyMsg.Type == tea.KeyRunes {
				keyRune := strings.ToLower(string(keyMsg.Runes))

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
				// Determine available buttons
				var buttons []ButtonSpec
				buttons = append(buttons, ButtonSpec{Text: "Select"})
				if m.backAction != nil {
					buttons = append(buttons, ButtonSpec{Text: "Back"})
				}
				buttons = append(buttons, ButtonSpec{Text: "Exit"})

				if idx, found := CheckButtonHotkeys(keyMsg, buttons); found {
					// Map index back to FocusItem
					switch buttons[idx].Text {
					case "Select":
						m.focusedItem = FocusSelectBtn
					case "Back":
						m.focusedItem = FocusBackBtn
					case "Exit":
						m.focusedItem = FocusExitBtn
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
		return FocusExitBtn
	case FocusBackBtn:
		return FocusExitBtn
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
		return FocusExitBtn
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
		return FocusExitBtn
	case FocusBackBtn:
		return FocusExitBtn
	}
	return FocusSelectBtn
}

// prevButtonFocus cycles the Left arrow through buttons only.
// From the list or first button, moves to the last button (Exit).
func (m MenuModel) prevButtonFocus() FocusItem {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		return FocusExitBtn
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
	return m, nil
}

// View renders the menu with custom styling (Phase 2)
func (m MenuModel) View() string {
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

	// Verify button box width matches
	buttonBoxWidth := lipgloss.Width(borderedButtonBox)
	if buttonBoxWidth != targetWidth {
		// If it doesn't match, explicitly set it
		borderedButtonBox = lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Width(targetWidth).
			Render(borderedButtonBox)
	}

	// Add equal margins around both boxes for spacing
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	paddedList := marginStyle.Render(borderedList)
	paddedButtons := marginStyle.Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (left-aligned, matching padded width)
	if m.subtitle != "" {
		paddedWidth := lipgloss.Width(paddedList)

		subtitleStyle := styles.Dialog.
			Width(paddedWidth).
			Padding(0, 1).
			Align(lipgloss.Left)

		// Parse tags for subtitle
		var subStr string
		subStr, subtitleStyle = ParseTitleTags(m.subtitle, subtitleStyle)

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

	// Mark zones for mouse interaction before returning
	// Note: Zones are scanned at root level (AppModel.View()), not here
	dialog = m.markZones(dialog)

	return dialog
}

// markZones marks clickable zones in the rendered dialog for mouse interaction
func (m MenuModel) markZones(dialog string) string {
	lines := strings.Split(dialog, "\n")

	// Calculate line positions based on actual rendering structure:
	// Line 0: Outer border top with title embedded
	// Line 1: Subtitle (if present) OR first line of paddedList
	// Line 1 or 2: Inner list border top (first line of borderedList inside paddedList)
	// Lines 2+ or 3+: Menu items
	// Line X: Inner list border bottom
	// Line X+1: Inner button border top
	// Line X+2: Button line
	// Line X+3: Inner button border bottom
	// Line X+4: Outer border bottom
	// Lines X+5+: Shadow (if enabled)

	lineIdx := 0

	// Line 0: Outer border top with title
	lineIdx++

	// Line 1: Subtitle (if present)
	if m.subtitle != "" {
		lineIdx++
	}

	// Next line: Inner list border top
	lineIdx++

	// Now we're at the first menu item
	// Mark each menu item line (entire line is clickable)
	for i := 0; i < len(m.items); i++ {
		if lineIdx < len(lines) {
			lines[lineIdx] = zone.Mark(fmt.Sprintf("item-%d", i), lines[lineIdx])
		}
		lineIdx++
	}

	// Skip inner list border bottom
	lineIdx++

	// Skip inner button border top
	lineIdx++

	// Button line - zones are already marked during rendering in renderSimpleButtons()
	// No need to mark here

	return strings.Join(lines, "\n")
}

// renderSimpleButtons creates a button row with evenly spaced sections
func (m MenuModel) renderSimpleButtons(contentWidth int) string {
	// Build button specs with focus state and explicit zone IDs
	specs := []ButtonSpec{
		{Text: " Select ", Active: m.focusedItem == FocusSelectBtn, ZoneID: "btn-select"},
	}
	if m.backAction != nil {
		specs = append(specs, ButtonSpec{Text: " Back ", Active: m.focusedItem == FocusBackBtn, ZoneID: "btn-back"})
	}
	specs = append(specs, ButtonSpec{Text: " Exit ", Active: m.focusedItem == FocusExitBtn, ZoneID: "btn-exit"})

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
		padding := strings.Repeat(" ", maxTagLen-tagWidth+colPadding)

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

	// Center in the available space
	centered := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogBox,
		lipgloss.WithWhitespaceBackground(styles.Screen.GetBackground()),
	)

	return centered
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
	result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, leftPad)))
	result.WriteString(borderStyleLight.Render(leftT))
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(titleStyle.Render(title))
	result.WriteString(borderStyleLight.Render(" "))
	result.WriteString(borderStyleLight.Render(rightT))
	result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, rightPad)))
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	// Content lines with left/right borders
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))

		// Pad line to actualWidth with styled padding to prevent black splotches
		textWidth := lipgloss.Width(line)
		padding := ""
		if textWidth < actualWidth {
			padding = lipgloss.NewStyle().Background(borderBG).Render(strings.Repeat(" ", actualWidth-textWidth))
		}

		// Use MaintainBackground to handle internal color resets within the line
		fullLine := MaintainBackground(line+padding, styles.Dialog)
		result.WriteString(fullLine)

		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	// Bottom border
	result.WriteString(borderStyleDark.Render(border.BottomLeft))
	result.WriteString(borderStyleDark.Render(strings.Repeat(border.Bottom, actualWidth)))
	result.WriteString(borderStyleDark.Render(border.BottomRight))

	return result.String()
}

// SetSize updates the menu dimensions
func (m *MenuModel) SetSize(width, height int) {
	m.width = width
	m.height = height
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
