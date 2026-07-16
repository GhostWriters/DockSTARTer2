package classic

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
	return PanelLineMsg(strings.Join(all, "\n"))
}

// waitForPanelLine blocks until the panel's own log subscription channel
// (ch, from logger.SubscribeLogLines -- see PanelModel.logSub) sends a line,
// then returns it as a message.
func waitForPanelLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return PanelLineMsg(line)
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
// ConsoleLinesMsg, or ConsoleDoneMsg on EOF.
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
			return ConsoleDoneMsg{Err: sc.Err(), ConfigChanged: configChanged, AppsChanged: appsChanged}
		}
		return ConsoleLinesMsg{Lines: []string{sc.Text()}}
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
	if m.PanelMode == "console" && !isDS2 && (len(args) == 0 || !strings.HasPrefix(args[0], "-")) {
		logger.Error(context.Background(), "Only ds2 commands are allowed in Console mode. Switch to 'System Console' for full shell access.")
		return func() tea.Msg { return ConsoleDoneMsg{} }
	}

	// In restricted console mode, enforce ConsoleSafe flag from the command registry.
	// This blocks privileged commands like --config-panel, --server, etc. even when
	// typed as ds2 commands — preventing a remote user from self-upgrading their access.
	if m.PanelMode == "console" {
		if isDS2 || (len(args) > 0 && strings.HasPrefix(args[0], "-")) {
			groups, err := commands.Parse(args)
			if err == nil {
				for _, g := range groups {
					if !commands.IsConsoleSafe(g.Command) {
						logger.Error(context.Background(),
							"Command '{{|UserCommand|}}%s{{[-]}}' is not permitted in Console mode.", g.Command)
						return func() tea.Msg { return ConsoleDoneMsg{} }
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
			return func() tea.Msg { return ConsoleDoneMsg{} }
		}

		// The edit-lock check happens per-group inside commands.Execute (matching
		// CLI behavior exactly), not here -- a pre-check across all groups
		// upfront would abort the entire command line at the first locked group,
		// silently skipping (and never logging) any earlier group that could
		// have run successfully, e.g. "ds2 --list-enabled -yp" should still run
		// --list-enabled even if -p is currently locked.
		configChanged := commands.GroupsNeedConfigReload(groups)
		appsChanged := commands.GroupsNeedAppsRefresh(groups)

		ctx, cancel := context.WithCancel(context.Background())
		m.ConsoleCancel = cancel
		m.replaceHeaderCount = -1
		pr, pw := io.Pipe()
		cmdCtx := console.WithPanelWriter(ctx, pw)

		go func() {
			// Log the command header into the pipe first
			if m.PanelMode == "system" {
				logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
			} else {
				logger.Notice(cmdCtx, "Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
			}

			commands.Execute(cmdCtx, groups, m.clientIP, m.connType, m.sessionKey)
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
	m.ConsoleCancel = cancel
	m.titleSpinner.Start()

	// If the command contains sudo, intercept it and prime the sudo credential cache.
	var containsSudo bool
	for _, t := range tokens {
		if t == "sudo" {
			containsSudo = true
			break
		}
	}

	if containsSudo && m.PanelMode == "system" {
		return tea.Batch(func() tea.Msg {
			pass, err := PromptTextHook("Sudo Password", "Password for '"+cmdStr+"':", true)
			if err != nil {
				if err == console.ErrUserAborted {
					return ConsoleDoneMsg{}
				}
				return ConsoleDoneMsg{Err: err}
			}

			// 1. Prime the sudo cache by running sudo -S -v with the password.
			// This updates the sudo timestamp so subsequent sudo calls in the user's
			// command can run without a prompt.
			primeCmd := exec.Command("sudo", "-S", "-v")
			primeCmd.Stdin = strings.NewReader(pass + "\n")
			if err := primeCmd.Run(); err != nil {
				return ConsoleDoneMsg{Err: fmt.Errorf("sudo: authentication failed")}
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
		if m.PanelMode == "system" {
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
		if m.InputFocused {
			var cmd tea.Cmd
			m.Input, cmd = m.Input.Update(msg)
			return m, cmd
		}
		return m, nil

	case ConsoleLockMsg:
		return m, m.lockSession(msg.ID, msg.Locked)

	case ConfigChangedMsg:
		m.PanelMode = EffectivePanelMode(msg.Config, m.connType)
		if m.PanelMode == "none" {
			m.Expanded = false
		}
		m.SetSize(m.width, m.totalHeight)
		m.applyInputStyles()
		return m, nil

	case PanelLineMsg:
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.AppendLines(strings.Split(string(msg), "\n"), panelRenderFn())
		return m, waitForPanelLine(m.logSub)

	case ConsoleLinesMsg:
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.CommandRunning = true
		m.Sv.AppendLines(msg.Lines, panelRenderFn())
		if m.consoleScanner == nil {
			return m, nil
		}
		return m, readConsoleBatchWithFlag(m.consoleScanner, m.ConsoleCancel, m.consoleConfigChanged, m.consoleAppsChanged)

	case ReplaceOutputMsg:
		SetActiveOutputWidth(m.Sv.Width())
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.CommandRunning = false
		if m.replaceHeaderCount < 0 {
			m.replaceHeaderCount = m.Sv.TotalLineCount()
		}
		m.Sv.ReplaceTailLines(m.replaceHeaderCount, msg.Lines, panelRenderFn())
		return m, nil

	case ConsoleDoneMsg:
		m.consoleScanner = nil
		m.ConsoleCancel = nil
		m.replaceHeaderCount = -1
		m.Sv.CommandRunning = false
		m.Sv.ClearSpinner()
		unlockCmd := m.lockSession("console.command", false)
		if !m.SessionActive() {
			m.InputFocused = true
			cmd := m.Input.Focus()
			return m, tea.Batch(unlockCmd, cmd, sinput.Blink)
		}
		return m, unlockCmd

	case TogglePanelMsg:
		m.Expanded = !m.Expanded
		if m.Expanded {
			m.panelChanged = false
			m.SetSize(m.width, m.totalHeight)
			m.Sv.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.HalfPageDown()
			}
			return m, nil
		}
		if msg.ID == IDConsoleInput && msg.Button == tea.MouseRight {
			return m, ShowInputContextMenu(m.Input, msg.X, msg.Y, m.width, m.totalHeight)
		}
		if msg.ID == panelZoneID {
			return m, func() tea.Msg { return TogglePanelMsg{} }
		}

	case DragDoneMsg:
		if msg.ID == ResizeZoneID {
			m.ResizeDrag.DragPending = false
			if m.ResizeDrag.PendingDragY != m.ResizeDrag.LastDragY {
				m.ResizeDrag.LastDragY = m.ResizeDrag.PendingDragY
				m.applyDragY(m.ResizeDrag.PendingDragY)
				m.ResizeDrag.DragPending = true
				return m, DragDoneCmd(ResizeZoneID)
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.ResizeDrag.StartDrag(msg.Y, m.PanelHeight, ScrollbarInfo{})
			if !m.Expanded {
				m.Expanded = true
				m.PanelHeight = 1
				m.SetSize(m.width, m.totalHeight)
				m.ResizeDrag.StartThumbTop = 1
				m.Sv.GotoBottom()
			}
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.ResizeDrag.Dragging {
			m.ResizeDrag.StopDrag()
			m.SetSize(m.width, m.totalHeight)
			return m, nil
		}

	case tea.MouseMotionMsg:
		if m.ResizeDrag.Dragging {
			m.ResizeDrag.PendingDragY = msg.Y
			if !m.ResizeDrag.DragPending {
				m.ResizeDrag.LastDragY = msg.Y
				m.applyDragY(msg.Y)
				m.ResizeDrag.DragPending = true
				return m, DragDoneCmd(ResizeZoneID)
			}
			return m, nil
		}

	case tea.MouseWheelMsg:
		if m.Expanded {
			if msg.Button == tea.MouseWheelUp {
				m.Sv.ScrollUp(3)
				return m, nil
			}
			if msg.Button == tea.MouseWheelDown {
				m.Sv.ScrollDown(3)
				return m, nil
			}
		}

	case tea.KeyPressMsg:
		if m.Focused && m.InputFocused {
			return m.updateInputFocused(msg)
		}
		if m.Expanded {
			switch {
			case key.Matches(msg, Keys.Home):
				m.Sv.GotoTop()
				return m, nil
			case key.Matches(msg, Keys.End):
				m.Sv.GotoBottom()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.Expanded {
		cmd = m.Sv.ViewUpdate(msg)
	}
	return m, cmd
}

// updateInputFocused handles key events when the input bar has focus.
func (m PanelModel) updateInputFocused(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, Keys.Esc):
		m.Input.Blur()
		m.InputFocused = false
		return m, nil

	case key.Matches(msg, Keys.Up):
		if len(m.history) == 0 {
			return m, nil
		}
		if m.historyIdx == -1 {
			m.historyDraft = m.Input.Value()
			m.historyIdx = len(m.history) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		}
		m.Input.SetValue(m.history[m.historyIdx])
		m.Input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Down):
		if m.historyIdx == -1 {
			return m, nil
		}
		m.historyIdx++
		if m.historyIdx >= len(m.history) {
			m.historyIdx = -1
			m.Input.SetValue(m.historyDraft)
		} else {
			m.Input.SetValue(m.history[m.historyIdx])
		}
		m.Input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Enter):
		cmdStr := strings.TrimSpace(m.Input.Value())
		if cmdStr == "" {
			return m, nil
		}
		m.history = append(m.history, cmdStr)
		m.historyIdx = -1
		m.historyDraft = ""
		m.Input.SetValue("")
		m.Input.Blur()
		m.InputFocused = false

		// Show the submitted command in the scrollback.
		m.Sv.AppendLines([]string{"> " + cmdStr}, panelRenderFn())

		return m, m.submitConsoleCommand(cmdStr)
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}
