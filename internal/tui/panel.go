package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/version"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	viewport viewport.Model
	lines    []string
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

	// Running console command state
	consoleScanner       *bufio.Scanner
	consoleCancel        context.CancelFunc
	consoleConfigChanged bool
	consoleAppsChanged   bool

	// sessionLockers tracks active locks by ID. Input is locked when non-empty.
	sessionLockers map[string]struct{}

	rawLines []string // un-wrapped, themed lines

	panelMode string // "log", "console", "system", or "none"
	connType  string // "local", "ssh", or "web"
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
	m.input.SetStyles(tiStyles)
}

// NewPanelModel creates a new console panel in the requested state.
func NewPanelModel(panelMode string, connType string) PanelModel {
	vp := viewport.New()

	ti := textinput.New()
	ti.Prompt = "> "
	inp := sinput.New(ti)

	m := PanelModel{
		viewport:   vp,
		input:      inp,
		historyIdx: -1,
		panelMode:  panelMode,
		connType:   connType,
		expanded:   false,
	}
	m.applyInputStyles()
	return m
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

// lockSession adds or removes a locker by ID.
// The input bar is locked while any lockers are registered.
func (m *PanelModel) lockSession(id string, lock bool) {
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
}

// SetSessionActive is kept for compatibility; uses a fixed ID.
func (m *PanelModel) SetSessionActive(active bool) {
	m.lockSession("__session__", active)
}

func (m *PanelModel) sessionActive() bool { return len(m.sessionLockers) > 0 }

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
		vpH := m.height - 4
		if vpH < 1 {
			vpH = 1
		}
		m.viewport.SetHeight(vpH)
		m.viewport.SetYOffset(m.viewport.YOffset())
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

	if oldWidth != width && width > 0 {
		m.reRenderLines()
	}

	m.viewport.SetWidth(width - ScrollbarGutterWidth)
	// Input box inner width: outer panel width - 2 (outer box) - 2 (inner border)
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

		vpH := m.height - 1
		if m.panelMode == "console" {
			vpH -= 3 // subtract 3-row input box
		}
		if vpH < 1 {
			vpH = 1
		}
		prevH := m.viewport.Height()
		m.viewport.SetHeight(vpH)
		if prevH == 0 && vpH > 0 && len(m.lines) > 0 {
			content := strings.Join(m.lines, "\n")
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
		} else if vpH > 0 {
			m.viewport.SetYOffset(m.viewport.YOffset())
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

// preloadPanelLog reads the last 200 lines of the log file.
func preloadPanelLog() tea.Msg {
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
	return panelLineMsg(strings.Join(all, "\n"))
}

// waitForPanelLine blocks until the logger sends a line, then returns it as a message.
func waitForPanelLine() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logger.SubscribeLogLines()
		if !ok {
			return nil
		}
		return panelLineMsg(line)
	}
}

// ─── Command execution ────────────────────────────────────────────────────────

// runShellCmd runs cmdStr as a shell command, streaming output to w.
// If stdinContent is provided, it is piped to the command's stdin.
func runShellCmd(ctx context.Context, cmdStr string, w io.Writer, stdinContent string) error {
	var shellCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shellCmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		shellCmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	shellCmd.Stdout = w
	shellCmd.Stderr = w
	if stdinContent != "" {
		shellCmd.Stdin = strings.NewReader(stdinContent + "\n")
	}
	return shellCmd.Run()
}

// readConsoleBatch reads up to 50 lines from the scanner and returns them as a
// consoleLinesMsg, or consoleDoneMsg on EOF.
func readConsoleBatch(sc *bufio.Scanner, cancel context.CancelFunc) tea.Cmd {
	return readConsoleBatchWithFlag(sc, cancel, false, false)
}

// readConsoleBatchWithFlag is like readConsoleBatch but carries post-execution
// flags so AppModel can trigger config reload and/or app list refresh.
func readConsoleBatchWithFlag(sc *bufio.Scanner, cancel context.CancelFunc, configChanged, appsChanged bool) tea.Cmd {
	return func() tea.Msg {
		var batch []string
		for i := 0; i < 50 && sc.Scan(); i++ {
			batch = append(batch, sc.Text())
		}
		if len(batch) > 0 {
			return consoleLinesMsg{lines: batch}
		}
		cancel()
		return consoleDoneMsg{err: sc.Err(), configChanged: configChanged, appsChanged: appsChanged}
	}
}

