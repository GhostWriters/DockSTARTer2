package tui

import (
	"bufio"
	"context"
	"time"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/tui/components/streamvp"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

const (
	panelZoneID    = IDPanelToggle
	resizeZoneID   = IDPanelResize
	viewportZoneID = IDPanelViewport
)

// ─── Message types ────────────────────────────────────────────────────────────

// panelLineMsg carries a new log line from the subscription channel.
type panelLineMsg string

// togglePanelMsg requests the log panel to expand or collapse.
type togglePanelMsg struct{}

// consoleLinesMsg carries a batch of lines from a running console command.
type consoleLinesMsg struct{ lines []string }

// ConsoleLockMsg is sent by any long-running operation to lock or unlock the
// console input bar. ID must be a unique identifier for the operation (e.g. a
// UUID or stable name). Send Locked:true when starting, Locked:false when done.
// The input bar unlocks only when all registered lockers have released.
type ConsoleLockMsg struct {
	ID     string
	Locked bool
}

// panelSpinnerTickMsg advances the panel title spinner by one frame while a command is running.
type panelSpinnerTickMsg struct{}

// consoleDoneMsg signals that a console command has finished.
type consoleDoneMsg struct {
	err           error
	configChanged bool // reload TUI styles (ConfigChangedMsg)
	appsChanged   bool // refresh app list (RefreshAppsListMsg)
}

// ─── Model ───────────────────────────────────────────────────────────────────

// PanelModel is the slide-up console panel that lives below the helpline.
// When collapsed it shows only a 1-line toggle strip (^).
// When expanded it shows a log/output viewport and a single-line input bar.
type PanelModel struct {
	expanded bool
	focused  bool
	sv       streamvp.Model
	width    int
	// totalHeight is the full height of the terminal (used for max constraint)
	totalHeight int
	// height is the current variable height of the panel (when expanded)
	height int

	// Resizing state
	resizeDrag ScrollbarDragState

	// maxHeight is the externally imposed height ceiling (set by AppModel).
	// Zero means "no override" — panelMaxHeight() is used as the fallback.
	maxHeight int

	// Console input bar
	inputFocused bool
	input        sinput.Model
	history      []string // in-session command history, oldest first
	historyIdx   int      // -1 = new command; >=0 = navigating history
	historyDraft string   // saved in-progress text when navigating up

	// Title bar focus state (for keyboard resize and press flash)
	TitleBarFocus

	// Running console command state
	consoleScanner       *bufio.Scanner
	consoleCancel        context.CancelFunc
	consoleConfigChanged bool
	consoleAppsChanged   bool

	// sessionLockers tracks active locks by ID. Input is locked when non-empty.
	sessionLockers map[string]struct{}

	panelMode            string // "log", "console", "system", or "none"
	connType             string // "local", "ssh", or "web"
	spinnerFrame         int
	lastLineTime         time.Time // when the last log line arrived; spinner runs until idle for spinnerIdleTimeout
	panelChanged         bool      // new content arrived while collapsed; cleared on expand
	replaceHeaderCount   int       // line count before first replaceOutputMsg (-1 = not yet set)
}

const spinnerIdleTimeout = 1500 * time.Millisecond

const (
	panelWidgetUp = TitleBarWidgetRefresh // reuses enum slot; panel has no Refresh action
	panelWidgetDn = TitleBarWidgetClose   // reuses enum slot; panel has no Close action
)

func (m *PanelModel) spinnerTickCmd() tea.Cmd {
	if !console.SpinnerEnabled {
		return nil
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 {
		fps = 100 * time.Millisecond
	}
	return tea.Tick(fps, func(time.Time) tea.Msg {
		return panelSpinnerTickMsg{}
	})
}

// changedIndicatorChar returns the character used to signal new content arrived while collapsed.
func changedIndicatorChar(lineCharacters bool) string {
	if lineCharacters {
		return "•"
	}
	return "*"
}

// currentSpinnerMarker returns the spinner frame to use in the panel title,
// the changed indicator when new content arrived while collapsed,
// or "" when idle.
func (m *PanelModel) currentSpinnerMarker() (indicatorL, indicatorR string, changed bool) {
	spinning := !m.lastLineTime.IsZero() && time.Since(m.lastLineTime) < spinnerIdleTimeout && console.SpinnerEnabled
	if spinning {
		ctx := GetActiveContext()
		l, r := console.TitleSpinnerFrames(m.spinnerFrame, ctx.LineCharacters)
		return l, r, !m.expanded
	}
	if m.panelChanged && !m.expanded {
		ctx := GetActiveContext()
		ch := changedIndicatorChar(ctx.LineCharacters)
		return ch, ch, true
	}
	return "", "", false
}

// panelRenderFn returns the render function for streamvp line rendering.
func panelRenderFn() func(string) string {
	styles := GetStyles()
	return func(raw string) string {
		return RenderConsoleText(raw, styles.Console)
	}
}

// applyInputStyles updates the sinput colours from the current theme.
func (m *PanelModel) applyInputStyles() {
	styles := GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	tiStyles.Cursor.Color = TextCursorColor()
	m.input.SetStyles(tiStyles)
}

// NewPanelModel creates a new console panel in the requested state.
func NewPanelModel(panelMode string, connType string) PanelModel {
	ti := textinput.New()
	ti.Prompt = "> "
	inp := sinput.New(ti)

	m := PanelModel{
		sv:                 streamvp.New("panel"),
		input:              inp,
		historyIdx:         -1,
		panelMode:          panelMode,
		connType:           connType,
		expanded:           false,
		replaceHeaderCount: -1,
	}
	m.applyInputStyles()
	return m
}

// vpHeight returns the viewport height for the current panel state.
func (m *PanelModel) vpHeight() int {
	vpH := m.height - 1
	if m.panelMode == "console" || m.panelMode == "system" {
		vpH -= 3
	}
	if vpH < 1 {
		vpH = 1
	}
	return vpH
}

// CollapsedHeight returns the height the panel always occupies (the toggle strip).
func (m PanelModel) CollapsedHeight() int { return 1 }

// Height returns the current rendered height of the panel.
func (m PanelModel) Height() int {
	if m.expanded {
		if m.height > 2 {
			return m.height
		}
		if m.totalHeight > 2 {
			return m.totalHeight / 2
		}
	}
	if m.panelMode == "none" {
		return 0
	}
	return 1
}

// SetMaxHeight updates the externally imposed height ceiling. Pass 0 to revert to default.
func (m *PanelModel) SetMaxHeight(h int) { m.maxHeight = h }

// ResizeBy adjusts the panel height by delta lines (positive = taller, negative = shorter).
// Returns true if the height actually changed.
func (m *PanelModel) ResizeBy(delta int) bool {
	if !m.expanded {
		return false
	}
	current := m.height
	if current == 0 {
		if m.totalHeight > 0 {
			current = m.totalHeight / 2
		} else {
			current = 10
		}
	}
	newHeight := current + delta
	maxH := m.effectiveMaxHeight()
	if newHeight > maxH {
		newHeight = maxH
	}
	if newHeight < 5 {
		newHeight = 5
	}
	if newHeight == m.height {
		return false
	}
	m.height = newHeight
	vpH := m.vpHeight()
	m.sv.SetSize(m.sv.Width(), vpH)
	m.sv.SetYOffset(m.sv.YOffset())
	return true
}

// lockSession adds or removes a locker by ID and returns a cmd that notifies
// the app of the new CommandInProgress state so the exit button updates immediately.
func (m *PanelModel) lockSession(id string, lock bool) tea.Cmd {
	before := m.CommandInProgress()
	if lock {
		if m.sessionLockers == nil {
			m.sessionLockers = make(map[string]struct{})
		}
		m.sessionLockers[id] = struct{}{}
		if m.inputFocused {
			m.input.Blur()
			m.inputFocused = false
		}
	} else {
		delete(m.sessionLockers, id)
	}
	after := m.CommandInProgress()
	if after != before {
		locked := after
		return func() tea.Msg { return PanelCommandLockChangedMsg{Locked: locked} }
	}
	return nil
}

// SetSessionActive is kept for compatibility; uses a fixed ID.
func (m *PanelModel) SetSessionActive(active bool) tea.Cmd {
	return m.lockSession("__session__", active)
}

func (m *PanelModel) sessionActive() bool { return len(m.sessionLockers) > 0 }

// CommandInProgress reports whether a destructive console command is currently running.
func (m *PanelModel) CommandInProgress() bool { return len(m.sessionLockers) > 0 }

// applyDragY computes the new panel height from the current mouse Y.
func (m *PanelModel) applyDragY(mouseY int) {
	delta := m.resizeDrag.StartMouseY - mouseY
	newHeight := m.resizeDrag.StartThumbTop + delta
	maxH := m.effectiveMaxHeight()
	if newHeight > maxH {
		newHeight = maxH
	}
	if newHeight < 5 {
		newHeight = 5
	}
	m.height = newHeight
	if m.expanded {
		vpH := m.vpHeight()
		m.sv.SetSize(m.sv.Width(), vpH)
		m.sv.SetYOffset(m.sv.YOffset())
	}
}

// effectiveMaxHeight returns the ceiling used for clamping.
func (m *PanelModel) effectiveMaxHeight() int {
	if m.maxHeight > 0 {
		return m.maxHeight
	}
	return panelMaxHeight(m.totalHeight)
}

// SetSize stores dimensions and adjusts the viewport and input bar.
func (m *PanelModel) SetSize(width, totalTermHeight int) {
	oldWidth := m.width
	m.width = width
	m.totalHeight = totalTermHeight

	svWidth := width - ScrollbarGutterWidth
	if oldWidth != width && width > 0 {
		m.sv.SetSize(svWidth, m.sv.Height())
		m.sv.ReRenderWith(panelRenderFn())
	} else {
		m.sv.SetSize(svWidth, m.sv.Height())
	}

	m.input.SetWidth(width - 4)

	if m.expanded {
		if m.height == 0 {
			m.height = totalTermHeight / 2
		}
		maxH := m.effectiveMaxHeight()
		if m.height > maxH {
			m.height = maxH
		}
		if m.height < 5 {
			m.height = 5
		}

		vpH := m.vpHeight()
		prevH := m.sv.Height()
		m.sv.SetSize(svWidth, vpH)
		if prevH == 0 && vpH > 0 {
			m.sv.GotoBottom()
		} else if vpH > 0 {
			m.sv.SetYOffset(m.sv.YOffset())
		}
	}
}

// Init preloads the log file and starts the live subscription.
func (m PanelModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return preloadPanelLog() },
		waitForPanelLine(),
	)
}

