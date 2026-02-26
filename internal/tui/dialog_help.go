package tui

import (
	"DockSTARTer2/internal/strutil"
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

	// Unified layout (deterministic sizing)
	layout DialogLayout
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
	if m.layout.Width == 0 {
		return ""
	}

	// Calculate target width for help content Snugger margins for the halo/shadow
	targetWidth := m.layout.Width - 10
	if targetWidth > 120 {
		targetWidth = 120
	}
	m.help.SetWidth(targetWidth)

	dialogStyle := SemanticStyle("{{|Theme_Dialog|}}")
	titleStyle := GetStyles().DialogTitleHelp
	haloColor := titleStyle.GetForeground() // Use the themed black from HelpTitle
	bgStyle := lipgloss.NewStyle().Background(dialogStyle.GetBackground())

	// Apply theme styles to the help component
	sepStyle := bgStyle.Foreground(dialogStyle.GetForeground())
	dimStyle := SemanticStyle("{{|Theme_HelpItem|}}")
	keyStyle := SemanticStyle("{{|Theme_HelpTag|}}")

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
		lineWidth := lipgloss.Width(line)
		paddedLine := " " + line + strutil.Repeat(" ", maxLineWidth-lineWidth) + " "
		lines[i] = MaintainBackground(bgStyle.Render(paddedLine), bgStyle)
	}
	content = strings.Join(lines, "\n")
	content = MaintainBackground(bgStyle.Render(content), bgStyle)

	// Force total content height if height is set
	// Overhead for Help: Halo (2) + Bordered Dialog (2) = 4
	heightBudget := m.layout.Height - 4
	if heightBudget > 0 {
		content = lipgloss.NewStyle().
			Height(heightBudget).
			Background(GetStyles().Dialog.GetBackground()).
			Render(content)
	}

	// Use standard RenderDialogCtx for the border area
	// We override the dialog background to Black for the border area and halo
	// but keep the title's original background so it "shows what it used to"
	ctx := GetActiveContext()
	ctx.Dialog = ctx.Dialog.Background(haloColor)
	ctx.DialogTitle = titleStyle.Background(GetStyles().Dialog.GetBackground())
	ctx.BorderColor = haloColor
	ctx.Border2Color = haloColor

	dialogStr := RenderDialogCtx("{{|Theme_TitleHelp|}}Keyboard & Mouse Controls", content, true, 0, ctx)

	// Add the solid black halo
	return AddPatternHalo(dialogStr, haloColor)
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
	m.calculateLayout()
}

func (m *helpDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Overhead for Help: Halo (2) + Bordered Dialog (2) = 4
	overhead := 4

	m.layout = DialogLayout{
		Width:    m.width,
		Height:   m.height,
		Overhead: overhead,
	}
}
