package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

const (
	logPanelZoneID    = "log-toggle"
	logViewportZoneID = "log-viewport"
)

// logLineMsg carries a new log line from the subscription channel.
type logLineMsg string

// toggleLogPanelMsg requests the log panel to expand or collapse.
type toggleLogPanelMsg struct{}

// LogPanelModel is the slide-up log viewer that lives below the helpline.
// When collapsed it shows only a 1-line toggle strip (^).
// When expanded it occupies half the terminal height.
type LogPanelModel struct {
	expanded bool
	focused  bool
	viewport viewport.Model
	lines    []string
	width    int
	// totalHeight is the full height to use when expanded (set via SetSize).
	totalHeight int
}

// NewLogPanelModel creates a new log panel in collapsed state.
func NewLogPanelModel() LogPanelModel {
	vp := viewport.New()
	return LogPanelModel{viewport: vp}
}

// CollapsedHeight returns the height the panel always occupies (the toggle strip).
func (m LogPanelModel) CollapsedHeight() int {
	return 1
}

// Height returns the current rendered height of the panel.
func (m LogPanelModel) Height() int {
	if m.expanded && m.totalHeight > 1 {
		return m.totalHeight / 2
	}
	return 1
}

// SetSize stores dimensions so the panel can size itself when expanded.
func (m *LogPanelModel) SetSize(width, totalTermHeight int) {
	m.width = width
	m.totalHeight = totalTermHeight
	if m.expanded {
		panelH := totalTermHeight / 2
		vpH := panelH - 1 // subtract toggle strip
		if vpH < 1 {
			vpH = 1
		}
		m.viewport.SetWidth(width)
		m.viewport.SetHeight(vpH)
	}
}

// Init preloads the log file and starts the live subscription.
func (m LogPanelModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return preloadLogFile() },
		waitForLogLine(),
	)
}

// preloadLogFile reads the last 200 lines of the log file and returns them as a
// single logLineMsg with embedded newlines so the panel can display history immediately.
func preloadLogFile() tea.Msg {
	path := logger.GetLogFilePath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		all = append(all, sc.Text())
	}

	const maxLines = 200
	if len(all) > maxLines {
		all = all[len(all)-maxLines:]
	}
	if len(all) == 0 {
		return nil
	}
	return logLineMsg(strings.Join(all, "\n"))
}

// waitForLogLine blocks until the logger sends a line, then returns it as a message.
func waitForLogLine() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logger.SubscribeLogLines()
		if !ok {
			return nil
		}
		return logLineMsg(line)
	}
}

// Update handles log lines, toggle requests, and viewport scroll events.
func (m LogPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case logLineMsg:
		styles := GetStyles()
		text := string(msg)
		newLines := strings.Split(text, "\n")
		for _, line := range newLines {
			rendered := RenderThemeText(line, styles.Console)
			// Truncate to viewport width to prevent overflow past borders
			if m.viewport.Width() > 0 {
				rendered = lipgloss.NewStyle().
					MaxWidth(m.viewport.Width()).
					Render(rendered)
			}
			m.lines = append(m.lines, rendered)
		}
		content := strings.Join(m.lines, "\n")
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, waitForLogLine()
		return m, nil

	case tea.MouseWheelMsg:
		if m.expanded {
			if msg.Button == tea.MouseWheelUp {
				m.viewport.ScrollUp(3)
				return m, nil
			}
			if msg.Button == tea.MouseWheelDown {
				m.viewport.ScrollDown(3)
				return m, nil
			}
		}
	}
	// Update viewport for other scrolling keys/events (only if expanded)
	var cmd tea.Cmd
	if m.expanded {
		m.viewport, cmd = m.viewport.Update(msg)
	}

	return m, cmd
}

// ViewString returns the panel content as a string for compositing
func (m LogPanelModel) ViewString() string {
	styles := GetStyles()

	// Choose line character: thick when focused, normal otherwise
	var sepChar string
	if m.focused {
		if styles.LineCharacters {
			sepChar = lipgloss.ThickBorder().Top // "━"
		} else {
			sepChar = "="
		}
	} else {
		sepChar = styles.SepChar
	}

	// Build label: arrow + title + arrow on both sides
	marker := "^"
	if m.expanded {
		marker = "v"
	}
	label := " " + marker + " Log " + marker + " "

	// Right-side scroll percent indicator (only when focused and expanded)
	rightIndicator := ""
	if m.focused && m.expanded {
		pct := int(m.viewport.ScrollPercent() * 100)
		rightIndicator = fmt.Sprintf(" %d%% ", pct)
	}

	// Center the label in the full width; indicator takes space from the right dashes only
	labelW := lipgloss.Width(label)
	dashW := (m.width - labelW) / 2
	if dashW < 0 {
		dashW = 0
	}
	leftDashes := strutil.Repeat(sepChar, dashW)

	rightTotal := m.width - dashW - labelW
	var stripContent string
	if rightIndicator != "" {
		indicatorW := lipgloss.Width(rightIndicator)
		rightDashW := rightTotal - indicatorW
		if rightDashW < 0 {
			rightDashW = 0
		}
		stripContent = leftDashes + label + strutil.Repeat(sepChar, rightDashW) + rightIndicator
	} else {
		stripContent = leftDashes + label + strutil.Repeat(sepChar, rightTotal)
	}

	// Use the dedicated LogPanel theme color for the strip line
	stripStyle := lipgloss.NewStyle().
		Width(m.width).
		Foreground(styles.LogPanelColor).
		Background(styles.HelpLine.GetBackground())
	strip := zone.Mark(logPanelZoneID, stripStyle.Render(stripContent))

	if !m.expanded {
		return strip
	}

	// Expanded: viewport below the strip
	vpStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.viewport.Height()).
		Background(styles.Console.GetBackground()).
		Foreground(styles.Console.GetForeground())
	m.viewport.Style = vpStyle

	// Use MaintainBackground to ensure console background is preserved through resets
	vpView := MaintainBackground(m.viewport.View(), styles.Console)
	vpViewMarked := zone.Mark(logViewportZoneID, vpView)
	return lipgloss.JoinVertical(lipgloss.Left, strip, vpViewMarked)
}

// View renders the panel at its current height.
func (m LogPanelModel) View() tea.View {
	return tea.NewView(m.ViewString())
}