// FocusInput transitions the panel to input-focused state (called from AppModel).
// Returns the Blink command needed to activate the hardware cursor.
func (m *PanelModel) FocusInput() tea.Cmd {
	hasInput := m.panelMode == "console" || m.panelMode == "system"
	if !hasInput || m.sessionActive() || !m.expanded {
		return nil
	}
	m.inputFocused = true
	cmd := m.input.Focus()
	return tea.Batch(cmd, sinput.Blink)
}

// GetInputCursor returns the hardware cursor position relative to the panel's
// top-left corner, cursor shape, and whether to show it.
func (m PanelModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if !m.expanded || !m.inputFocused || !m.input.Focused() {
		return 0, 0, tea.CursorBar, false
	}
	vpH := m.height - 4
	relY = vpH + 2
	relX = 1 + m.input.CursorColumn()
	if m.input.IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

// DragScrollbar scrolls the viewport to match the dragged thumb position.
func (m PanelModel) DragScrollbar(mouseY int, drag *ScrollbarDragState, sbAbsTopY int, info ScrollbarInfo) (PanelModel, bool) {
	total := m.sv.TotalLineCount()
	visible := m.sv.Height()
	if total <= visible {
		return m, false
	}
	maxOff := total - visible
	newOff, _ := drag.ScrollOffset(mouseY, sbAbsTopY, maxOff, info)
	if newOff == m.sv.YOffset() {
		return m, false
	}
	m.sv.GotoTop()
	m.sv.ScrollDown(newOff)
	return m, true
}

// panelMaxHeight returns the maximum height the console panel may occupy.
func panelMaxHeight(totalTermHeight int) int {
	layout := GetLayout()
	shadowH := 0
	if currentConfig.UI.Shadow {
		shadowH = layout.ShadowHeight
	}
	usable := totalTermHeight - layout.ChromeHeight(1) - layout.BottomChrome(layout.HelplineHeight) - shadowH
	maxH := usable / 2
	if maxH < 3 {
		maxH = 3
	}
	return maxH
}
