package tui

import (
	"fmt"
	"strings"

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

	// Zone manager for mouse support (TODO: remove when bubbles/list handles mouse)
	zoneManager *zone.Manager

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

	// Create bubbles list with default styling (Phase 1 - keep it simple!)
	l := list.New(listItems, list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

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
		zoneManager: zone.New(),
		list:        l,
	}
}

// Init implements tea.Model
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model (Phase 1: delegate to bubbles/list)
func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Execute action for selected item
			selectedItem := m.list.SelectedItem()
			if item, ok := selectedItem.(MenuItem); ok {
				if item.Action != nil {
					return m, item.Action
				}
			}
			return m, nil

		case "esc":
			if m.backAction != nil {
				return m, m.backAction
			}
			return m, tea.Quit
		}
	}

	// Delegate all other messages to the list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

/* OLD UPDATE METHOD - Kept for reference
func (m MenuModel) updateOld(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		// TODO: Mouse support disabled due to position offset issues
		// Will be re-enabled when switching to bubbles/list (has built-in mouse support)
		// For now, use keyboard navigation
		return m, nil

		// DISABLED - Position offset issues
		// Handle mouse clicks using zones (automatic position tracking)
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if Select button was clicked
			if m.zoneManager.Get("btn-select").InBounds(msg) {
				if m.cursor >= 0 && m.cursor < len(m.items) {
					if m.items[m.cursor].Action != nil {
						return m, m.items[m.cursor].Action
					}
				}
				return m, nil
			}

			// Check if Back button was clicked
			if m.backAction != nil && m.zoneManager.Get("btn-back").InBounds(msg) {
				return m, m.backAction
			}

			// Check if Exit button was clicked
			if m.zoneManager.Get("btn-exit").InBounds(msg) {
				return m, tea.Quit
			}

			// Check if any menu item was clicked
			for i := range m.items {
				if m.zoneManager.Get(fmt.Sprintf("item-%d", i)).InBounds(msg) {
					m.cursor = i
					menuSelectedIndices[m.id] = i
					return m, nil
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.focusedItem == FocusList {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.items) - 1
				}
				menuSelectedIndices[m.id] = m.cursor
			}

		case "down", "j":
			if m.focusedItem == FocusList {
				m.cursor++
				if m.cursor >= len(m.items) {
					m.cursor = 0
				}
				menuSelectedIndices[m.id] = m.cursor
			}

		case "tab", "right":
			m.focusedItem = m.nextFocus()

		case "shift+tab", "left":
			m.focusedItem = m.prevFocus()

		case "enter":
			return m.handleEnter()

		case "esc":
			if m.backAction != nil {
				return m, m.backAction
			}
			return m, tea.Quit

		default:
			// Check for shortcut keys
			if len(msg.String()) == 1 {
				r := []rune(msg.String())[0]
				for i, item := range m.items {
					if unicode.ToLower(item.Shortcut) == unicode.ToLower(r) {
						m.cursor = i
						m.focusedItem = FocusList
						menuSelectedIndices[m.id] = m.cursor
						if item.Action != nil {
							return m, item.Action
						}
						return m, nil
					}
				}
			}
		}
	}

	return m, nil
}
*/

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

func (m MenuModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		if m.cursor >= 0 && m.cursor < len(m.items) {
			if m.items[m.cursor].Action != nil {
				return m, m.items[m.cursor].Action
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

// View renders the menu
// View renders the menu using bubbles/list (Phase 1 - simple version)
func (m MenuModel) View() string {
	// Phase 1: Use default bubbles/list rendering
	// Set list size to fill available space
	m.list.SetSize(m.width, m.height)

	// Return simple list view (no custom styling yet)
	return m.list.View()
}

/* OLD CUSTOM RENDERING - Kept for reference (Phase 2 will add back custom styling)
func (m MenuModel) viewOld() string {
	styles := GetStyles()

	// Calculate dimensions
	maxTagLen := 0
	maxDescLen := 0
	for _, item := range m.items {
		if len(item.Tag) > maxTagLen {
			maxTagLen = len(item.Tag)
		}
		if len(item.Desc) > maxDescLen {
			maxDescLen = len(item.Desc)
		}
	}

	colPadding := 2
	contentWidth := maxTagLen + colPadding + maxDescLen + 4

	// Ensure minimum width for title
	titleWidth := len(m.title) + 4
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
		tagWidth := len(item.Tag)
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

// addZonesToRenderedDialog adds zone markers to specific lines in the fully rendered dialog
func (m MenuModel) addZonesToRenderedDialog(dialog string) string {
	lines := strings.Split(dialog, "\n")

	// Calculate line offsets based on dialog structure
	lineIdx := 0

	// Line 0: Title in border
	lineIdx++

	// Subtitle (if present)
	if m.subtitle != "" {
		lineIdx++
	}

	// Outer padding
	lineIdx++

	// List box top border
	lineIdx++

	// Now we're at the first menu item
	firstItemLine := lineIdx

	// Mark menu item lines
	for i := 0; i < len(m.items); i++ {
		itemLineIdx := firstItemLine + i
		if itemLineIdx < len(lines) {
			lines[itemLineIdx] = m.zoneManager.Mark(fmt.Sprintf("item-%d", i), lines[itemLineIdx])
		}
	}

	// Find button lines (they're near the end)
	// Button box structure: border, button line, border
	// It's after the list box bottom border
	buttonLineIdx := firstItemLine + len(m.items) + 2 // +1 for list bottom border, +1 for button top border
	if buttonLineIdx < len(lines) {
		// Mark the button line with all button zones
		lines[buttonLineIdx] = m.addButtonZonesToLine(lines[buttonLineIdx])
	}

	return strings.Join(lines, "\n")
}

// addButtonZonesToLine marks button zones on a single line
func (m MenuModel) addButtonZonesToLine(line string) string {
	// The buttons are already positioned in sections
	// We need to mark each section with its zone
	// For now, mark the entire line and we'll refine based on click position

	// This is simplified - we'd need to calculate exact button positions
	// For now, mark the whole line and check button zones in mouse handler
	line = m.zoneManager.Mark("btn-select", line)

	return line
}

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

	// Add padding and full border
	boxStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)
	boxStyle = Apply3DBorder(boxStyle)

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
	border := styles.Border

	// Style definitions
	borderBG := styles.Dialog.GetBackground()
	borderStyleLight := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(borderBG)
	borderStyleDark := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(borderBG)
	titleStyle := styles.DialogTitle.Copy().
		Background(borderBG)

	// Get actual content width
	lines := strings.Split(content, "\n")
	actualWidth := 0
	if len(lines) > 0 {
		actualWidth = lipgloss.Width(lines[0])
	}

	// Build top border with title
	titleText := " " + m.title + " "
	titleLen := lipgloss.Width(titleText)
	leftPad := (actualWidth - titleLen) / 2
	rightPad := actualWidth - titleLen - leftPad

	var result strings.Builder

	// Top border
	result.WriteString(borderStyleLight.Render(border.TopLeft))
	result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, leftPad)))
	result.WriteString(titleStyle.Render(titleText))
	result.WriteString(borderStyleLight.Render(strings.Repeat(border.Top, rightPad)))
	result.WriteString(borderStyleLight.Render(border.TopRight))
	result.WriteString("\n")

	// Content lines with left/right borders
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		result.WriteString(line)
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
