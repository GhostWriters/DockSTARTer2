package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	logPanelZoneID    = IDLogToggle
	logResizeZoneID   = IDLogResize
	logViewportZoneID = IDLogViewport
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
	// totalHeight is the full height of the terminal (used for max constraint)
	totalHeight int
	// height is the current variable height of the panel (when expanded)
	height int

	// Resizing state
	isDragging        bool
	dragStartY        int
	heightAtDragStart int
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
	if m.expanded {
		if m.height > 1 {
			return m.height
		}
		// Fallback if height not set yet
		if m.totalHeight > 1 {
			return m.totalHeight / 2
		}
	}
	return 1
}

// SetSize stores dimensions so the panel can size itself when expanded.
func (m *LogPanelModel) SetSize(width, totalTermHeight int) {
	m.width = width
	m.totalHeight = totalTermHeight

	if m.expanded {
		// If height is unset (0), default to half screen
		if m.height == 0 {
			m.height = totalTermHeight / 2
		}
		// Ensure height is within bounds (e.g., if terminal shrank)
		maxH := totalTermHeight - 4 // Leave room for header/footer
		if m.height > maxH {
			m.height = maxH
		}
		if m.height < 2 {
			m.height = 2
		}

		vpH := m.height - 1 // subtract toggle strip
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
			// If viewport is not yet expanded, use m.width
			targetWidth := m.viewport.Width()
			if targetWidth <= 0 && m.width > 0 {
				targetWidth = m.width
			}
			if targetWidth > 0 {
				rendered = lipgloss.NewStyle().
					MaxWidth(targetWidth).
					Render(rendered)
			}
			m.lines = append(m.lines, rendered)
		}
		content := strings.Join(m.lines, "\n")
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, waitForLogLine()

	case toggleLogPanelMsg:
		m.expanded = !m.expanded
		// If expanding, ensure we have the correct size immediately
		if m.expanded {
			m.SetSize(m.width, m.totalHeight)
			// The viewport's YOffset may be out of bounds if logs arrived while
			// the panel had 0 height. Scrolling to bottom corrects it.
			m.viewport.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if msg.ID == logPanelZoneID {
			return m, func() tea.Msg { return toggleLogPanelMsg{} }
		}

	case tea.MouseClickMsg:
		// Handle drag start on resize zones
		if msg.Button == tea.MouseLeft {
			// No direct zone check here, handled by AppModel forwarding
			m.isDragging = true
			m.dragStartY = msg.Y
			m.heightAtDragStart = m.height
			if !m.expanded {
				m.expanded = true
				m.height = 1
				m.SetSize(m.width, m.totalHeight)
				m.heightAtDragStart = 1
			}
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.isDragging {
			m.isDragging = false
			return m, nil
		}

	case tea.MouseMotionMsg:
		if m.isDragging {
			// Calculate delta. Y increases downwards.
			// Dragging UP (smaller Y) means height increases.
			// Delta = dragStartY - msg.Y
			delta := m.dragStartY - msg.Y
			newHeight := m.heightAtDragStart + delta

			// Clamp height
			maxH := m.totalHeight - 4 // Leave space for header
			if newHeight > maxH {
				newHeight = maxH
			}
			if newHeight < 2 {
				newHeight = 2 // Minimum 1 line content + 1 line strip
			}

			m.height = newHeight
			if m.expanded {
				m.SetSize(m.width, m.totalHeight) // Refresh viewport
			}
			return m, nil
		}

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

	case tea.KeyPressMsg:
		if m.expanded {
			switch {
			case key.Matches(msg, Keys.Home):
				m.viewport.GotoTop()
				return m, nil
			case key.Matches(msg, Keys.End):
				m.viewport.GotoBottom()
				return m, nil
			case key.Matches(msg, Keys.HalfPageUp):
				m.viewport.HalfPageUp()
				return m, nil
			case key.Matches(msg, Keys.HalfPageDown):
				m.viewport.HalfPageDown()
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
	ctx := GetActiveContext()

	// Choose line character: thick when focused, normal otherwise
	var sepChar string
	if m.focused {
		if ctx.LineCharacters {
			sepChar = lipgloss.ThickBorder().Top // "━"
		} else {
			sepChar = "="
		}
	} else {
		if ctx.LineCharacters {
			sepChar = "─"
		} else {
			sepChar = "-"
		}
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

	// Use the dedicated LogPanel theme color for the strip line
	stripStyle := lipgloss.NewStyle().
		Foreground(ctx.LogPanelColor).
		Background(ctx.HelpLine.GetBackground())

	// Build content parts
	leftContent := stripStyle.Render(leftDashes)
	labelContent := stripStyle.Render(label)

	var rightContentStr string
	if rightIndicator != "" {
		indicatorW := lipgloss.Width(rightIndicator)
		rightDashW := rightTotal - indicatorW
		if rightDashW < 0 {
			rightDashW = 0
		}
		rightContentStr = strutil.Repeat(sepChar, rightDashW) + rightIndicator
	} else {
		rightContentStr = strutil.Repeat(sepChar, rightTotal)
	}
	rightContent := stripStyle.Render(rightContentStr)

	// Combine
	// Just join normally, hit-layers are handled in Layers()
	strip := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, labelContent, rightContent)
	strip = lipgloss.NewStyle().Width(m.width).Render(strip)

	// Expanded: viewport below the strip
	vpStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.viewport.Height()).
		Background(ctx.Console.GetBackground()).
		Foreground(ctx.Console.GetForeground())
	m.viewport.Style = vpStyle

	// Use MaintainBackground to ensure console background is preserved through resets
	vpView := MaintainBackground(m.viewport.View(), ctx.Console)
	return lipgloss.JoinVertical(lipgloss.Left, strip, vpView)
}

// Layers returns a single layer with the panel content for visual compositing
func (m LogPanelModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZLogPanel).ID(IDLogPanel),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m LogPanelModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	// Calculate layout for the toggle strip
	marker := "^"
	if m.expanded {
		marker = "v"
	}
	label := " " + marker + " Log " + marker + " "
	labelW := lipgloss.Width(label)
	dashW := (m.width - labelW) / 2
	if dashW < 0 {
		dashW = 0
	}
	rightTotal := m.width - dashW - labelW

	// Left resize handle
	regions = append(regions, HitRegion{
		ID:     IDLogResize,
		X:      offsetX,
		Y:      offsetY,
		Width:  dashW,
		Height: 1,
		ZOrder: ZLogPanel + 1,
	})

	// Toggle label
	regions = append(regions, HitRegion{
		ID:     IDLogToggle,
		X:      offsetX + dashW,
		Y:      offsetY,
		Width:  labelW,
		Height: 1,
		ZOrder: ZLogPanel + 1,
	})

	// Right resize handle
	regions = append(regions, HitRegion{
		ID:     IDLogResize,
		X:      offsetX + dashW + labelW,
		Y:      offsetY,
		Width:  rightTotal,
		Height: 1,
		ZOrder: ZLogPanel + 1,
	})

	// Viewport area (when expanded)
	if m.expanded {
		vpH := m.viewport.Height()
		regions = append(regions, HitRegion{
			ID:     IDLogViewport,
			X:      offsetX,
			Y:      offsetY + 1,
			Width:  m.width,
			Height: vpH,
			ZOrder: ZLogPanel + 1,
		})
	}

	return regions
}

// View renders the panel at its current height.
func (m LogPanelModel) View() tea.View {
	return tea.NewView(m.ViewString())
}
