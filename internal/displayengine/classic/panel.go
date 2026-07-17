package classic

import (
	"bufio"
	"context"
	"time"

	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/tui/components/streamvp"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

const (
	panelZoneID    = IDPanelToggle
	ResizeZoneID   = IDPanelResize
	viewportZoneID = IDPanelViewport
)

// ─── Message types ────────────────────────────────────────────────────────────

// PanelLineMsg carries a new log line from the subscription channel.
type PanelLineMsg string

// TogglePanelMsg requests the log panel to expand or collapse.
type TogglePanelMsg struct{}

// ConsoleLinesMsg carries a batch of lines from a running console command.
type ConsoleLinesMsg struct{ Lines []string }

// ConsoleLockMsg is sent by any long-running operation to lock or unlock the
// console input bar. ID must be a unique identifier for the operation (e.g. a
// UUID or stable name). Send Locked:true when starting, Locked:false when done.
// The input bar unlocks only when all registered lockers have released.
type ConsoleLockMsg struct {
	ID     string
	Locked bool
}


// ConsoleDoneMsg signals that a console command has finished.
type ConsoleDoneMsg struct {
	Err           error
	ConfigChanged bool // reload TUI styles (ConfigChangedMsg)
	AppsChanged   bool // refresh app list (RefreshAppsListMsg)
}

// ─── Model ───────────────────────────────────────────────────────────────────

// PanelModel is the slide-up console panel that lives below the helpline.
// When collapsed it shows only a 1-line toggle strip (^).
// When expanded it shows a log/output viewport and a single-line input bar.
type PanelModel struct {
	Expanded bool
	Focused  bool
	Sv       streamvp.Model
	width    int
	// totalHeight is the full height of the terminal (used for max constraint)
	totalHeight int
	// PanelHeight is the current variable height of the panel (when expanded)
	PanelHeight int

	// Resizing state
	ResizeDrag ScrollbarDragState

	// maxHeight is the externally imposed height ceiling (set by AppModel).
	// Zero means "no override" — panelMaxHeight() is used as the fallback.
	maxHeight int

	// Console input bar
	InputFocused bool
	Input        sinput.Model
	history      []string // in-session command history, oldest first
	historyIdx   int      // -1 = new command; >=0 = navigating history
	historyDraft string   // saved in-progress text when navigating up

	// Title bar focus state (for keyboard resize and press flash)
	TitleBarFocus

	// Running console command state
	consoleScanner       *bufio.Scanner
	ConsoleCancel        context.CancelFunc
	consoleConfigChanged bool
	consoleAppsChanged   bool

	// sessionLockers tracks active locks by ID. Input is locked when non-empty.
	sessionLockers map[string]struct{}

	PanelMode            string // "log", "console", "system", or "none"
	connType             string // "local", "ssh", or "web"
	clientIP             string // the owning session's real client IP/address, for edit-lock terminal attribution (see sessionlocks.SessionManager.AcquireEditLock)
	sessionKey           string // identifies the owning TUI session for edit-lock re-entry (see sessionlocks.SessionManager.localSessionKey)
	logSub               <-chan string // this panel's own subscription to logger.SubscribeLogLines -- see UnsubscribeLog
	logUnsub             func()        // releases logSub; call once when this panel's session ends (see UnsubscribeLog)
	confirmFunc          func(title, question string, defaultYes bool) bool // this session's own confirm callback, set via SetConfirmFunc; attached to console commands' ctx so their prompts reach this session, not whichever session's Program is currently the global tui.program
	promptFunc           func(title, question string, sensitive bool, initialValue ...string) (string, error) // this session's own text-prompt callback, set via SetPromptFunc; same purpose as confirmFunc but for TextPrompt (e.g. the sudo password prompt)
	titleSpinner         TitleSpinner
	lastLineTime         time.Time // when the last log line arrived; spinner runs until idle for spinnerIdleTimeout
	panelChanged         bool      // new content arrived while collapsed; cleared on expand
	replaceHeaderCount   int       // line count before first ReplaceOutputMsg (-1 = not yet set)
}

const spinnerIdleTimeout = 1500 * time.Millisecond

const (
	PanelWidgetUp = "panel-up" // widget ID for the resize-up button in the panel title bar
	PanelWidgetDn = "panel-dn" // widget ID for the resize-down button in the panel title bar
)

// AdvanceSpinners advances the panel title spinner and the inline streamvp
// spinner if their interval has elapsed. Returns true if anything changed.
// Called by the global tick in AppModel.Update.
func (m *PanelModel) AdvanceSpinners(now time.Time) bool {
	isSpinnerActive := !m.lastLineTime.IsZero() && time.Since(m.lastLineTime) < spinnerIdleTimeout
	if isSpinnerActive {
		m.titleSpinner.Start()
	} else {
		m.titleSpinner.Stop()
	}
	changed := m.titleSpinner.AdvanceSpinner(now)
	if m.Sv.AdvanceSpinner(now) {
		changed = true
	}
	return changed
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
	if l, r := m.titleSpinner.Indicators(); l != "" {
		return l, r, !m.Expanded
	}
	if m.panelChanged && !m.Expanded {
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
	m.Input.SetStyles(tiStyles)
}

// NewPanelModel creates a new console panel in the requested state.
func NewPanelModel(panelMode string, connType string, clientIP string, sessionKey string) PanelModel {
	ti := textinput.New()
	ti.Prompt = "> "
	inp := sinput.New(ti)

	logSub, logUnsub := logger.SubscribeLogLines()
	m := PanelModel{
		Sv:                 streamvp.New(),
		Input:              inp,
		historyIdx:         -1,
		PanelMode:          panelMode,
		connType:           connType,
		clientIP:           clientIP,
		sessionKey:         sessionKey,
		logSub:             logSub,
		logUnsub:           logUnsub,
		Expanded:           false,
		replaceHeaderCount: -1,
	}
	m.applyInputStyles()
	return m
}

// UnsubscribeLog releases this panel's log-line subscription. Call once when
// the owning session ends -- otherwise its subscription (and the goroutine
// blocked reading from it) leaks for the lifetime of a long-running
// server-daemon process, since a new subscription is created per session but
// nothing else ever removes it.
func (m *PanelModel) UnsubscribeLog() {
	if m.logUnsub != nil {
		m.logUnsub()
	}
}

// SetConfirmFunc sets this panel's session-scoped confirm callback (see
// confirmFunc). Called once by the tui package after this session's own
// *tea.Program is constructed, since PanelModel is built before that Program
// exists (NewProgram needs the already-built AppModel/PanelModel as input).
func (m *PanelModel) SetConfirmFunc(fn func(title, question string, defaultYes bool) bool) {
	m.confirmFunc = fn
}

// ConfirmFunc returns this panel's session-scoped confirm callback, or nil
// if SetConfirmFunc was never called.
func (m *PanelModel) ConfirmFunc() func(title, question string, defaultYes bool) bool {
	return m.confirmFunc
}

// SetPromptFunc sets this panel's session-scoped text-prompt callback (see
// promptFunc). Called alongside SetConfirmFunc for the same reason.
func (m *PanelModel) SetPromptFunc(fn func(title, question string, sensitive bool, initialValue ...string) (string, error)) {
	m.promptFunc = fn
}

// PromptFunc returns this panel's session-scoped text-prompt callback, or
// nil if SetPromptFunc was never called.
func (m *PanelModel) PromptFunc() func(title, question string, sensitive bool, initialValue ...string) (string, error) {
	return m.promptFunc
}

// vpHeight returns the viewport height for the current panel state.
func (m *PanelModel) vpHeight() int {
	vpH := m.PanelHeight - 1
	if m.PanelMode == "console" || m.PanelMode == "system" {
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
	if m.Expanded {
		if m.PanelHeight > 2 {
			return m.PanelHeight
		}
		if m.totalHeight > 2 {
			return m.totalHeight / 2
		}
	}
	if m.PanelMode == "none" {
		return 0
	}
	return 1
}

// SetMaxHeight updates the externally imposed height ceiling. Pass 0 to revert to default.
func (m *PanelModel) SetMaxHeight(h int) { m.maxHeight = h }

// ResizeBy adjusts the panel height by delta lines (positive = taller, negative = shorter).
// Returns true if the height actually changed.
func (m *PanelModel) ResizeBy(delta int) bool {
	if !m.Expanded {
		return false
	}
	current := m.PanelHeight
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
	if newHeight == m.PanelHeight {
		return false
	}
	m.PanelHeight = newHeight
	vpH := m.vpHeight()
	m.Sv.SetSize(m.Sv.Width(), vpH)
	m.Sv.SetYOffset(m.Sv.YOffset())
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
		if m.InputFocused {
			m.Input.Blur()
			m.InputFocused = false
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

func (m *PanelModel) SessionActive() bool { return len(m.sessionLockers) > 0 }

// CommandInProgress reports whether a destructive console command is currently running.
func (m *PanelModel) CommandInProgress() bool { return len(m.sessionLockers) > 0 }

// applyDragY computes the new panel height from the current mouse Y.
func (m *PanelModel) applyDragY(mouseY int) {
	delta := m.ResizeDrag.StartMouseY - mouseY
	newHeight := m.ResizeDrag.StartThumbTop + delta
	maxH := m.effectiveMaxHeight()
	if newHeight > maxH {
		newHeight = maxH
	}
	if newHeight < 5 {
		newHeight = 5
	}
	m.PanelHeight = newHeight
	if m.Expanded {
		vpH := m.vpHeight()
		m.Sv.SetSize(m.Sv.Width(), vpH)
		m.Sv.SetYOffset(m.Sv.YOffset())
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
	SetActiveOutputWidth(svWidth) // publish width so compose bars reflow on resize
	if oldWidth != width && width > 0 {
		m.Sv.SetSize(svWidth, m.Sv.Height())
		m.Sv.ReRenderWith(panelRenderFn())
	} else {
		m.Sv.SetSize(svWidth, m.Sv.Height())
	}

	m.Input.SetWidth(width - 4)

	if m.Expanded {
		if m.PanelHeight == 0 {
			m.PanelHeight = totalTermHeight / 2
		}
		maxH := m.effectiveMaxHeight()
		if m.PanelHeight > maxH {
			m.PanelHeight = maxH
		}
		if m.PanelHeight < 5 {
			m.PanelHeight = 5
		}

		vpH := m.vpHeight()
		prevH := m.Sv.Height()
		m.Sv.SetSize(svWidth, vpH)
		if prevH == 0 && vpH > 0 {
			m.Sv.GotoBottom()
		} else if vpH > 0 {
			m.Sv.SetYOffset(m.Sv.YOffset())
		}
	}
}

// Init preloads the log file and starts the live subscription.
func (m PanelModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return preloadPanelLog() },
		waitForPanelLine(m.logSub),
	)
}

// FocusInput transitions the panel to input-focused state (called from AppModel).
// Returns the Blink command needed to activate the hardware cursor.
func (m *PanelModel) FocusInput() tea.Cmd {
	hasInput := m.PanelMode == "console" || m.PanelMode == "system"
	if !hasInput || m.SessionActive() || !m.Expanded {
		return nil
	}
	m.InputFocused = true
	cmd := m.Input.Focus()
	return tea.Batch(cmd, sinput.Blink)
}

// GetInputCursor returns the hardware cursor position relative to the panel's
// top-left corner, cursor shape, and whether to show it.
func (m PanelModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if !m.Expanded || !m.InputFocused || !m.Input.Focused() {
		return 0, 0, tea.CursorBar, false
	}
	vpH := m.PanelHeight - 4
	relY = vpH + 2
	relX = 1 + m.Input.CursorColumn()
	if m.Input.IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

// DragScrollbar scrolls the viewport to match the dragged thumb position.
func (m PanelModel) DragScrollbar(mouseY int, drag *ScrollbarDragState, sbAbsTopY int, info ScrollbarInfo) (PanelModel, bool) {
	total := m.Sv.TotalLineCount()
	visible := m.Sv.Height()
	if total <= visible {
		return m, false
	}
	maxOff := total - visible
	newOff, _ := drag.ScrollOffset(mouseY, sbAbsTopY, maxOff, info)
	if newOff == m.Sv.YOffset() {
		return m, false
	}
	m.Sv.GotoTop()
	m.Sv.ScrollDown(newOff)
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

