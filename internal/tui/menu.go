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

	// Create list content with padding
	listContent := b.String()
	paddedList := styles.Dialog.
		Width(contentWidth).
		Padding(0, 1).
		Render(listContent)

	// Create separator line (full width including padding)
	border := styles.Border
	borderStyleLight := lipgloss.NewStyle().
		Foreground(styles.BorderColor).
		Background(styles.Dialog.GetBackground())
	borderStyleDark := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(styles.Dialog.GetBackground())

	separator := borderStyleLight.Render(border.Left) +
		borderStyleLight.Render(strings.Repeat(border.Top, contentWidth)) +
		borderStyleDark.Render(border.Right)

	// Create buttons with padding
	buttons := m.renderButtons(contentWidth)
	paddedButtons := styles.Dialog.
		Width(contentWidth).
		Padding(0, 1).
		Render(buttons)

	// Combine all parts
	dialogContent := paddedList + "\n" + separator + "\n" + paddedButtons

	// Wrap in dialog frame
	return m.renderDialog(dialogContent, contentWidth)
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

func (m MenuModel) renderDialog(content string, contentWidth int) string {
	styles := GetStyles()

	// Subtitle (left-aligned like bash version) with same width as content
	var subtitle string
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, 1).
			Align(lipgloss.Left)
		subtitle = subtitleStyle.Render(m.subtitle)
	}

	// Combine subtitle and content
	var parts []string
	if m.subtitle != "" {
		parts = append(parts, subtitle)
	}
	parts = append(parts, content)

	inner := lipgloss.JoinVertical(lipgloss.Center, parts...)

	// Wrap in dialog border with title in border (3D effect for rounded borders)
	var dialogBox string

	if m.title != "" {
		// Manually render with title in border
		titleInBorder := " " + m.title + " "
		border := styles.Border

		// Content is already padded, just apply background
		contentRendered := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Render(inner)

		// Get content width
		cWidth := lipgloss.Width(contentRendered)

		// Build top border with title
		titleLen := lipgloss.Width(titleInBorder)
		leftBorder := (cWidth - titleLen) / 2
		rightBorder := cWidth - titleLen - leftBorder

		titleStyle := lipgloss.NewStyle().
			Foreground(styles.DialogTitle.GetForeground()).
			Background(styles.Dialog.GetBackground())

		borderStyleLight := lipgloss.NewStyle().
			Foreground(styles.BorderColor).
			Background(styles.Dialog.GetBackground())

		topBorder := borderStyleLight.Render(border.TopLeft+strings.Repeat(border.Top, leftBorder)) +
			titleStyle.Render(titleInBorder) +
			borderStyleLight.Render(strings.Repeat(border.Top, rightBorder)+border.TopRight)

		// Build middle lines with left and right borders
		lines := strings.Split(contentRendered, "\n")
		var result strings.Builder
		result.WriteString(topBorder + "\n")

		borderStyleDark := lipgloss.NewStyle().
			Foreground(styles.Border2Color).
			Background(styles.Dialog.GetBackground())

		for _, line := range lines {
			leftB := borderStyleLight.Render(border.Left)
			rightB := borderStyleDark.Render(border.Right)
			result.WriteString(leftB + line + rightB + "\n")
		}

		// Bottom border (dark)
		bottomBorder := borderStyleDark.Render(border.BottomLeft + strings.Repeat(border.Bottom, cWidth) + border.BottomRight)
		result.WriteString(bottomBorder)

		dialogBox = result.String()
	} else {
		// No title, use standard border
		dialogBoxStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		dialogBoxStyle = Apply3DBorder(dialogBoxStyle)
		dialogBox = dialogBoxStyle.Render(inner)
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
