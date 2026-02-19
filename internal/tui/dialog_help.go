package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// helpDialogModel displays a keyboard shortcut reference dialog.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type helpDialogModel struct {
	help   help.Model
	width  int
	height int
}

func newHelpDialogModel() *helpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &helpDialogModel{help: h}
}

func (m *helpDialogModel) Init() tea.Cmd { return nil }

func (m *helpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Any key closes the help dialog (? toggles it off, Esc also works)
		_ = msg
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseClickMsg:
		// Any click closes the help dialog
		_ = msg
		return m, func() tea.Msg { return CloseDialogMsg{} }
	}
	return m, nil
}

// ViewString returns the dialog content as a string for compositing
func (m *helpDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()
	bgStyle := lipgloss.NewStyle().Background(styles.Dialog.GetBackground())

	// Determine a reasonable width for the help component
	// We want it to be wide enough for at least 2 columns if window allows
	const minDesiredWidth = 60
	targetWidth := m.width - 8 // Snugger margins for the halo/shadow
	if targetWidth > 120 {
		targetWidth = 120 // Max width for readability
	}
	if targetWidth < minDesiredWidth && m.width > minDesiredWidth+4 {
		targetWidth = minDesiredWidth
	}
	m.help.SetWidth(targetWidth)

	// Apply theme styles to the help component
	dimStyle := bgStyle.Foreground(styles.ItemNormal.GetForeground())
	// Ensure keys use the theme's TagKey style (including flags like Bold/Italic)
	// but override background to match the help dialog background
	keyStyle := styles.TagKey.Background(bgStyle.GetBackground())
	sepStyle := bgStyle.Foreground(styles.BorderColor)

	m.help.Styles.ShortKey = keyStyle
	m.help.Styles.ShortDesc = dimStyle
	m.help.Styles.ShortSeparator = sepStyle
	m.help.Styles.FullKey = keyStyle
	m.help.Styles.FullDesc = dimStyle
	m.help.Styles.FullSeparator = sepStyle
	m.help.Styles.Ellipsis = dimStyle

	content := m.help.View(Keys)

	// Apply dialog background and add 1 space indent on both sides
	lines := strings.Split(content, "\n")

	// Find the actual max width of the rendered help content to make the box snug
	maxLineWidth := 0
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		lines[i] = trimmed
		w := lipgloss.Width(trimmed)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}

	for i, line := range lines {
		// Indent 1 space + content + pad to max width + 1 space trailing
		lineWidth := lipgloss.Width(line)
		paddedLine := " " + line + strings.Repeat(" ", maxLineWidth-lineWidth) + " "
		lines[i] = MaintainBackground(bgStyle.Render(paddedLine), bgStyle)
	}
	content = strings.Join(lines, "\n")

	// Use RenderUniformBlockDialog for a distinct look for help (uniform Border2Color)
	// Passing content directly - RenderDialog logic will handle vertical growth
	dialogStr := RenderUniformBlockDialog("{{|Theme_TitleHelp|}}Keyboard Shortcuts", content)

	// Use AddPatternHalo instead of AddShadow for a surrounding "halo" effect
	return AddPatternHalo(dialogStr)
}

func (m *helpDialogModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *helpDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.SetWidth(w - 12)
}
