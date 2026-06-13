package console

// cliViewportModel is a minimal Bubble Tea program that shows a full-screen
// scrollable viewport. We manage the alt screen ourselves with raw escapes so
// Bubble Tea's renderer never issues its own save/restore sequences on exit.

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// -- messages -----------------------------------------------------------------

type cliAppendMsg struct{ lines []string }
type cliSetLinesMsg struct{ lines []string }
type cliSetHeaderMsg struct{ line string }
type cliQuitMsg struct{}

// -- model --------------------------------------------------------------------

type cliViewportModel struct {
	vp              viewport.Model
	historyLines    []string
	header          string
	composeRendered []string
	width           int
	termHeight      int
	ready           bool
	readyCh         chan struct{}
}

func newCLIViewportModel(history []string) *cliViewportModel {
	m := &cliViewportModel{
		vp:      viewport.New(),
		readyCh: make(chan struct{}),
	}
	m.historyLines = append(m.historyLines, history...)
	return m
}

func (m *cliViewportModel) Init() tea.Cmd { return nil }

func (m *cliViewportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.termHeight = msg.Height
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(msg.Height)
		if !m.ready {
			m.ready = true
			close(m.readyCh)
		}
		m.rebuild(true)
		return m, nil

	case cliAppendMsg:
		atBottom := m.vp.AtBottom()
		for _, l := range msg.lines {
			m.historyLines = append(m.historyLines, m.renderLine(l))
		}
		m.rebuild(atBottom)
		return m, nil

	case cliSetHeaderMsg:
		m.header = m.renderLine(msg.line)
		m.rebuild(m.vp.AtBottom())
		return m, nil

	case cliSetLinesMsg:
		atBottom := m.vp.AtBottom()
		m.composeRendered = make([]string, len(msg.lines))
		for i, l := range msg.lines {
			m.composeRendered[i] = m.renderLine(l)
		}
		m.rebuild(atBottom)
		return m, nil

	case cliQuitMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			viewportSendSignal(0x03)
			return m, nil
		case "ctrl+\\":
			viewportSendSignal(0x1C)
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.MouseMsg:
		mouse := msg.Mouse()
		switch mouse.Button {
		case tea.MouseWheelUp:
			m.vp.ScrollUp(3)
		case tea.MouseWheelDown:
			m.vp.ScrollDown(3)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
}

// View never sets AltScreen — we own the alt screen via raw escapes in
// viewport.go so Bubble Tea's renderer never issues its own restore sequence.
func (m *cliViewportModel) View() tea.View {
	content := ""
	if m.ready {
		content = m.vp.View()
	}
	return tea.View{
		Content:   content,
		AltScreen: false,
		MouseMode: tea.MouseModeCellMotion,
	}
}

func (m *cliViewportModel) renderLine(line string) string {
	if m.width > 0 {
		return lipgloss.NewStyle().MaxWidth(m.width).Render(line)
	}
	return line
}

func (m *cliViewportModel) rebuild(scrollToBottom bool) {
	var parts []string
	if len(m.historyLines) > 0 {
		parts = append(parts, strings.Join(m.historyLines, "\n"))
	}
	if m.header != "" {
		parts = append(parts, m.header)
	}
	if len(m.composeRendered) > 0 {
		parts = append(parts, strings.Join(m.composeRendered, "\n"))
	}
	m.vp.SetContent(strings.Join(parts, "\n"))
	if scrollToBottom {
		m.vp.GotoBottom()
	}
}
