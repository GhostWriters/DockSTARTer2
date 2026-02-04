package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MenuItem defines an item in a menu
type MenuItem struct {
	Tag      string  // Display name (first letter used as shortcut)
	Desc     string  // Description text
	Help     string  // Help line text shown when item is selected
	Shortcut rune    // Keyboard shortcut (usually first letter of Tag)
	Action   tea.Cmd // Command to execute when selected
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

	return MenuModel{
		id:          id,
		title:       title,
		subtitle:    subtitle,
		items:       items,
		cursor:      cursor,
		backAction:  backAction,
		focused:     true,
		focusedItem: FocusList,
	}
}

// Init implements tea.Model
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Handle mouse clicks for menu selection
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Mouse support for list items would require tracking positions
			// For now, focus on keyboard navigation
			// TODO: Implement proper mouse click detection for menu items
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
func (m MenuModel) View() string {
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

	// Ensure minimum width for title and buttons
	titleWidth := len(m.title) + 4
	if titleWidth > contentWidth {
		contentWidth = titleWidth
	}
	buttonsWidth := 28
	if m.backAction != nil {
		buttonsWidth += 10
	}
	if buttonsWidth > contentWidth {
		contentWidth = buttonsWidth
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

	// Create list content with padding and border
	listContent := b.String()
	listStyle := styles.Dialog.
		Padding(0, 1)
	listStyle = Apply3DBorder(listStyle)
	paddedList := listStyle.Render(listContent)

	// Create buttons in a full bordered box matching menu width
	listWidth := lipgloss.Width(paddedList)
	// Subtract padding (2) since buttonBox adds its own
	innerWidth := listWidth - 2
	buttons := m.renderButtons(innerWidth)
	buttonBox := m.renderButtonBox(buttons, innerWidth)

	// Wrap in dialog frame (button box has its own border)
	return m.renderDialog(paddedList, buttonBox, listWidth)
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

	// Create styled spacing with dialog background
	spacing := styles.Dialog.Render("  ")

	// Create button row (width and padding handled by caller)
	var buttons string
	if m.backAction != nil {
		buttons = lipgloss.JoinHorizontal(lipgloss.Center, selectBtn, spacing, backBtn, spacing, exitBtn)
	} else {
		buttons = lipgloss.JoinHorizontal(lipgloss.Center, selectBtn, spacing, exitBtn)
	}

	return buttons
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

	// Build inner content parts (menu section only)
	var innerParts []string

	// Subtitle (left-aligned, matching content width)
	if m.subtitle != "" {
		// TODO: Investigate why foreground color isn't rendering in terminal
		subtitle := styles.Dialog.Copy().
			Width(listWidth).
			Padding(0, 1).
			Render(m.subtitle)
		innerParts = append(innerParts, subtitle)
	}

	// Menu content (already padded)
	innerParts = append(innerParts, menuContent)

	// Join menu parts
	menuSection := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Render with borders and append button box
	var dialogBox string

	if m.title != "" {
		dialogBox = m.renderBorderWithTitle(menuSection, buttonBox, listWidth)
	} else {
		// No title, use standard border around menu, then append button box
		menuBoxStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		menuBoxStyle = Apply3DBorder(menuBoxStyle)
		menuRendered := menuBoxStyle.Render(menuSection)
		dialogBox = menuRendered + "\n" + buttonBox
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

func (m MenuModel) renderBorderWithTitle(menuContent, buttonBox string, contentWidth int) string {
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

	// Get actual content width (should be contentWidth + 2 for padding)
	lines := strings.Split(menuContent, "\n")
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

	// Menu content lines with left/right borders only (no bottom border)
	for _, line := range lines {
		result.WriteString(borderStyleLight.Render(border.Left))
		result.WriteString(line)
		result.WriteString(borderStyleDark.Render(border.Right))
		result.WriteString("\n")
	}

	// Append button box (which has its own full border - top is separator, bottom is overall bottom)
	result.WriteString(buttonBox)

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
