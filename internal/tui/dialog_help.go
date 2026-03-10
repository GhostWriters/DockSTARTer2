package tui

import (
	"DockSTARTer2/internal/strutil"
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// HelpDialogModel displays a keyboard shortcut reference dialog.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type HelpDialogModel struct {
	help   help.Model
	width  int
	height int

	focused bool // tracks global focus

    keyMap help.KeyMap

	// Unified layout (deterministic sizing)
	layout DialogLayout
}

func NewHelpDialogModel() *HelpDialogModel {
	return NewHelpDialogModelWithMap(Keys)
}

func NewHelpDialogModelWithMap(km help.KeyMap) *HelpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &HelpDialogModel{help: h, focused: true, keyMap: km}
}

func (m *HelpDialogModel) Init() tea.Cmd { return nil }

func (m *HelpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Any key closes the help dialog (? toggles it off, Esc also works)
		_ = msg
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case LayerHitMsg:
		// Any click on the help dialog (or its components) closes it
		return m, func() tea.Msg { return CloseDialogMsg{} }
	}
	return m, nil
}

// ViewString returns the dialog content as a string for compositing
func (m *HelpDialogModel) ViewString() string {
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
	haloColor := lipgloss.Color("0") // Solid black halo
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

	content := m.help.View(m.keyMap)

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

	// Content sizes naturally to its lines - no forced height expansion

	// Ensure the title is visible on the black border bar
	// We use the original themed Dialog background for the title text area
	ctx := GetActiveContext()
	ctx.Dialog = ctx.Dialog.Background(haloColor)
	ctx.DialogTitleHelp = GetStyles().DialogTitleHelp.
		Background(GetStyles().Dialog.GetBackground()).
		Foreground(GetStyles().DialogTitleHelp.GetForeground())
	ctx.BorderColor = haloColor
	ctx.Border2Color = haloColor

	// Render the dialog with solid block borders
	// Render the dialog with solid block borders
	// We pass raw text so it uses the ctx.DialogTitleHelp base style without tag overrides
	dialogStr := RenderUniformBlockDialogCtx(" Keyboard & Mouse Controls ", content, ctx)

	// Add the solid black halo
	return AddPatternHalo(dialogStr, haloColor)
}

// View implements tea.Model
func (m *HelpDialogModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView
func (m *HelpDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZScreen + 1).ID("Dialog.Help"),
	}
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *HelpDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *HelpDialogModel) SetFocused(f bool) {
	m.focused = f
}

func (m *HelpDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Overhead for Help: Halo (2) + Bordered Dialog (2) = 4
	overhead := 4

	m.layout = DialogLayout{
		Width:    m.width,
		Height:   0, // height is content-driven, not forced
		Overhead: overhead,
	}
}