// isDS2Prefix reports whether tok is a recognized ds2 command prefix —
// the detected binary name (e.g. "dockstarter2"), "ds2", or "ds".
func isDS2Prefix(tok string) bool {
	lower := strings.ToLower(tok)
	cmdName := strings.ToLower(version.CommandName)
	return lower == cmdName || lower == "ds2" || lower == "ds"
}

// submitConsoleCommand parses and runs cmdStr.
// ds2 commands (starting with - or prefixed with "ds2") are executed internally
// via commands.Parse + commands.Execute; output flows through the logger subscription.
// Everything else is run as a shell command and streamed via the pipe/scanner path.
func (m *PanelModel) submitConsoleCommand(cmdStr string) tea.Cmd {
	tokens := strings.Fields(cmdStr)
	if len(tokens) == 0 {
		return nil
	}
	isDS2 := isDS2Prefix(tokens[0])
	args := tokens
	if isDS2 {
		args = tokens[1:]
	}

	// In restricted console mode, only ds2 commands are allowed — shell is blocked.
	if m.panelMode == "console" && !isDS2 && !(len(args) > 0 && strings.HasPrefix(args[0], "-")) {
		logger.Error(context.Background(), "Only ds2 commands are allowed in Console mode. Switch to 'System Console' for full shell access.")
		return func() tea.Msg { return consoleDoneMsg{} }
	}

	// In restricted console mode, enforce ConsoleSafe flag from the command registry.
	// This blocks privileged commands like --config-panel, --server, etc. even when
	// typed as ds2 commands — preventing a remote user from self-upgrading their access.
	if m.panelMode == "console" {
		if isDS2 || (len(args) > 0 && strings.HasPrefix(args[0], "-")) {
			groups, err := commands.Parse(args)
			if err == nil {
				for _, g := range groups {
					if !commands.IsConsoleSafe(g.Command) {
						logger.Error(context.Background(),
							"Command '{{|UserCommand|}}%s{{[-]}}' is not permitted in Console mode.", g.Command)
						return func() tea.Msg { return consoleDoneMsg{} }
					}
				}
			}
		}
	}

	// ds2 command: prefixed with ds/ds2/executable, or first token starts with -
	if isDS2 || (len(args) > 0 && strings.HasPrefix(args[0], "-")) {
		groups, err := commands.Parse(args)

		if err != nil {
			logger.Error(context.Background(), "%s", err.Error())
			return func() tea.Msg { return consoleDoneMsg{} }
		}

		configChanged := commands.GroupsNeedConfigReload(groups)
		appsChanged := commands.GroupsNeedAppsRefresh(groups)

		ctx, cancel := context.WithCancel(context.Background())
		m.consoleCancel = cancel
		pr, pw := io.Pipe()
		cmdCtx := console.WithTUIWriter(ctx, pw)

		go func() {
			// Log the command header into the pipe first
			if m.panelMode == "system" {
				logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
			} else {
				logger.Notice(cmdCtx, "Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
			}

			commands.Execute(cmdCtx, groups)
			pw.Close()
		}()

		sc := bufio.NewScanner(pr)
		m.consoleScanner = sc
		m.consoleConfigChanged = configChanged
		m.consoleAppsChanged = appsChanged
		return readConsoleBatchWithFlag(sc, cancel, configChanged, appsChanged)
	}

	// Shell command
	ctx, cancel := context.WithCancel(context.Background())
	m.consoleCancel = cancel

	// If the command contains sudo, intercept it and prime the sudo credential cache.
	var containsSudo bool
	for _, t := range tokens {
		if t == "sudo" {
			containsSudo = true
			break
		}
	}

	if containsSudo && m.panelMode == "system" {
		return func() tea.Msg {
			pass, err := PromptText("Sudo Password", "Password for '"+cmdStr+"':", true)
			if err != nil {
				if err == console.ErrUserAborted {
					return consoleDoneMsg{}
				}
				return consoleDoneMsg{err: err}
			}

			// 1. Prime the sudo cache by running sudo -S -v with the password.
			// This updates the sudo timestamp so subsequent sudo calls in the user's
			// command can run without a prompt.
			primeCmd := exec.Command("sudo", "-S", "-v")
			primeCmd.Stdin = strings.NewReader(pass + "\n")
			if err := primeCmd.Run(); err != nil {
				return consoleDoneMsg{err: fmt.Errorf("sudo: authentication failed")}
			}

			// 2. Run the user's original command line exactly as typed.
			pr, pw := io.Pipe()
			cmdCtx := console.WithTUIWriter(ctx, pw)
			go func() {
				// Log the command header into the pipe first
				logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
				err := runShellCmd(ctx, cmdStr, pw, "")
				pw.CloseWithError(err)
			}()

			sc := bufio.NewScanner(pr)
			m.consoleScanner = sc
			return readConsoleBatch(sc, cancel)
		}
	}

	pr, pw := io.Pipe()
	cmdCtx := console.WithTUIWriter(ctx, pw)
	go func() {
		// Log the command header into the pipe first
		if m.panelMode == "system" {
			logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
		} else {
			logger.Notice(cmdCtx, "Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
		}
		err := runShellCmd(ctx, cmdStr, pw, "")
		pw.CloseWithError(err)
	}()

	sc := bufio.NewScanner(pr)
	m.consoleScanner = sc
	return readConsoleBatch(sc, cancel)
}

// appendConsoleLines adds rendered lines to the scrollback.
func (m *PanelModel) appendConsoleLines(lines []string) {
	styles := GetStyles()
	targetWidth := m.viewport.Width()
	if targetWidth <= 0 && m.width > 0 {
		targetWidth = m.width - ScrollbarGutterWidth
	}
	for _, line := range lines {
		m.rawLines = append(m.rawLines, line)
		rendered := RenderConsoleText(line, styles.Console)
		if targetWidth > 0 {
			rendered = lipgloss.NewStyle().MaxWidth(targetWidth).Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}

	// Limit history to 5000 lines
	maxHistory := 5000
	if len(m.rawLines) > maxHistory {
		m.rawLines = m.rawLines[len(m.rawLines)-maxHistory:]
		m.lines = m.lines[len(m.lines)-maxHistory:]
	}

	content := strings.Join(m.lines, "\n")
	m.viewport.SetContent(content)
	if m.viewport.Height() > 0 {
		m.viewport.GotoBottom()
	}
}

// reRenderLines re-wraps all stored raw lines for the current width.
func (m *PanelModel) reRenderLines() {
	styles := GetStyles()
	targetWidth := m.width - ScrollbarGutterWidth
	if targetWidth <= 0 {
		return
	}

	m.lines = make([]string, 0, len(m.rawLines))
	style := lipgloss.NewStyle().MaxWidth(targetWidth)
	for _, line := range m.rawLines {
		rendered := RenderConsoleText(line, styles.Console)
		rendered = style.Render(rendered)
		m.lines = append(m.lines, rendered)
	}

	// Limit history to 5000 lines
	maxHistory := 5000
	if len(m.rawLines) > maxHistory {
		m.rawLines = m.rawLines[len(m.rawLines)-maxHistory:]
		m.lines = m.lines[len(m.lines)-maxHistory:]
	}

	content := strings.Join(m.lines, "\n")
	m.viewport.SetContent(content)
	if m.viewport.Height() > 0 {
		m.viewport.GotoBottom()
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m PanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
		if m.inputFocused {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil

	case ConsoleLockMsg:
		m.lockSession(msg.ID, msg.Locked)
		return m, nil

	case ConfigChangedMsg:
		m.panelMode = EffectivePanelMode(msg.Config, m.connType)
		if m.panelMode == "none" {
			m.expanded = false
		}
		m.SetSize(m.width, m.totalHeight)
		m.applyInputStyles()
		return m, nil

	case panelLineMsg:
		m.appendConsoleLines(strings.Split(string(msg), "\n"))
		return m, waitForPanelLine()

	case consoleLinesMsg:
		m.appendConsoleLines(msg.lines)
		return m, readConsoleBatchWithFlag(m.consoleScanner, m.consoleCancel, m.consoleConfigChanged, m.consoleAppsChanged)

	case consoleDoneMsg:
		m.consoleScanner = nil
		m.consoleCancel = nil
		if !m.sessionActive() {
			m.inputFocused = true
			cmd := m.input.Focus()
			return m, tea.Batch(cmd, sinput.Blink)
		}
		return m, nil

	case togglePanelMsg:
		m.expanded = !m.expanded
		if m.expanded {
			m.SetSize(m.width, m.totalHeight)
			content := strings.Join(m.lines, "\n")
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.HalfPageDown()
			}
			return m, nil
		}
		if msg.ID == IDConsoleInput && msg.Button == tea.MouseRight {
			return m, ShowInputContextMenu(m.input, msg.X, msg.Y, m.width, m.totalHeight)
		}
		if msg.ID == panelZoneID {
			return m, func() tea.Msg { return togglePanelMsg{} }
		}

	case DragDoneMsg:
		if msg.ID == resizeZoneID {
			m.resizeDrag.DragPending = false
			if m.resizeDrag.PendingDragY != m.resizeDrag.LastDragY {
				m.resizeDrag.LastDragY = m.resizeDrag.PendingDragY
				m.applyDragY(m.resizeDrag.PendingDragY)
				m.resizeDrag.DragPending = true
				return m, DragDoneCmd(resizeZoneID)
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.resizeDrag.StartDrag(msg.Y, m.height, ScrollbarInfo{})
			if !m.expanded {
				m.expanded = true
				m.height = 1
				m.SetSize(m.width, m.totalHeight)
				m.resizeDrag.StartThumbTop = 1
				content := strings.Join(m.lines, "\n")
				m.viewport.SetContent(content)
				m.viewport.GotoBottom()
			}
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.resizeDrag.Dragging {
			m.resizeDrag.StopDrag()
			m.SetSize(m.width, m.totalHeight)
			return m, nil
		}

	case tea.MouseMotionMsg:
		if m.resizeDrag.Dragging {
			m.resizeDrag.PendingDragY = msg.Y
			if !m.resizeDrag.DragPending {
				m.resizeDrag.LastDragY = msg.Y
				m.applyDragY(msg.Y)
				m.resizeDrag.DragPending = true
				return m, DragDoneCmd(resizeZoneID)
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
		if m.focused && m.inputFocused {
			return m.updateInputFocused(msg)
		}
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

	var cmd tea.Cmd
	if m.expanded {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// updateInputFocused handles key events when the input bar has focus.
func (m PanelModel) updateInputFocused(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, Keys.Esc):
		m.input.Blur()
		m.inputFocused = false
		return m, nil

	case key.Matches(msg, Keys.Up):
		if len(m.history) == 0 {
			return m, nil
		}
		if m.historyIdx == -1 {
			m.historyDraft = m.input.Value()
			m.historyIdx = len(m.history) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		}
		m.input.SetValue(m.history[m.historyIdx])
		m.input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Down):
		if m.historyIdx == -1 {
			return m, nil
		}
		m.historyIdx++
		if m.historyIdx >= len(m.history) {
			m.historyIdx = -1
			m.input.SetValue(m.historyDraft)
		} else {
			m.input.SetValue(m.history[m.historyIdx])
		}
		m.input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Enter):
		cmdStr := strings.TrimSpace(m.input.Value())
		if cmdStr == "" {
			return m, nil
		}
		m.history = append(m.history, cmdStr)
		m.historyIdx = -1
		m.historyDraft = ""
		m.input.SetValue("")
		m.input.Blur()
		m.inputFocused = false

		// Show the submitted command in the scrollback.
		m.appendConsoleLines([]string{"> " + cmdStr})

		return m, m.submitConsoleCommand(cmdStr)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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

// ─── View ─────────────────────────────────────────────────────────────────────

func (m PanelModel) ViewString() string {
	if m.Height() <= 0 {
		return ""
	}
	ctx := GetActiveContext()

	marker := "^"
	if m.expanded {
		marker = "v"
	}
	titleText := "Console"
	switch m.panelMode {
	case "log":
		titleText = "Log"
	case "system":
		titleText = "System Console"
	}
	title := marker + " " + titleText + " " + marker

	rightTitle := ""
	if m.focused && m.expanded && !m.inputFocused {
		pct := int(m.viewport.ScrollPercent() * 100)
		rightTitle = fmt.Sprintf(" %d%% ", pct)
	}

	// Input box occupies 3 rows (top border + 1 content + bottom border).
	hasInput := m.panelMode == "console" || m.panelMode == "system"
	vpH := m.height - 1
	if hasInput {
		vpH -= 3
	}
	if vpH < 1 {
		if m.totalHeight > 0 {
			fullH := (m.totalHeight / 2) - 1
			if hasInput {
				fullH -= 3
			}
			vpH = fullH
		}
		if vpH < 1 {
			vpH = 1
		}
	}
	if m.viewport.Height() != vpH {
		m.viewport.SetHeight(vpH)
	}
	if m.viewport.Width() != m.width-ScrollbarGutterWidth {
		m.viewport.SetWidth(m.width - ScrollbarGutterWidth)
	}

	vpStyle := lipgloss.NewStyle().
		Width(m.viewport.Width()).
		Height(vpH).
		Background(ctx.Console.GetBackground()).
		Foreground(ctx.Console.GetForeground())
	m.viewport.Style = vpStyle

	vpView := MaintainBackground(m.viewport.View(), ctx.Console)
	vpView = ApplyScrollbarColumn(vpView, len(m.lines), vpH, m.viewport.YOffset(), ctx.LineCharacters, ctx)

	// Input box — bordered with submenu styling.
	inputBoxWidth := m.width - 2 // inner content width (outer panel has no side borders)
	m.input.SetWidth(inputBoxWidth - 2)
	if m.sessionActive() {
		m.input.Placeholder = ""
		st := m.input.Styles()
		st.Focused.Placeholder = SemanticRawStyle("MarkerLocked")
		st.Blurred.Placeholder = SemanticRawStyle("MarkerLocked")
		m.input.SetStyles(st)
		marker := lockedMarker
		if !ctx.LineCharacters {
			marker = lockedMarkerAscii
		}
		// Consolidated lock marker and message into the Prompt for reliable styling
		m.input.Prompt = RenderThemeText("{{|MarkerLocked|}}"+marker+" Session active — input locked{{[-]}} ", ctx.Dialog)
	} else {
		m.input.Placeholder = ""
		st := m.input.Styles()
		st.Focused.Placeholder = lipgloss.NewStyle()
		st.Blurred.Placeholder = lipgloss.NewStyle()
		m.input.SetStyles(st)
		m.input.Prompt = "> "
	}
	inputTitleTag := "TitleSubMenu"
	if m.inputFocused {
		inputTitleTag = "TitleSubMenuFocused"
	}
	inputContent := lipgloss.NewStyle().
		Width(inputBoxWidth - 2).
		Background(ctx.Dialog.GetBackground()).
		Render(m.input.View())
	inputBox := RenderBorderedBoxCtx(
		"{{|"+inputTitleTag+"|}}Command{{[-]}}",
		inputContent,
		inputBoxWidth-2,
		3,
		m.inputFocused,
		true,
		true,
		ctx.SubmenuTitleAlign,
		inputTitleTag,
		ctx,
	)

	// Inject INS/OVR label into the bottom-left of the Command section border.
	modeLabel := "INS"
	if m.input.IsOverwrite() {
		modeLabel = "OVR"
	}
	ibLines := strings.Split(inputBox, "\n")
	if len(ibLines) > 0 {
		ibLines[len(ibLines)-1] = BuildLabeledBottomBorderCtx(inputBoxWidth, modeLabel, m.inputFocused, ctx)
		inputBox = strings.Join(ibLines, "\n")
	}

	combined := vpView
	if hasInput {
		combined += "\n" + inputBox
	}

	consoleTitleStyle := SemanticRawStyle("ConsoleTitle")
	consoleBorderStyle := SemanticRawStyle("ConsoleBorder")

	return RenderTopBorderBoxCtx(title, rightTitle, combined, m.width, m.focused, consoleTitleStyle, consoleBorderStyle, ctx)
}

// Layers returns a single layer with the panel content for visual compositing.
func (m PanelModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZPanel).ID(IDPanel),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing.
func (m PanelModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	panelHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Viewer",
		PageText:   "Displays live application logs and accepts ds2/shell commands.",
		ItemTitle:  "Console Panel",
		ItemText:   "Scroll with the mouse wheel or use Home/End/PgUp/PgDn when focused.",
	}
	inputHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Input",
		PageText:   "Type ds2 commands or shell commands and press Enter to run them.",
		ItemTitle:  "Console Input",
		ItemText:   "Enter: run | Up/Down: history | Esc: exit input",
	}

	ctx := GetActiveContext()
	marker := "^"
	if m.expanded {
		marker = "v"
	}
	title := marker + " Console " + marker

	titleWidth := WidthWithoutZones(RenderThemeText(title, ctx.Dialog))
	titleSectionLen := 1 + 1 + titleWidth + 1 + 1
	actualWidth := m.width - 2
	var leftPad int
	if ctx.PanelTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	titleStart := 1 + leftPad
	titleEnd := titleStart + titleSectionLen

	regions = append(regions, HitRegion{
		ID:     IDPanelResize,
		X:      offsetX,
		Y:      offsetY,
		Width:  titleStart,
		Height: 1,
		ZOrder: ZPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDPanelToggle,
		X:      offsetX + titleStart,
		Y:      offsetY,
		Width:  titleSectionLen,
		Height: 1,
		ZOrder: ZPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDPanelResize,
		X:      offsetX + titleEnd,
		Y:      offsetY,
		Width:  m.width - titleEnd,
		Height: 1,
		ZOrder: ZPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})

	if m.expanded {
		vpH := m.height - 4
		regions = append(regions, HitRegion{
			ID:     IDPanelViewport,
			X:      offsetX,
			Y:      offsetY + 1,
			Width:  m.width,
			Height: vpH,
			ZOrder: ZPanel + 1,
			Label:  "Console Panel",
			Help:   panelHelp,
		})

		// Input bar region (3 rows: top border + content + bottom border)
		// Text X: 1 (input box left border) + promptWidth
		m.input.SetScreenTextX(offsetX + 1 + m.input.PromptWidth())
		regions = append(regions, HitRegion{
			ID:     IDConsoleInput,
			X:      offsetX,
			Y:      offsetY + 1 + vpH,
			Width:  m.width,
			Height: 3,
			ZOrder: ZPanel + 1,
			Label:  "Console Input",
			Help:   inputHelp,
		})

		// Scrollbar hit regions
		if currentConfig.UI.Scrollbar {
			sbInfo := ComputeScrollbarInfo(m.viewport.TotalLineCount(), m.viewport.Height(), m.viewport.YOffset(), vpH)
			if sbInfo.Needed {
				sbX := offsetX + m.viewport.Width()
				sbTopY := offsetY + 1

				regions = append(regions, HitRegion{
					ID: IDPanel + ".sb.up", X: sbX, Y: sbTopY,
					Width: 1, Height: 1, ZOrder: ZPanel + 2,
				})
				if aboveH := sbInfo.ThumbStart - 1; aboveH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.above", X: sbX, Y: sbTopY + 1,
						Width: 1, Height: aboveH, ZOrder: ZPanel + 2,
					})
				}
				if thumbH := sbInfo.ThumbEnd - sbInfo.ThumbStart; thumbH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.thumb", X: sbX, Y: sbTopY + sbInfo.ThumbStart,
						Width: 1, Height: thumbH, ZOrder: ZPanel + 3,
					})
				}
				if belowH := (sbInfo.Height - 1) - sbInfo.ThumbEnd; belowH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.below", X: sbX, Y: sbTopY + sbInfo.ThumbEnd,
						Width: 1, Height: belowH, ZOrder: ZPanel + 2,
					})
				}
				regions = append(regions, HitRegion{
					ID: IDPanel + ".sb.down", X: sbX, Y: sbTopY + sbInfo.Height - 1,
					Width: 1, Height: 1, ZOrder: ZPanel + 2,
				})
			}
		}
	}

	return regions
}

// GetInputCursor returns the hardware cursor position relative to the panel's
// top-left corner, cursor shape, and whether to show it.
func (m PanelModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if !m.expanded || !m.inputFocused || !m.input.Focused() {
		return 0, 0, tea.CursorBar, false
	}
	// Vertical offset: 1 (top border) + viewport height + 1 (input top border)
	vpH := m.height - 4
	relY = vpH + 2
	// Horizontal offset: 1 (outer border) + input cursor column (which includes prompt)
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
	total := m.viewport.TotalLineCount()
	visible := m.viewport.Height()
	if total <= visible {
		return m, false
	}
	maxOff := total - visible
	newOff, _ := drag.ScrollOffset(mouseY, sbAbsTopY, maxOff, info)
	if newOff == m.viewport.YOffset() {
		return m, false
	}
	m.viewport.GotoTop()
	m.viewport.ScrollDown(newOff)
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

// View renders the panel at its current height.
func (m PanelModel) View() tea.View {
	return tea.NewView(m.ViewString())
}
