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
	"time"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/version"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

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

// readConsoleBatch reads one line from the scanner and returns it as a
// consoleLinesMsg, or consoleDoneMsg on EOF.
func readConsoleBatch(sc *bufio.Scanner, cancel context.CancelFunc) tea.Cmd {
	return readConsoleBatchWithFlag(sc, cancel, false, false)
}

// readConsoleBatchWithFlag is like readConsoleBatch but carries post-execution
// flags so AppModel can trigger config reload and/or app list refresh.
func readConsoleBatchWithFlag(sc *bufio.Scanner, cancel context.CancelFunc, configChanged, appsChanged bool) tea.Cmd {
	if sc == nil || cancel == nil {
		return nil
	}
	return func() tea.Msg {
		if !sc.Scan() {
			cancel()
			return consoleDoneMsg{err: sc.Err(), configChanged: configChanged, appsChanged: appsChanged}
		}
		return consoleLinesMsg{lines: []string{sc.Text()}}
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
	if m.panelMode == "console" && !isDS2 && (len(args) == 0 || !strings.HasPrefix(args[0], "-")) {
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

		// Pre-check the edit lock for any session-locked command so the panel
		// shows a clear error immediately, instead of a line that can get
		// buried under unrelated background output once execution is blocked.
		for _, g := range groups {
			if def, ok := commands.Registry[g.Command]; ok && def.SessionLocked {
				if sessionlocks.Sessions.IsEditLocked() {
					info := sessionlocks.Sessions.ReadEditInfo()
					closing := fmt.Sprintf("Cannot run '{{|UserCommand|}}%s{{[-]}}' while the configuration is being edited.", cmdStr)
					logger.Error(context.Background(), sessionlocks.EditLockLines(info, closing))
					return func() tea.Msg { return consoleDoneMsg{} }
				}
				break
			}
		}

		configChanged := commands.GroupsNeedConfigReload(groups)
		appsChanged := commands.GroupsNeedAppsRefresh(groups)

		ctx, cancel := context.WithCancel(context.Background())
		m.consoleCancel = cancel
		m.replaceHeaderCount = -1
		pr, pw := io.Pipe()
		cmdCtx := console.WithPanelWriter(ctx, pw)

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
		m.titleSpinner.Start()
		lockCmd := m.lockSession("console.command", true)
		return tea.Batch(lockCmd, readConsoleBatchWithFlag(sc, cancel, configChanged, appsChanged))
	}

	// Shell command
	ctx, cancel := context.WithCancel(context.Background())
	m.consoleCancel = cancel
	m.titleSpinner.Start()

	// If the command contains sudo, intercept it and prime the sudo credential cache.
	var containsSudo bool
	for _, t := range tokens {
		if t == "sudo" {
			containsSudo = true
			break
		}
	}

	if containsSudo && m.panelMode == "system" {
		return tea.Batch(func() tea.Msg {
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
			cmdCtx := console.WithPanelWriter(ctx, pw)
			go func() {
				// Log the command header into the pipe first
				logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
				err := runShellCmd(ctx, cmdStr, pw, "")
				pw.CloseWithError(err)
			}()

			sc := bufio.NewScanner(pr)
			m.consoleScanner = sc
			return readConsoleBatch(sc, cancel)
		})
	}

	pr, pw := io.Pipe()
	cmdCtx := console.WithPanelWriter(ctx, pw)
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
	lockCmd := m.lockSession("console.command", true)
	return tea.Batch(lockCmd, readConsoleBatch(sc, cancel))
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
		return m, m.lockSession(msg.ID, msg.Locked)

	case ConfigChangedMsg:
		m.panelMode = EffectivePanelMode(msg.Config, m.connType)
		if m.panelMode == "none" {
			m.expanded = false
		}
		m.SetSize(m.width, m.totalHeight)
		m.applyInputStyles()
		return m, nil

	case panelLineMsg:
		m.lastLineTime = time.Now()
		if !m.expanded {
			m.panelChanged = true
		}
		m.sv.AppendLines(strings.Split(string(msg), "\n"), panelRenderFn())
		return m, waitForPanelLine()

	case consoleLinesMsg:
		m.lastLineTime = time.Now()
		if !m.expanded {
			m.panelChanged = true
		}
		m.sv.CommandRunning = true
		m.sv.AppendLines(msg.lines, panelRenderFn())
		if m.consoleScanner == nil {
			return m, nil
		}
		return m, readConsoleBatchWithFlag(m.consoleScanner, m.consoleCancel, m.consoleConfigChanged, m.consoleAppsChanged)

	case replaceOutputMsg:
		setActiveOutputWidth(m.sv.Width())
		m.lastLineTime = time.Now()
		if !m.expanded {
			m.panelChanged = true
		}
		m.sv.CommandRunning = false
		if m.replaceHeaderCount < 0 {
			m.replaceHeaderCount = m.sv.TotalLineCount()
		}
		m.sv.ReplaceTailLines(m.replaceHeaderCount, msg.lines, panelRenderFn())
		return m, nil

	case consoleDoneMsg:
		m.consoleScanner = nil
		m.consoleCancel = nil
		m.replaceHeaderCount = -1
		m.sv.CommandRunning = false
		m.sv.ClearSpinner()
		unlockCmd := m.lockSession("console.command", false)
		if !m.sessionActive() {
			m.inputFocused = true
			cmd := m.input.Focus()
			return m, tea.Batch(unlockCmd, cmd, sinput.Blink)
		}
		return m, unlockCmd

	case togglePanelMsg:
		m.expanded = !m.expanded
		if m.expanded {
			m.panelChanged = false
			m.SetSize(m.width, m.totalHeight)
			m.sv.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if m.expanded && msg.Button != HoverButton {
				m.sv.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if m.expanded && msg.Button != HoverButton {
				m.sv.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if m.expanded && msg.Button != HoverButton {
				m.sv.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if m.expanded && msg.Button != HoverButton {
				m.sv.HalfPageDown()
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
				m.sv.GotoBottom()
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
				m.sv.ScrollUp(3)
				return m, nil
			}
			if msg.Button == tea.MouseWheelDown {
				m.sv.ScrollDown(3)
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
				m.sv.GotoTop()
				return m, nil
			case key.Matches(msg, Keys.End):
				m.sv.GotoBottom()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.expanded {
		cmd = m.sv.ViewUpdate(msg)
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
		m.sv.AppendLines([]string{"> " + cmdStr}, panelRenderFn())

		return m, m.submitConsoleCommand(cmdStr)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
